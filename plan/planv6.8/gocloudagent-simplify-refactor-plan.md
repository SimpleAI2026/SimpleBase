# gocloudagent 精简、通用逻辑下沉与边界重构计划

> 日期：2026-11-23。状态：**§0–§12 已按「激进，不考虑兼容」策略实施，2026-11-24 两轮复核收尾（§12.7–§12.9）**。
> **2026-11-24 追加 §13（已实施）**：通用 Plan 能力（goal / progress / 文件化计划）、群聊式「Plan 栅栏决策」、工具集扩展、`agentbridge` 通用能力继续下沉，以及云助手前后端缺陷修复。§13 与前文冲突时以 §13 为准。实施进度（2026-11-25 回写，第二轮）：**B9–B15 全部落地，原 B14「CLI 群模式」与 CB-BE-13 / CB-FE-03/04/10/13 也已补齐；剩余只有需要人工核对或产品另议的项，见 §13.15.6**。逐批状态见 §13.11 状态列，实施记录见 §13.15。
> 方法：以用户指定的 `skills/simplify-code/SKILL.md` v2.2.0 为准，采用 Assess / R-branch，并参照 Go、backend、refactoring、assessment 指南。
> 评估阶段仅产出本计划；实施阶段的变更、测试与偏离记录在 §12。
> 目标不是把 `agentbridge` 搬空，而是让通用机制只有一份实现、宿主策略有明确归属，并降低运行循环、协议适配和本地 CLI 的维护成本。

## 0. 范围、证据与基线

### 0.1 评估边界

- 主范围：`gocloudagent/{agentcore,cloudagent,cli,mcp,skills,tools,cmd}`、模块根 CLI 门面。
- 下沉来源：`internal/agentbridge`，包括 `agentskills` 的文档读取机制，但不迁移 SimpleBase 技能正文及命令授权表。
- 联动核查：`internal/app/assemble_agent.go`、相关 `internal/api` 调用者、当前架构约束及 `plan/planv6.8/internal-simplify-refactor-plan.md`。
- 不在本轮实施范围：`gosandbox` 驱动、`godatabase`、`llmgateway` 内部重构、数据库表结构、前端、评测框架、依赖升级。
- 生产模块进行了分区阅读；关键候选核查了实现和测试。不是全仓形式化等价证明，也不是一次完整安全审计。
- 文中行号对应评估时工作树，后续以**路径和符号名**定位；新增目标文件和 API 名均为设计建议，不代表已经存在。

### 0.2 当前工作树，而不是旧提交

评估时 HEAD 为 `dead32f`，工作区有大量已暂存及未暂存改动。本计划基于**当前文件内容**，不能将已有改动重置到 HEAD 后照搬。

已确认的前置工作，不重复列入本计划收益：

1. `internal/agentbridge/aliases.go` 已删除；不要再次安排删除或重建。
2. `agentbridge/bench` 已迁到 `bench/agent`，研究文档已迁到 `gocloudagent/docs`。
3. `agentskills` 的测试专用文档对账逻辑已有迁移；不要把它搬回生产代码。
4. 主体标识已采用 `auth.Principal.Actor()`，不恢复旧身份拼接函数或另造通用 Principal DTO。
5. `internal` 计划的历史“激进审批”仅针对其明确列出的范围，**不自动授权本计划改变 gocloudagent 公共 API 或运行语义**。
6. 保留文件头说明并在迁移时同步维护；不以批量删除注释充当主要精简收益。

### 0.3 实测规模

统计为 `.go` 文本物理行数，包含注释和空行，不含文档、生成物及依赖。

| 目录 | 生产文件 | 生产行 | 测试文件 | 测试行 |
| --- | ---: | ---: | ---: | ---: |
| `gocloudagent` 根门面 | 1 | 32 | 1 | 35 |
| `agentcore` | 5 | 925 | 3 | 277 |
| `cloudagent` | 16 | 2020 | 13 | 1994 |
| `cli` | 13 | 3665 | 12 | 2656 |
| `cmd` | 1 | 36 | 0 | 0 |
| `mcp` | 5 | 680 | 3 | 413 |
| `skills` | 1 | 88 | 1 | 32 |
| `tools` | 3 | 971 | 2 | 541 |
| **gocloudagent 小计** | **45** | **8417** | **35** | **5948** |
| `internal/agentbridge` | 20 | 2993 | 17 | 2541 |
| `internal/agentbridge/agentskills` | 2 | 146 | 1 | 259 |
| **总计** | **67** | **11556** | **53** | **8748** |

较大的生产文件：`cloudagent/collab_plan.go` 624 行、`tools/tools.go` 504 行、`agentcore/model.go` 491 行、`cli/repl.go` 467 行、`cli/client.go` 426 行、`cli/command.go` 418 行、`cli/agent_loop.go` 398 行。行数只是导航依据，不是强制拆分阈值。

### 0.4 本轮实际检查

| 检查 | 结果 / 限制 |
| --- | --- |
| `git status --short` 及目标范围状态核查 | 已执行，确认存在大量用户改动 |
| `git rev-parse --short HEAD` | `dead32f` |
| `go version` | `go1.26.1 darwin/arm64`；模块声明 `go 1.25.0` |
| `gofmt -l gocloudagent internal/agentbridge` | 已执行，无输出 |
| `git diff --check -- gocloudagent internal/agentbridge` | 已执行，通过；只覆盖该命令检查的 diff，不代表所有暂存/未跟踪文件验证 |
| Python 只读文件规模、函数锚点、测试名与依赖字符串统计 | 已执行；不是 AST 可达性、类型检查或编译验证 |
| 生产源码的 SimpleBase import 路径扫描 | 命中本模块路径及 `gosandbox/host`，未发现直接 import 根模块业务包；不能替代 `go list -deps` |
| `go build` / `go vet` / `go test` / `-race` / benchmark | **本轮未运行**；本次为只读规划，未建立运行基线，后续执行前必须补跑 |
| Linux / Windows / Go 1.25 环境 | 未验证；本机版本较新不构成最低版本兼容证明 |
| `init_db`、DDL、真实 LLM/MCP/S3 调用 | **未执行，后续亦不作为默认重构验证手段** |

旧计划中的“测试全部通过”“deadcode 清零”不能作为本计划当前工作树的实测结论。当前没有足够证据认定某个导出 API 可删除。

## 1. 总结与目标边界

### 1.1 优先级判断

收益最大的方向按顺序是：

1. **下沉重复的破坏性操作确认状态机**，保持命令分类与业务授权在宿主。
2. **下沉工具调用时序闸门**，让宿主不再实现模型事件的同步机制；事件顺序修复单列。
3. **下沉 argv 子进程执行与有界输出机制**，不混入本机 shell、云沙盒、令牌注入或业务退出码。
4. **复用工具 JSON 编码、技能文档读取、MCP HTTP 包装机制**。
5. **按职责拆分核心流协议与运行事件处理**，不强行合并云端和 CLI 的完整运行循环。
6. 删除已确认的无效计算、私有别名和纯转发；再考虑有指标的 glob、DAG、历史处理优化。

### 1.2 职责归属

| 能力 | 最终归属 | 为什么 |
| --- | --- | --- |
| 模型协议、native/text 回退、流分片合并、通用脱敏 | `agentcore` | 不应理解 run 持久化、宿主资源或 CLI 交互 |
| run 生命周期、事件、线程占用、确认等待、调用时序协调 | `cloudagent` | Runtime 与宿主工具都需要的 Agent 运行机制 |
| argv 子进程执行机制 | 先落 `cloudagent/process.go` | 当前服务于 run 工具，通过既有宿主业务入口调用；不引入万能进程框架 |
| MCP 协议、Session、HTTP 包装机制 | `mcp` | 与 server 存储及用户身份无关；继续强制宿主提供 HTTP client |
| 本机文件、shell、搜索、Workspace 确认 | `tools` | 本机工作区安全和输出协议；不是云业务工具仓库 |
| 本地命令、REPL、JSONL 会话、压缩、配置 | `cli` | 与用户终端和本地落盘紧密相关 |
| Markdown 读取、frontmatter 去除、通用 Catalog | `skills` | 不解释 SimpleBase skill ID、白名单或授权 |
| Principal、权限继承、只读降权、委托签发 | `agentbridge` / `auth` | 业务授权事实来源，不能搬入通用模块 |
| SQL/数据库/KV/S3/日志/云沙盒工具 | `agentbridge` | 需要项目资源、系统库保护和业务端口 |
| 固定六阶段、阶段工具权限、模块提示词、项目快照 | `agentbridge` | 是 SimpleBase 产品流程，不是通用 Agent 机制 |
| 持久化确认、HTTP/SSE、API 错误映射、服务装配 | 现有 `internal/api` / `app` | 本轮不把宿主适配变成新的运行平台 |

### 1.3 依赖约束

- `gocloudagent` 任何包均不得 import SimpleBase 根模块业务包。
- `cloudagent` 与 `cli` 不互相 import；确需共享的纯机制放既有下层包，不能让 CLI 为了十行去重依赖云 Runtime。
- `agentcore`、`skills` 不 import 上层；`mcp` 不回调到宿主包。
- 宿主新增运行能力优先从 `cloudagent` 门面使用。既有 `agentbridge` 对 `mcp`、`agentcore`、`skills` 的底层依赖与 README“唯一业务包”表述之间已有差异：本计划不顺手做全量 import 收口；必要时在 `cloudagent` 提供有用途的窄转发，禁止两侧各复制实现。
- 不新增根级平行目录、万能 `common`/`utils` 包、注册中心、工具执行框架或插件体系。
- 不改 `go.mod`、不升级 Eino/MCP SDK、不更换模型框架。

## 2. 必须冻结的契约

### 2.1 公共边界

- 保留 `Runtime`、`RunRequest`、`RunPreparation`、`RunResult`、`Event`、`ChatClient`、`Model`、`HTTPClient`、`SessionRunner`、`Session` 的现有导出签名和可用构造方式。
- `cloudagent/ports.go` 的公开模型别名是隔离 `agentcore` 的宿主契约，不等同于已删除的 `agentbridge/aliases.go`。
- `cli_entry.go` 的 `RunCLI` / `SetCLIVersion` 及 `cmd` 只依赖根门面的规则保持。
- HTTP 路径、CLI 命令与退出码、JSON/JSONL 字段和空值形态、SSE/NDJSON 事件、错误码及文案均不借精简调整。
- 不把生产不可达等同于库 API 无价值；公共删除需明确兼容性决策。

### 2.2 授权与执行顺序

SimpleBase CLI 当前顺序需建立特征测试：获取运行上下文 → 非 text 模式等待 gate → 检查取消 → argv/stdin 校验 → 当前令牌检查 → 已加载技能检查 → 策略缺失/错误拒绝 → Agent writable 检查 → 阶段只读检查 → 破坏性确认 → `tool_start` → CLI 配置与令牌检查 → 执行 → 结果映射。

其中部分次序未必理想，但改变后会影响首个错误、事件和确认次数，不能包装成等价重构。

MCP 当前顺序：分类 → writable → headless/确认 → `tool_start` → 调用及结果转换。不能在抽象时把确认移到 writable 之前，也不能把未知策略当作只读。

### 2.3 已发现的语义差异清单

| 看似相同的逻辑 | 当前真实差异 | 处理 |
| --- | --- | --- |
| CLI 与云 MCP 确认 | CLI 主体来自 `rc.Principal.Actor()`，MCP 来自 `req.Subject` | 两侧显式传入主体字符串；核心不解析身份 |
| headless 判定 | 宿主 `req.Headless || rc.Headless`；MCP 只看 `req.Headless` | 调用方归一为布尔值，不偷偷改来源 |
| 缺失 call ID | CLI 不兜底；MCP 用 `mcp:<server>/<tool>` | 在调用方保留，不由确认器统一生成 |
| Prepare 与 emit | 现有两侧都只在 emit 非 nil 时尝试 `Prepare` | 首轮保留；改变需独立确认 |
| gate 与事件 | `consumeEvents` 先调用 `OnToolCall`，再 emit `tool_call` | 注释并不等于保证；见 BUG-02 |
| MCP schema | 云端非法 schema 报错跳过；本地 `localToolInfo` 保留无参数 schema 的工具 | 不直接用同一个严格转换器替换两侧 |
| 失败结果截断 | 云 MCP 按 rune；本地 MCP 按 byte；协作提示词又有独立省略号规则 | 不统一默认值、单位或后缀 |
| 失败信封脱敏 | `toolFailure` 脱敏 message；`skillFail` / `toolsErrEnvelope` 未做同样处理 | 只共享编码机制，安全变化另列 |
| 环境变量过滤 | shell 与 MCP 的敏感键集合不同 | 不取并集/交集后声称等价 |
| 关闭次序 | Runtime 的 defer 逆序；MCP `sessionSet.Close` 和本地 `closeAll` 在集合内正序 | 分清“资源类别栈”与“集合内部顺序” |
| 历史预算 | 单消息 rune 截断后用字节长度计预算 | 不把 `len` 改成 rune 数 |
| 模型错误 | native→text 与 stream→chat 根据原始具体错误判定 | 归一化仍只在最终出口 |

## 3. 宿主通用逻辑下沉清单

本节全部为提案。P1/P2/P3 表示维护收益，不代表 bug 严重程度或已获执行授权。风险独立标注。置信度为“已读源码后推理，未执行用例”。

### M-01：确认流程下沉到 `cloudagent`（P1 / 高风险）

**证据**：`agentbridge/skilltool.go` 的 `execSimplebase` 121–160 行，与 `cloudagent/mcptool.go` 的 `InvokableRun` 60–74 行、`askConfirm` 131–151 行，重复 headless 拒绝、Prepare-before-notify、Wait、取消判断和 resolved 事件。

**方案**：新增 `cloudagent/confirmation.go`，提供一个窄的确认函数及请求参数结构（暂名 `RequestConfirmation` / `ConfirmationRequest`）。输入只包含 run/call/name/subject、headless、脱敏展示参数与文案、timeout、waiter、emit；输出明确区分批准、拒绝、不可确认与取消。不要把 SimpleBase 错误码、argv 分类、SQL 语义放入其中。

- 宿主先完成 `CommandPolicy`、writable 与阶段权限检查，再调用通用确认。
- MCP 仍先使用 `MCPPolicy`。两侧负责最终失败信封，避免错误码被统一器抹平。
- CLI 现有 confirmation requested/resolved trace 必须保持时点；用局部 emit 包装或最少的观察钩子实现，不造多阶段中间件链。
- `Prepare` 必须在 required 事件之前；`Wait` 接受的主体必须和 Prepare 相同。
- 保留 emit=nil、waiter=nil、callID 空值以及确认超时语义。尤其 `Wait` 返回 `context.Canceled` 但父 ctx 未取消时，现有 `ctx.Err()` 可能为 nil：先测试并登记 BUG-04，不在抽取中悄悄修正。

**删除终态**：`execSimplebase` 中完整确认状态机被替换，`mcpTool.askConfirm` 不再维护第二份同构实现；有意义的 MCP 参数/文案构造保留。

**验证**：迁移/扩展 `TestMCPToolDestructiveConfirmFlow`、`TestMCPToolDeniedReturnsEnvelope`、headless/writable/failclosed 用例；增加跨两个消费者的事件轨迹测试。用 fake waiter 的 Prepare 回调同步 Resolve，证明不产生提前确认 404。

### M-02：工具调用闸门下沉（P1 / 高风险）

**证据**：`agentbridge/callgate.go` 10–85 行完全不依赖宿主资源；`RuntimeProvider.Prepare` 72–74 行和 `execSimplebase` 77–81 行把通用模型事件同步机制留在宿主。

**第一阶段方案**：迁到 `cloudagent/callgate.go`，最小暴露 `ToolCallGate` 及 Wait/Release/DropRun。保留 `(runID, callID)` 复合键、空 callID no-op、1 秒等待上限和调用方对 text 模式的跳过；`RunPreparation.OnToolCall` 公共契约不删。宿主只接线，不再维护 mutex/channel 实现。

**第二阶段候选**：若 run 生命周期内部化能同时简化多个工具，才考虑 per-run coordinator 与自动回收；不在第一阶段新增全局工具包装层，不把 MCP、普通内置工具都强行套上一秒等待。

**必须拆开的行为问题**：

- 当前 Release 早于 `tool_call` emit，不能宣称“事件已经发送才执行”；改顺序归 BUG-02。
- 当前释放的检查和 close 不在互斥区，同 key 并发 Release 存在重复 close 风险；归 BUG-03。
- Wait 当前不接收 context，增加即时取消会改变延迟与签名，属于可靠性改进，不是纯迁移。
- 超时之后迟到 Release、DropRun 与等待交错、不同 run 相同 callID 都需覆盖。

**测试迁移**：`TestCallGate_WaitCleanupOnTimeout`、`WaitCleanupOnRelease`、`DropRun`、`DropRunDoesNotStrandOtherRunWaiter` 迁到机制所属包；宿主保留 `TestExecSimplebase_TextProtocolSkipsGate` 和接线测试。旧 `callgate_export_test.go` 只有在所有消费者迁好后才删除。

### M-03：argv 子进程与有界输出下沉（P1 / 中高风险）

**证据**：`agentbridge/cliexec.go` 的 `CLIRunner`、`ValidateCLIPath`、`cliStartFailure`、`SubprocessCLI.Run`、`limitOut`、`limitWriter`，绝大部分只依赖标准库。

**最小落点**：`cloudagent/process.go` / `process_output.go`。暂名 `ProcessRunner`、`ValidateExecutable`。不放 `tools.Workspace`，不引入 `/bin/sh -c`，不依赖 `gosandbox/vm`。

**宿主保留**：

- 空 Path 回落 `simplebase`、临时目录前缀 `sbcli-run-`、输出 64 KiB 等现有适配默认值。
- `SIMPLEBASE_URL/TOKEN/PROJECT_ID/OUTPUT/AGENT_RUN` 环境构造及固定 PATH。
- 函数测试 120 秒与普通命令 60 秒、CLI 退出码 0/2/4 映射。
- `InProcessCLI` 与 sbcli 的进程内执行适配。

**契约陷阱**：

1. stdout/stderr 分别限流，不能换成合并缓冲。
2. `*exec.ExitError` 当前转换为退出码且 err=nil；启动失败保留 err，不顺手改 `errors.As` 匹配范围。
3. `cmd.Env=nil` 在 Go 中表示继承父环境，非 nil 空切片才是显式空环境；现有正常宿主路径传完整非 nil env。通用 API 应明确这项差异；若要求 nil 一律拒绝/清空，需单独批准，不能称纯搬移。
4. 临时目录创建失败当前继续执行；改成 fail-closed 是额外行为变化，先建立失败注入测试。
5. 输出截断保留 UTF-8 边界、边界以下非法字节的现状和无截断标记格式。
6. 当前 Writer 跨越上限时短写无错误，见 BUG-01；不要先复制缺陷再把结果宣称等价安全。

**迁移方式**：保留宿主 `CLIRunner` 消费端口；`SubprocessCLI` 变成设置 SimpleBase 默认值的薄适配，而不是新增第二套执行器。测试中不依赖根服务二进制的输出/退出/临时目录用例迁到模块；`TestSubprocessCLI_DatabaseListReadOnly` 继续留宿主验证完整接线。

### M-04：工具 JSON 编码集中（P2 / 中风险）

**证据**：`agentbridge/tools.go` 231–236 行与 `cloudagent/tools.go` 61–66 行的 `marshalToolJSON` 同为 marshal → 原样返回编码错误 → `RedactSecrets`。其余失败信封结构也相近，但脱敏策略并不相同。

**方案**：先在 `cloudagent` 提供明确含义的 `MarshalToolJSON`，宿主直接调用，云端内部只保留一份实现。若本机工具确有相同消费者，再将纯实现下沉 `agentcore/tool_result.go` 并保留 cloudagent 门面；不为预测中的消费者预建包。

- `skillFail`、`toolFailure`、`tools.ErrEnvelope`、`toolsErrEnvelope` 先分组对照：编码形状可共享，脱敏时机不默认统一。
- map 换 struct 可能改变 JSON 字段顺序；当前工具结果作为字符串进入模型或 golden，先做字节级对照。
- `nil`、空数组、空 map、编码错误、中文、包含凭据样式正文都要测试。
- 保留“JSON 编码后做文本脱敏”的既有语义；改为逐字段脱敏或保证 JSON 可解析是独立方案。

### M-05：技能单文档读取复用（P2 / 低中风险）

