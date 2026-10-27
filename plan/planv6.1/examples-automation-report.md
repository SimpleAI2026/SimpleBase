# 能力示例自动化执行报告

> 日期：2026-10-15。分支 `cursor/examples-automation-6b53`。
> 服务由 `./build.sh` 编译后在本机 `:8080` 启动，存储保持 `config.example.yaml` 的本地盘（`storage.database/files: local`）。
> 本文只记命令、是否通过、耗时和原因。不含密钥、令牌或 API Key。

## 1. 原有示例盘点

`examples/` 里原来是四个业务案例，由 `examples/init.sh` 和各目录 `simplebase.json` 初始化，不直接覆盖资源 CLI 的每一条命令：

| 目录 | 项目 | 侧重点 |
|---|---|---|
| `shop` | `ex-shop1` | JS SDK、事务下单、云函数、商品图片 |
| `community` | `ex-cmty1` | Go SDK、文档/SQL 双模型、附件 |
| `iot-telemetry` | `ex-iot01` | HTTP、遥测汇总、定时任务 |
| `ops-assistant` | `ex-ops01` | 工单事务、只读 Key、SLA |

这四个案例保留。新增的是 `examples/capabilities/`，按 `cmd/simplebase` → `internal/sbcli` 的真实命令面来写。

## 2. 新增示例

每个目录都有 `README.md`、`run.sh`、`expected.md`。公共步骤在 `examples/capabilities/lib/common.sh`。总入口是 `examples/run_all.sh`。

| 目录 | 覆盖 |
|---|---|
| `01-project` | `project list/create`、`settings set/get`、`quota get`、`audit list`、`log search`、`log retention get` |
| `02-auth` | `user create/get/list/delete`、`apikey create/revoke`、只读 Key 可读、不可建库、吊销后拒绝 |
| `03-sql` | SQL 库、`schema create-table/add-column/show/rows`、`sql exec/query` 的插入、更新、删除 |
| `04-documents` | 集合库、集合、文档插入/列表/更新/删除 |
| `05-kv` | `SET/GET/INCR/HSET/HGET/DEL` |
| `06-functions` | 创建、读取、调用、新版本、试跑、激活 |
| `07-storage` | 上传、列表、预签名、删除 |
| `08-cron` | 目标函数、interval 任务（默认关闭）、手动触发、运行记录、删除 |
| `agent` | 云助手自然语言。`@数据库` `@键值` `@云函数` `@对象存储` `@项目` `@定时任务`，SSE，删除确认 |

云沙盒没有进这轮。`config.example.yaml` 里 `sandbox.enabled: false`，也没有沙盒 API Key。用户和 API Key 只在 CLI 里测：当前 skill 清单（`internal/gocloudagent/skills.go`）没有认证模块，`@` 无法把 `user` / `apikey` 交给助手。

重复执行：`./examples/run_all.sh --start`。已有服务时去掉 `--start`。`--only 06-functions,08-cron` 和 `--skip-agent` 可缩小范围。`RUN_ID` 默认随机 6 位十六进制，避免和上一轮资源重名。

## 3. 环境

四个密钥环境变量都已存在且非空：`SIMPLEBASE_S3_ACCESS_KEY`、`SIMPLEBASE_S3_SECRET_KEY`、`SIMPLEBASE_AUTH_APIKEY_SECRET`、`SIMPLEBASE_LLM_PROVIDER_API_KEY`。值只写入本地 `.env`（权限 0600，已被 `.gitignore`），没有进入提交、PR 或本报告。

`SIMPLEBASE_S3_ENDPOINT`、`SIMPLEBASE_S3_REGION`、`SIMPLEBASE_S3_BUCKET`、`SIMPLEBASE_S3_PREFIX` 为空，因此没有做 S3 连通性探测，对象存储用例走本地盘。

`.env` 里的非密钥项与任务要求一致：`SIMPLEBASE_CONFIG_PATH=config.yaml`、`SIMPLEBASE_PERF_STAGE_TIMING=true`、`SIMPLEBASE_LOG_LEVEL=debug`、`SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL=zai-org/GLM-5.3-Flash`、`SIMPLEBASE_LLM_PROVIDER_BASE_URL=https://api.siliconflow.cn/v1`。

