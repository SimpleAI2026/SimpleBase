# SimpleBase 内部模块说明

`internal/` 下的包按职责分层。所有业务路由经 `api` 层进入，底层依赖通过 `app` 装配。分层约束与禁止项见 [`AGENTS.md`](AGENTS.md)。

## 模块清单

### 入口与装配

- `app`：依赖装配与服务生命周期。`assembleDeps` 按序创建对象存储、系统库、catalog、registry、auth、llmgateway、usage、audit、云 Agent、cron、备份等；`Close` 反向释放；`RunWithSignal` 监听 SIGINT/SIGTERM 并在超时内优雅退出；`reset.go` / `doctor.go` 是运维子命令。
- `config`：从 `config.yaml` 加载非密钥配置，再用 `SIMPLEBASE_` 前缀环境变量覆盖（密钥与 `SIMPLEBASE_CONFIG_PATH` 引导路径除外）。`config.example.yaml` 是可加载的字段清单。
- `api`：HTTP 入口内核（Echo v4）。`router.go` 装配全局中间件与 `/v1` 路由；`routes_*.go` 只做路由注册与守卫（`routeGuards` / `policyGate` / `projectContextMiddlewareEcho`）；`error.go` 是 Echo 错误兜底（E2B 分流 + `httpkit.ErrorHandler`）；`stage_timing.go` 只剩认证子阶段观察者；内置 MCP（`mcp_builtin*.go` / `mcp_dispatch.go` / `mcp_handler.go`）留在内核侧，因为它经 `subRequest` 回到完整路由树；`Dependencies` 直接引用业务子包类型，不保留 `*_compat.go` 转发层。业务 handler 全部在子包：`httpkit`（协议内核）、`agenthttp`（云 Agent / 协作群）、`datahttp`（数据库 / SQL / Schema / 集合 / KV / 实时）、`sandboxhttp`（沙盒 + E2B + SSH 终端）、`functionhttp`（云函数 / 触发器 / 定时任务）、`filehttp`（对象存储 + 站点）、`iamhttp`（登录 / 用户 / API Key / 终端用户）、`llmhttp`（OpenAI 兼容出口 + trace 查询）、`opshttp`（指标 / 日志 / 设置 / 配额审计 / 迁移状态）、`cataloghttp`（项目）、`backuphttp`（备份恢复）。`perfbench/` 是压测用例。`TestKernelResidueFiles` 锁定顶层文件清单。
- `web`：`go:embed` 内嵌的前端构建产物与 SPA fallback。

### 身份、元数据与数据

- `auth`：`Principal`、`Permission`（`database:read/write/admin`、`llm:invoke`、`project:admin` 等）、API Key / 会话 / 终端用户校验、委托令牌；`Require` 在路由层按权限拦截。
- `catalog`：单实例元数据（tenants / projects / databases / api_keys …）与状态机（creating→ready、deleting→deleted；degraded 保持降级）；`errors.go` 定义领域错误。
- `systemdb`：系统库（DuckLake）bootstrap、最终态建表（`schema.go`，无版本化迁移）、常驻连接与各实体仓储（agents / cron / 沙盒 / 站点 / 审计 / 日志 …）。
- `dbbridge`：宿主与独立模块 `godatabase` 的适配层（`catalog.Database` ↔ `Spec`、`Logger`/`Observer` 适配、`Registry` 包装）。
- `collection`：集合（文档）数据模型的纯逻辑。
- `objectstore`：S3 边界（KeyBuilder、descriptor、文件存储、探测）。对象键使用内部 UUID，禁止用户可控字符串入键；磁盘契约的金标测试在本包。
- `backup`：定时备份与恢复。
- `realtime`：集合变更的进程内 hub（SSE / 订阅）。

### 计算与自动化

- `functionrun`：`/go`、试跑、cron、事件触发器共享的活动版本执行器与全局并发槽位，并写调用流水。
- `trigger`：记录 / 文件 / HTTP 事件的有界进程内队列、匹配与重试。
- `cronjob`：cron 任务调度。
- `sandbox`（含 `sshd/`）：云沙盒业务层（Manager、限额、路径、reaper）；执行后端来自独立模块 `gosandbox/vm`。
- `e2bproto`：E2B 兼容协议的帧编解码。

### Agent 与 LLM

- `agentbridge`：SimpleBase 宿主 Agent 业务与独立模块 `gocloudagent` 的适配（`RuntimeProvider`、工具、模块目录、命令授权、委托签发、`InProcessCLI`、agent 定时任务）。`agentskills/simplebase/` 是 `go:embed` 的技能文档。
- `agentrun`：Agent 协作上下文的应用服务。固定助手会话的 run（创建 / 执行 / 收尾 / 取消 / 审批放行）、协作群轮次（提交 / 订阅 / 动作）、群定时（实现 `agentbridge.GroupCronRunner`）、run 事件环与确认表、规划工具装配、工作台标签解析都在这里；它是群聊引擎的宿主适配（`group_store.go` / `group_runner.go` / `group_engine.go` 实现 `gocloudagent/group` 的 Store / Runner / Members 端口）。生命周期归 `app`：`BeginShutdown` 取消在途 run / 轮次，`Wait` 等收尾落库；仓储经消费方窄接口注入，不认识 HTTP。
- `llmgateway`：provider 配置、Chat / Stream、CredentialRef；密钥禁止从环境变量自动发现。
- `sbcli`：`simplebase` 命令行（含 `dev plan/apply`）与命令只读判定 `CommandReadOnly`。

