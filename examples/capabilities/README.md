# 能力示例

按 SimpleBase 资源 CLI（`cmd/simplebase` → `internal/sbcli`）覆盖主要能力。每个目录有 `README.md`、可执行 `run.sh` 和 `expected.md`。已有的四个业务案例（`shop`、`community`、`iot-telemetry`、`ops-assistant`）仍由 `examples/init.sh` 初始化，不在这里重复。

| 目录 | 能力 |
|---|---|
| `01-project` | 创建项目、项目设置、配额、审计、日志检索、日志保留 |
| `02-auth` | 创建用户、签发只读 API Key、写操作被拒绝、吊销 |
| `03-sql` | SQL 库建表、加字段、插入、查询、更新、删除 |
| `04-documents` | 集合库、文档插入、列出、更新、删除 |
| `05-kv` | SET / GET / INCR / HSET / HGET / DEL |
| `06-functions` | 云函数创建、调用、新版本试跑、激活 |
| `07-storage` | 上传、列举、预签名、删除 |
| `08-cron` | 定时任务创建、触发、运行记录、删除 |
| `09-triggers` | 事件触发器：record_change / http invoke、触发记录与队列丢弃计数 |
| `10-realtime` | 文档 SSE：create/update/delete 事件、订阅更新、Last-Event-ID 回放 |
| `11-backup` | 备份：create / list / download / restore / delete，KV 快照恢复 |
| `12-documents-advanced` | 文档进阶：patch / batch / schema / filter+fields / export→import / truncate |
| `13-dev-replay` | 本地→云端重放：dev validate / plan / apply，第二次全 skip |
| `14-sandbox` | 可选：云沙盒（E2B 兼容）create/get/list/pause/delete；不可用时 SKIP |
| `15-llm` | 可选：OpenAI 兼容 /v1/models、/chat/completions（含流式）；不可用时 SKIP |
| `agent` | 云助手自然语言、`@` 子 skill、SSE、删除确认 |

```bash
./examples/run_all.sh --start
./examples/run_all.sh --url http://127.0.0.1:8080 --skip-agent
./examples/run_agent.sh
```

`--start` 会执行 `./build.sh`（需要已有或将生成的 `config.yaml` 与根目录 `.env`），在后台启动 `./simplebase`，跑完 CLI 与云助手后停止本次拉起的进程。存储默认本地盘。S3 只在同时具备 endpoint、region、bucket 时才有意义；缺任何一项时这些示例不切换到对象存储。

口令、API Key 和访问令牌只留在进程环境里。`RESULT_FILE`（默认 `/tmp/simplebase-examples/results.jsonl`）里的失败详情会打码。

## 09–15 新增用例（P6）

