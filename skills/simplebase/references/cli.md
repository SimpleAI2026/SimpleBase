# simplebase CLI 速查

`simplebase` 不带参数时启动服务；带 `<resource> <action>` 时作为资源 CLI。重放脚本只是对它做了编排，排查问题时可以直接调用。

## 连接

优先级：命令行 flag > 环境变量 > `$XDG_CONFIG_HOME/simplebase/config.json`（必须是 0600，否则 `config_insecure`）。

| flag | 环境变量 |
| --- | --- |
| `--endpoint` | `SIMPLEBASE_URL` |
| `--token` | `SIMPLEBASE_TOKEN`，也认 `SIMPLEBASE_API_KEY`（两者都设时 `SIMPLEBASE_TOKEN` 优先；空值视为未设置） |
| `--project` | `SIMPLEBASE_PROJECT_ID` |

不要把 token 写进命令行（会出现在 shell 历史和 `ps` 里），用环境变量。需要手动调用时：

```bash
set -a; . ./.env; set +a; simplebase function list
```

这条命令会把 Key 载入当前 shell，不要随后执行 `env`、`printenv` 或 `echo`。旧版二进制不认 `SIMPLEBASE_API_KEY`，遇到 `config_missing` 时改用 `SIMPLEBASE_TOKEN="$SIMPLEBASE_API_KEY"`。

## 输出协议

- 成功：stdout 一行 `{"ok":true,"data":...}`，退出码 0。
- 失败：stderr 一行 `{"ok":false,"error":{"code":"...","message":"..."}}`。API 错误退出码 2，用法错误 1，配置错误 4。
- 输出会自动把 `password`/`token`/`secret`/`api_key` 这类字段打成 `***`。

## 常用命令

```bash
simplebase database list --limit 20
simplebase database create --name main --data-model sql
simplebase sql batch --database <库ID> --file batch.json --transactional   # [{"sql":"...","args":[]}]
simplebase function get --name hello                    # 含生效源码、active_version、latest_version
simplebase function create --name hello --file functions/hello.go --description "示例"   # 创建并生效
simplebase function create --name hello --file functions/hello.go --activate false       # 只建 v1，不生效；试跑后再 activate
simplebase function versions create --name hello --file functions/hello.go --note fix   # 不生效
simplebase function test --name hello --version 2 --export Hello --input '{"name":"a"}'
simplebase function activate --name hello --version 2
simplebase function invoke --name hello --export Hello --input '{}'
simplebase cron list
simplebase cron create --name daily --schedule-kind cron --cron "0 3 * * *" --func-file hello --func-export Hello --input '{}'
simplebase cron update --id <任务ID> --enabled false     # 改调度时要把调度字段和目标字段一并传
simplebase cron runs --id <任务ID>                       # status: running/completed/failed/canceled
simplebase object upload --key index.html --file dist/index.html   # 默认 octet-stream，不触发 JS/CSS 415
simplebase object list --prefix ""
simplebase object presign --key index.html               # 15 分钟私有下载链接（非站点渲染 URL）
simplebase object delete --key index.html
simplebase log search --limit 50
```

工作区重放（对应 `scripts/sbdev.py validate|plan|apply|status`，退出码 `3` 表示远端写操作需要 `--yes`）：

```bash
simplebase dev validate --dir . [--manifest F]  # 纯离线校验，不需要任何连接配置
simplebase dev plan   --dir .                   # 只读，列出 create/update/apply/skip
simplebase dev apply  --dir . [--seed]          # 本地直接执行
simplebase dev apply  --dir . --yes             # 远端目标必须带 --yes，否则只输出计划并以退出码 3 退出
simplebase dev status --dir .                   # 读取 .simplebase/lock.json，同样不需要凭据
```

静态站点上传与子域名发布（编排 `object upload` 与 `/v1/projects/:pid/s3/{site,public}`，详见 `sites.md`）：

```bash
python3 scripts/sbdev.py site status    -C .
python3 scripts/sbdev.py site deploy    -C . --dist dist [--prefix app/] [--no-publish] [--yes]
python3 scripts/sbdev.py site unpublish -C . [--yes]
```

