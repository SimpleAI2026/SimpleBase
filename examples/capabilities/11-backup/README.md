# 备份恢复：项目单文件快照

验证 `backup create/list/restore/delete` 与下载接口：写一条项目级 KV → 建快照 → 覆盖 KV →
恢复 → KV 回到快照时的值 → 下载快照校验字节数 → 删除备份后列表不再包含。

备份快照的是**用户项目单文件数据库**（项目级 KV 数据面），不含系统库，也不打包对象存储文件。
备份默认关闭：`config.yaml` 的 `backup.enabled`（或 `SIMPLEBASE_BACKUP_ENABLED=true`）必须打开，
否则 `/backups` 路由不挂载：CLI 拿到 HTTP 404 `not_found`（启用但实例只读时才是 503 `backup_disabled`），两种都按「未启用」整体打印 SKIP 并退出 0。

## 前置

- 服务端已启动并启用备份；`SIMPLEBASE_BIN` / `SIMPLEBASE_URL` / `SIMPLEBASE_TOKEN` / `SIMPLEBASE_PROJECT_ID` 已导出。
- 备份接口需要 `project:admin` 权限；下载/列表同样需要。
- 若当前项目的项目文件已经不可快照（见下方「已知平台约束」），用例会新建 `cap11-<suffix>` 项目并在
  干净项目上验证；这要求 token 能 `project:admin` 建项目，否则打印 SKIP。
- 需要可写实例；只读实例写接口返回 503 `writer_unavailable`。

## 运行

```bash
export CASE=11-backup
./examples/capabilities/11-backup/run.sh
```

## 命令与接口

```bash
simplebase kv exec --command SET --arg "cap11:<suffix>:k" --arg before
simplebase backup create                      # POST /v1/projects/:p/backups → 201 快照元数据
simplebase backup list                        # GET  /v1/projects/:p/backups
simplebase kv exec --command SET --arg ... --arg after
simplebase backup restore --id "$BID"         # POST /v1/projects/:p/backups/:id/restore → 204
simplebase kv exec --command GET --arg ...
curl -o backup.bin "$SIMPLEBASE_URL/v1/projects/$PROJECT/backups/$BID/download"
simplebase backup delete --id "$BID"          # DELETE → 204
```

## 断言点

- `backup create` 返回 `status=ready`、`engine` 非空、`sha256` 为 64 位十六进制、`size_bytes>0`。
- `backup list` 包含刚创建的 id；恢复后 KV GET 回到快照时的 `before`。
- 下载接口返回 200，字节数与元数据 `size_bytes` 一致。
- `backup delete` 后列表不再包含该 id；KV 键清理完成。

## 已知平台约束（会影响用例顺序）

- 一个项目只有一个用户数据库，且所有数据库文件路径都指向**同一个项目单文件**。
  `simplebase database delete`（catalog `DeleteDatabaseSync` → `filedb.PurgeProjectFiles`）会删除
  这个共享文件，连项目级 KV 一起清掉。若此前的用例删过数据库，当前项目的项目文件可能已被
  unlink，后续 `backup create` 会返回 500 `internal_error`（日志：
  `filedb: open snapshot source: ... no such file or directory`）。
- 本用例用「探测 + 新建专用项目」绕过：探测失败且错误码是 `internal_error` 时新建
  `cap11-<suffix>` 项目再验证（`project create --name`，服务端生成 8 位项目 ID，不传 `--id`）。
  **本用例会新建一个专用项目且无法删除**——原因是删库会清空项目文件，当前项目已不可快照；
  CLI 目前没有 `project delete`，该专用项目会保留（与 `01-project` 每次跑都留下项目同理）。
  新建项目失败或 token 无 `project:admin` 时，用例打印 SKIP 并退出 0，jsonl 的 `detail` 写明原因。
- 备份恢复会**覆盖**当前项目数据库；生产环境应先停写并额外做一份安全快照。
- 恢复失败码：`backup_not_found`（404）、`project_busy`（409）、`snapshot_mismatch`（409）。
