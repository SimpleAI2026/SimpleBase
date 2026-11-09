# 预期结果

| 步骤 | 通过条件 |
|---|---|
| 前置探测 | `backup list` 成功；返回 `backup_disabled` 时整体 SKIP 并退出 0 |
| 项目探针 | `backup create` 成功则沿用当前项目；返回 `internal_error` 时新建 `cap11-<suffix>` 项目继续 |
| kv-set / kv-get-before | `data == "OK"` / `data == "before"` |
| create | `status=ready`、`engine` 非空、`sha256` 64 位、`size_bytes>0`，拿到备份 id |
| list | `items` 里含该 id |
| kv-overwrite / kv-get-after | `data == "OK"` / `data == "after"` |
| restore | `ok=true`（HTTP 204） |
| kv-get-restored | `data == "before"`（快照生效） |
| download | HTTP 200，下载字节数等于 `size_bytes` |
| delete | `ok=true`（HTTP 204） |
| list-after-delete | `items` 里不再有该 id |
| kv-del | 删除测试键成功 |

> 备份关闭（503 `backup_disabled`）→ 用例 SKIP，不计 FAIL。
> 当前项目文件被删库副作用破坏（500 `internal_error`）→ 新建专用项目继续；新建也失败（无
> `project:admin`）→ SKIP，不把环境缺陷记为用例失败。