**证据**：`agentbridge/agentskills/skills.go` 117–135 行和 `gocloudagent/skills/skills.go` 79–87 行重复 frontmatter 去除；两侧读取后均 TrimSpace。

**方案**：在现有 `skills` 包增加 `ReadDocument(fs.FS, path) (string, error)`，负责读取、去 frontmatter、TrimSpace；`Catalog.Load` 复用。宿主 `readSkill` 调用它并保留当前“读取失败返回空字符串”的行为。

**不采用直接全量 Catalog 替换的原因**：

- 宿主 `Prompt(nil)` 读取顶层 `simplebase/SKILL.md`；通用 `Catalog.Prompt(nil)` 返回空，不相同。
- 宿主按调用时读取，测试可替换 `skillFS`；全局缓存改变可见性和错误时机。
- 全量 Load 会让无关文档读取失败影响原本成功的请求。
- 维持输入 ID 顺序、重复 ID 行为和未知 ID 跳过行为。
- 解析器当前只识别特定 LF frontmatter 形态，不顺手“兼容”CRLF/BOM或更严格的 YAML 边界。

**绝不迁移**：`skillPrefixes`、`AllowArgv`、`AllowWriteArgv`、固定 skill 路径映射与 SimpleBase 文档。

### M-06：MCP HTTP 包装机制下沉（P2 / 中高风险）

**证据**：`agentbridge/mcp.go` 239–285 行的 Header 克隆注入、响应体限流；`cli/mcp_config.go` 243–260 行有相同 Header 包装骨架，但没有响应体限流。

**方案**：在 `gocloudagent/mcp/http_transport.go` 提供可组合的 Header transport 与限定响应体包装。宿主先构造自己的加固 Transport，再包通用机制；CLI 只复用 Header 部分，**不默认新增 body cap**。需要收口宿主业务入口时，由 `cloudagent` 提供窄门面，不复制实现。

**宿主不下沉**：URL scheme/AllowHTTP、`DisableExternal`、系统库存储、内置 server 身份、凭据查询、实际 IP 校验、禁止重定向、TLS 最低版本等云端策略。

**保留**：原请求不被修改、Header.Set 覆盖语义、MCP map 单值与 CLI Header 多值的差别、原始 RoundTrip 错误、Close 透传、超限错误文本。当前 `limitedBody` 在 remain=0 时即报错，不会再探测 EOF；换成 `io.LimitReader` 会从“超限错误”变成 EOF，不等价。

### M-07：计划校验复用仅列可选（P3 / 中风险，默认暂缓）

**证据**：`agentbridge/plan_stages.go` 34–61 行与 `cloudagent/plan.go` 18–48 行重复 goal/skip 与前序依赖校验，但宿主还有固定数量、顺序、首阶段不可 skip；通用版本额外做 ID trim 和重复 ID 检查。

不推荐“先检查全部固定顺序，再整批调用通用 validator”：一个早期 goal 错误和后期顺序错误同时出现时，首个错误会改变。也不推荐为了几十行增加可配置校验框架。

后续确有新增消费者时，可以提取**单阶段依赖检查**，让两侧保持原来的逐阶段顺序；固定阶段常量、阶段权限和提示词永久留宿主。当前两份实现短且差异明确，允许保留。

## 4. gocloudagent 内部精简与优化候选

### 4.1 核心模型与运行循环

| ID | 优先级 / 风险 | 位置与证据 | 方案及冻结点 |
| --- | --- | --- | --- |
| C-01 | P1 / 高 | `agentcore/model.go:142–269` 的 `Model.Stream` 同时做请求、回退、流桥接、TOOL_CALL 前缀判定和原生工具分片 | 先同包拆 `model_request.go`、`model_stream.go`、`tool_protocol.go`；再提取私有流状态，保留 Pipe 容量 8、Close 顺序、部分输出错误、Usage 次数、pending ID 和分片顺序 |
| C-02 | P2 / 中 | `Model.Generate:107–140` 与 `Stream:142–179` 重复请求与 native→text 重试 | 先统一纯响应→schema 转换，避免一个带多个回调的泛型重试器；最多重试次数、400/422且含 tool/function、auto限定、全局降级缓存更新时机不变 |
| C-03 | P2 / 低 | `buildRequest:279–325` 当前始终返回 nil error；参数名 `forceText` 与实际布尔含义相反（false 才强制文本） | 核对所有私有调用后移除无效 error 通道和不可达分支；参数改清晰的私有模式表达。不能改 `Model` 公共方法及 Protocol 空值现状 |
| C-04 | P1 / 中高 | `cloudagent/runtime.go:200–300` 的 `consumeEvents` 集中维护文本、卡片、索引、时间、事件与trace | 提取同包私有 `runEventState`，按 `handleToolResult` / `handleAssistant` 分段；保留无ID生成、同名工具倒序匹配、重复ID覆盖、map引用共享和事件实际顺序 |
| C-05 | P2 / 中 | `cloudagent/runtime.go:48–112` 混合 preparation、MCP、日志、agent创建、timeout | 仅提取内聚的工具装配段；清理所有权仍显式归 StartRun。Prepare报错也调用其Cleanup；不要把timeout提前到Prepare/MCP之前 |
| C-06 | P2 / 低中 | `cloudagent/collab_plan.go` 混合声明、解析、校验、提示词、状态归约 | 同包拆 `collab_types.go`、`collab_parse.go`、`collab_validate.go`、`collab_prompt.go`、`collab_state.go`，只按真实职责拆；公开符号和错误不变 |
| C-07 | P2 / 中，测量型 | `ValidateDispatchPlan` 重复调用递归 `taskDepth:368–379` | 由于依赖只能指向前序，可按原校验顺序计算depth数组，避免重复遍历共享依赖；保留unknown/later/depth首错顺序、重复边规则与MaxDepth归一化。用链、菱形、稠密DAG测量，不许声称已有提速 |
| C-08 | P3 / 低 | `ReduceTurnState:525–559` 多次扫描statuses | 可一次收集标志和Completed后保留原switch优先级；未知状态当前不自动报错，空集合为failed，不能“修正”默认分支 |
| C-09 | P2 / 低，测量型 | `historyToSchema:12–42` 对已选内容做两次相同truncate | 私有selected项携带已截断content，保持倒序选择→正序输出、遇超预算立即break、字节预算、额外summary预算；不改历史内容或序列 |
| C-10 | P3 / 低 | `cloudagent/trace.go` 私有 `tracer = Tracer`、`hashSummary` 纯转发；`delegation.go` 已无委托逻辑 | 包内统一显式 `Tracer` / `HashSummary`；确认默认超时移入 `confirmation.go`，空文件才删除。公开门面保留 |

**不强行合并 CLI 与云 Runtime**：`newCLIAgent` 显式顺序执行工具并有 unknown-tool handler；云端配置不同。CLI 写 JSONL、做重复检测与压缩，云端维护线程、卡片和宿主事件。现有 `agentcore.ConsumeAssistant` / `ConsumeVariant` 已共享底层流消费，继续在这一层收敛，不能回退到两个完整循环完全同构的假设。

### 4.2 CLI、MCP 与本机工具

| ID | 优先级 / 风险 | 位置与证据 | 方案及冻结点 |
| --- | --- | --- | --- |
| L-01 | P2 / 低 | `cli/mcp_config.go:186–193` 计算timeout，`ConnectHTTP` 实际固定传DefaultConnectTimeout | 删除无效局部计算；不要改成传timeout，那是改变连接超时。加连接配置断言 |
| L-02 | P2 / 中 | `cli/client.go` 包含HTTP、wire类型、SSE及错误翻译 | 同包拆 `client_wire.go` / `client_stream.go`；HTTP入口只共享请求创建/发送。Chat读8MiB、Stream错误体读1MiB、成功stream由调用者关闭等差异保留 |
| L-03 | P2 / 中 | `cli/command.go` 命令分派、sessions、config、usage混合 | sessions/config各移一个文件，保持帮助、别名、参数默认、stdout/stderr和退出码；不引入通用命令注册框架 |
| L-04 | P2 / 中高 | `cli/repl.go:261–362` slash分派与run的signal/context管理交织 | 提取明确的slash处理结果（未处理/已处理/退出）及单轮执行函数；保留模板匹配顺序、提示符、EOF、signal.Stop与cancel生命周期。`/compact` 的信号行为另列BUG，不顺手改 |
| L-05 | P3 / 低 | `mcpDisplay` 只是 `mcp.DisplayName` 转发，repl仅一处调用 | 可直接调用并删除私有包装，更新头注释；不以此作为主要收益 |
| L-06 | P2 / 中 | 云/本地MCP schema适配重复但失败路径不同 | 仅提取schema解码公共内核，调用方分别保留严格拒绝/宽松无schema；暂不新增严格选项旗标。若净复杂度不降，保留两份 |
| L-07 | P2 / 中高 | `mcpTool.InvokableRun` 与 `mcpLocalTool.InvokableRun` 重复调用和结果转换 | 先共用协议层调用事实，保留两侧授权、trace/Record、截断/脱敏差异；不做参数十余项的万能工具包装器 |
| L-08 | P1 / 低中，测量型 | `tools/fsutil.go:55–103` 每个候选文件重新Compile同一个glob正则 | 每次glob操作只编译一次，无全局cache；保留nil结果、排序、20,000文件上限、无权限跳过、skip调用及计数顺序。非法模式处理不能新增可见错误 |
| L-09 | P2 / 中 | `globWalkSkip` 与 `grepWalkSkip` 无glob路径重复WalkDir规则 | 提取私有文件遍历器，仅共享目录过滤、相对路径和计数；glob与grep保留各自输出顺序、匹配上限与truncated语义，不增加跟随symlink或新忽略规则 |
| L-10 | P2 / 低中 | `tools/tools.go` 同时维护Workspace、shell、文件编辑、glob/grep | 同包拆 `workspace.go`、`shell.go`、`file.go`，现有 `fsutil.go` 保留遍历；优先保留LocalTools注册顺序。仅拆文件不宣称净删行 |
| L-11 | P2 / 高，可选可靠性 | `tools/tools.go:215` 裸shell用CombinedOutput，事后才截断 | 改为有界写入需单独验证stdout/stderr交错、exitCode、取消和截断标记；衡量保留缓冲上界与内存。不复用宿主分离stdout/stderr的执行器来改变协议 |
| L-12 | P3 / 中，可选性能 | `cli/client.go:59–65` 默认每次克隆Transport | 先用本地httptest统计连接次数与分配，才决定客户端复用；公共HTTP字段可覆盖、DefaultTransport可替换、CloseIdleConnections生命周期必须明确，不直接加sync.Once锁住可变配置 |

### 4.3 小文件不是天然冗余

- `cloudagent/stream.go` 的 consumeAssistant 包装承担 Event DTO 映射，保留。
- `cloudagent/errors.go` 与 `agentbridge/errors.go` 正确分工：通用错误与宿主 llmgateway 哨兵；不再下沉宿主错误分类。
- `cloudagent/ports.go` 的模型别名、`RunProviderFunc` 适配和根 CLI 门面有真实隔离价值，保留。
- `skills.Catalog` 仅88行，主要问题是宿主重复读取，不需要拆成parser/registry/service三层。
- `PlanStageID` 宿主类型与通用string形状可保留适配，不为减少一次转换把固定业务类型公开进通用模块。

## 5. 疑似缺陷与额外行为变化：不计入等价精简

以下依据源码推理，**本轮没有执行复现程序**。必须先用确定性测试确认；修复独立批次，不改旧测试期待值来隐藏差异。

### BUG-01：输出Writer跨上限短写且无错误

- 位置：`agentbridge/cliexec.go:114–126`。
- 构造：缓冲剩2字节，传入4字节；当前截短p后返回2,nil，而不是报告完整输入已消费或给出短写错误。
- 影响：`io.Copy` 路径可能得到 `io.ErrShortWrite`，使本该“输出被截断但命令成功”变成CLI启动/执行失败；端到端表现依赖子进程退出与复制路径，需复现。
- 修复方向：保留原输入长度，限量存储、其余消费丢弃。断言n==输入长度、缓冲上限、后续写入不失败；stdout/stderr分别测。
- 优先在M-03搬移前修复，防止通用化复制缺陷。

### BUG-02：gate释放不保证tool_call先于确认

- 位置：`cloudagent/runtime.go:290–295` 与 `agentbridge/callgate.go`。
- 当前顺序：登记卡片 → `OnToolCall(id)`释放gate → `emit(tool_call)`。
- 构造：release唤醒工具goroutine，后者发confirmation_required；此时event goroutine尚未emit tool_call。
- 修复候选：在确定的事件登记/发送完成点释放gate；需界定emit是同步写出、同步排队还是仅回调，以及nil emit如何处理。
- 不可只交换两行就宣称安全：emit中等待工具会形成潜在死锁。用可控channel模拟顺序与阻塞，定义语义后再做。

### BUG-03：相同gate key并发Release可能double close

- 位置：`agentbridge/callgate.go:60–69`，slot获取锁后即释放，select/check与close在锁外。
- 构造：两个goroutine取到同一未关闭channel，均走default，然后close。
- 目前尚未证明生产调用者会对同key并发release；标记为防御性并发缺陷候选，而非已发生事故。
- 方案：关闭也在同一互斥区内，或slot持有once；同时证明timeout删除与迟到release不导致旧等待者失联。用barrier和race测试验证。

### BUG-04：确认取消可能返回nil错误

- 位置：`skilltool.go:144–146`、`mcptool.go:142–144`。
- 构造：fake waiter返回context.Canceled，但父ctx仍有效；代码进入取消分支后返回ctx.Err()==nil。
- 影响：CLI可能返回空成功结果，MCP可能落为拒绝而不是取消；需要分别锁定当前行为。
- 修复需明确优先返回父ctx错误还是Wait错误，保留正常取消的终态事件策略；不偷偷吞进M-01。

### BUG-05：CLI HTTP错误可能错误触发stream→chat

- 位置：`cli/client.go:374–404` 与 `agentcore/model.go:156–178`。
- 证据：`apiError`只有tools_not_supported恢复LiteLLMError，其它401/429等返回普通fmt错误；Model.Stream对非LiteLLMError且非context错误会回退Chat。
- 构造：httptest始终返回401/429，调用真实HTTPClient→Model.Stream，统计请求次数，预期当前可能发起第二次非流式请求。
- 修复方向：传递可辨别HTTP状态的错误，同时维持Error()文本兼容；保留tools_not_supported的400/422正确回退。不要把所有400都当不支持tools。

### BUG-06：MCP确认摘要注释称脱敏但实现只截断

- 位置：`cloudagent/mcptool.go:154–160` 的 `mcpConfirmArgv`。
- 构造：参数包含Authorization/password等样式，当前summary仅TrimSpace+truncate，进入confirmation_required事件。
- 应核对最终接收端是否另有脱敏；本轮未证明所有出口均泄漏。
- 若修复，在摘要构造点复用现有RedactSecrets，独立断言事件/trace不含原值；这会改变可观察文案，需独立登记。

### BUG-07：本地/云模型“长度终止”传播不一致

- 位置：`Model.Generate` 检查FinishReason=length，`Model.Stream` 回退Chat分支未作同等处理；`cli/client.go` 的wireChunk有FinishReason，但 `deltasFromChunk` 未透传该原因。
- 影响：部分流路径无法产生统一length终态提示；本轮只确认分支差异，未做全链路复现。
- 涉及StreamDelta公共契约或内部哨兵传播，单独设计，不在C-02去重时隐式补齐。

### 其他仅登记、不并入本轮默认修复

- `cli/repl.go` 的 `/compact` 注释称可Ctrl+C取消，但其局部context没有与正常run相同的signal接线；需真实交互或信号注入验证。
- `Model.Protocol` 注释称空值按text，`useNative` 实际把空值走缓存分支；不要按注释“修复”现行为。
- `ResetUnsupportedNativeTools` 公开且被跨包测试使用；直接重置sync.Map也有并发契约问题。先确认需求，再讨论测试归属和兼容，不能直接搬到 `_test.go`。

## 6. 目标文件布局与删除清单

### 6.1 目标是少一份实现，不是多一层转发

建议新增/拆分路径（按批次需要创建，不要求一次全建）：

- `gocloudagent/cloudagent/confirmation.go`：M-01及confirm默认值；删除误名的旧 `delegation.go` 前先清空有效符号。
- `gocloudagent/cloudagent/callgate.go`：M-02通用同步机制。
- `gocloudagent/cloudagent/process.go` / `process_output.go`：M-03执行和输出；宿主适配不携带通用实现。
- `gocloudagent/cloudagent/runtime_events.go`：C-04事件状态，`runtime.go` 保留编排。
- `gocloudagent/cloudagent/collab_{types,parse,validate,prompt,state}.go`：C-06同包职责拆分。
- `gocloudagent/agentcore/model_{request,stream}.go` / `tool_protocol.go`：C-01，名称以最终职责为准。
- `gocloudagent/mcp/http_transport.go`：M-06，不含宿主SSRF策略。
- `gocloudagent/skills/skills.go`：M-05优先在已有小文件扩展，不额外建目录。
- `gocloudagent/cli/client_{wire,stream}.go`、`command_{sessions,config}.go`：L-02/L-03。
- `gocloudagent/tools/{workspace,shell,file}.go`：L-10，同包移动。

### 6.2 可删除或收缩的内容

| 内容 | 删除前提 | 不得误删 |
| --- | --- | --- |
| `agentbridge/callgate.go` 实现 | 所有调用/测试改为通用gate，旧export-test引用清零 | CLI协议模式判断与宿主接线 |
| 宿主确认流程重复块 | M-01两个消费者及事件测试通过 | CommandPolicy、readonly stage检查、业务文案 |
| `cliexec.go` 输出/进程细节 | M-03完成，宿主适配仅设默认值 | CLIRunner端口、InProcessCLI、SimpleBase退出码映射 |
| 两份 `marshalToolJSON` 中一份 | M-04字节级与脱敏特征测试通过 | 对外错误文案和编码失败行为 |
| 宿主 `stripFrontmatter` | M-05已复用通用读取 | `skillPrefixes` 和Prompt空参数的索引逻辑 |
| 宿主Header/body包装 | M-06已接线，边界测试通过 | guardDialContext、isBlockedIP、URL/redirect策略 |
| `connectOne`无效timeout计算 | 保持固定连接超时 | HTTPClient调用超时计算 |
| 私有 `tracer` / `hashSummary` / `mcpDisplay` | 核对所有同包调用 | 公开 `Tracer` / `HashSummary` / `DisplayName` |
| `buildRequest`无效error分支 | 私有调用者全部编译通过 | Generate/Stream真实错误传播 |

对于未证实的“死代码”，默认不删。后续可运行已有 staticcheck/deadcode 工具，但必须区分 CLI入口、库导出、测试可达、反射/接口可达；不为本计划安装新工具。

## 7. 分批执行与回滚边界

### B0：建立运行基线与冻结测试

- 重新读取工作区状态和相关规则；记录原有失败，不覆盖用户改动。
- 跑模块及宿主定向build/vet/test，记录Go版本与平台。
- 补确认轨迹、gate交错、子进程输出、JSON字符串、技能读取、HTTP body上限的特征测试。
- 列出本批会改的公开符号；新增公共能力也要评审零值和生命周期。
- 验收：能区分“旧行为不理想”与“本批引入回归”，不存在只靠编译的高风险迁移。

### B1：低风险删除与纯文件移动

范围：L-01、L-05、C-10、C-03；C-06/L-02/L-03/L-10按实际需要选取纯移动部分。

- 先移动后精简，不同时改默认值和结构。
- 公共API/JSON/错误零变化。
- 每个职责一个逻辑检查点，不按文件数机械分割。

### B2：通用读取与编码

范围：M-04、M-05。

- 优先已有包落点，删除源端重复，不留下两份实现。
- Golden同时比较结构和字符串；维持未知技能、读取错误和重复ID行为。

### B3：确认与gate

范围：M-01、M-02。

- 先完成BUG-02/03/04的确定性复现与取舍。
- 等价搬移和获批可靠性修复必须是独立检查点；若修复影响新抽象设计，先修复再搬移。
- 同步 `app/assemble_agent.go` 与API确认服务装配；不同时留两套可漂移的timeout/Confirm配置。
- 核查直接运行、计划阶段、协作任务和cron/headless四条接入路径。

### B4：进程与MCP传输

范围：M-03、M-06。