### 治理与观测

- `usage`：配额检查与用量聚合。
- `audit`：不可变审计事件，默认脱敏（只记 SHA-256 与元数据）。
- `alerts`：磁盘等告警总线。
- `observability`：结构化日志（Zap）、Prometheus 指标、健康检查聚合。

### 公共与测试

- `utils`：宿主私有的零业务依赖工具（`crontab` / `strutil` / `mask` / `keyedlock`），均只 import 标准库。数据库运行库与宿主共用的 `atomicfile`、`hashx`、`limit`、`singleflight`、`sqlval`、`sqlx` 在 `godatabase` 公共包，本目录不再保留副本。入库标准见 [`utils/README.md`](utils/README.md)。
- `testutil/fakellm`：测试与基准使用的假 LLM 服务。

## 依赖关系

以下由 `go list` 生成，只列 `internal/` 内部依赖，依赖只能自上而下。`api/<context>http` 是业务域入站适配（只提供 handler，
路由注册仍在 `api`，不得 import `api` / `app` / 同级子包），`api/httpkit` 是协议内核叶子包（错误协议 / 上下文 / SSE /
审计辅助 / 计时，不 import `api` 与子包）：

```text
app         → agentbridge agentrun alerts api api/backuphttp api/cataloghttp api/datahttp api/functionhttp api/httpkit api/sandboxhttp
              audit auth backup catalog collection config cronjob dbbridge functionrun llmgateway objectstore observability sandbox
              sandbox/sshd systemdb trigger usage utils/strutil
api         → agentbridge agentrun api/agenthttp api/backuphttp api/cataloghttp api/datahttp api/filehttp api/functionhttp api/httpkit
              api/iamhttp api/llmhttp api/opshttp api/sandboxhttp auth catalog config cronjob functionrun objectstore observability systemdb web
api/httpkit → agentbridge agentrun audit auth catalog llmgateway objectstore observability sandbox systemdb usage
api/agenthttp → agentbridge agentrun api/httpkit auth systemdb utils/strutil
api/datahttp → api/httpkit auth catalog collection collection/query dbbridge observability realtime utils/strutil
api/sandboxhttp → api/httpkit auth catalog config e2bproto observability sandbox sandbox/sshd systemdb utils/strutil
api/filehttp → api/httpkit config objectstore systemdb utils/strutil
api/functionhttp → agentbridge agentrun api/httpkit collection cronjob functionrun objectstore systemdb trigger utils/{crontab,strutil}
api/iamhttp → api/httpkit auth collection utils/strutil
api/llmhttp → agentbridge api/httpkit llmgateway systemdb
api/opshttp → api/httpkit auth config observability systemdb utils/strutil
api/cataloghttp → api/httpkit auth catalog observability
api/backuphttp → api/httpkit backup catalog
agentrun    → agentbridge audit auth catalog llmgateway systemdb usage   （cronjob / agentbridge 不得反向 import）
agentbridge → agentbridge/agentskills auth catalog cronjob dbbridge llmgateway objectstore sbcli systemdb utils/{mask,strutil}
systemdb    → auth catalog dbbridge objectstore observability utils/{mask,strutil,keyedlock}
llmgateway  → catalog observability systemdb
usage       → catalog llmgateway observability
backup      → catalog config dbbridge objectstore utils/crontab
catalog     → auth objectstore observability
cronjob     → systemdb ;  functionrun → systemdb ;  trigger → functionrun systemdb
audit       → catalog observability ;  dbbridge → catalog observability
sandbox     → config ;  sandbox/sshd → sandbox ;  sbcli → utils/mask
alerts / objectstore → observability ;  auth / config / observability → utils/strutil
collection / e2bproto / realtime / web / utils/* → （无 internal 依赖）；collection/query → collection utils/strutil
```
`catalog → auth`、`systemdb → auth` 是实际存在的合法依赖（`auth` 只依赖 `utils/strutil`，无环）。

嵌套独立模块（不得 import 根模块任何包）：`godatabase`、`gocloudagent`、`gosandbox`、`gofunction`，均位于仓库根；宿主类型经 `dbbridge` / `agentbridge` 适配注入。

## 测试

每个模块带单元测试；handler 测试用 fake service，不依赖真实 DuckDB / S3；新增写路径必须补「系统库拒绝」用例。

```bash
go build ./internal/... ./cmd/... && go vet ./internal/... && go test ./internal/...
```