| 目录 | 命令主线 | 断言要点 | 前置 / SKIP 条件 | 资源影响 |
|---|---|---|---|---|
| `09-triggers` | `function create` → `POST /function-triggers`（record_change、http）→ `document insert` → `GET /function-triggers` | 本地 HTTP sink 收到**恰好 1 次**带该 `record_id` 的回调；http 触发器 invoke 返回 202 且 sink 收到一次；`dropped=0`；PATCH 停用、DELETE 204 | 需要 `project:admin`；非系统项目；服务端与脚本同机才做 sink 断言（远端打印 SKIP 该步） | 无库则新建 1 个用户库并删除；函数、触发器均清理 |
| `10-realtime` | `curl -N .../realtime?collections=…&events=…&include_data=1` → insert/patch/delete → `POST /realtime/subscriptions` → `Last-Event-ID: 0` 重连 | connect 帧带 `client_id`；5s 内收到 `record.create/update/delete`（字段 id/collection/action/at；include_data 带正文）；订阅更新 204；回放命中 create 帧 | `database:read`；SSE 连接需要长连接超时保护（用例内部 kill） | 无库则新建 1 个用户库并删除；集合名带随机后缀 |
| `11-backup` | KV SET → `backup create` → KV 覆盖 → `backup restore` → KV GET → download → `backup delete` | create 返回 `status=ready`/engine/sha256/size；restore 后 KV 回到 before；下载字节数==size_bytes；删除后列表为空 | 服务端 `backup.enabled=true`，否则 `/backups` 路由不挂载、`backup list` 返回 404 → **SKIP**（只读实例 503 `backup_disabled` 同样 SKIP）；token 需 `project:admin` | 默认复用当前项目；若项目文件已被前序删库清理（`internal_error`）则**新建专用 `cap11-*` 项目（无法删除）**再验证；备份文件删除 |
| `12-documents-advanced` | `document patch/batch`、`collection schema-put`、`document list --filter/--fields`、`document export/import`、`collection truncate` | patch 是 merge；batch 2 篇；未注册字段不可 filter；缺 required 字段返回 `required_field`；导出 3 行→导入 scanned/imported=3；truncate 后 0 行 | 需要 `database:write`；truncate 需要 `project:admin` | 无库则新建 1 个用户库并删除；集合带随机后缀（复用时残留集合无冲突） |
| `13-dev-replay` | `dev validate` → `dev plan --seed` → `dev apply --seed --yes` ×2 → `dev status`；可选 `sbdev.py validate` 对账 | 第二次 apply `changes=0` 且所有动作 `verb=skip`；CLI 读回 db/function/cron 各 1；临时工作区不落仓库 | 目标项目不能已有其他名字的用户库（否则 409）；`dev apply` 远端目标需 `--yes`（用例已带） | 新建 `devmain-*` 用户库 + `devfn*` 函数 + `devcron-*` 定时任务，结尾删除；`mktemp` 工作区整体删除 |
| `14-sandbox` | `sandbox capabilities/create/get/list/pause/delete` | 可用时 sandboxID 一致、list 1 个、delete 后 0 个 | 依赖本机隔离：Linux bwrap+Landlock / macOS sandbox-exec；`available=false` 或 `sandbox_unavailable` → **SKIP** | 可用时创建 1 个沙盒并删除；不建库 |
| `15-llm` | `GET /v1/projects/:p/v1/models`、`POST /chat/completions`（含 `stream=true`）、无鉴权 401 | models 为 OpenAI list 形状；chat `object=chat.completion`、content 非空、usage 非零；流式 chunk + `[DONE]`；无 key 401 `unauthenticated` | 需配置 LLM provider 与 `llm:invoke`；路由 404/403、模型列表为空、上游 502/504 → **SKIP** | 不建库、不写项目文件（只产生用量记录） |

### 推荐注册顺序

```text
01-project → 02-auth → 11-backup → 03-sql → 04-documents → 05-kv → 06-functions
→ 07-storage → 08-cron → 09-triggers → 10-realtime → 12-documents-advanced
→ 13-dev-replay → 14-sandbox → 15-llm
```

- `03`/`04` 结尾会 `database delete`：删库只清空用户表、视图与集合元数据，项目级 KV 与备份保留，
  所以 `11-backup` 放在任意位置都能复用 `01-project` 的项目（推荐顺序沿用原排列）。
- `09`/`10`/`12`/`13` 都会新建并在结尾删除自己的用户库；任一用例失败留下库时，后续用例会收到
  409 `database_already_exists`（每个项目只允许一个用户数据库）。`13-dev-replay` 会先清理
  `devmain-*` 残留；其他用例无法安全清理别人的库。
- `14`/`15` 是可选能力，放最后，前置不满足时打印 SKIP 并 exit 0，不计 FAIL。
- run_all 的 `cases` 数组由维护者统一注册；本目录不修改 `run_all.sh`。

### 每个 run.sh 的公共约定

- 只依赖 `lib/common.sh` 的 `step/expect/record/done_step/finish_case` 与 `jq`；
- 需要环境变量 `SIMPLEBASE_BIN`、`SIMPLEBASE_URL`、`SIMPLEBASE_TOKEN`、`SIMPLEBASE_PROJECT_ID`、
  `RUN_ID`、`RESULT_FILE`、`CASE`；
- 资源名带本次运行随机后缀，可重复执行；结束时清理自己创建的函数 / 触发器 / 定时任务 / 沙盒 /
  备份，用户库只在「本次新建」时删除；
- 失败详情经 `redact` 脱敏后写入 `RESULT_FILE`。