- BUG-01先独立复现并处理；不把坏Writer推广为公共工具。
- generic进程测试用临时目录/fake子进程；服务二进制→HTTP集成测试留宿主。
- MCP测试不访问真实网络资源，使用fake RoundTripper/本地httptest/in-memory session。
- 不改变云端SSRF策略、本地局域网支持或隔离不可用时拒绝的行为。

### B5：核心职责精简

范围：C-01/C-02/C-04/C-05及按需L-04/L-06/L-07。

- 先冻结错误与流事件，再拆状态对象。
- 不合并云/本地完整runner，不统一截断或结果信封策略。
- 保持原始错误直到最终出口；测试必须覆盖native/text/auto和stream/chat组合。

### B6：有指标的优化

范围：L-08/L-09/C-07/C-08/C-09；L-11/L-12需额外批准与基线。

- 使用相同输入、同一工具链、冷/热条件、重复次数做前后对照。
- 没有测得收益且增加复杂度则放弃优化；可以保留独立成立的可读性改进。
- 不承诺总行数下降比例，不将纯移动行数计为净删除。

### B7：文档、兼容与全量门禁

- 更新已有 `gocloudagent/README.md`、相关文件头和 `internal/AGENTS.md` / 根指南中的归属说明；必须以实际落地为准。
- 修正“会话逆序关闭”“显式MCP凭据保留”等与代码不符的说明，但不借文档修改改变行为。
- 回写本计划逐项状态、实际文件、测试日志和偏离；关联 `internal-simplify-refactor-plan.md`，不改写其历史事实。
- 回滚只能撤销本批自己的逻辑改动，禁止hard reset/整体restore或重写用户工作树。

## 8. 验收测试矩阵

### 8.1 场景与现有测试锚点

| 领域 | 现有测试 / 文件 | 必补边界 |
| --- | --- | --- |
| provider准备与清理 | `cloudagent/runtime_test.go` 的 `TestRuntimeProviderControlsPreparationAndCleanup` | Prepare返回资源+error、MCP装配失败、agent创建失败；清理次数与顺序 |
| 确认授权 | `mcptool_test.go` 的policy/writable/headless/confirm测试；`agentbridge/failclosed_test.go`、`writable_test.go` | emit=nil、waiter=nil、空callID、Wait取消但父ctx有效、Prepare中同步Resolve、deadline与cancel区别 |
| gate | `agentbridge/skilltool_test.go` 的4个CallGate测试 | 并发同key release、跨run同callID、timeout后release、drop与wait交错、事件阻塞时序 |
| 子进程 | `TestLimitOutKeepsValidUTF8`、`TestValidateCLIPathAndSafeFailure`、`TestSubprocessCLI_DatabaseListReadOnly` | 超限Writer n/err、双输出独立上限、signal退出、父ctx取消、nil/空env、临时目录失败 |
| 技能 | `skills/skills_test.go`、`agentskills/skills_test.go`、`module_skills_test.go` | 空ID列表、重复/未知ID、fs失败、LF/CRLF/BOM、无终止frontmatter、读取时机 |
| 协议回退 | `model_native_test.go` 的auto fallback、stream auth不重试、读错后不执行不完整工具 | 实际CLI HTTPClient的401/429、400/422工具错误、普通传输错误、Chat失败、Usage调用次数 |
| 流事件 | `agentcore/stream_toolcall_test.go`、runtime优化/stream测试 | TOOL_CALL逐字符拆分、空白前缀、合法普通文本、空ID/重复ID、多个同名工具、读错部分文本 |
| 历史 | `runtime_history_test.go`、`cli/session_test.go`、`compaction_test.go` | 中文字节预算、超预算break、工具摘要、最后一轮超预算、孤立tool结果、JSONL不变 |
| 协作任务 | `collab_plan_test.go` 的Parse/Validate/Depth/Reduce/NextTask/Blocked | 菱形/稠密DAG、重复依赖、首错优先级、未知状态、pending依赖非法索引 |
| MCP传输 | `agentbridge/mcp_test.go`、`mcp/server_test.go`、`cli/mcp_adapter_test.go` | 原request不变、Header覆盖、L-1/L/L+1字节、Close一次、nil body、上游错误透传 |
| 本机文件/shell | `tools/tools_test.go`、`tools/isolation_test.go`、`cli/shell_isolation_test.go` | deny_read调用、symlink现行为、超长行/扫描错误现行为、shell输出交错、隔离不可用拒绝 |
| CLI命令 | `command_test.go`、`config_test.go`、`budget_test.go`、`cli_entry_test.go` | 帮助/别名/退出码/stdout/stderr快照、EOF、模板、取消、RunCLI门面 |

### 8.2 后续应执行的命令（本轮均未执行）

在仓库根目录使用以下命令；执行前先确认现有测试只使用隔离fixtures，不连接用户运行库。

1. 模块构建：`(cd gocloudagent && GOWORK=off go build ./...)`。
2. 模块静态检查：`(cd gocloudagent && GOWORK=off go vet ./...)`。
3. 模块测试：`(cd gocloudagent && GOWORK=off go test ./... -count=1)`。
4. 并发门禁：`(cd gocloudagent && GOWORK=off go test -race ./... -count=1)`。
5. 宿主定向：`go test ./internal/agentbridge/... ./internal/api/ ./internal/app/ ./internal/auth/ -count=1`。
6. 宿主编译：`go build ./internal/... ./cmd/...`。
7. 最终根模块：`go test ./...`。注意它不覆盖嵌套 `gocloudagent` 模块，不能替代第3项。
8. 格式：`gofmt -l gocloudagent internal/agentbridge`，期望无输出。
9. 差异：分别运行 `git diff --check` 与 `git diff --cached --check`；检查未跟踪新增文件的格式与内容。
10. 依赖：`(cd gocloudagent && GOWORK=off go list -deps ./...)`，确认无根模块业务包；另核对 `cloudagent`/`cli` 不互依赖。
11. 重复压力：针对gate/确认/取消的**精确测试名**使用 `-race -count=50`，避免用含糊正则零匹配却误认通过。
12. 新增benchmark后：对指定包执行 `go test -run='^$' -bench='<精确基准名>' -benchmem -count=5`；保留改前改后原始数据。已有benchstat才使用，不安装依赖。

不得运行 `go mod tidy` 来掩盖迁移后的依赖问题；不得运行 `init_db`、ALTER、reset或带真实凭据的端到端脚本作为门禁。需要最低版本与Linux隔离验证时在可控CI环境执行，并如实记录SKIP。

### 8.3 建议新增测量

- glob：同一棵临时目录树，1k/10k文件、匹配/不匹配模式，统计ns/op、allocs/op；目标是每次搜索仅一次Compile，无缓存状态跨请求。
- DAG：8/32任务的链、菱形、共享依赖图，统计校验耗时及分配；输出和首错文本必须完全一致。
- history：ASCII/中文混合，0/20/200条及超预算输入；验证结果序列一致再对比分配。
- shell限流：输出量超过上限若干倍，确定性断言保留缓冲有界；若测RSS，必须记录平台和采样方式，不能用缓冲长度替代进程RSS结论。
- HTTP复用：本地server统计连接次数，显式区分自定义HTTPClient与默认client路径；不测真实供应商网络延迟来证明本地代码性能。

## 9. 明确不做与审批边界

### 9.1 默认不做

- 不迁移SimpleBase身份、委托claims、SQL安全规则、业务工具、模块目录、固定六阶段、cron业务与系统库适配。
- 不把 `guardDialContext` 及云网络出口策略变成CLI默认策略。
- 不抽象所有错误为同一个“通用工具错误”；保留取消的Go错误与业务失败信封区别。
- 不统一byte/rune截断、nil/空slice、关闭顺序、超时起点或环境过滤表。
- 不删除公开别名、`ResetUnsupportedNativeTools`、`Session`/`HTTPClient`等仅凭仓内引用不足的API。
- 不把小函数全部改成泛型、把所有工具登记改成反射、把短校验变为规则引擎。
- 不引入新配置来源、新持久化字段、新依赖、新自动迁移或外部LLM评测成本。

### 9.2 需要明确选择的项目

| 决策 | 推荐默认 | 变化范围 |
| --- | --- | --- |
| 通用确认与gate API | 同意最小下沉，现有运行公开接口保留 | 新增窄API，不删除已有公共字段 |
| BUG-01/02/03/04/05/06/07 | 先确定性复现，逐项批准修复 | 行为/并发/错误/安全文案变化，不能视为全部自动获批 |
| nil环境、临时目录失败 | 第一阶段保持适配现状并明确文档 | 若改fail-closed，新增拒绝路径需批准 |
| CLI MCP严格schema/统一截断与脱敏 | 本轮不做统一 | 可能改变工具集合及返回字符串 |
| 公共Reset函数删除 | 不删 | 未知外部消费者与测试兼容 |
| M-07计划validator框架化 | 暂缓 | 收益不足，首错优先级风险 |
| L-11 shell有界缓冲 | 值得单列可靠性改进 | 输出/内存/子进程复制错误语义 |
| L-12 Transport复用 | 先测量，未测不改 | 连接池、可变配置及资源生命周期 |

## 10. 完成标准与回写模板

实施完成不以“搬了多少行”判断，而以以下事实判断：

- [x] 宿主不再实现通用确认、gate和子进程限流的重复机制。
- [x] 同一规则只有一份实现，但业务授权仍清晰留在宿主。
- [x] 每个新增公开入口有零值、错误、生命周期、并发说明及测试。
- [x] 所有原有可观察行为保持，获批变化有单独测试和迁移说明（见 §12.3）。
- [x] 无根模块反向依赖、无cloudagent/cli循环依赖、无新第三方依赖（`go list -deps` 已核对）。
- [x] 同一逻辑不再同时存在旧实现与新实现；保留的包装都有真实适配理由。
- [x] 事件时序、清理顺序、输出边界、错误回退和权限fail-closed测试通过。
- [x] 模块与宿主两侧构建/测试通过，平台和未运行检查明确登记（见 §12.4）。
- [x] 性能改进有改前改后数据；纯重构不宣称性能提升（见 §12.5）。
- [x] 用户原有改动、`.codebuddy`、本地数据和配置未受影响。

后续每批在此追加记录：批次/条目 → 实际落点 → 删除的旧实现 → 保留兼容层及原因 → 实际命令与结果 → 已有失败/新增失败 → 未运行检查 → 获批行为差异 → 回滚点。本轮实施结果见 §12。

## 11. 并行复核补充与最终收敛（2026-11-23）

本节合并三个分区评估的迟到结果，补充主清单遗漏并修正候选优先级。**与前文冲突时以本节为准**。复核仍为静态阅读；未运行测试、构建、真实服务或数据库操作。重复报告的gate、短写、取消、length和HTTP回退问题不另编号。

### 11.1 对原方案的收紧

1. **L-06 默认暂缓**：本地无效schema仍注册无参数工具、云端跳过并关闭空集合，已有各自测试锁定。为了少量转换代码新增公共API和策略分支不划算。不同于下述C-11：后者仅是 `agentcore` 内部两处同语义参数编码。
2. **L-07 默认暂缓**：MCP协议调用已经共享 `mcp.Session`；云端/本地的确认、记录、失败脱敏与截断有真实差异。不为了再缩短两个方法发明工具执行框架。
3. **L-04 为备选**：先补REPL输出/信号golden，再拆分；优先实施下面局部、可验证的删除与去重。
4. **M-03 不强制增加内部包**：公共入口仍从 `cloudagent` 暴露；只有实现已形成独立的标准库机制且确有复用价值时，才增加 `gocloudagent/internal/process`。若增加，依赖只能 `cloudagent → internal/process`，后者不得 import `agentcore`、`cli`、`cloudagent` 或宿主。当前规模先放 `cloudagent/process.go` 也合理。
5. **M-03 补齐进程内消费者**：`InProcessCLI.Run` 同样调用 `limitOut`。迁移后它应经窄的 `cloudagent.LimitProcessOutput` 门面复用，不残留第二份UTF-8逻辑；`SubprocessCLI{Path}`、`ValidateCLIPath`、`CLIRunner` 现有形状保留。进程内先用无界Buffer后截断的问题另列可靠性改善，不能假称已有界。
6. **M-05 维持单文档读取**：不改为全量Catalog初始化。CLI提示词模板解析还有不同的CRLF及结束标记行尾规则，不纳入本次frontmatter去重。
7. **M-01 保留配置入口与日志时点**：不删除 `Runtime.Confirm/ConfirmTimeout` 或宿主对应字段。窄的 `OnRequested/OnResolved` 观察钩子可用于保持CLI trace时点，不引入泛化中间件。
8. **当前宿主Agent映射恒为Writable=true**：`agentbridge/principal.go` 与 `TestAgentMappingIsAlwaysWritableAndKeepsTools` 锁定这一事实。通用Runtime依然必须保留Writable=false契约及测试；本计划不恢复旧UI开关，也不改变宿主当前映射。
9. **冻结精确清理顺序**：成功注册所有defer后，Runtime为 stop(cancel+clearCancel) → mcpSet.Close → prep.Cleanup → releaseThread；集合内sessions正序、全部尝试Close、返回首错。CLI closeAll正序忽略错误，in-memory会话先客户端后服务端，不能统一成逆序或聚合错误。

### 11.2 新增低风险候选

以下沿用前文风险/置信度口径；都是未实施提案。路径默认相对 `gocloudagent/`。

| ID | 优先级 / 风险 | 证据与最小方案 | 验证及不等价陷阱 |
| --- | --- | --- | --- |
| C-11 | P2 / 低 | `agentcore/model.go` 的 `buildRequest:304–316` 与 `toolProtocolPrompt:349–369` 重复ParamsOneOf→JSONSchema→Marshal；提私有 `toolParameters` | native/text的外层wire字段、omitempty、工具顺序和无效schema省略行为不变；扩展GenerateAndOptions、NativeToolsAndUsage测试 |
| C-12 | P2 / 低 | `agentcore/stream.go` 的 `ConsumeAssistant:164–170,179–185` 重复正文累积与token事件 | 私有helper保持先acc再emit；nil acc/emit、含ToolCalls时不发正文；不合并吞读错误的ConsumeVariant |
| C-13 | P2 / 低中 | `cloudagent/runtime.go:313–353` 的 `finishRun` 六处重复trace+return，改有序分类后单次出口 | max_iterations→max_tokens→canceled→deadline→其它的优先级和原始error不变；不能用提前捕获result的defer；补ctx-only取消、复合错误、trace次数 |
| L-13 | P2 / 低 | `cli/mcp_adapter.go:128–132` 的 `toolsErrEnvelope` 与 `tools/tools.go:77–81` 的 `ErrEnvelope` 同构；CLI四处直接调用现有tools函数 | 精确JSON字节、转义、空值、中文、非法UTF-8；不把云端脱敏版本一并替换，不新增公共封套框架 |
| L-14 | P2 / 低 | `cli/mcp_config.go:230–236` 的调用超时可复用 `mcpCallTimeout`；`cli/config.go` 的私有上限常量可引用 `mcp.MaxCallTimeout` | 与L-01同批；固定连接超时保持；测负数/0/30/60/61/300/301与大整数，不顺手改变溢出语义 |
| L-15 | P3 / 低 | `mcp/adapter.go` 的私有 `jsonRaw` 仅client一处调用，直接 `json.RawMessage`；`tools/tools.go:223–227` 的cmdErr只是err复制，直接传err | 与L-05同批；空arguments仍不设置params，shell非ExitError映射不变；公开门面不删 |
| L-16 | P2 / 低 | `cli/mcp_config.go:166–170` 对具体 `*mcpLocalTool.Info` 检查error，但该方法恒返回info,nil；删除不可达检查和warn | 不改Info接口或schema策略；验证server顺序、工具上限、session关闭和非法schema仍保留工具 |
| L-17 | P2 / 低 | `cli/config.go:265–283` 四段正整数env解析，复用已有私有 `setPositiveInt`，按职责移到config文件 | env最后同名项优先，非法/0/负数不覆盖base；测+1、空白、空串、overflow；不反射化配置 |

**C-03补充**：`schemaToChat` 每个分支均只返回一个元素，核对唯一消费点后可改为私有单值返回，避免临时单元素slice；保留未知Role投影和nil输入跳过规则。

**局部注释纠错**：`cli/mcp_config.go`“显式凭据保留”、`cli/agent_loop.go`“Warn=nil写stderr”、`cli/session.go`“fsync失败”与当前实现不符；`normalizeRefs`“去自引用”也不符合实际。只改说明，不引入凭据放行、stderr输出、Sync或自动消除自依赖。文件头仍保留并同步。

### 11.3 M-08：工具结果失败标志复用——有具体反例，默认暂缓

- 位置：`internal/agentbridge/plan_skills.go:93–106` 的 `toolResultFailed` 与 `cloudagent/tools.go:50–59` 的 `toolResultIsError`。
- 正常JSON下两者都识别顶层ok:false/is_error:true；但宿主的IsError字段为 `*bool`，云端为 `bool`，**并非所有输入等价**。
- 具体静态反例：`{"is_error":true,"is_error":null}`。Go JSON对非指针bool的null不改原值，对指针字段会置nil；预期云端true、宿主false。必须用表驱动测试核实，不可据“逻辑类似”直接删除一份。
- 空白、malformed、null、非对象、字段类型错误、重复键和大小写键匹配也应锁定。比如其他字段类型错时，现有两侧会因整体Unmarshal错误返回false，不能改成部分容错解析。
- 选择：第一阶段保留两个短函数；若批准统一重复键/null语义，再在 `agentcore` 提纯函数、通过 `cloudagent` 门面供宿主使用。不要为了十余行增加可配置解析框架。
- `PlanStageDone` 的业务argv匹配、固定阶段与成功条件始终留宿主。下面BUG-08与是否共享解析函数无关，应独立处理。

### 11.4 新增待复现问题

以下是分区评估发现的候选，关键路径已静态核查；不代表端到端复现、漏洞利用或修复完成。安全问题只采用fake、临时目录和本地服务测试，不读取用户真实凭据或工作区外私人文件。

| ID | 证据 / 场景 | 后续验证与边界 |
| --- | --- | --- |
| BUG-08 | `cloudagent/runtime.go:248–259` 先截断4000rune再解析失败JSON，合法长ok:false结果变非法JSON，IsError可能false；宿主PlanStageDone再看截断content也无法辨别 | 完整结果先分类、展示再截断是可见bugfix；补长ok:false/is_error:true、普通文本、malformed及宿主阶段完成集成用例，不能混入C-04 |
| BUG-09 | `cli/agent_loop.go:349–365` 历史assistant投影未传Entry.ToolCalls，随后却保留对应tool结果；`compaction.entryToSchema`已有带工具调用的投影 | 两轮真实SessionRunner+fake模型及保存/恢复测试；单轮TestRunnerToolCallSequence不够。复用entryToSchema会修行为，不是纯去重 |
| BUG-10 | `cli/compaction.go:97–106,158–175` 第二次摘要只读当前Messages的head，未把旧摘要送入模型；history只使用LatestCompaction | 连续两次压缩，fake模型捕获旧事实是否仍在请求；压缩失败不得丢历史。现有单次测试不能证明连续压缩正确 |
| BUG-11 | `cli/mcp_cmd.go:105–109` 的mcp tools入口构造env时无Workspace；stdio隔离检查只检查非nil Workspace.Isolation | 用临时假命令记录是否启动，对比普通Run与mcp tools；只列工具也需要启动stdio进程，不能当无执行副作用。独立修复入口安全策略 |
| BUG-12 | `cli/mcp_config.go:225–229` Headers原样Set，未展开配置中承诺的 `${VAR}` | fake RoundTripper断言占位符、未知变量和空值；凭据不落日志。修复独立于HTTP装配去重 |
| BUG-13 | 本机resolvePath主要校验词法路径，read/write/edit及grep后续可能追随文件符号链接 | 用两个临时目录做边界测试；处理macOS路径别名、尚不存在的写目标与TOCTOU，不能简单EvalSymlinks后宣称完整隔离 |
| BUG-14 | `cli/agent_loop.go:287–295` 工具Arguments直接写EntryToolCa和事件，仅assistant正文调用redact；配置/URL展示也有候选脱敏遗漏 | 分别检查JSONL的ToolCalls字段、CLIEvent、config show/json中的SearchAPIKey/MCP Headers/Env及URL userinfo/fragment。当前正文脱敏测试不足；逐出口修复，不能改实际执行参数 |
| BUG-15 | `mcp/client.go` ListTools只按累计有效工具数限额，全无效schema或空分页可持续翻页；普通CLI连接使用Background，单HTTP超时不限制整体翻页 | fake分页server及可取消ctx验证总预算；新增页数/时限是可靠性行为变化。不要误说mcp tools也完全无总超时：该入口已有30秒context |
| BUG-16 | `Runtime.Prepare`、MCP准备发生在startRunTimeout之前，期间CancelRun尚未登记cancel | fake Provider阻塞至ctx.Done，短RunTimeout+Background检验当前行为；是否要求覆盖准备期需产品决策。提前注册改变超时起点和清理上下文，C-05必须保持原语义 |

