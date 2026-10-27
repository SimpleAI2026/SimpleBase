# 云助手 CLI 与会话链路 Bugfix 计划

> **日期**：2026-10-12  
> **状态**：修复已落地（隔离测试通过），开发服务/容器部署及真实会话人工验收待用户执行；未对运行库执行 init_db 或任何 DDL  
> **相关计划**：[`cloud-assistant-skills-cli-v2-plan.md`](./cloud-assistant-skills-cli-v2-plan.md)、[`cloud-assistant-chat-ux-plan.md`](./cloud-assistant-chat-ux-plan.md)  
> **约束**：遵循 `AGENTS.md`、`internal/AGENTS.md`、`ui/AGENTS.md`；不新增依赖；任何数据库结构变更不得由修复脚本或代理自动执行

## 1. 现象与结论

本地 `http://127.0.0.1:5173/console/agents` 中点名 `@Database`，模型调用 `simplebase` 工具，参数为 `{"argv":["database","list"]}`，工具返回 `{"ok":false,"error":{"code":"cli_failed","message":"无法启动 CLI"}}`。

**已验证的本地直接原因**：监听 `:8080` 的 `simplebased` 由 `go run` 启动，可执行文件位于 Go 构建缓存目录；其同目录不存在 `simplebase`，仓库根目录也没有该可执行文件。本地 `config.yaml` 未显式设置 `agent.cli_path`。`internal/app/app.go:893-918` 在未配置 CLI 路径时按 `os.Executable()` 同目录寻找 `simplebase`，`internal/gocloudagent/cli_runner.go:43-69` 启动子进程，`internal/gocloudagent/cli_tool.go:252-268` 将非退出码类启动错误统一转换成 `cli_failed`。因此这次请求尚未走到 SDK/HTTP 的数据库列表接口；不能把本次报错归因于数据库内容、权限或 `@Database` 的解析。

`@Database` 是助手名称而非显式 skill（`ui/src/utils/agent-skills.ts:25-42`）；后端根据 Database 助手的 `database` 模块回落加载 database skill（`internal/api/agent_handler.go:552-560`、`internal/gocloudagent/skills.go:81-89`），所以本次不属于 `skill_not_loaded`。

## 2. 缺陷清单及优先级

| 优先级 | 缺陷与影响 | 代码证据 |
| --- | --- | --- |
| P0 | 开发模式 `go run` 令默认 CLI 路径指向 Go 缓存；`build.sh dev` 不构建同目录 CLI。`skills_cli=true` 时，依赖 CLI 的助手命令普遍无法启动。完整构建虽将两个二进制放在仓库根，仍不能修复一个**继续以 `go run` 启动**的进程 | `build.sh:307-310,459-465`；`internal/app/app.go:898-918`；`internal/config/config.go:452-455` |
| P0 | Docker 镜像只构建并复制 `simplebased`，不包含默认路径需要的 `/simplebase`；CLI 开启时镜像环境同样不可用 | `Dockerfile:6-12`；`internal/app/app.go:898-918` |
| P1 | 定时执行的 `RunRequest` 不带 `Skills`，但 `execSimplebase` 严格按本轮 skills 放行 `argv[0]`；因此即使 CLI 可执行，定时任务也会报 `skill_not_loaded` | `internal/gocloudagent/scheduler.go:287-291`；`internal/gocloudagent/cli_tool.go:194-200`；`internal/gocloudagent/skills.go:81-124` |
| P1 | 选择过 `@Database` 后，输入框 `selected` 缓存只追加不清理；删掉点名文本或进入后续一轮，仍可能携带旧助手 ID，意外路由到 Database 助手 | `ui/src/components/ai/AiChatComposer.vue:119,142-168`；`ui/src/pages/AgentManager.vue:705-714` |
| P1 | 发送及重试结束后刷新会话列表时漏传 `activeId`，把项目内其他助手的会话混入当前助手；切换混入的会话仅更新 `threadId`，不更新当前助手，可能导致同项目内会话与助手不匹配 | `ui/src/pages/AgentManager.vue:511-559,715-732`；`ui/src/services/http-api.ts:1281-1285` |
| P2 | 数据库列表 SQL 忽略 `page.Cursor`，当结果超过 page limit 时下一页重复第一页，next cursor 也重复。此缺陷**不是**当前 `cli_failed` 的原因 | `internal/catalog/sql_repository.go:223-256`；`internal/sbcli/commands.go:257-263` |
| P2 | database skill 示例传 `--data-model relational`，而实际只接受 `collection`/`sql`；照示例建库会得到参数错误 | `skills/simplebase/database/SKILL.md:34-39`；`internal/catalog/data_model.go:8-18` |
| P2 | CLI 启动时的原始错误没有安全的诊断出口：`cli_failed` 仅暴露通用描述；需在不泄露 token、argv、SQL、LLM 正文的前提下提供安全分类和可观测性 | `internal/gocloudagent/cli_tool.go:252-268` |