启动进程时另外在内存里把同一把模型密钥映射到命名 provider `openai`，超时 120s，并把 `SIMPLEBASE_LLM_AGENT_RUN_TIMEOUT=8m`、`SIMPLEBASE_LLM_AGENT_MAX_ITERATIONS=12`。这三项不在 `.env` 里。不设 `SIMPLEBASE_LLM_PROVIDERS` 时，配置加载会走简写并把超时固定成 60s，多轮工具调用容易不够。

`./build.sh` 本身不会生成 `config.yaml`（只有 `./build.sh dev` 会）。第一次启动因此失败：`open config.yaml: no such file or directory`。已从 `config.example.yaml` 复制本地副本，并让 `run_all.sh --start` 在文件缺失时做同样的复制。`config.yaml` 不提交。

## 4. CLI 结果

第一次全量（`RUN_ID=a80345`）：**60 通过 / 3 失败 / 63 步**。命令耗时合计约 3.2s；定时任务轮询另有约 20s 睡眠。两处失败是示例断言写错，已改脚本后用新的 `RUN_ID` 重跑，13/13 通过。剩下的失败是产品缺陷，脚本仍会失败，没有改产品代码。

修正后的有效结果：**62 通过 / 1 失败 / 63 步**。

### 4.1 通过项（第一次，毫秒为单条命令）

| 用例 | 步骤 | 耗时 |
|---|---|---|
| 01-project | list 38、create 385、settings-set 63、settings-get 25、quota 85、audit 37、log-search 38、log-retention 27 | 8/8 |
| 02-auth | user-create 70、user-get 26、user-list 30、apikey-create 36、readonly-list 34、readonly-write-rejected 14、apikey-revoke 33、revoked-rejected 26、user-delete 47 | 9/9 |
| 03-sql | create-db 69、create-table 143、add-column 51、schema-show 31、insert 31、query-insert 25、update 36、query-update 24、delete-row 32、query-empty 23、delete-db 129 | 11/12，见失败项 |
| 04-documents | create-db 75、create-collection 132、insert 26、list 21、update 28、list-updated 19、delete-doc 25、list-empty 19、delete-db 122 | 9/9 |
| 05-kv | set 65、get 25、incr 46、hset 42、hget 31、del 46、get-missing 24 | 7/7 |
| 06-functions | create 124、get 35、invoke-v1 33、version-create 58、activate 49、invoke-v2 38 | 第一次 6/7；重跑后 test-v2 47ms 也通过，7/7 |
| 07-storage | upload 38、list 21、presign 19、delete 30、list-after 21 | 5/5 |
| 08-cron | function 78、create 101、get 49、trigger 38、delete 59 | 第一次 5/6；重跑后 runs 27ms 通过，6/6 |

重跑的函数名和任务名用了新的 `RUN_ID`，没有和 `greeta80345` / `ticka80345` 冲突。

### 4.2 失败项

| 步骤 | 第一次 | 处理 | 最终 |
|---|---|---|---|
| `03-sql/schema-rows` | 34ms。CLI 退出非 0：`usage` / `gosdk: decode response: json: cannot unmarshal object into Go struct field TableRows.columns of type string` | 产品缺陷，不修。`expected.md` 改为写明当前行为 | 仍失败 |
| `06-functions/test-v2` | 47ms。断言 `.data.message`，实际试跑体是 `{ok,status_code,data:{message}}`，CLI 上是 `.data.data.message`。同一次 invoke 的 `.data.message` 是对的 | 改了 `run.sh` 和 `expected.md` | 重跑通过，47ms |
| `08-cron/runs` | 记录了约 20s。触发后的运行记录已经是 `status=completed`，`response_json` 含 `"n":2`，耗时 16ms。脚本只认 `success`，把成功结果记成失败 | 改为接受 `completed` 或 `success` | 重跑通过，27ms |