**BUG-06扩展，不重复计数**：还应检查 `skilltool.go` 的confirmation_required.Message。Argv调用Policy.RedactArgv，但 `confirmMessage(argv)` 直接拼原argv。需追踪最终SSE/确认存储是否二次脱敏后再判完整影响；共享确认器不能假设当前传入Message已安全。

**资源风险附记，不计为已确认bug**：`Model.Stream` 忽略sw.Send的关闭反馈，reader提前Close后上游是否继续读取需用可控stream验证；MCP结果先拼全后截断、SSE ReadString无单行限额且EOF尾行处理、grep Scanner.Err忽略，也应与L-11分开评估。新增读取限额或错误返回会改变行为，不是字符串函数替换。

### 11.5 更新后的执行与验收顺序

- B0先锁定同构函数的异常输入：特别是M-08重复键/null反例，不把静态相似当等价证明。
- B1优先加入C-11/C-12/C-13、L-13至L-17；私有替换和不可达分支删除比跨端抽象收益更确定。
- B2维持M-04/M-05；M-08暂缓，失败信封仅共享已证明相同的编码路径。
- B3/B4/B5维持原顺序，但L-06/L-07默认不实施，L-04按golden覆盖情况选择。
- BUG-08至BUG-16与已有BUG一样，先复现再独立确认；不得因本节登记就自动修复。
- 新增验收重点：两轮/恢复工具历史、两次连续压缩、mcp tools隔离入口、原始长结果分类与阶段完成、全部工具参数落盘/展示脱敏、分页整体取消、准备期取消覆盖。
- 强化原text跳gate测试：必须注入非空的真实compose tool call ID，否则text/native都因空ID绕过gate，不能证明协议分支有效。
- 运行测试前检查fixture副作用：现有宿主测试包含构建根服务子进程及内存SQLite建表；与用户运行库DDL严格区分。若授权范围连隔离内存schema也不允许，则跳过并明确记录，绝不代跑真实库迁移。

### 11.6 最终状态

本计划仍为**待实施**。本轮补充只更新本文件，不创建源码、不迁移测试、不运行构建/测试。新增条目保持唯一编号；已经报告的缺陷只扩展影响，不重复计数。批准范围、测试运行结果和实际收益只能在后续实施批次中回写。

## 12. 实施记录（2026-11-24，激进策略：不保留兼容层）

### 12.1 已实施条目与落点

| 条目 | 落点 | 删除的旧实现 |
| --- | --- | --- |
| M-01 确认编排 | `cloudagent/confirmation.go`：`AwaitConfirmation`（含 `OnRequested/OnResolved` 观察钩子） | 宿主 `execSimplebase` 内联状态机；`mcpTool.askConfirm`；`delegation.go` |
| M-02 调用闸门 | `cloudagent/callgate.go`：`ToolCallGate` | `agentbridge/callgate.go` 与 `callgate_export_test.go` |
| M-03 子进程与限幅 | `cloudagent/process.go`：`ProcessRunner`、`ValidateExecutable`、`ProcessStartFailure`、`LimitProcessOutput` | `cliexec.go` 的执行与限幅实现、`ValidateCLIPath`、`cli_limit_test.go`；宿主只留 `CLIRunner` 端口与 `SubprocessCLI` 默认值适配 |
| M-04 工具结果编码 | `agentcore/toolresult.go`：`MarshalToolJSON`、`ErrorEnvelope` | 宿主 `marshalToolJSON`、`skillFail`；`cloudagent` 与 `tools` 的重复实现 |
| M-05 技能读取 | `skills.ReadDocument`；宿主 `readSkill` 委托 | 宿主 `stripFrontmatter` |
| M-06 MCP 传输 | `mcp/http_transport.go`：`HeaderTransport`；宿主保留 `MaxResponseBytes`，CLI 不加上限 | `headerRoundTripper`、`limitedBody`、`mcpHeaderTransport` |
| M-08 失败标志 | `agentcore.ToolResultIsError` | 宿主 `toolResultFailed`（语义差异见 §12.3-②） |
| C-01/C-02/C-03/C-11/C-12 | `model.go` / `model_stream.go`：`openStream`、`responseMessage`、`toolParameters`、`emitToken`；`buildRequest` 去除不可达 error | — |
| C-04/C-05/C-13 | `cloudagent/runtime_events.go`（工具卡片收集）、`finishRun` 单出口、`StartRun` 的超时与取消覆盖准备期 | — |
| C-06/C-07/C-08/C-09/C-10 | `collab_{types,parse,validate,prompt,state}.go`；深度按序递推；`ReduceTurnState` 一次统计；历史只截断一次；私有 `tracer`/`hashSummary` 已移除 | `collab_plan.go` |
| L-01~L-03、L-05、L-13~L-17 | CLI/MCP 小项清理；`client.go` 拆 `client_wire.go`/`client_stream.go`；`command.go` 拆 `command_{sessions,config}.go` | 多处私有转发与不可达分支 |
| L-04 | REPL：`runBuiltin`/`replCompact`/`replMCP`/`runTurn`/`interruptContext` | — |
| L-08/L-09 | 工作树中已存在（一次编译、`walkFiles` 共享）；本轮补语义、边界测试与基准 | — |
| L-11 | `tools`：裸 shell 改用有界写入器 `boundedBuffer` | `CombinedOutput` |
| L-12 | `HTTPClient` 惰性创建并复用默认客户端，新增 `CloseIdleConnections`，会话 `Close` 钩子调用 | 每次请求克隆 Transport |

### 12.2 缺陷修复（均先写复现测试，确认失败后再修复）

| 编号 | 结论 |
| --- | --- |
| BUG-01 | `limitWriter` 报告完整输入长度；`ProcessRunner` 超限输出仍成功 |
| BUG-02 | `toolEvents.calls` 先发 `tool_call` 再释放闸门；阻塞 emit 的测试证明 emit 期间闸门保持关闭 |
| BUG-03 | `ToolCallGate.Release` 在锁内用 `released` 标记；32 路并发 `-race` 通过 |
| BUG-04 | `AwaitConfirmation` 取消时总返回非 nil 错误 |
| BUG-05 | `apiError` 统一返回带 `StatusCode` 的 `LiteLLMError`（`Error()` 文案不变）；401/403/429/500 不再触发第二次 Chat；`tools_not_supported` 回退保留 |
| BUG-06 | MCP 确认摘要与宿主确认 `Message` 均基于已脱敏内容 |
| BUG-07 | `Stream→Chat` 回退共用 `responseMessage`，`length` 以 `ErrMaxTokensExceeded` 终止流 |
| BUG-08 | 先分类失败再截断展示 |
| BUG-09 | 历史投影复用 `entryToSchema`，保留 assistant `tool_calls` |
| BUG-10 | 连续压缩时摘要请求携带上一份摘要 |
| BUG-11 | `mcpServerEnv.IsolationEnabled` 独立于 `Workspace`；`mcp tools` 入口遵守隔离 |
| BUG-12 | HTTP MCP 的 Header 值展开 `${VAR}`；配置中保留占位符 |
| BUG-13 | `resolvePath` 增加符号链接真实路径检查（fail-closed，支持不存在的写目标及符号链接形式的 workspace 根）；`glob`/`grep` 跳过越界链接。撤销修复后复现测试失败，恢复后通过 |
| BUG-14 | 工具参数落盘与事件脱敏；`maskURL` 隐藏 userinfo/fragment；`config json`/`show` 掩码 `search_api_key` 与 MCP Headers/Env；`config get` 保持原值供脚本使用 |
| BUG-15 | `ListTools` 自行翻页并设 `MaxListPages=8` 总预算（`ErrTooManyPages`）；CLI 启动期整体限时 30 秒。旧行为下复现测试一直跑到 10 秒 ctx 超时 |
| BUG-16 | `RunTimeout` 与 `CancelRun` 覆盖 `Provider.Prepare` 与 MCP 装配；清理顺序保持 stop→mcpSet.Close→prep.Cleanup→releaseThread |

### 12.3 与「纯重构」的偏离，以及与计划原文的差异

① **行为变化均有测试固定**：BUG-02/04/05/07/08/09/10/11/12/13/14/15/16 改变了可观察行为，属于用户明确授权的修复，而非等价搬移。

② **M-08 的重复键/null 反例**：§11.3 担心 `*bool`（宿主）与 `bool`（云端）在 `{"is_error":true,"is_error":null}` 上不等价。统一实现选用 `*bool`，因此云端该输入由 true 变为 false；正常信封不受影响。这是畸形输入上的结论趋同，没有对外承诺。

③ **ProcessRunner 比原计划更严格**：临时工作目录创建失败时拒绝执行（原先退回当前目录）。这是 §3-M-03 陷阱 4 中列为「需批准」的 fail-closed 改动，按「激进」授权执行。

④ **计划中未实施**：L-06（跨端 MCP schema 共享）、L-07（工具执行包装）、M-07（计划校验框架）按 §11.1 暂缓；`gocloudagent/internal/process` 未建，实现直接放 `cloudagent/process.go`。

⑤ **text 协议跳过 gate 的测试局限**：eino 没有公开的 call ID 注入器，`TestExecSimplebase_TextProtocolSkipsGate` 无法区分 text/native 分支；已在测试注释说明，gate 语义由 `ToolCallGate` 单测保证。

### 12.4 验证结果

| 检查 | 结果 |
| --- | --- |
| `gocloudagent`：`go build` / `go vet` / `go test ./... -race -count=1` | 通过（基线与最终均通过） |
| 宿主：`go build ./...`、`go vet`、`go test ./internal/agentbridge/... ./internal/app/... ./internal/api/...` | 通过（`api` 约 167s） |
| `gofmt -l`（`gocloudagent`、`internal/agentbridge`、`internal/app`） | 无输出 |
| 依赖约束：`go list -deps` 中模块不含根模块 `internal`；`cloudagent` 与 `cli` 无互相导入 | 通过 |
| **未运行** | Linux / Windows / Go 1.25 最低版本；根模块 `go test ./...` 全量（只跑了受影响的 3 个包）；真实 LLM/MCP/S3 调用；`init_db`/DDL（始终未执行） |
| 耗时提示 | `TestWalkFilesCap` 创建 20001 个文件、`TestShellHugeOutputDoesNotBufferEverything` 约 6 秒，`-short` 均跳过 |

### 12.5 规模与性能：如实记录

- **代码行数没有下降**：生产代码 `gocloudagent` 8417 → 9216，`agentbridge` 2993 → 2860，合计 11410 → 12076（+666）。原因是新增通用实现、注释和缺陷修复。收益是重复实现收敛为一份、边界更清晰、缺陷关闭，**不是行数减少**。
- **符号链接防护**：2000 文件的 glob 基线约 2.3ms/op；首版逐条目 `EvalSymlinks` 约 24ms/op（慢 10 倍，9.7MB/op），改为只解析符号链接条目后约 2.3~2.4ms/op（+1~4%），采用后者。
- **L-12 连接复用**：5 次顺序请求的 TCP 连接数 5 → 1。
- **L-11**：200MB 输出下堆增长从约 574MiB 降到预算内（<64MiB）。
- **C-07/C-08**：用旧递归实现做差分测试（300 张随机 DAG × 5 个深度上限）和 10 种状态全组合验证等价；未做耗时基准，不宣称提速。

### 12.6 遗留与后续（2026-11-24 第二轮后更新）

- 导出符号清理：消费者清单已产出（§12.8-3）——全仓零引用死代码为 **0**；26 个仅包内使用的导出收缩仍按 §9.1 需外部消费者证据，清单留档。
- BUG-13 的 TOCTOU 窗口：窗口本身是固有限制，防护链已确认（§12.8-4），`resolvePath` 注释声明保留，不改代码。
- `Model.Protocol` 空值注释与 `ResetUnsupportedNativeTools` 并发契约：**已在 §12.8 处理完毕**（只改说明 / race-safe 实现 + 契约测试）。

### 12.7 复核记录（2026-11-24）

对 §12 实施结果做只读复核；除下述两处注释收尾外未改任何代码。工作树仍基于 `dead32f`，实施改动未提交，与本文件记录一致。

**逐项核验**：

- §12.1 全部落点存在且符号吻合：`cloudagent/{confirmation,callgate,process,runtime_events,collab_*}.go`、`agentcore/{toolresult,model_stream}.go`、`skills.ReadDocument`、`mcp/http_transport.go`、`cli/{client_wire,client_stream,command_sessions,command_config}.go`；`agentbridge/callgate.go`、`delegation.go`、`collab_plan.go` 已删除，`cliexec.go` 收缩为 `cloudagent.ProcessRunner` 薄适配，`InProcessCLI` 经 `cloudagent.LimitProcessOutput` 复用限幅（§11.1-5 要求满足）。
- 抽查语义：BUG-02（`runtime_events.go` 注释与实现均为先发 `tool_call` 再释放闸门）、BUG-13（`resolvePath` 已注明 TOCTOU 边界，`glob`/`grep` 越界链接跳过）、BUG-04（`AwaitConfirmation` 取消返回非 nil）。
- §5「其他仅登记」的 `/compact` Ctrl+C 问题已随 L-04 解决：`replCompact` 经 `interruptContext` 接收信号，与正常 run 同口径。
- §11.2「局部注释纠错」：`mcp_config.go` 凭据注释已改准；`normalizeRefs` 已随重构消失（项 moot）。**两处未正确落地，本轮修复**：
  1. `cli/agent_loop.go` 的 `warn` 注释仍称「退化为写 stderr」，实现是 no-op；已改为「未注入 Warn 时 no-op（不写 stderr，避免打扰终端输出）」。
  2. `cli/session.go` 的 `Append` 注释仍提「fsync 失败只影响耐久性」，但实现根本没有 `Sync()` 调用；已改为「写入即返回，不做 fsync，进程崩溃可能丢尾部行（既有会话语义）」。

**复核命令与结果**：

| 检查 | 结果 |
| --- | --- |
| `(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./...)` | 通过 |
| `(cd gocloudagent && GOWORK=off go test ./... -count=1 -short)` | 全部 ok（含注释修复后的 `cli` 包） |
| `go build ./internal/... ./cmd/...` | 通过 |
| `go test ./internal/agentbridge/... -count=1` | 通过（6.6s / 0.3s） |
| `gofmt -l gocloudagent internal/agentbridge` | 无输出 |
| **本轮未重跑** | `-race`、根模块全量 `go test ./...`、`internal/api` / `internal/app`（§12.4 实施时已通过；本轮仅注释收尾，无行为变更） |

**结论**：计划已按 §10 完成标准全部落地；§12.3-④ 列明的 L-06 / L-07 / M-07 为有意暂缓，§12.6 遗留项为有意保留，无需再开新批次。后续仅在外部消费者清单齐备（导出符号清理）或产品决策明确（`Model.Protocol` 空值语义）时才另立计划。

> 第二轮更新（同日）：§12.6 遗留项已按推荐在 §12.8 处理（消费者清单产出、Protocol 注释、Reset 并发契约、TOCTOU 防护链确认）；L-06 / L-07 / M-07 定稿为「评估后不做」，见 §12.9。

### 12.8 第二轮收尾记录（2026-11-24）

按 §12.6 遗留项的推荐路径逐项执行；除 ①② 外无代码变更。

1. **`Model.Protocol` 空值注释纠错**（只改说明，行为不变，遵循 §5「不要按注释修复现行为」）：原注释「空值按 text 处理」与实现不符——`useNative()` 对空值走降级缓存分支（默认 native），且 `fallback()` 要求 `Protocol == "auto"`，空值永不回退。新注释写明空值实际语义并提示经 `NewModel` 构造或显式赋值。
2. **`ResetUnsupportedNativeTools` 并发契约**：原实现 `unsupportedNativeTools = sync.Map{}` 整体重赋值与并发 `Load`/`Store` 构成数据竞争。改为 `Range`+`Delete` 逐键清理（sync.Map 语义下并发安全），注释写明并发契约并禁止改回整体重赋值；新增 `TestResetUnsupportedNativeToolsConcurrent` 在 `-race` 下锁定 Store/Load/Reset 交错。公开 API 与可观察行为（缓存清空）不变。
3. **导出符号消费者清单**（只读词法统计，分层：包内/包外 × 生产/测试，扫描 gocloudagent + 根模块 internal/cmd/skills/scripts/examples/packages）：
   - 全仓零引用的导出符号：**0 个**——不存在可删死代码，「真正减少代码」的路径关闭。
   - 仅包内生产使用、无任何包外消费者的导出：**26 个**，为 API 面收缩候选——`cloudagent` 的 Collab 常量（`CollabTask*` 6、`CollabTurn*` 8、`CollabRoute*` 4）与哨兵（`CollabErr*` 4）、`ErrDispatchPlan*` 4、`mcp.CodeInputRequired`。按 §9.1「仅凭仓内引用不足」，不删不去导出；外部消费者证据齐备后再收缩。
4. **BUG-13 TOCTOU 防护链确认**（无代码变更）：`resolvePath`（词法 Rel + 符号链接真实路径）→ 文件读与 glob/grep 的 `DeniesRead`（deny_read 策略同时守卫）→ shell 执行的 `shell_isolation`（`host.Run`）。检查与打开之间的窗口为固有限制，已在 `tools.go` 注释声明，真正防护依赖隔离开启。

**验证**：`(cd gocloudagent && GOWORK=off go build ./... && go vet ./... && go test ./... -count=1 -short)` 全过；`go test -race ./agentcore/ -count=1` 过；`go build ./internal/... ./cmd/...` 过；`gofmt -l gocloudagent` 无输出。

### 12.9 评估后不做（定稿）

- **L-06 跨端 MCP schema 适配共享**：本地无效 schema 保留无参工具、云端跳过并关空集合，失败路径有真实差异且各有测试锁定（§11.1-1）。
- **L-07 `mcpTool`/`mcpLocalTool` 调用包装**：协议调用已共享 `mcp.Session`，授权 / trace / 截断 / 脱敏差异真实（§11.1-2），不做万能工具包装器。
- **M-07 计划校验框架化**：首错优先级风险、收益不足（§3-M-07、§9.2）。
- **公共 API 收缩**（§12.8-3 的 26 个符号）：需外部消费者证据，本轮不动。
- 其余 §9.1 / §9.2 的默认不做项继续有效，不在此重复。

## 13. 第二阶段：通用 Plan 能力 + 群聊式栅栏决策 + 云助手全面改造（2026-11-24，实施中）

> 本节是新需求的实施计划（进度见 §13.11 状态列）。证据来自对当前工作树的只读阅读：`ui/src/{pages/AgentManager.vue,composables/useAgent{Collaboration,Sender,RunLayer}.ts,composables/useThreadNavigation.ts,services/agent-stream.ts,services/http-api/agent.ts,components/agent/*}`、`internal/api/{collab_coordinator,collab_handler,collab_stream,collab_query_handler,agent_dispatch,agent_run_auto}.go`、`internal/agentbridge/*`、`gocloudagent/{cloudagent,tools,cli,mcp}`。行号以评估时为准，实施时按符号名定位。
> 策略沿用 §12「激进，不考虑兼容」：允许改公共 API、SSE 帧、工具名与前端组件；**不允许** DDL / `init_db` / 真实凭据端到端作为门禁（§8.2 仍有效）。

### 13.1 需求拆解与目标

| # | 用户诉求 | 落到本计划 |
| --- | --- | --- |
| R1 | 控制台云助手页面样式美化 | §13.9 UI 方案（群聊视觉、Plan 卡、确认弹窗、Composer、侧栏） |
| R2 | 每个 tag 会话像一个群；**每条消息先经过 Plan 栅栏决策**：@ 哪些助手、给每个助手发什么、何时弹框让人确认 | §13.3 栅栏协议 + §13.4 `plan` 工具 schema + §13.10 SSE v3 |
| R3 | `gocloudagent` 提供通用 Plan 能力，对标 Codex / Pi；复杂任务要有 goal 与进度，类似 planning-with-files | §13.5 `gocloudagent/plan` 包、文件化计划存储 |
| R4 | 把 `agentbridge` 的通用能力下沉到 `gocloudagent`，使其能以 CLI / 库独立运行 | §13.7 迁移清单 + §13.8 群聊引擎下沉 |
| R5 | 增加工具：mcp、reflect、工具基类与共享辅助、路径安全、文件系统、HTTP+SSRF、Shell、Todo、Web；已存在的忽略 | §13.6 工具清单（逐项标注「已存在/改名/新增」） |
| R6 | Plan 通过工具提供（类似 `explore` schema），schema 内含 @ 助手、每个助手的消息、何时弹框 | §13.4 |
| R7 | 修复云助手前后端已有 bug | §13.2 缺陷清单（CB-FE-* / CB-BE-*） |