## 3. 修复方案与顺序

### A. 先恢复 CLI 可用性（P0）

1. 明确部署契约：`simplebased` 与 `simplebase` 必须位于相同目录，或由 `agent.cli_path` / `SIMPLEBASE_AGENT_CLI_PATH` 指向实际可执行文件。不要依赖 shell PATH，也不要把 CLI 放进云沙盒；CLI 保持在服务宿主机运行、仅接收既有最小化环境变量和委托令牌。
2. 调整 `build.sh dev`：在启用 CLI skill 的开发配置下先构建 `cmd/simplebase` 到稳定位置，再显式配置**开发服务进程**的 CLI 绝对路径；继续使用 `go run` 时不能依赖其临时 `os.Executable()` 同目录。注意 `config.yaml` 可覆盖默认值，环境变量只用于既有 `SIMPLEBASE_*` 配置渠道。
3. 调整镜像构建和复制步骤，确保运行镜像中服务与 CLI 同目录并均可执行；检查镜像所需 CGO/动态库是否满足现有后端要求。不要仅为回避缺文件而关闭 `skills_cli`。
4. 在装配或启动阶段为启用 CLI skill 的生产/开发部署增加可执行文件可用性检查，返回可定位的**路径类别**与失败类别（不存在、权限、格式等）；日志不得记录委托 token 或模型正文。避免把真实路径或敏感参数回传给模型。
5. 增加真实子进程的最小只读集成测试：通过临时目录中的服务/CLI 布局或显式路径启动 `simplebase database list`，让测试走 CLI → SDK → 真实 router/fake catalog；现有 `InProcessCLI` 用例不能覆盖子进程缺失。分别覆盖 dev 路径、同目录打包、缺 CLI 的启动诊断及 Docker 产物检查。

**第一验收门槛**：用实际 CLI 可执行文件和测试项目，执行只读 `@Database` → `database list`，确认工具返回 API JSON，而不是 `cli_failed`。不得在验证时执行数据库初始化、ALTER/DDL 或创建/删除资源。

### B. 修复定时任务 skill 装载（P1）

- 在调度入口为本轮按助手 module 生成与交互式 run 相同的 normalized skills；保持 allowlist 强约束，不通过放宽 `AllowArgv` 解决。确认 general 等无默认 module skill 的调度规则：无显式授权时不得自动开放全部命令；必要时在配置/调度记录里显式持久化允许的 skill（若涉及迁移，**仅定义迁移，由用户手动执行，或正常启动流程应用；`TIMER_NO_RUN=1` 不会运行迁移**）。
- 用 fake LLM + CLI/真实 router 测 `Database` 定时 `database list` 成功、未授权命令 `skill_not_loaded`、破坏性命令仍 `confirmation_unavailable`；勿通过生产定时任务进行测试。

### C. 修复前端点名与会话归属（P1）

- `mentionsForSend` 只从**当前输入文本中实际存在**且仍属于当前项目助手列表的点名构建；`selected` 仅辅助候选选择/光标体验，在文本删除、成功发送、切换助手或线程后清理，避免跨轮遗留。测试覆盖先选择 `@Database` 后删除点名、发送下一轮、切换助手的实际 `mentions`。
- 发送和重试结束时带 `activeId` 请求会话列表，并在异步返回时核对请求时的 project/agent 是否仍为当前上下文，再更新 `threads`。在选择会话、创建 run 的服务端边界核对 thread 与 agent 归属，避免错误列表或手工构造请求导致同项目跨助手错投；勿削弱现有 project、鉴权和委托校验。测试覆盖多助手、快速切换、旧请求晚到及错误 thread/mention 组合。

### D. 独立修复数据库分页与 skill 示例（P2）

- `internal/catalog/sql_repository.go` 的查询同时使用 cursor 和稳定排序字段（现排序 `created_at,id`）；基于 cursor ID 取其排序锚点或定义明确的复合游标，确保筛选、排序、分页、next cursor 一致；检查无效/删除/跨项目 cursor 的行为及系统库分支。不改变系统库只读边界，不执行生产数据库结构变更。
- 修正 database skill 的数据模型示例为 `--data-model sql`，补测试使嵌入的示例值与 `NormalizeDataModel` 可接受值一致。数据库列表测试至少覆盖超过一页、重复时间戳、末页、跨项目与无效游标，并分别通过 repository 和 CLI/SDK/handler 的只读链路验证。

## 4. 回归与完成标准