`dev validate` 与 `dev status` 不访问远端，所以不要求 URL / token / project；在只有二进制、没有凭据的 CI 里用 `dev validate`（脚本版是 `sbdev.py validate`）。

需要用户同意才能执行的命令：`database delete`、`function delete`、`cron delete`、`cron trigger`、`object delete`、`document delete`、`collection truncate`、包含 DROP/DELETE/TRUNCATE 的 `sql exec`，以及远端的 `site deploy` / `site unpublish`。`database delete` 会清空这个库的全部表、视图与集合（不可恢复，必要时先 `backup create`）；同项目的 KV、对象文件、已发布站点和已有备份保留。

管理员建项目（本地与云端用同一 ID）：`simplebase project create --id myapp001 --name "My App"`，需要超管 token。

## 错误码 → 处理

| code | 处理 |
| --- | --- |
| `config_missing` | 缺 URL/token/project（退出码 4；`dev validate` 与 `dev status` 除外）。检查 `.env`；gocloudagent 的 shell 会剥离 `SIMPLEBASE_*`，在那里要用 config.json |
| `usage` + `unknown command dev ...` | 二进制没有 `dev` 子命令（早于 planv6.4 P0b），改用 `scripts/sbdev.py` |
| `config_insecure` | `chmod 600 ~/.config/simplebase/config.json` |
| `project_mismatch` | 委托令牌绑定的项目与 `--project` 不一致 |
| `unauthenticated` / `invalid_api_key` / `api_key_revoked` / `invalid_or_expired_token` | Key 缺失、无效或已吊销，请用户重新签发并更新 `.env` |
| `forbidden` | 权限不足：建库与开启站点/公开路径（`/s3/site`、`/s3/public`）需要 `database:admin`（隐含 `database:write` 与 `database:read`），迁移/函数/cron/对象上传需要 `database:write`，plan 读取需要 `database:read`；`database:write` 不隐含 read |
| `cross_project_denied` | Key 不属于该项目，核对 `SIMPLEBASE_PROJECT_ID` |
| `project_not_found` / 访问项目时报 `database_not_found` | 目标实例上还没有这个项目，请用户在控制台用相同 ID 创建，或由管理员执行 `simplebase project create --id <ID> --name <名称>` |
| `invalid_api_key` 换环境后出现 | `.env` 还是旧实例的 Key；每个实例单独签发，`.env` 与 `.env.cloud` 分开放 |
| `database_already_exists` | 每个项目只能有一个用户数据库；清单只声明一个库，或在目标项目先确认现有库名 |
| `site_not_publishable`（400） | 当前项目无法发布子域名站点：调 `GET /v1/projects/<ID>/s3/site` 查看 `reason`（`sites_disabled` 实例未开启 `sites.enabled` / `base_domain`；`project_id_not_dns_safe` 项目 ID 含大写或首尾 `-`；`system_project` 系统项目禁止发布）。见 `sites.md` |
| `mime_mismatch`（415） | 上传声明的 Content-Type 与内容嗅探结果不一致。上传静态站点（`.js`/`.css`/`.html`）用 `simplebase object upload` 或声明 `application/octet-stream`；`.svg` 去掉 `<?xml ...?>` 头；文本类（CSV/JSON/YAML/Markdown）可声明原类型或 `text/plain` |
| `writer_unavailable` / `readonly_mode` | 实例只读（单写实例约束），换可写实例 |
| `gofunction_*`、`function_not_found` | 见 `gofunction.md` |
| `sql_not_allowed`、`data_model_mismatch` 等 | 见 `database.md` |
| `invalid_cron_expression`、`invalid_interval`、`invalid_run_at` | 按清单规则修字段 |
| `cron_job_exists` | 同名任务已存在；重放会按名称更新，手工创建时换名或改用 update |
| 提示两个环境引擎不一致 | 迁移只按一种方言写，把两个项目的数据库引擎调成一致（引擎在建项目时决定） |
| `cron_scheduler_unavailable` | 调度器未运行，如实告知用户 |
| `lease_acquiring` / `lease_held` / `too_many_go_runs` | 稍后重试一次 |