**完成定义**：群会话内任何一条用户消息都会产出一份**持久化、可回放、可确认**的 Plan 决策，Plan 决策只通过 `plan` 工具的结构化参数产生；通用的规划 / 调度 / 工具 / 网络防护在 `gocloudagent` 内只有一份实现，SimpleBase 只提供存储、身份、业务工具与 HTTP 适配；§13.2 的缺陷全部有复现测试并修复。

### 13.2 云助手现存缺陷（先复现再修，修复独立检查点）

以下均为静态阅读结论，**未运行复现**；每项实施前先写失败测试。严重度：S1 功能不可用 / S2 数据错乱或状态错误 / S3 体验或一致性。

#### 13.2.1 前端

| ID | 严重度 | 位置与证据 | 影响 | 修复方向 |
| --- | --- | --- | --- | --- |
| CB-FE-01 | S1 | `AgentManager.vue` 只把 `conv.confirmation` 传给 `ConversationView`，`@confirm` 调 `conv.resolveConfirm`；协作会话的待确认存在 `collab.confirmations`，**从未渲染、无法放行** | 群会话里任何破坏性工具都会挂到确认超时 | 统一「待确认」数据源：`useAgentCollaboration` 暴露 `confirmationList` + `resolveConfirm(runId, callId, approve)`（调既有 `agentThreads.confirm`），页面按会话类型选数据源；每个任务气泡下渲染自己的 `AgentConfirmCard` |
| CB-FE-02 | S1 | `useAgentCollaboration.ensureTurnContainer` 用 `turn.value?.id ?? ''` 建容器；流里从不读取 `X-Turn-Id` 响应头，`route` 帧也不带 turn_id | `cancel` / `retryTask` / `refresh` 拿到空 turnId → 404；停止按钮在群会话无效 | 后端首帧下发 `turn` 帧（含 `turn_id`）；`agent-stream.ts` 读 `X-Turn-Id` 并回调 `onTurnId`；前端缺 turnId 时禁用停止并提示 |
| CB-FE-03 | S2 | `bubbleOf` 用 `messages.find(m => m.role==='assistant' && m.agentId===task.targetId)` | 同一助手在历史里回答过 → 新 token **覆盖历史气泡**；同一轮两个任务指向同一助手 → 共用一个气泡 | 气泡按 `taskId` 绑定（`ChatMsg` 增 `taskId`/`turnId`），只在本轮新增消息范围内查找 |
| CB-FE-04 | S2 | `flushTaskToMessage` 用 `m.routing === false` 判重，但气泡从不设置 `routing` | 任务完成后**重复插入一条相同气泡** | 删除 flush 逻辑（气泡已实时存在），或按 `taskId` 判重 |
| CB-FE-05 | S1 | `useThreadNavigation.detachRun` 只调 `conv.clear()`，不调 `collab.detach()`；`collab` 的 `messages()` 读的是当前 `chatMessages` | 切换会话后旧轮次的 token 写进**新会话**；`collab.running` 残留导致新会话「发送中」 | `detachRun` 同时 `collab.detach()` + 重置 runtimes；handler 闭包捕获 `threadId`，不匹配即丢帧 |
| CB-FE-06 | S2 | `resumeActiveRun` 只查最近 run 并用 `conv.attach`；群会话没有轮次续接，`GET /agent-turns/:id/events` 前端未使用 | 刷新/回到群会话后：分工卡、人工待办、多任务气泡全部丢失；子 run 被当成单助手 run | 新增 `agentThreads.turns(threadId, limit)` 查最近轮次 → 未终态则 `agentTurns.get` 恢复快照 + `agentTurns.events(after)` 续接（§13.10） |
| CB-FE-07 | S1 | `agentTurns.act({action:'retry_task'})` 与 `agentTurns.reply` 用 axios 期望 JSON；后端 `continueTurn` / `ReplyHumanTask` 实际返回 **SSE 流** | 重试/人工回复后请求阻塞到整轮结束，响应体解析失败，界面无实时输出 | 后端这两个动作改为「受理即返回 JSON 快照 + 后台推进」，前端随后走 CB-FE-06 的续接；见 CB-BE-01 |
| CB-FE-08 | S2 | 轮次流断线重连：`openAgentRunStream` 的 `runId` 被子 run 的 `run` 帧覆盖，重连拉的是**子 run 事件**，其 `end` 结束整轮流 | 网络抖动后后续任务全部丢失 | 轮次流使用独立 `openTurnStream`：按 turn `seq` 续传 `/agent-turns/:id/events?after=` |
| CB-FE-09 | S3 | `acceptSeq` 导出但没有任何 handler 调用；`lastSeq` 无效 | 重放重复帧会重复追加 token | 所有轮次帧带 `seq`，在流入口统一去重 |
| CB-FE-10 | S3 | `routeHint` 只在 `dispatch_mode==='auto'` 显示，群会话为 `collaborative` | 群会话永远看不到「本轮 @X / 自动分工」提示 | 改为 Plan 状态条（§13.9） |
| CB-FE-11 | S3 | `runningAgentIds` 依赖 `m.routing`，协作路径从不设置 | 助手条「运行中」指示不准 | 由任务状态派生 |
| CB-FE-12 | S3 | `rt.thinking` 只写 runtime 不写气泡 | 群会话看不到思考过程 | 同步到 `taskId` 气泡 |
| CB-FE-13 | S3 | `pickPrompt` 使用 `window.confirm` | 违反 `ui/AGENTS.md`「确认统一 ConfirmAction」 | 换 `ConfirmAction` 受控模式 |
| CB-FE-14 | S2 | `useAgentSender.onSend` 在 `collab.start` 前清空草稿，`start` 立即本地 push 用户消息；`/turns` 预检错误（`agent_not_in_tag`/`turn_busy`）后草稿丢失、孤立用户消息残留 | 用户需重打 | 失败回滚：移除乐观消息并恢复草稿（单助手路径已有同类处理） |
| CB-FE-15 | S3 | 卡片头对群会话仍显示「线程固定助手」徽标；`AgentDispatchCard` 用原生 `<button>` + `text-[10px]`，`canceled` 状态不提供重试（后端允许） | 视觉与规范不一致 | 并入 Plan 卡重做（§13.9） |

#### 13.2.2 后端

| ID | 严重度 | 位置与证据 | 影响 | 修复方向 |
| --- | --- | --- | --- | --- |
| CB-BE-01 | S1 | `collab_handler.go` 的 `continueTurn`、`ReplyHumanTask` 无条件 `openTurnStream`；`TurnAction` 的 cancel 返回 JSON | 同一端点族响应形态不一致（见 CB-FE-07） | 动作统一返回 JSON 快照（202），推进交给后台轮次执行器（CB-BE-02） |
| CB-BE-02 | S1 | `runTurn` / `advanceTurn` 使用 `c.Request().Context()`；单助手 run 路径已用 `context.WithoutCancel` | 前端「只断开 SSE」实际会**取消整轮与子 run**，与产品语义（切会话后台继续）相反 | 轮次执行器在后台 goroutine 以 `WithoutCancel` + 轮次级 cancel 运行，登记到 `turnRegistry`；SSE 只是订阅者（§13.10 broker） |
| CB-BE-03 | S2 | `cancelTurn` 只改库并 `CancelRun`；仍在跑的 `advanceTurn` 随后 `setTaskStatus(failed/completed)` 覆盖 canceled，`settleTurn` 归约成 failed | 用户取消显示为失败；可能继续执行下一个 pending 任务 | 取消走轮次 ctx；`setTaskStatus` 对终态（canceled）不回写；`advanceTurn` 每步检查轮次状态 |
| CB-BE-04 | S2 | `turnStream.recordEvent` 先 `LastAgentTurnSeq` 再 `+1` 插入，非原子；SSE `id` 用连接内 `sseWriter.seq`，与事件表 `seq` 不同 | 并发写重复 seq；`after=` 续传用错序号 | 事件 seq 由 store 原子分配（单语句 `INSERT … SELECT COALESCE(MAX(seq),0)+1` 或进程内 per-turn 计数器+唯一约束重试），SSE id 必须等于事件 seq |
| CB-BE-05 | S2 | 子 run 的 token/tool/confirmation 帧不进轮次事件表；`TurnEvents` 回放帧形状 `{type,seq,task_id,data}` 与实时帧不同 | 续接无法还原；前端需两套解析 | 结构帧（turn/plan/task/human/confirmation/progress）全部落表且与实时帧同形；内容帧仍走 run 事件缓存，按 `task.run_id` 二级续接 |
| CB-BE-06 | S2 | `dispatchByModel` 注释称「排除本轮用户消息」，但 `prepareTurn` 已先 `AppendAgentMessage`，`ListAgentMessages` 包含本轮消息 | 路由提示词里本轮请求出现两次，影响分工 | 读取历史时排除 `userMsg.ID` |
| CB-BE-07 | S2 | `executeAgentTask` 能力不可用（`cli_skills_unavailable`/`sandbox_unavailable`）分支只改库不发 `task` 帧 | 前端任务永远 pending | 统一经 `markTask` 同时落库+发帧 |
| CB-BE-08 | S3 | `HumanName: tc.humanID`、`emitHuman` 的 `HumanName: task.AssigneeID` 直接把 `user:<id>` 主体串给模型与界面 | 提示词与 UI 出现内部 ID | 宿主解析用户显示名（无则「当前用户」） |
| CB-BE-09 | S3 | `turnStream.close()` 无条件写 `end`，`settleTurn` 终态又 `writeEnd` | 重复 `end` 帧 | broker 保证每个订阅只收到一次 `end` |
| CB-BE-10 | S3 | 群禁用人工（`human_policy=disabled`）时显式 @人 返回 `human_assignee_required` | 错误码误导 | 新增 `human_policy_disabled` 错误码（前后端 i18n 同步） |
| CB-BE-11 | S2 | `setTaskStatus` 吞掉所有错误；`resetTaskSubtree`/`cancelTurn` 用 `UpdateAgentTask(…,0)` 跳过乐观锁 | 并发动作互相覆盖、静默失败 | 引擎内统一 `TaskStore.Transition(from→to)`，非法迁移返回错误并记日志 |
| CB-BE-12 | S3 | 显式 @ 时 `BuildMentionDispatchPlan` 把**同一段原文**发给每个助手，包含给别人的指令 | 助手越权回答别人的部分 | 由 Plan 栅栏为每个 @ 目标生成独立消息（§13.3） |
| CB-BE-13 | S3 | `/runs` 自动路由（`agent_run_auto.go` 的 `Route`）与 `/turns` 的 `RoutePlan` 是两套决策 | 两套提示词与回退策略漂移 | 群会话只走 `/turns` + Plan 栅栏；`/runs` 仅保留固定助手单聊，删除 `Route`/`buildDispatchPrompt`/`parseDispatchAnswer` |


### 13.3 Plan 栅栏（Plan Fence）协议：群消息的唯一入口

**原则**：群会话里的每条用户消息都必须先通过 Plan 栅栏，栅栏产出一份 `Decision`，执行器只按 `Decision` 行事。用户的 `@` 只是**提示与约束**，不再绕过栅栏直接分派（取代 `BuildMentionDispatchPlan` 的「同一段原文发给每个人」，修复 CB-BE-12）。

```
用户消息 ──► [Fence] ──► Decision{goal, dispatches[], confirm, progress}
                │                    │
                │         confirm.required? ──是──► 弹框（awaiting_confirm）──► 批准 / 修改 / 拒绝
                │                    │否                                    │
                ▼                    ▼                                      ▼
          规则短路（见下）      [Executor] 按依赖调度 @助手（每人收到自己的 message）
                                     │
                              每个任务结束 ──► progress 更新；若 plan.replan 触发 ──► 回到 Fence（有上限）
```

**栅栏决策顺序（确定性，先规则后模型）**：

1. **输入校验**：内容空、群已归档、配额、协作表未就绪 → 直接错误（沿用现有错误码）。
2. **规则短路**（不调模型，但仍生成 `Decision` 并落库，前端展示一致）：
   - 群内仅 1 个可用成员且未 @人 → `dispatches=[{agent, message: 原文}]`。
   - 模型未配置 → 只用 `fallback_agent_id`；无兜底 → `routing_failed`（保持现有「不静默选第一个」）。
3. **模型决策**：调用 Planner（§13.5），模型**必须**以 `plan` 工具调用输出（native tool call；不支持 tools 的模型走 text 协议的 `TOOL_CALL` 回退，沿用 `agentcore` 已有能力）。不再解析自由 JSON 文本。
   - 用户显式 @ 的目标作为**硬约束**传入：Decision 必须覆盖每个被 @ 的助手（可以给不同 message），不得加入群外成员；违反则校验失败。
   - 校验失败 → 一次带错误信息的修复重试 → 仍失败则回退到「被 @ 的目标各收原文」或 fallback（保持可用）。
4. **确认判定**（`confirm.required` 由「模型建议 ∪ 宿主强制规则」得出，宿主规则只能加严不能放宽）：
   - 模型声明 `confirm.required=true`（含理由）；
   - 任务数 ≥ 群配置阈值（默认 3）或依赖深度 ≥ 2；
   - 存在 `risk: destructive` 的 dispatch；
   - 群配置 `confirm_policy=always`；`never` 只关闭「模型建议」，**不关闭** destructive 强制项；
   - headless（cron / 告警）下需要确认 → 不弹框，按群策略 `deny`（默认）或转人工待办。
5. **落库与广播**：Decision 写入轮次（`route_json` 扩展为 `plan_json`，不新增表；见 §13.10），广播 `plan` 帧；需要确认时轮次进入 `awaiting_plan_confirm`。
6. **人工确认**：弹框支持 **批准 / 编辑后批准 / 拒绝**。编辑只允许：删除某个 dispatch、修改某个 dispatch 的 message、调整依赖顺序；不允许新增群外成员。编辑后重新走第 3 步的校验（不调模型）。
7. **执行与再规划**：执行器按依赖调度；每个任务结束后更新 progress。任务失败或助手调用 `plan(action="replan")` 时回到栅栏（每轮最多 `MaxReplans=2`，超出则转人工待办）。

**与工具级确认的区别**（必须在 UI 与代码中严格区分）：

| | Plan 确认（新增） | 工具调用确认（已有 `AwaitConfirmation`） |
| --- | --- | --- |
| 时机 | 执行任何助手前 | 某个助手执行破坏性工具时 |
| 粒度 | 整份分工 | 单次工具调用（runId+callId） |
| 端点 | `POST /agent-turns/:id/plan-confirmations` | 既有 `POST /agent-runs/:runId/confirmations/:callId` |
| 交互 | 模态弹框（需要看全局分工后决定） | 行内卡片（保留 `ui/AGENTS.md` 现有约定） |

> `ui/AGENTS.md`「运行确认用行内卡片」约定只覆盖工具确认；Plan 确认使用 `SbModal`，实施时同步更新 `ui/AGENTS.md` 的确认交互条目。

### 13.4 `plan` 工具 schema（Planner 唯一输出通道，参考 `explore`）

工具定义放 `gocloudagent/plan/tool.go`，与宿主无关；助手 ID、确认策略等由调用方通过 `plan.Constraints` 注入校验。

```text
name = "plan"
description =
  "Record or update the structured plan for the current request BEFORE any assistant is dispatched. "
  "Decide which assistants to @, the exact message each one receives, dependencies between them, "
  "and whether a human must confirm the plan first. For complex work also maintain a goal and "
  "step-level progress (planning-with-files style: the plan is persisted to plan.md/progress.md). "
  "It DOES NOT execute anything; the host validates it against group membership and policy, then "
  "dispatches. Call again with action=update/replan as understanding or results change."
```

```json
{
  "type": "object",
  "properties": {
    "action": {
      "type": "string", "enum": ["create", "update", "replan", "complete"],
      "description": "create on first decision; update to record progress; replan after failure or new info; complete when the goal is met.",
      "default": "create"
    },
    "goal": {"type": "string", "description": "Single sentence: what the user ultimately wants. Required for create/replan."},
    "complexity": {
      "type": "string", "enum": ["simple", "complex"],
      "description": "simple = one-shot dispatch, no step tracking; complex = maintain steps/progress and persist plan files."
    },
    "reason": {"type": "string", "description": "Short rationale for this split (shown to the user)."},
    "dispatches": {
      "type": "array",
      "description": "Who to @ and what each one receives. Order is execution order unless depends_on says otherwise.",
      "items": {
        "type": "object",
        "properties": {
          "ref": {"type": "string", "description": "Local id like d1/d2 used by depends_on."},
          "target_kind": {"type": "string", "enum": ["agent", "human"]},
          "target_id": {"type": "string", "description": "Candidate assistant id, or the given human id."},
          "message": {"type": "string", "description": "The exact message sent to this target. Must be self-contained; do not include instructions meant for other targets."},
          "expected_output": {"type": "string", "description": "What this target should hand back (used for progress and downstream context)."},
          "depends_on": {"type": "array", "items": {"type": "string"}, "description": "Refs that must complete first; only earlier refs."},
          "context_from": {"type": "array", "items": {"type": "string"}, "description": "Refs whose results are forwarded into this message as context."},
          "risk": {"type": "string", "enum": ["read", "write", "destructive"], "description": "Highest expected side effect; destructive forces plan confirmation."},
          "step_id": {"type": "string", "description": "For complex plans: which step this dispatch advances."}
        },
        "required": ["ref", "target_kind", "target_id", "message"]
      }
    },
    "confirm": {
      "type": "object",
      "description": "Whether a human must approve this plan before dispatch.",
      "properties": {
        "required": {"type": "boolean"},
        "reason": {"type": "string", "description": "Why confirmation is needed (shown in the dialog)."},
        "question": {"type": "string", "description": "Optional concrete question for the user, e.g. which environment to target."},
        "options": {"type": "array", "items": {"type": "string"}, "description": "Optional quick choices rendered as buttons."},
        "editable": {"type": "boolean", "description": "Whether the user may edit messages/remove dispatches before approving.", "default": true}
      },
      "required": ["required"]
    },
    "steps": {
      "type": "array",
      "description": "Complex plans only: ordered milestones toward the goal.",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "title": {"type": "string"},
          "status": {"type": "string", "enum": ["pending", "in_progress", "done", "blocked", "skipped"]},
          "evidence": {"type": "string", "description": "What proves it is done (cite task/result)."}
        },
        "required": ["id", "title", "status"]
      }
    },
    "progress": {
      "type": "object",
      "properties": {
        "percent": {"type": "integer", "minimum": 0, "maximum": 100},
        "summary": {"type": "string", "description": "One-line status shown in the group header."},
        "next": {"type": "string", "description": "Next concrete action."}
      }
    },
    "need_summary": {"type": "boolean", "description": "Whether a final summary message should be produced after all dispatches finish."},
    "findings": {"type": "array", "items": {"type": "string"}, "description": "Facts learned so far (persisted to findings.md for complex plans)."},
    "unknowns": {"type": "array", "items": {"type": "string"}, "description": "Open questions; a non-empty list with no dispatch implies asking the human."}
  },
  "required": ["action"]
}
```

**校验规则**（`plan.Validate(decision, constraints)`，纯函数，替代 `ValidateDispatchPlan`）：

- `create/replan` 必须有 `goal` 且 `dispatches` 非空，或 `unknowns` 非空且含一个 `target_kind=human` 的 dispatch。
- `target_id` ∈ 候选集或等于给定 human id；human 不可用时禁止 human dispatch（错误码 `human_assignee_required` / `human_policy_disabled`）。
- 被用户显式 @ 的每个助手至少出现一次（硬约束）。
- `depends_on` / `context_from` 只能引用更早的 ref，无重复；深度、数量、message 长度沿用 `PlanLimits`（保留 C-07 的线性深度算法）。
- `complexity=simple` 时忽略 `steps`；`complex` 时 `steps` 非空且每个 dispatch 的 `step_id` 必须存在。
- `update` 只允许改 `steps/progress/findings/unknowns`；改分工必须 `replan`。
- 错误按**首错**返回，错误码保留现有 `routing_failed` / `invalid_dispatch_graph` 等，便于前端已有 i18n 复用。