1. 后端：`go build ./internal/... ./cmd/...`、`go test ./...`；重点验证 `internal/gocloudagent`、`internal/api`、`internal/sbcli`、`internal/catalog` 的针对性测试。使用 fake catalog/router，不连接或修改运行中的真实库。
2. 前端：`cd ui && yarn test && yarn build`；测试真实点名行为与会话切换，不仅断言桩组件的 emit。
3. 开发/容器部署分别验证 CLI 路径；确认缺 CLI 时能从安全的日志判断失败类别，同时工具返回不泄漏 token、argv、SQL 参数或 LLM 正文。
4. 只读人工验收顺序：进入现有线程 → `@Database` 列库 → 测多页分页 → 删除点名再发送 → 切换助手检查会话列表；定时任务通过隔离测试验证，不在用户环境触发写动作。
5. 如需修改表结构，仅提交迁移定义与说明，**不自动运行 `init_db` 或任何 ALTER/DDL**；用户自行执行迁移，或由 `TIMER_NO_RUN=0` 的正常启动流程执行。

## 5. 实施状态（2026-10-12）

| 阶段 | 状态 | 落地内容与验证 |
| --- | --- | --- |
| A 开发环境 CLI | 代码完成，待重启开发服务 | `build.sh dev` 在启用 skill 且未显式配置路径时构建 `simplebase`，通过 `SIMPLEBASE_AGENT_CLI_PATH` 将绝对路径传给 `go run` 服务；不会覆盖用户显式路径或禁用配置。`bash -n build.sh` 通过；**尚未运行 `build.sh dev`**，避免触发启动数据库迁移。 |
| A 镜像产物 | 代码完成，镜像未实测 | `Dockerfile` 同时构建/复制两个二进制；本机 Docker 不可用，未执行镜像构建。Dockerfile 原有 `CGO_ENABLED=0` / distroless 静态镜像对 DuckDB 的适配未在此计划里解决，应在有 Docker 的环境单独验证镜像能否启动。 |
| A CLI 诊断和只读链路 | 完成隔离测试 | 云助手装配时按 `not_found` / `not_executable` 等安全类别发预警，不记录路径、令牌或正文；工具回复继续只返回通用 `cli_failed`。`TestValidateCLIPathAndSafeFailure` 和 `TestSubprocessCLI_DatabaseListReadOnly` 通过：在测试临时目录构建真实 CLI，并对 `httptest` 只读 GET 请求验证 CLI → SDK → HTTP；未执行用户线程。 |
| B 定时 skill | 完成 | 调度请求按 module 调用 `NormalizeSkills` 后传 `RunRequest.Skills`；`TestSchedulerScheduledSuccess` 验证 Database 回落，general 不扩大授权，原有 headless 确认拒绝行为保留。未在运行中的定时任务上验收。 |
| C 点名及会话 | 完成 | `AiChatComposer` 仅由当前输入中的精确点名生成 mentions，不跨轮缓存 ID；`AgentManager` 列表按当前 project/agent 刷新且旧响应不覆盖新状态，只选择当前助手会话；后端 `CreateRun` 拒绝与绑定助手不符的 thread/mention。前后端隔离测试通过，旧未绑定 thread 维持兼容。 |
| D 数据库分页/示例 | 完成隔离测试 | repository 使用 `(created_at,id)` 稳定游标，错误/跨项目游标返回 `invalid_cursor` 400，系统库同一分页路径；测试覆盖两页、末页、同时间戳系统库、跨项目和无效游标。skill 示例改为 `--data-model sql` 并补测试。未执行线上 SQL 或迁移。 |
| 配置 | 完成 | `config.example.yaml` 与本地忽略追踪的 `config.yaml` 均补充 CLI 二进制位置、`go run` 陷阱、环境覆盖及定时确认说明；本地凭据未更改。 |

**已执行并通过**：`bash -n build.sh`、`go build ./internal/... ./cmd/...`、`go test ./...`、`cd ui && yarn test`（覆盖率 functions 95.17%、branches 95.08%）、`cd ui && yarn build`。隔离测试使用临时测试库/测试 HTTP 服务，未运行真实 `init_db` 或 ALTER/DDL。

**仍待人工验收**：用户在允许正常服务启动（及自行处理所需迁移）的环境中重启后，按 §4 第 4 项验证真实 `@Database` 线程、开发脚本的 CLI 路径以及 Docker 镜像可用性。当前已在运行的 `go run` 进程不会自动获得新编译的 CLI 或加载新代码；不能宣称该线程已恢复。

## 6. 边界

本计划不将上述独立缺陷解释为本次 `cli_failed` 的直接原因；也不据此断言整个 Agent 系统均不可用。任何破坏性 CLI 命令、凭据读取或运行库迁移都不属于本计划的自动验收步骤。