`schema rows` 的接口 `schemaRowsResponse.Columns` 是 `[]schemaColumnJSON`（`internal/api/schema_rows.go`），Go SDK `TableRows.Columns` 是 `[]string`（`packages/go-sdk/databases.go`）。`sql query` 的列名才是字符串数组，两边被 SDK 用同一个字段类型解码。直接调 HTTP 可以拿到行；只有走 CLI/SDK 的 `schema rows` 失败。

## 5. 云助手结果

模型 `zai-org/GLM-5.3-Flash`，经服务端 OpenAI 兼容出口。内置通用助手。`examples/capabilities/agent/run.mjs` 解析 `@`（与控制台 `skillsInText` 同一组别名），`POST .../runs` 且 `stream:true`，在 SSE 仍打开时 `POST .../confirmations/:callID` `{"approve":true}`。写用例之前把该助手 `PATCH {writable:true}`，只读守卫先 `PATCH {writable:false}`。

`RUN_ID=a397ed`。SSE 八条全部通过，墙钟合计约 188s。

| 用例 | 指令要点 | 结果 | 耗时 | CLI 是否达成 |
|---|---|---|---|---|
| readonly-guard | `@数据库` 创建 `ro-a397ed` | 通过 | 17.4s | 调用了 `database create`。工具错误 `write_not_allowed`。库未创建。确认次数 0 |
| sql-crud | `@数据库` 建 SQL 库、建表、加列、插入、查询 | 通过 | 44.4s | 6 次 CLI。表 `items` 列含 `id,title,qty`，`qty=3` |
| documents-delete | `@数据库` 集合文档，删除需确认 | 通过 | 46.9s | 插入后 `document delete` 发出确认并被批准，再列表为空 |
| kv-delete | `@键值` SET/GET/DEL | 通过 | 25.8s | `DEL` 进入确认。之后 GET 为 null |
| function-invoke | `@云函数`，源码走 stdin，`--file -` | 通过 | 10.5s | `function create` + `invoke`。返回 `hello simplebase` |
| object-delete | `@对象存储` 上传后删除 | 通过 | 21.7s | `object upload --file -`，`object delete` 已确认，对象不在 |
| quota | `@项目` 只读 `quota get` | 通过 | 6.4s | 只调用了 `quota get`，无 `skill_not_loaded` |
| cron | `@定时任务` `@云函数`，创建关闭的任务，不触发不删除 | 通过 | 14.8s | 函数 `aticka397ed`，任务 `enabled=false`，`func_export=Tick` |

删除确认出现在文档、KV、对象三条。只读守卫没有弹出确认，写权限拦截发生在确认之前，和 `cli_tool.go` 的顺序一致。

### 5.1 本地 `gocloudagent` 二进制

这是另一条入口：本机 shell/文件工具，模型仍走服务端。headless `--json -p` 没有界面确认通道，所以只要求它执行 `database list`。

| 次序 | 结果 | 耗时 | 原因 |
|---|---|---|---|
| 1 | 失败 | 5.3s | 模型调用了 shell 和 `simplebase database list`，但 shell 会丢掉全部 `SIMPLEBASE_*` / `GOCLOUDAGENT_*`（`internal/gocloudagent/tools/tools.go` `sanitizedEnv`）。CLI 回报 `config_missing`。模型正确说出 `ok` 为 false |
| 2 | 失败（脚本） | 7.1s | 改为写入模式 0600 的 XDG 配置后，模型列出了真实库并说明 `ok` 为 true。检查器在未解析的 JSON 事件文本里找 `"ok":true`，引号被转义，误判失败 |
| 3 | 通过 | 6.8s | 先解析 NDJSON 事件，再看工具参数和信封。配置目录在进程结束后删除 |

本地破坏性 shell 在没有确认通道时会 `confirmation_denied`。这次没有做删除。

## 6. CLI 与云助手是否够用

CLI 够用。项目、认证、SQL、文档、KV、函数、对象、定时任务、配额、审计、日志都能用真实进程走通。唯一被命令面挡住的是 `schema rows`。