**同族工具（规划工具组，均在 `gocloudagent/plan`）**：

| 工具 | 作用 | 说明 |
| --- | --- | --- |
| `plan` | 上述分工 + goal/progress | Planner 必用；执行中的助手也可调用 `action=update/replan`（仅当宿主授予） |
| `explore` | 记录探索笔记（goal/questions/reads/findings/unknowns），追加到会话 `explore.jsonl` | 按用户给的 schema 原样实现；**不读文件**，只固化证据 |
| `reflect` | 复杂/卡住/临近决策时的反思：目标一致性检查、对假设与失败模式的对抗式审查、建议下一步，并把持久事实写入 `facts.md` | 按用户给的 schema（仅 `reason`）；内部用一次无工具的模型调用产出结构化结果，**由模型自主决定调用，不自动触发** |
| `todo_write` | 会话内任务列表（见 §13.6 TodoManager） | 简单计划的轻量进度；与 `plan.steps` 并存：`steps` 面向群/用户，todo 面向单助手自身 |

### 13.5 `gocloudagent/plan` 包：通用 Plan 能力（对标 Codex / Pi / planning-with-files）

**定位**：与 `cloudagent.Runtime` 平级的纯机制包，零 SimpleBase 依赖，可在 CLI、库、云端复用。

```
gocloudagent/plan/
  types.go        Decision / Dispatch / Step / Progress / Confirm / Constraints / Limits
  tool.go         plan / explore / reflect 工具定义（eino InvokableTool），参数解码与 Validate
  validate.go     Validate(decision, constraints) —— 吸收 cloudagent/collab_validate.go
  planner.go      Planner：给定 Request{Goal?, UserText, Candidates, Mentions, History, Constraints} 调模型并强制 plan 工具输出
  prompt.go       Planner 系统提示（吸收 collab_prompt.go；群路由提示词仍作为“用户数据”放 user 消息）
  store.go        Store 接口：Load/Save(Decision)、AppendExplore、AppendFact、WriteFiles
  filestore.go    FileStore：planning-with-files 布局（CLI 默认）
  progress.go     由任务状态归约 Steps/Progress（吸收 ReduceTurnState 的 progress 部分）
```

**Planner 行为**（对标对象及取舍）：

| 能力 | Codex | Pi | 本方案 |
| --- | --- | --- | --- |
| 结构化计划工具 | `update_plan`（steps+status） | todo/plan 扩展 | `plan`，额外承载**多助手分工与确认** |
| 计划持久化 | 会话内 | 会话文件 | `Store` 接口；CLI 用 `FileStore`，云端用宿主 DB 实现 |
| 复杂度分级 | 模型自决 | 模型自决 | `complexity` 字段 + 宿主阈值；`simple` 不写文件，减少开销 |
| 反思 | 无独立工具 | 无 | `reflect`（模型决定调用） |
| 人在环 | 审批模式 | 确认扩展 | `confirm` 字段 + 宿主强制规则（只能加严） |

**planning-with-files 布局**（`FileStore`，根目录由调用方指定，CLI 默认 `<session_dir>/plan/`；云端不落本地盘）：

| 文件 | 内容 | 写入时机 |
| --- | --- | --- |
| `plan.md` | goal、steps（checkbox）、当前分工、确认记录 | `plan` create/replan/complete |
| `progress.md` | 时间线：每次 update 的 percent/summary/next 与任务结果摘要 | `plan` update、任务结束 |
| `findings.md` | `findings` 去重追加 | `plan` / `explore` |
| `facts.md` | `reflect` 抽取的持久事实（可跨会话复用） | `reflect` |
| `explore.jsonl` | `explore` 原始记录 | `explore` |

写入采用临时文件 + rename；路径经 §13.6 `safePath` 校验；文件大小上限（默认 256 KiB/文件，超出时滚动截断旧条目）。

**云端存储**：宿主实现 `plan.Store`，Decision 存 `sys_agent_turns.route_json`（列已存在，JSON 形状升级为 `{"version":3,"decision":{...},"confirm":{...}}`），progress/findings 写轮次事件表；`facts.md` 对应的云端存储本轮**不实现**（需要新表，属 DDL，见 §13.13）。

**复杂度判定**：Planner 先让模型选择 `complexity`；宿主可用规则强制 `complex`（dispatch ≥ 3、存在依赖、用户消息超过阈值）。`complex` 时执行器在每个任务结束后把结果摘要回灌 Planner 做 `update`（低成本：无工具、限 token），必要时 `replan`。


### 13.6 工具集扩展（`gocloudagent/tools` 重组；已存在的只做对齐）

**现状盘点**：`tools/tools.go` 已有 `shell`、`read_file`、`write_file`、`edit_file`、`glob`、`grep`（`Workspace` 方法，路径校验 `resolvePath`+符号链接检查，`skipDirs`，`boundedBuffer`，敏感环境变量过滤）；`tools/search.go` 已有 `web_search`（Brave / DuckDuckGo）；MCP 工具装配在 `cloudagent/mcptool.go` 与 `cli/mcp_adapter.go`。**缺**：工具基类/共享辅助的统一层、HTTP 工具与 SSRF 检查（SSRF 只在宿主 `agentbridge/mcp.go` 的 `guardDialContext`/`isBlockedIP`）、`web_fetch`、Todo、reflect、explore、plan、MCP 作为通用工具入口。

**目标布局**（同包按职责拆文件，不建框架；工具仍是 eino `tool.BaseTool`）：

| 用户列出的项 | 目标文件 / 符号 | 状态 | 说明 |
| --- | --- | --- | --- |
| 工具基类与共享辅助 `_tool_exec / _RootMixin / _s / _as_str_list` | `tools/base.go`：`Exec`（统一执行包装：参数解码→执行→`MarshalToolJSON`/`ErrEnvelope`→脱敏→截断）、`Root`（工作区根与 `safePath` 的嵌入式 mixin）、`argString`/`argStrings`（宽松取参） | 新增 | Go 里不用继承：`Root` 是可嵌入 struct；`Exec` 是泛型函数 `Exec[T any](name, desc string, fn func(ctx, T) (any, error)) tool.BaseTool`，只收敛现有 6 个工具里重复的样板，**不做注册中心** |
| 路径安全 `_safe_path / IGNORED / grep 常量` | `tools/path.go`：`SafePath`（= 现 `resolvePath`，公开给 plan FileStore 复用）、`Ignored`（= 现 `skipDirs`，补 `.venv`、`__pycache__`、`.idea`、`target`、`build`）、`grepMaxMatches`/`grepMaxLineBytes`/`maxWalkFiles` | 改名 + 小补 | 行为保持 §12 BUG-13 的符号链接防护；忽略目录扩充是可见变化，单测锁定 |
| 文件系统 `ReadTool / GlobTool / GrepTool / WriteTool / EditTool` | `tools/file.go`、`tools/search_fs.go` | 已存在 → 迁移到 `Exec`/`Root` | 工具名统一为 `read` / `glob` / `grep` / `write` / `edit`（与 `explore` 描述里引用的名字一致）；激进策略下**直接改名**，`read_file` 等旧名不保留别名；同步 CLI 提示词与测试 |
| HTTP `HttpTool + SSRF 检查` | `tools/http.go`：`http` 工具（method/url/headers/body/timeout，响应体上限 1 MiB，文本化输出）；`tools/netguard.go`：`GuardDialContext`、`IsBlockedIP`、`CheckURL`（scheme 白名单、禁止 userinfo、DNS 解析后逐 IP 检查、禁止重定向到内网） | 新增 + 从宿主下沉 | `agentbridge/mcp.go` 的 `guardDialContext`/`isBlockedIP` 迁到这里，宿主 MCP HTTP 客户端改为调用；CLI 默认 `AllowPrivate=false`，配置 `http.allow_private=true` 才能访问局域网（显式 opt-in，与 §9.1「不把云策略变 CLI 默认」的冲突在此**有意改变**，登记为行为变化） |
| Shell `BashTool` | `tools/shell.go` | 已存在 | 工具名 `shell` → `bash`；保留隔离、危险命令确认、`boundedBuffer`、环境过滤 |
| Todo `TodoManager / TodoWriteTool` | `tools/todo.go`：`TodoManager`（会话内存 + 可选持久化回调）、`todo_write` 工具（`todos[{id,content,status}]`，`merge` 语义，最多一个 `in_progress`） | 新增 | CLI 写入会话 JSONL 的 `todo` 条目；云端经宿主回调写轮次事件（前端可展示） |
| Web `WebSearchTool / WebFetchTool` | `tools/web.go`：`web_search`（已存在，迁移到 `Exec`）、`web_fetch`（GET + HTML→文本、`CheckURL` SSRF、正文上限 256 KiB、可选 `max_chars`） | search 已存在；fetch 新增 | `web_fetch` 必须复用 `netguard`，不得另写一套 |
| MCP | `tools/mcp.go`：`MCPTools(sessions mcp.SessionSet, policy, confirm) []tool.BaseTool` | 收敛 | 把 `cloudagent/mcptool.go` 与 `cli/mcp_adapter.go` 中「MCP 工具 → eino 工具」的公共部分提到这里；§12.9 L-07「不做万能包装」的结论被本需求推翻的部分仅限**工具构造**，授权/截断/脱敏差异继续由 `MCPToolOptions` 显式参数表达 |
| reflect | `plan/tool.go`：`reflect` | 新增 | schema 按用户给定；输出 `{aligned, risks[], counterpoints[], next_actions[], facts[]}`，`facts` 写 `Store.AppendFact` |
| explore | `plan/tool.go`：`explore` | 新增 | schema 按用户给定；追加 `explore.jsonl`，返回累计摘要 |
| plan | `plan/tool.go`：`plan` | 新增 | §13.4 |

**工具集装配**：`tools.Toolset{Root, Shell, HTTP, Web, Todo, MCP}` + `Toolset.Build(enabled []string)`，按名字选择；CLI 与宿主都从这里取。宿主业务工具（SQL/KV/S3/日志/沙盒/SimpleBase CLI）仍在 `agentbridge`，通过 `RunPreparation.Tools` 追加，不进入 `gocloudagent`。

**统一约束**：所有工具输出经 `agentcore.RedactSecrets`；失败统一 `ErrorEnvelope`；超长输出统一 `truncate` 并标注 `truncated`；`write`/`edit`/`bash`/`http` 非 GET 视为写操作，受 `Writable` 与确认策略约束（复用 `AwaitConfirmation`）。

### 13.7 `agentbridge` → `gocloudagent` 通用能力下沉清单

判定标准沿用 §1.2：**不理解 SimpleBase 资源/身份/业务流程**的机制下沉；业务授权与产品流程留宿主。

| 来源（`internal/agentbridge`） | 去向 | 理由 / 宿主保留部分 |
| --- | --- | --- |
| `mcp.go`：`guardDialContext`、`isBlockedIP`、TLS 最低版本、禁止重定向的 `http.Client` 构造 | `tools/netguard.go` + `mcp.NewGuardedHTTPClient(opts)` | 纯网络防护机制；宿主保留 server 存储、凭据查询、`DisableExternal`、内置 server 身份 |
| `mcp.go`：`MCPPolicy` 的分类逻辑（read_only_tools / 默认 fail-closed） | `mcp.Policy`（基于配置的通用实现） | 宿主保留「从 systemdb 读策略」的适配器 |
| `plan_stages.go`：`ValidatePlanStages` 通用部分 | 删除，宿主改用 `plan.Validate` + 固定阶段约束 `Constraints.FixedOrder` | §3-M-07 曾暂缓，本需求引入统一 plan 后两份校验无存在理由；首错顺序以 `plan` 为准（行为变化，测试更新） |
| `plan_skills.go`：`PlanStageDone` 的「工具卡是否成功」判断 | 复用 `agentcore.ToolResultIsError`（已统一） | argv 匹配、阶段→技能映射仍留宿主 |
| `prompt.go`：`AssembleInstruction` 的分段拼装与截断 | `cloudagent.PromptBuilder`（段落有序、每段上限、总上限） | SimpleBase 模块模板与技能正文仍留宿主 |
| `principal.go`：`MessagesFor` 的「跨助手消息加 [名字] 前缀」投影 | `cloudagent.ProjectGroupHistory(msgs, currentAgent, names)` | 群聊通用语义；`systemdb.AgentMessage` → `cloudagent.Message` 的转换留宿主 |
| `cron_agent.go`：调度无关的「headless 运行 + 群路由」编排 | 群引擎（§13.8）提供 `Engine.RunHeadless` | cron 触发、配额、`GroupCronBinding` 存储留宿主 |
| `errors.go`：`ClassifyError` | 留宿主 | 依赖 llmgateway 哨兵（§4.3） |
| `command.go`、`skilltool.go`、`tools*.go`、`modules.go`、`store.go`、`ports.go`、`delegation.go`、`context.go`、`cli*.go` | 留宿主 | SimpleBase 命令授权、业务工具、委托令牌、模块目录 |
| `internal/api/collab_coordinator.go` 的推进循环、`agent_dispatch.go` 的 `RoutePlan`、`collab_stream.go` 的事件形状 | 群引擎（§13.8） | API 层只剩 HTTP 绑定与持久化适配 |
| `cloudagent/collab_*.go` | 并入 `gocloudagent/group`（状态机）与 `gocloudagent/plan`（决策/校验/提示词） | 删除 `DispatchPlan`/`ParseDispatchPlan`/`BuildMentionDispatchPlan`/`BuildCollabDispatchPrompt`；§12.8-3 的 26 个 Collab 常量随之重定义 |

**依赖方向（更新 §1.3）**：

```
agentcore ◄── tools ◄── plan ◄── group ◄── cloudagent(Runtime 门面) ◄── 宿主 internal/*
     ▲          ▲                               ▲
     └── mcp ◄──┘                               └── cli（可依赖 plan/group/tools，不依赖 cloudagent 的宿主门面）
```

- `plan` 不依赖 `group`；`group` 不依赖 `cli`；`cloudagent` 与 `cli` 仍不互相 import（§1.3 保持）。
- 新增 `gocloudagent/group`、`gocloudagent/plan` 两个包；不建 `common`/`utils`。
- `go list -deps` 门禁增加：`plan`/`group`/`tools` 不得 import `internal/...`、`cli`、`cloudagent`。

### 13.8 `gocloudagent/group`：群聊引擎（栅栏 + 调度，CLI 与云端共用）

```
gocloudagent/group/
  types.go      Group{ID, Members[], HumanID, Policy}、Turn、Task、Status 常量（替代 Collab* 常量）
  engine.go     Engine{Planner, Runner, Store, Events, Clock}
                  Submit(ctx, TurnInput) (TurnID, error)      // 栅栏 → 落库 → 若无需确认则启动执行
                  ConfirmPlan(ctx, turnID, Approval) error     // 批准 / 编辑后批准 / 拒绝
                  ReplyHuman(ctx, taskID, Reply) error
                  Cancel(ctx, turnID) / RetryTask(ctx, taskID) / Resume(ctx, turnID)
                  RunHeadless(ctx, TurnInput) (TurnResult, error)   // cron / 告警
  fence.go      Fence：§13.3 的规则短路 + Planner 调用 + 确认判定
  scheduler.go  依赖调度（吸收 NextTaskIndex/BlockedTasks；支持无依赖任务并发，默认并发 1 保持现行为，可配置）
  state.go      Turn/Task 状态机与合法迁移表（修复 CB-BE-03/11）
  registry.go   进程内活动轮次登记：轮次级 cancel、单线程占用（替代 ClaimAgentTurn 的进程内部分）
  broker.go     事件广播：单调 seq、落库、多订阅者、只发一次 end（修复 CB-BE-04/05/09）
  ports.go      宿主端口：Store（turn/task/event/decision CRUD + 乐观锁迁移）、Runner（执行单个助手任务 → cloudagent.Runtime.StartRun）、Members（成员与能力可用性）
```

**状态机**（新增 `awaiting_plan_confirm`；`routing` 改名 `planning`，激进策略直接替换，前端同步）：

```
Turn:  planning → awaiting_plan_confirm → running ⇄ waiting_human → completed | failed | canceled | interrupted
                └──────── rejected（拒绝计划，终态，不执行任何任务）
Task:  pending → running → completed | failed ; pending → blocked ; * → canceled ; running → interrupted
       human: waiting_human → completed | failed(unable)
```

终态不可被非取消路径覆盖；所有迁移经 `Store.Transition(id, from, to, version)`。

**执行语义**：

- 执行在后台运行（`context.WithoutCancel` + 轮次 cancel），HTTP 请求只是 `Submit` 并订阅事件（修复 CB-BE-02）。
- 每个 dispatch 的 `message` 作为该助手本轮 `UserText`；`context_from` 引用的上游结果（截断后）以「[来自 @X 的结果]」段落拼入；历史投影用 `cloudagent.ProjectGroupHistory`。
- `need_summary=true` 时在全部任务完成后由 Planner 产出总结消息（speaker=plan）。
- 进程重启：沿用 `convergeCollaboration` 语义收敛为 `interrupted`，迁入 `Engine.Converge`。

**CLI 落地**（证明独立可用）：新增 `gocloudagent group` 子命令与 REPL `/group` 模式：本地 `~/.gocloudagent/groups/<name>.json` 定义成员（每个成员 = 模型 + 指令 + 工具子集），`FileStore` 持久化计划，确认弹框在终端以 `[a]pprove/[e]dit/[r]eject` 交互；headless（`--yes` 未给）时按策略拒绝。


### 13.9 云助手前端改造与样式美化

**交互模型**：每个 tag = 一个群。群会话的消息流从上到下是：用户消息 → **Plan 卡**（栅栏决策）→ 各助手气泡（按任务分组，带「@来自 Plan 的指令」引用）→ 可选总结。单助手固定会话（`/runs`）保持现有气泡样式。

**组件调整**（遵守 `ui/AGENTS.md`：Tailwind class、reka 真实属性、`SbModal`、`ConfirmAction`、按钮尺寸规范、i18n 双语同构）：

| 组件 | 动作 | 要点 |
| --- | --- | --- |
| `AgentPlanCard.vue`（新，替代 `AgentDispatchCard.vue`） | 新增 / 删除旧件 | 头部：goal + `complexity` 徽标 + 进度条（`progress.percent`）+ `summary`；主体：dispatch 列表（目标头像/名字、message 折叠展开、依赖箭头、risk 徽标、状态点）；complex 时显示 steps checklist；底部：重试失败任务（`Button size="xs" variant="outline"`）、展开 findings。保留 `data-dispatch-*` 测试钩子 |
| `AgentPlanConfirmModal.vue`（新） | 新增 | `SbModal :max-width="720"`；展示 reason/question/options；每个 dispatch 可编辑 message、可移除（`editable=true` 时）；底部「拒绝」(`destructive` outline)/「批准并执行」(`default`)。打开时聚焦批准按钮，`Esc`=关闭不决策（仍待确认，可从 Plan 卡再次打开） |
| `ConversationView.vue` | 改 | 消息按 `turnId` 分段；assistant 气泡上方显示 `@助手名` + 「来自 Plan」引用（hover 显示该助手收到的 message）；工具确认卡渲染在对应任务气泡内（修复 CB-FE-01）；思考过程同步（CB-FE-12）；`key` 改用消息 id 而非下标 |
| `AgentComposer.vue` / `AiChatComposer.vue` | 改 | 输入框上方 Plan 状态条：「将由 Plan 决定分工」/「本轮 @A、@B（Plan 会为每人生成独立指令）」；发送后显示「Plan 决策中…」骨架（替代 CB-FE-10） |
| `AgentRosterBar.vue` | 改 | 成员胶囊：头像 + 名字 + 运行中脉冲（由任务状态派生，CB-FE-11）；点击插入 @ |
| `AgentWorkspaceSidebar.vue` | 美化 | 群行：群头像（成员头像叠放，最多 3 个）、未读/待办角标、运行中指示；会话行：标题 + 相对时间 + 最近一条摘要（单行截断）；保持 `h-11`/`sm:h-9`、`gap-1.5` |
| `AgentManager.vue` 卡片头 | 改 | 群会话显示「群名 · N 位成员」+ 成员头像叠放 + 进度摘要；固定助手会话保留助手徽标（CB-FE-15）；`pickPrompt` 改 `ConfirmAction`（CB-FE-13） |
| `AgentHumanTaskCard.vue` | 美化 | 与 Plan 卡同风格；明确区分「Plan 确认」「工具确认」「人工待办」三类视觉（图标 + 主色/琥珀/中性） |

**视觉规范**（只用 `style.css` 现有 token，不新增 CSS 文件）：

- 群聊气泡：用户右侧 `bg-primary/10 border-primary/20`；助手左侧 `bg-card`；Plan 卡 `bg-muted/40 border-dashed`，与气泡区分为「系统决策」。
- 头像：模块图标保持，群成员用名字首字 + 模块色环；人用 `UserIcon`。
- 状态色：completed `emerald-500`、running `primary` 脉冲、awaiting/waiting `amber-500`、failed/blocked `destructive`、canceled `muted-foreground`；`prefers-reduced-motion` 下停止脉冲。
- 间距：消息间 `gap-3`，同一轮内 `gap-2`；最大宽度沿用 `max-w-[75%]`（窄屏 85%）。
- 暗色模式：所有新组件在 `dark` 下截图核对（Playwright 不在依赖内，人工核对 + 单测断言 class）。

**状态层重构**：

- `useAgentCollaboration` 拆为 `useGroupTurn`（轮次/任务/Plan/确认状态，按 `turnId`+`taskId` 索引）与 `services/group-stream.ts`（轮次流：`X-Turn-Id`、按 seq 去重、`after=` 续接、子 run 内容二级续接）；修复 CB-FE-02/03/04/08/09。
- `useThreadNavigation.detachRun` 同时断开轮次流（CB-FE-05）；`resumeActiveRun` 按会话类型分别续接 run 或 turn（CB-FE-06）。
- `useAgentSender`：群会话发送失败回滚乐观消息与草稿（CB-FE-14）；@ 目标作为 `mentions` 传给栅栏，不再在前端拼目标列表的兜底（`threadAgentId` 兜底删除，交给栅栏规则短路）。
- `services/types/agent.ts`：新增 `AgentPlanDecision`、`AgentPlanDispatch`、`AgentPlanStep`、`AgentPlanProgress`、`AgentPlanConfirm`（`interface`，camelCase，经 `converters/agent.ts` 映射）；`AgentTurn.status` 增 `planning`/`awaiting_plan_confirm`/`rejected`，删 `routing`。
- API：`agentTurns.confirmPlan(projectId, turnId, {approve, edits?, expectedVersion})`、`agentTurns.events(projectId, turnId, after, handlers)`、`agentThreads.turns(projectId, threadId, limit)`；`act`/`reply` 改为纯 JSON（CB-FE-07）。

### 13.10 API 与 SSE v3 契约（激进替换，`protocol_version: 3`）

**HTTP**：

| 端点 | 变化 |
| --- | --- |
| `POST /agent-threads/:id/turns` | 请求体不变（`content/mentions/idempotency_key/stream`）；`stream=true` 时为订阅流，断开**不取消**轮次 |
| `GET /agent-threads/:id/turns?limit=` | 前端开始使用（续接） |
| `GET /agent-turns/:id` | 快照增加 `plan`（Decision + confirm 状态）与 `last_event_seq` |
| `GET /agent-turns/:id/events?after=` | 帧形状与实时帧**完全一致**；`id:` = 事件 seq |
| `POST /agent-turns/:id/plan-confirmations`（新） | `{approve: bool, edits?: [{ref, message?, remove?}], expected_version}` → 202 + 快照 |
| `POST /agent-turns/:id/actions` | cancel/retry_task/resume 全部 202 + JSON 快照，后台推进 |
| `POST /agent-tasks/:id/replies` | 202 + JSON 快照，后台推进 |
| `GET /agent-workspace-capabilities` | `protocol_version: 3`，增加 `confirm_policy` 可选值 |

**SSE 帧**（统一 `{type, seq, turn_id, task_id?, run_id?, agent_id?, ...}`）：

| type | 字段 | 落事件表 |
| --- | --- | --- |
| `turn` | `status`, `index`, `total` | 是（首帧必发，携带 `turn_id`，修复 CB-FE-02） |
| `plan` | `decision`（§13.4 结构，snake_case）, `confirm_required`, `version` | 是 |
| `plan_confirmation` | `status: required/approved/rejected`, `edited: bool` | 是 |
| `progress` | `percent`, `summary`, `next`, `steps` | 是 |
| `task` | `status`, `index`, `total`, `message`, `error_code?` | 是 |
| `human` | `human_name`（显示名，CB-BE-08）, `message` | 是 |
| `confirmation_required` / `confirmation_resolved` | 工具确认，增加 `run_id` | 是（仅结构信息，argv 已脱敏） |
| `token` / `thinking` / `tool_*` / `usage` | 子 run 内容帧，带 `task_id`/`run_id` | 否（经 run 事件缓存按 `run_id` 续接） |
| `end` | `reason` | 每个订阅只一次 |

删除：`route` 帧（由 `plan` 取代）。`routing` 状态改为 `planning`。

**存储**：不新增表、不改列（`route_json` 存 v3 Decision；`status` 为 VARCHAR 无 CHECK，可直接写新值）。事件 seq 原子分配（CB-BE-04）优先用单条 SQL 实现；若方言不支持则进程内 per-turn 计数器 + 唯一约束冲突重试，二者择一实现并测试。**若实施中发现必须加索引/列，停止并单独申请 DDL，不得自行执行 `init_db`/ALTER。**

### 13.11 分批实施（B8 起，接续 §7）

> 状态标注（2026-11-24）：✅ 完成 / 🔶 部分完成 / 🚧 进行中 / ⬜ 未开始。

| 批次 | 范围 | 状态 | 说明 |
| --- | --- | --- | --- |
| **B8 缺陷复现与快修** | CB-FE-01/02/05/07/14、CB-BE-02/03/06/07 先在现有架构上写失败测试；其中不依赖新协议的（FE-05/14、BE-06/07）直接修 | ✅ 并入 B11–B13 | 未单独开批：缺陷随群引擎、宿主接线与前端改造一并修复，各自的锁定测试见 §13.15.3 |
| **B9 tools 重组** | §13.6：`base.go`/`path.go`/`netguard.go` 下沉、工具改名、`http`/`web_fetch`/`todo_write`/`tools/mcp.go` | ✅ | 落地：`tools/base.go`（`Exec` / `Args` / `ArgString(s)`）、`netguard.go`（`NetPolicy` / `CheckURL` / `IsBlockedIP` / `GuardDialContext` / `NewGuardedClient`）、`Ignored` 扩充、`Workspace.SafePath`、工具改名（bash/read/write/edit/glob/grep）、`http.go`（http + web_fetch）、`todo.go`、`tools/mcp.go`（`MCPToolInfo`）、`toolset.go`、CLI 接入（`repl.go` Toolset、`config.go` `http_allow_private`）。§13.11.1-2/3 的缺口已补：`netguard_test.go`（IP 表驱动、URL 规则、回环主机名、重定向到内网、http 响应上限与写确认、web_fetch）、`todo_test.go`、`-race` 通过；`agentbridge/mcp.go` 改用 `tools.NetPolicy{}.NewGuardedTransport()`，宿主私有 `guardDialContext` / `isBlockedIP` 已删（`mcp_test.go` 只保留对 `tools.IsBlockedIP` 的接线锁定） |
| **B10 plan 包** | §13.4/§13.5：`plan`/`explore`/`reflect` 工具、`Validate`、`Planner`、`FileStore` | ✅ | 落地：`types.go` / `schema.go` / `tool.go` / `validate.go` / `planner.go` / `prompt.go` / `store.go`；测试 `planner_test.go`（native / text 回退 / 修复重试）、`validate_test.go`（首错表驱动）、`store_test.go`（原子写与上限）、`tool_test.go`（解码、mergeUpdate、ReplayCall、explore / reflect）；CLI 接线 `cli/plan_tools.go`（`Toolset.Extra` + 会话 `FileStore`） |
| **B11 group 引擎** | §13.8：状态机、调度、broker、registry、Converge、RunHeadless；`cloudagent/collab_*` 删除 | ✅ | 落地：`group/{engine,fence,scheduler,state,registry,broker,ports,types}.go`；`engine_test.go`（提交与释放占用、取消不变成失败、人工回复续接依赖任务、失败阻塞整条依赖链、重试复活、Converge / Resume、replan 预算持久化、禁用 replan、destructive replan 等确认、headless 在人工任务处返回）、`broker_test.go`（回放与实时同序、慢订阅只收一次 end）；`cloudagent/collab_*` 与 `DispatchPlan` / `ParseDispatchPlan` / `BuildMentionDispatchPlan` / `BuildCollabDispatchPrompt` 已删除 |
| **B12 宿主接线** | `internal/api` 改为 `group.Engine` 适配：Store/Runner/Members 端口实现；删除 `collab_coordinator.go` 推进循环、`agent_dispatch.go` 的 `Route`/`RoutePlan`；`agentbridge` 下沉项改调模块；cron 走 `RunHeadless` | ✅ | 落地：`internal/api/collab_group_{engine,store,runner,stream}.go`（Store 写 `sys_agent_turns.route_json` v3 + `sys_agent_turn_events`，Runner 桥到 `Runtime.StartRun`，Members 解析群与候选）、`collab_coordinator.go` 只剩提交入口、`collab_handler.go` 的 `ConfirmTurnPlan`、`collab_cron_runner.go` 走 `RunHeadless`，`ProtocolVersion: 3`。CB-BE-13 已收口（第二轮）：群会话上的 `/runs` 返回 `400 turns_required`，`createAutoRun` / `agent_dispatch.go` / `route` 帧 / `route_kind` / `sys_agent_run_routes` / `dispatch_mode=auto` 整体删除，见 §13.15.7 |
| **B13 前端** | §13.9/§13.10：`group-stream.ts`、`useGroupTurn`、Plan 卡/确认弹窗、样式美化、其余 CB-FE | ✅ 核心链路 | 落地（文件名与计划不同）：轮次流与 seq 去重 / `X-Turn-Id` / `after=` 续接在 `services/agent-stream.ts`，轮次状态索引在 `composables/useAgentCollaboration.ts`（未另建 `useGroupTurn.ts` / `group-stream.ts`），整轮 Plan 确认在 `components/agent/PlanConfirmDialog.vue`（`SbModal`，可编辑 message / 移除 dispatch），分工卡沿用 `AgentDispatchCard.vue`。专项测试：`tests/agent-turn-stream.test.ts`、`useAgentCollaboration.test.ts`、`PlanConfirmDialog.test.ts`、`agent-collab-ui.test.ts`、`agent-skills-stream.test.ts`。**第二轮补齐**：`AgentDispatchCard.vue` → `AgentPlanCard.vue`（目标 / 步骤 / 进度条 / 发现）、气泡按 `taskId` 绑定（CB-FE-03/04）、Plan 状态条（CB-FE-10）、`pickPrompt` 改受控 `ConfirmAction`（CB-FE-13）、群头像叠放（卡片头 + 左栏群行）、会话行最近一条摘要；暗色 / 窄屏仍未人工核对。`ui/AGENTS.md` 的确认交互条目已补 Plan 弹窗例外与 Plan 卡 / 气泡绑定约定 |
| **B14 CLI 群模式** | §13.8 CLI 落地（原定义） | ✅（第二轮） | `gocloudagent/cli/group_{config,store,runtime,cmd}.go`：群定义 `~/.gocloudagent/groups/<name>.json`（成员 = 模型 + 指令 + 工具子集）、文件版 `group.Store`、成员任务 = 独立 `SessionRunner` 会话、`plan.FileStore` 落计划文件；`gocloudagent group list/show/new/run`（`--yes` 批准需确认的计划，否则 headless 拒绝）；REPL `/group [name|status|resume|off]`、终端 `[a]pprove/[e]dit/[r]eject`、人工待办就地回复或 `/reply` / `/unable`。见 §13.15.7 |
| **B14 六阶段 → plan.Tools** | 删除固定六阶段计划模式，所有云助手 run 统一经 `plan.Tools` 规划 | ✅ | 见 §13.15.4：删除 `plan_handler.go`、`systemdb/plans.go` 与两张表、`agentbridge/plan_stages.go` / `plan_skills.go`、`cloudagent/plan.go`、`/agent-threads/:id/plans*` 路由；新增 `internal/api/agent_plan_tools.go`（单聊 + 群任务共用装配、历史重放、`plan` 帧）与 `plan.ReplayCall` |
| **B15 文档与回写** | `gocloudagent/README.md`（新包、工具名、群引擎、SSE v3）、`ui/AGENTS.md`、`internal/AGENTS.md`；本节逐项回写结果 | ✅ | README 重写（包职责、依赖方向、plan.Tools / group.Engine 接入、MCP 安全、CLI）；根 `AGENTS.md`、`internal/AGENTS.md`、`internal/README.md`、`ui/AGENTS.md` 同步；本表与 §13.15 即回写 |

每批独立检查点；行为变化（工具改名、SSE v3、状态改名、CLI 内网默认拒绝、plan 首错顺序）在回写时逐项登记。

### 13.11.1 中断点与恢复清单（2026-11-24 标注，2026-11-25 全部关闭）

> 以下 5 项均已在 B9–B14 处理完毕，保留原文作为历史记录。

按恢复优先级排列；恢复实施时从第 1 项开始：

1. **编译失败（阻塞一切验证）**：`cli/repl.go:227` 的 `planTools(session)` 未定义——需在 `cli` 包新增该函数（用 `plan.Tools{Store: ...}` 之类的入口把 plan/explore/reflect 三工具接到会话，具体形状实施时定）；同时删除 `cloudagent/mcptool.go` 与 `cli/mcp_adapter.go` 的未用 `encoding/json` import。
2. **B9 门禁缺口**：`tools` 包缺 SSRF 表驱动测试（127/8、10/8、169.254、::1、fc00::/7、DNS rebinding、重定向到内网、userinfo）、todo merge / 单 in_progress 测试、`http`/`web_fetch` 行为测试（响应上限、max_chars、AllowPrivate）；`go test -race ./tools/...` 待补跑。
3. **B9 下沉接线缺口**：`internal/agentbridge/mcp.go` 改调 `tools.NetPolicy`/`tools.IsBlockedIP`（删除宿主私有 `guardDialContext`/`isBlockedIP`）；`mcp_test.go` 的 `isBlockedIP` 断言同步迁移。
4. **B10 缺口**：`plan/planner.go` + `prompt.go`（fake ChatClient 覆盖 native / text 回退 / 修复重试 / 回退原文）；plan 包全部单元测试（Validate 首错表驱动、FileStore 原子写与上限、tool 解码与 mergeUpdate、explore.jsonl 追加）。
5. **B8 复现测试**：按 §13.2 写失败测试（可在 B11/B12/B13 各自范围内先建测试再修）。

### 13.12 测试矩阵补充

| 领域 | 必测场景 |
| --- | --- |
| 栅栏 | 单成员短路；无模型 + 有/无 fallback；显式 @ 硬约束缺失→修复重试→回退；群外 ID；human 不可用/禁用；超限任务数/深度/消息长度；文本协议回退 |
| 确认判定 | 模型建议、阈值触发、destructive 强制、`confirm_policy=never` 仍拦 destructive、headless deny/转人工、编辑后重新校验、拒绝为终态 |
| 执行 | 依赖顺序、`context_from` 注入与截断、失败→blocked→retry 子树、replan 上限、need_summary、取消不被覆盖、订阅断开后后台继续 |
| 事件 | seq 原子单调、实时帧与回放帧逐字节同形、`end` 只一次、续接 after=N 无丢无重、子 run 内容二级续接 |
| 工具 | 各工具改名后 CLI 提示词/测试同步；`http`/`web_fetch` SSRF 全表；todo merge 与单 in_progress；explore/reflect 落盘与上限；MCP 工具构造在云/本地两种 options 下的差异保持 |
| 前端 | 群会话工具确认可放行（FE-01）；停止有效（FE-02）；同助手多任务不串气泡（FE-03）；无重复气泡（FE-04）；切会话不串流（FE-05）；刷新后恢复 Plan 卡与进度（FE-06）；重试/回复不阻塞（FE-07）；断线续接（FE-08）；Plan 弹框编辑/拒绝；暗色与窄屏布局；`modal-input-layout` 与 `ui-state-attributes` 既有守卫通过 |

### 13.13 明确不做 / 需另行审批

- **不做 DDL**：`facts.md` 的云端持久化、Plan 决策独立表、事件表索引若必要均另立计划审批；本轮云端 `reflect` 的 facts 只写入轮次事件。
- 不引入新依赖（前端动画、后端 HTML 解析：`web_fetch` 用标准库 + 现有 `stripTags` 简化实现）。
- 不改 `gosandbox`、`godatabase`、`llmgateway` 内部。
- 不保留旧工具名别名、旧 `route` 帧、旧 `routing` 状态（激进策略）；外部若有消费者需在回写中列出。
- 宿主业务工具、SimpleBase 命令授权、固定六阶段产品流程、委托令牌**永不下沉**（§9.1 继续有效）。
- 并发执行无依赖任务默认关闭（并发 1），开启需观测配额与确认交互后再决定。

### 13.14 待确认的决策（已于 2026-11-24 拍板：全部按推荐默认执行）

| 决策 | 推荐默认 | 状态 |
| --- | --- | --- |
| 工具名统一为 `read/write/edit/glob/grep/bash/http/web_search/web_fetch/todo_write/plan/explore/reflect` 且不留别名 | 是 | ✅ 已拍板；B9 中工具改名已落地（bash/read/write/edit/glob/grep、http/web_fetch/todo_write） |
| Plan 确认阈值（任务数）与默认 `confirm_policy` | 3 / `auto` | ✅ 已拍板；待 B10/B11 实施 |
| 用户显式 @ 是否也必须经模型栅栏（多一次模型调用） | 是（为每人生成独立 message）；单成员与无模型场景走规则短路 | ✅ 已拍板；待 B11 实施 |
| CLI `http`/`web_fetch` 默认拒绝内网 | 是，需 `allow_private` 显式开启 | ✅ 已拍板；B9 已落地（`config.go` `http_allow_private`，默认拒绝） |
| 轮次状态 `routing` 直接改名 `planning` | 是 | ✅ 已拍板；待 B11/B12/B13 实施（当前 `internal/api` 仍为 `routing`，`ProtocolVersion: 2`） |
| headless 下需要 Plan 确认时的默认 | 拒绝并记录（群可配为转人工待办） | ✅ 已拍板；待 B11 实施 |

### 13.15 实施记录（2026-11-25 回写：B11–B15）

> 策略沿用 §12 / §13「激进，不考虑兼容」。本节只记录**实际落地**与**验证过**的事实；未验证的写进「未运行」。

#### 13.15.1 落点与删除

| 批次 | 实际落点 | 删除的旧实现 |
| --- | --- | --- |
| B9 | `gocloudagent/tools/{base,netguard,http,todo,mcp,toolset}.go`；`cli/repl.go` Toolset 组装、`cli/config.go` `http_allow_private` | 宿主 `agentbridge/mcp.go` 的 `guardDialContext` / `isBlockedIP`；裸 shell `CombinedOutput`（§12.1 L-11） |
| B10 | `gocloudagent/plan/*`；`cli/plan_tools.go` | — |
| B11 | `gocloudagent/group/*` | `cloudagent/collab_*`、`DispatchPlan` / `ParseDispatchPlan` / `BuildMentionDispatchPlan` / `BuildCollabDispatchPrompt` |
| B12 | `internal/api/collab_group_{engine,store,runner,stream}.go`、`collab_coordinator.go`（提交入口）、`collab_handler.go`（`ConfirmTurnPlan`、动作统一 202 + 快照）、`collab_cron_runner.go`（`RunHeadless`）、`systemdb/agent_{turns→collab,tasks,events}.go` | `collab_coordinator.go` 的请求内推进循环、`buildDispatchPrompt` / `parseDispatchAnswer`、`RoutePlan` |
| B14 | `internal/api/agent_plan_tools.go`（`planningTools` / `runPlanningTools` / `turnPlanningTools` / `replayPlan` / `planCallArguments` / `requestsReplan`）；`plan.ReplayCall`；`plan.Tools.OnDecision(ctx, d)`；`cloudagent.Event.Decision`（v3 `plan` 帧的 decision 字段） | `internal/api/plan_handler.go`（+测试）、`/agent-threads/:threadID/plans*` 五条路由、`cloudAgentHandler.planLocks`、`ErrPlanInvalid` / `ErrPlanBusy`、`internal/systemdb/plans.go` 与 `sys_agent_plans` / `sys_agent_plan_stages` 建表、`bootstrap.go` 的 `InterruptRunningPlans`、`agentbridge/plan_stages.go` / `plan_skills.go`（+测试）、`RunContext.PlanStage` / `ReadOnlySkills`、阶段提示词与 `plan_stage` / `readonly_skills` 环境行、`skilltool.go` 的阶段只读拦截、`agentskills.AllowWriteArgv`、`cloudagent/plan.go`（`PlanStageSpec` / `ValidatePlanStages`，+测试） |