云助手（服务端 SSE + `@` skill）在这八条自然语言上够用：会选对子命令，会先 `database list` 拿库 id，会把源码放进 stdin 而不是读宿主机文件，删除会停在确认上而不是自己跳过。写权限默认关闭是有效的；打开之后写路径与 CLI 一致。

本地 `gocloudagent` 不能靠环境变量把令牌交给 shell，这是故意的。要让它操作同一套 CLI，得事先写好 `simplebase` 配置文件，并且不要让模型把文件打出来。它没有服务端那套 `@数据库` skill 和确认框。

## 7. 体验与缺陷

按优先级。本 PR 不改产品代码。

1. **P0 `schema rows` CLI/SDK 解码失败。** 列元数据是对象数组，SDK 按 `[]string` 解。控制台若直接调 HTTP 则不受影响；脚本和 Go SDK 用户会在「看表数据」这一步失败。修法二选一：SDK 的 `TableRows.Columns` 改成与 `SchemaColumn` 相同的对象，或接口在该字段上返回列名字符串（与 `sql query` 对齐）。需要补一条解码测试。
2. **P1 设置接口的 JSON 字段是 `Key` / `ValueJSON`。** `settings get` 能用，但和其他资源的 snake_case 不一致。调用方必须写 Go 导出名。
3. **P1 内置助手默认不可写。** 行为正确，自然语言「建一个库」会得到 `write_not_allowed`，而且不会先弹确认。控制台需要让这个开关好找；skill 说明里可以写明「未开写权限时写命令会直接失败」。
4. **P1 本地 gocloudagent 的 shell 与 CLI 凭据隔绝。** 安全上合理，但模型只能看到 `config_missing`，无法区分「没配置」和「被剥离」。帮助文本可以写明：shell 看不到 `SIMPLEBASE_*`，请用 `simplebase` 配置文件。服务端助手已经有专用 `simplebase` 工具，不经过这条限制。
5. **P2 云函数 `test` 与 `invoke` 的 JSON 形状不同。** invoke 的 CLI `data` 就是函数返回值；test 的 CLI `data` 是试跑信封，函数返回值在 `data.data`。文档或 `function test --help` 应写明。
6. **P2 定时任务运行状态是 `completed`。** 示例最初按 `success` 判断。若对外文档写了 `success`，需要改成 `completed`。
7. **P2 `./build.sh` 不生成 `config.yaml`。** `./build.sh dev` 会。只编译再启动的人会看到找不到配置文件。`run_all.sh --start` 已自行复制示例配置。
8. **P2 认证不在任何 skill 里。** CLI 可以管用户和 API Key，云助手不能。若希望助手代管，要新增 skill，并单独设计确认（创建用户、签发 Key 都是敏感写操作）。
9. **P3 简写 LLM 超时固定 60s。** 多轮工具调用建议走命名 provider 并把超时调到 120s 以上。本次为了跑通，启动脚本在内存里做了这件事。

## 8. 后续

1. 修 `schema rows` 的 SDK 与接口契约，并加一条 `ListTableRows` 解码测试。这是唯一让能力示例红灯的产品缺陷。
2. `settings` 响应改为 snake_case，或在 CLI 输出里归一成 `key` / `value_json`。
3. 在云助手帮助里写清：默认不可写、本地 shell 不继承 `SIMPLEBASE_*`、`function test` 的返回值多包一层、任务运行状态是 `completed`。
4. 沙盒示例等 `sandbox.enabled` 和 API Key 具备后再加。不要在默认关闭时把用例算进 `run_all.sh` 的失败。
5. 业务案例（`shop` 等）仍用 `examples/init.sh`，不并进 `run_all.sh`。它们依赖控制台账号和可选的前端，和这条资源 CLI 链路分开。

助手这轮留下的资源在测试项目里：SQL 库 `sql-agent-a397ed`、集合库 `doc-agent-a397ed`、函数 `agreeta397ed` 与 `aticka397ed`、关闭的定时任务 `tick-agent-a397ed`。任务没有触发。需要清空时删这些对象即可，不要对整个实例 `reset`。