#### 13.15.2 行为变化（均已登记，测试锁定）

- 工具名统一为 `read` / `write` / `edit` / `glob` / `grep` / `bash` / `http` / `web_fetch` / `web_search` / `todo_write` / `plan` / `explore` / `reflect`，不留别名；CLI `http` / `web_fetch` 默认拒绝内网（`http_allow_private` 显式开启）。
- SSE v3：`turn` / `plan` / `plan_confirmation` / `progress` / `task` / `human` / `confirmation_required` / `confirmation_resolved` / `end`；轮次状态 `routing` → `planning`，新增 `awaiting_plan_confirm` / `rejected`；`ProtocolVersion: 3`。
- 动作（取消 / 重试 / 恢复 / 人工回复 / Plan 确认）统一 202 + 快照，推进在后台（`WithoutCancel` + 轮次级 cancel）；SSE 断开不再取消轮次。
- **B14**：固定六阶段计划模式整体下线。群会话每条消息经栅栏产出 `plan.Decision`；固定助手单聊（`/runs`）与群会话旧 `/runs` 的 run 也带 `plan` / `explore` / `reflect`，单聊只做结构校验（`dispatches` 可空），当前计划从会话历史里的 `plan` 工具卡片经 `plan.ReplayCall` 重放（跨 run 可 `update`），决策经 run 的 `Emit` 以 `plan` 帧下发（`{"type":"plan","run_id":…,"agent_id":…,"decision":{…}}`，不带 `confirm_required`）；群任务的规划工具以轮次栅栏的 Decision 打底并按群约束校验，`reflect` 现在也走模型（此前群任务为确定性清单）。`plan` 工具描述改为模式中立（先讲 goal / steps / progress，再讲群分工）。
- 修复：`streamGroupTurn` 的断开监听协程在 handler 返回后读取 echo `Context`（`-race` 报数据竞争，`Context` 会被池化复用给下一个请求）；现改为只捕获请求 ctx 并在返回前等待协程退出（`collab_group_stream.go`）。

#### 13.15.3 缺陷清单处置（§13.2）

| 编号 | 处置 |
| --- | --- |
| CB-BE-01/02/03/04/05/06/07/08/09/11/12 | 已修：`respondTurn` 202 快照；后台执行器；状态机 + `Store.Transition*`；`AppendAgentTurnEventAtomic` 分配 seq；结构帧落 `sys_agent_turn_events` 且回放同形；历史在 `BeforeFence` 落用户消息之前读取；任务失败统一经引擎发 `task` 帧；`humanJSON(g.HumanName…)`；broker 每个订阅只收一次 `end`；非法迁移报错不静默；每个 dispatch 独立 message |
| CB-BE-10 | 已修：错误码 `human_policy_disabled`（`plan.ErrorCode(ErrHumanDisabled)`），前端 `utils/agent-errors.ts` 的 `CODE_KEYS` 与两棵语言树补齐群轮次 / Plan 栅栏错误码（`turn_busy` / `turn_conflict` / `routing_failed` / `invalid_dispatch_graph` / `agent_not_in_tag` / `mention_not_planned` / `human_*` / `task_not_retryable` / `group_*` / `workspace_upgrade_required` / `collaboration_budget_exceeded`）；文案里的 `@` 按 `ui/AGENTS.md` 写成 `{'@'}` |
| CB-BE-13 | 已修（第二轮）：`/runs` 只服务固定助手会话，群会话返回 `400 turns_required`（`ErrTurnsRequired`）；`agent_run_auto.go` / `agent_dispatch.go` 及其测试删除（`finishAgentRun` 迁到 `agent_run_handler.go`，`workspaceCandidates` 迁到 `collab_group_engine.go`）；`dispatcher` / `routeCancels` / `ErrNoAvailableAgent` / `ErrAgentNotInTag` / `ErrRetryAgentUnavailable`、`messageDTO.route_kind`、`workspaceV2Requested`、`sys_agent_run_routes` 表与 `InsertAgentRunRoute` / `ListAgentRunRoutes` / `UpdateAgentRunAgent`、`DispatchAuto` / `TurnStatusRouting` 全部删除；`dispatch_mode` 只接受 `collaborative`，`agentbridge.MessagesFor`（第二份群历史投影）随之删除 |
| CB-FE-01~15 | 核心链路已由 B13 专项测试覆盖（工具确认按任务放行、停止、切会话断流、刷新恢复、重试 / 回复不阻塞、断线续接、seq 去重、Plan 弹窗、草稿回滚）。第二轮补齐：CB-FE-03/04 气泡按 `taskId` 绑定（`ChatMsg.taskId/turnId`、`bubbleOf` 不再按 `agentId` 找历史气泡，任务完成不再另插结果气泡，快照恢复只给仍在执行的任务补气泡）、CB-FE-10 Plan 状态条（`planDecides` / `planMentioned` / `planDeciding`）、CB-FE-13 `pickPrompt` 改受控 `ConfirmAction`、`AgentDispatchCard.vue` → `AgentPlanCard.vue`、CB-FE-11 运行中指示只由活动 run 派生；气泡 key 改用消息 id / taskId。暗色 / 窄屏未人工核对 |

#### 13.15.4 B14 设计说明

- **为什么删而不是改**：前端自 B13 起不再调用 `/agent-threads/:id/plans*`（首页「一句话建应用」在 `#通用` 建协作会话并发起轮次，经栅栏产出 Plan），六阶段协调器、阶段表与阶段技能白名单在生产上已不可达；按「激进，不留兼容」整体删除，`schema.go` 直接改（`internal/AGENTS.md`：无版本化迁移，reset 重建）。已有库里残留的 `sys_agent_plans*` 两张空表无害，`schema_status` 只报缺失表。
- **规划能力归位**：`gocloudagent/plan` 是唯一实现；宿主只做装配（`agent_plan_tools.go`）与出口（`plan` 帧 / 轮次事件）。单聊的「当前计划」不新表、不新列：assistant 消息的 `tool_calls_json` 已持久化每次 `plan` 调用的参数，`plan.ReplayCall` 按 create / replan 替换、update / complete 合并的规则重放，与工具实现共用 `mergeUpdate` / `validateSteps`，不会出现两套合并语义。
- **不做的部分**：单聊没有「整份计划确认」流程（confirm 由群轮次的栅栏负责，`plan` 帧不带 `confirm_required`）；单聊的 Plan 卡片 UI 不在本次范围（`ui/AGENTS.md`：`/runs` 固定助手会话保持现有气泡样式；`agent-stream.ts` 的 `onPlan` 处理已兼容 run 流，接入时只需在 `useAgentConversation` 传 `onPlan`）。

#### 13.15.5 验证

| 检查 | 结果 |
| --- | --- |
| `(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./... -race -count=1)`（含 2 万文件遍历与大输出用例） | 通过 |
| `go build ./internal/... ./cmd/...`、`go vet ./internal/agentbridge/... ./internal/api/ ./internal/systemdb/ ./internal/app/` | 通过 |
| `go test ./internal/agentbridge/... ./internal/systemdb/ ./internal/auth/ -count=1` | 通过 |
| `go test ./internal/api/ -count=1`（全量，单独两次 + 根模块全量一次） | 通过（154.9s / 157.5s / 160.2s）；新增 `agent_plan_tools_test.go`（5 例）、`agent_plan_tools_e2e_test.go`（原生 tool_calls 全链路：请求体工具集、`plan` 帧、卡片落库、第二条 run `update` 重放）、`agent_routes_test.go`（清单去掉 `/plans*`，补 `plan-confirmations`，新增 `TestB3AgentPlanModeRoutesRemoved`）；`-race -count=25` 跑 `TestCollab*` / `TestTurnEventsReplay` / `TestGroupCron*` 通过 |
| `cd ui && yarn typecheck` / `yarn test` / `yarn build` | 全部通过：162 个文件 / 1527 例，coverage statements 97.63% / branches 93.36% / functions 91.7%（阈值 90%），无未处理错误；`tsc --noEmit` 0 错误；`vite build` 成功 |
| **B13 遗留的前端测试漂移（本轮修复）** | 全量 `yarn test` 原有 33 例失败（`AgentManager.test.ts` 30 / `loading-surfaces.test.ts` 2 / `http-api.test.ts` 1）且进程因未处理错误退出 1。根因：`AgentManager.vue` 的 `bootstrap()` 对 `api.agentThreads.capabilities()` 同步调 `.then`，测试 mock 无默认值 → `TypeError` 中断整条加载链（`pageBy` 从未被调用、页面落到「当前标签没有可用助手」）；`useThreadNavigation.resumeActiveRun` 对 `api.agentThreads.turns()` 同样无默认值 → 未处理拒绝；`toAgentTag` 现默认 `confirmPolicy: 'auto'`，映射回退用例的期望未更新。修复：`src/test/api-mock.ts` 的 `applyApiDefaults()` 补 `capabilities`（v3 形状）与 `turns`（`[]`）默认值，`http-api.test.ts` 期望补 `confirmPolicy: 'auto'` |
| `gofmt -l internal/ gocloudagent/ cmd/` | 无输出 |
| 依赖门禁（`go list -deps` / `go list -f '{{.Imports}}'`） | `gocloudagent` 各包不含根模块 `internal`；`plan` / `group` 只依赖 `agentcore`（+ `plan`）；`cloudagent` 与 `cli` 互不 import；`internal/api → group / plan / cloudagent / agentcore / mcp`，`agentbridge → cloudagent / agentcore / mcp / tools` |
| **既有失败（与本次无关）** | `internal/app` 的 `TestNewLocalStorageAppUsesSystemDatabase`（`catalog must not live in cache_dir`）：把被删的 `plans.go`、两张表与 `InterruptRunningPlans` 临时还原后同样失败，属工作树内 `godatabase/ducklake` 目录放置改动，未处理 |
| **已知抖动（测试环境）** | 协作 e2e 用 sqlite `cache=shared` 内存库；在 `-cpu 1` 且并行跑全量时会出现 `database table is locked`，引擎据此把轮次标成 `turn_store_failed` / `run_failed`（`TestCollabHumanTaskReplyContinuesTurn`、`TestCollabTurnResumeAfterInterrupt`）。正常全量运行两次均通过；`waitCollabTurnStatus` 的失败信息已带 `error_code` 便于定位 |
| `go test ./... -count=1`（根模块全量） | 31 个包 ok，唯一失败是上行的既有 `internal/app` 用例（其余含 `internal/api` 全部通过） |
| **未运行** |Linux / Windows / Go 1.25 最低版本；真实 LLM / MCP / S3 调用；`init_db` / DDL（始终未执行） |

#### 13.15.6 遗留（按优先级）

1. 暗色模式 / 窄屏（<sm）下的群聊视觉（Plan 卡、Plan 状态条、头像叠放、气泡）只做了单测断言 class，未做人工截图核对。
2. `internal/app` 的 `TestNewLocalStorageAppUsesSystemDatabase`（`catalog must not live in cache_dir`）是与本计划无关的既有失败（见 §13.15.5），未处理。
3. 产品另议项（不在本计划范围）：并发执行无依赖任务（引擎 `Policy.Concurrency` 预留，默认 1）；`facts.md` 的云端持久化；单助手固定会话的 Plan 卡片 UI（`plan` 帧已下发，`agent-stream.ts` 已能解析，接入时只需在 `useAgentConversation` 传 `onPlan`）。

#### 13.15.7 第二轮记录（2026-11-25）：CB-BE-13、CLI 群模式、前端 Plan 卡与气泡绑定

**CB-BE-13 收口（后端 + 前端一起删）**

- 后端删除：`internal/api/agent_run_auto.go`（`createAutoRun` / `decideRoute` / `runAutoRoute` / `streamAutoRun` / `completeAutoRun` / `buildAutoRunRequest` / `routeDecision`）、`agent_dispatch.go`（`agentDispatcher` / `llmDispatcher` / `buildDispatchPrompt` / `parseDispatchAnswer` / `fallbackAgent`；`workspaceCandidates` 迁到 `collab_group_engine.go`）及两份测试；`cloudAgentHandler.dispatcher` / `routeCancels`、`CancelRun` 的路由期取消；`ErrNoAvailableAgent` / `ErrAgentNotInTag` / `ErrRetryAgentUnavailable`（`agent_not_in_tag` 仍由 `plan.ErrMemberMissing` 产生）；`messageDTO.route_kind` 与 `ListMessages` 的路由补全；`completeRun` 响应的 `route_kind`；`workspaceV2Requested` 与 `toThreadDTOResolved` 的 v1/v2 折叠；`systemdb` 的 `sys_agent_run_routes` 表、`AgentRunRoute`、`Route*` 常量、`InsertAgentRunRoute` / `ListAgentRunRoutes` / `UpdateAgentRunAgent`、`DispatchAuto`、`TurnStatusRouting`（`CreateAgentTurn` 默认 `planning`）；`agentbridge.MessagesFor` / `toolNames`（唯一消费者是被删的 `buildAutoRunRequest`，群历史投影只剩 `collabGroupRunner.history` 一份）。
- 新行为：`CreateRun` 对 `dispatch_mode=collaborative` 返回 `400 turns_required`（`agent_run_guard_test.go` 同时断言不落 run、不落用户消息）；建会话的 `dispatch_mode` 只接受 `collaborative`（`auto` 400）；`finishAgentRun` 迁到 `agent_run_handler.go`（审计不再记 `route=`）。
- 前端删除：`AgentRouteKind` / `AgentRouteEvent` / `AgentDispatchStep` / `ChatMsg.routing` / `ChatMsg.routeKind` / `AgentMessage.route_kind`、`agent-stream.ts` 的 `route` 帧解析与 `onRun` 的 `routeKind`、`useAgentConversation` 的 routing 阶段与 `onRoute`、`useAgentCollaboration.onRoute`、`useAgentRunLayer` 按 `m.routing` 推导运行中、`ConversationView` 的「正在选择助手」与路由原因徽标、`AgentManager` 的 `routeAgentName` / `routeHint`、`useAgentSender` / `useThreadNavigation` 的 `routeAgentName`、`isDispatchMode` 对 `auto` 的折叠、i18n `agentPhase.routing` / `conversation.routing|routeMention|routeFallback|routeRetry` / `agentWorkspace.autoRoute|mentionThisTurn`；测试同步（`http-api` 断言旧 `route` 帧被忽略、`auto` 读成未知模式）。

**CB-FE-03/04/10/11/13 与视觉项**

- 气泡绑定：`ChatMsg` 增 `id` / `taskId` / `turnId`；`bubbleOf` 按 `taskId` 找（不再按 `agentId` 命中历史气泡），`ensureBubble` 带上已到达的正文 / 工具卡，任务帧非 pending 即确保气泡；`flushTaskToMessage` 删除（完成不再另插结果气泡）；`syncTasks` 只为仍在执行的任务补气泡（已完成任务以服务端消息为准，避免重复）；`ConversationView` 的 key 改用消息 id / taskId；`useAgentCollaboration` 暴露 `planning`。
- `AgentPlanCard.vue` 取代 `AgentDispatchCard.vue`：目标 + `complexity=complex` 徽标 + 进度条（`role="progressbar"`）+ 摘要（Plan 的 progress.note 或任务计数）、complex 计划的步骤清单（状态优先取 Plan，缺省按关联任务归约）、分工列表（依赖箭头 / risk / 状态 / 失败重试，`data-dispatch-*` 钩子不变）、发现折叠；`bg-muted/40 border-dashed` 与气泡区分。
- Plan 状态条：`AgentComposer` 的 `routeHint` → `planHint` + `planning`（「分工由 Plan 决定；@ 助手会作为硬约束」/「本轮 @A、@B（Plan 会为每人生成独立指令）」/「Plan 决策中…」脉冲点 `motion-reduce:animate-none`）。
- 气泡上方的「来自 Plan」引用：`ConversationView.planInstructions`（taskId → 指令，`title` 悬停全文）。
- 卡片头与左栏群行：成员头像叠放（最多 3 个，模块图标）+ 「N 位成员」+ 进度摘要；会话行增加最近一条摘要（单行截断）。
- CB-FE-13：`pickPrompt` 改为受控 `ConfirmAction`（空草稿直接填入；已有草稿弹确认，确认才替换；`window.confirm` 不再出现，测试用 spy 断言）。
- CB-FE-11：`runningAgentIds` 只由活动 run 派生。

**CLI 群模式（原 B14）**

- 文件：`cli/group_config.go`（`GroupConfig` / `GroupMember`、`LoadGroup` / `SaveGroup` / `ScaffoldGroup` / `ListGroups`，id 规则 `^[a-z0-9][a-z0-9_-]{0,31}$`，文件 0600）、`cli/group_store.go`（`fileGroupStore` 实现 `group.Store`：`groups/<name>/state.json` 整份原子写，事件每轮最多 1000 条，`TransitionTurn/Task` 以 from 为前置条件返回 `ErrIllegalTransition`，占用位不跨进程；另有 `ListResumableTurns` 供 `/group resume`）、`cli/group_runtime.go`（`groupRuntime`：`newGroupRuntime` 装配 `group.Engine` 并 `Converge`；`groupMembers` 实现 `group.Members`（`HumanID="me"`，`human_tasks=false` 时禁用）；`groupRunner` 实现 `group.Runner`：成员工具子集 + 绑定群约束的 `plan.Tools`（`plan.FileStore` 在 `groups/<name>/plan/`），每个任务一个 `SessionRunner` 会话（`Entry.Group/Task` 元数据），系统提示词 = 成员指令 + 本轮目标 + Plan 下发的指令 + `context_from` 上游结果 + 近期群聊历史，成员调用 `plan(action=replan)` 时上报 `NeedReplan`；终端输出经 `lockedWriter` 串行化）、`cli/group_cmd.go`（`group list/show/new/run`）、`repl.go`（`/group [name|status|resume|off]`、`/reply`、`/unable`；`buildRunner` 拆出 `buildLocalTools` 供群模式复用；`runREPL` 持有当前 `groupRuntime`）。
- 交互：整份计划需确认时 `[a]pprove/[e]dit/[r]eject`（`e`：改某 ref 的指令、移除某 ref，编辑后由引擎重新 `Validate`）；人工待办在轮次停下时就地回复，或 `/later` 保留后 `/reply <说明>` / `/unable [说明]`；stdin 结束时一律按拒绝 / 保留处理，不空转。headless（`group run`）：`--yes` 批准，否则拒绝并提示到 REPL 处理；人工任务只提示。计划决策同时写入 `plan.FileStore`（`plan.md` / `decision.json`）。跟随循环对「running 却 15 秒无事件」的轮次停止跟随并提示 `/group resume`。
- 测试（`cli/group_test.go`，`-race` 通过）：群定义脚手架 / 校验 / 列表；文件存储的幂等、CAS、seq、占用位、重开恢复；单成员规则短路（不调 Planner，成员会话带元数据，计划文件落盘，历史进入下一轮）；双成员经 Planner 分工并按依赖执行（指令与上游结果进入成员提示词）；`confirm_policy=always` 的批准 / 拒绝 / 编辑后批准；人工任务的就地回复 / 稍后 `/reply` / `/unable`；headless 的拒绝 / `--yes` / 人工任务提示与命令面退出码；重启收敛与 `/group resume`；工具子集过滤；REPL 集成（`/group` → 消息 → `status` → `off` → 单助手）。

**验证（第二轮）**

| 检查 | 结果 |
| --- | --- |
| `(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./... -race -count=1)` | 通过（含 `cli` 群模式用例） |
| `go build ./internal/... ./cmd/...`、`go vet`（agentbridge / systemdb / api） | 通过 |
| `go test ./internal/api/ -count=1`（全量） | 通过（137.8s）；新增 `agent_run_guard_test.go`，`agent_tag_handler_test.go` 断言 `auto` 被拒，`agent_routes_test.go` 路由清单不变（`/runs` 保留给固定助手会话） |
| `cd ui && yarn typecheck` / `yarn test` / `yarn build` | 通过：162 文件 / 1530 例，statements 97.64% / branches 93.47% / functions 91.65% |
