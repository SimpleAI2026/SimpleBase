# gocloudagent 独立模块拆分计划（planv6.2）

> 本文只规划迁移，不代表代码已实施。目标是把 `cmd/gocloudagent` 和 `internal/gocloudagent` 迁入仓库根目录的嵌套 Go 模块 `gocloudagent/`，并由根模块的 `internal/` 引入它；以 `gosandbox/` 的 `require + replace` 模式接入。迁移不得改变 HTTP 协议、CLI 行为、权限和沙盒安全边界。

## 1. 目标与边界

1. 建立 `gocloudagent/go.mod`、`gocloudagent/go.sum`，模块路径为 `github.com/linkxzhou/SimpleBase/gocloudagent`；根 `go.mod` 增加 `require .../gocloudagent v0.0.0` 与 `replace .../gocloudagent => ./gocloudagent`。
2. CLI 入口改为 `gocloudagent/cmd/gocloudagent`；运行时、`agentcore`、`cli`、`tools` 改为模块内包，由 `internal/app`、`internal/api`、`internal/agentjob` 从新模块引入。
3. **真正独立**：新模块的生产代码及测试都不 import `github.com/linkxzhou/SimpleBase/internal/...` 或根模块 `skills`；仅依赖标准库、已有第三方库和独立模块 `gosandbox`。不得通过新模块 `go.mod` 反向 `require/replace` 根模块来绕过这一约束。迁出前所有宿主类型与服务调用必须在根模块侧适配。
4. 根模块负责认证、系统库、SQL 边界、资源 CLI、模型服务、日志构造以及应用装配；新模块负责 Agent 运行、提示词、工具策略、CLI 交互与本机工具。`gosandbox/host` 仍只用于本机 shell，不作为 Cloud 沙盒失败的回退。
5. 不引入第三方依赖、不升级现有版本、不改表结构、不执行数据库迁移；需要调整的 `go.mod`/`go.sum` 只为完成用户要求的模块拆分与既有依赖归属调整。

## 2. 现状与阻塞项

- 根 `go.mod` 已以 `require + replace` 使用 `gofunction` 和 `gosandbox`；目前云助手代码在 `internal/gocloudagent/{agentcore,cli,tools}`，CLI 入口为 `cmd/gocloudagent/main.go`。根模块 `internal/api`、`internal/app`、`internal/agentjob` 已引用该包；API 测试还引用 `agentcore`、`cli`。
- `internal/gocloudagent/runtime.go`、`access.go` 直接以 `auth.Principal`、`systemdb.CloudAgent`、`systemdb.AgentMessage`、`systemdb.LogEvent`/`LogLevelCount`/`LLMSettings`、`observability.Logger` 作为运行时字段或接口类型；`store_access.go` 构造 `systemdb.Store` 适配器。它们不是简单替换 import 路径即可解决的依赖。
- `errors.go` 依赖 `llmgateway` 错误哨兵；`cli_tool.go` 依赖 `auth`、`sbcli.CommandReadOnlyWithArgv` 和 `database/sqlguard`；`cli_runner.go` 的 `InProcessCLI` 直接调用 `sbcli.RunIO`。
- `skills.go` 引用根模块 `skills.Simplebase`；技能文档目前由 `skills/embed.go` 的 `//go:embed simplebase` 提供。`cli`、`tools` 引用的 `gosandbox/host` 则已是独立模块依赖。
- `build.sh` 从根目录构建 `./cmd/gocloudagent`；`Dockerfile` 在 `go mod download` 前复制现有嵌套模块的 `go.mod/go.sum`；`scripts/test.sh`、`scripts/smoke.sh` 显式引用 `./internal/gocloudagent`。嵌套模块不会被根目录的 `go test ./...` 覆盖。

**关键约束**：Go 的包级无环不等于模块独立。即使在本地 `replace` 下把根模块作为新模块的依赖暂时编译成功，也不符合“独立模块”的目标；验收以 gocloudagent 的依赖图中没有根模块为准。

## 3. 目标结构与依赖方向

```text
gocloudagent/                         # 独立模块：github.com/linkxzhou/SimpleBase/gocloudagent
  go.mod  go.sum
  cmd/gocloudagent/main.go            # 只 import gocloudagent 根包门面
  runtime.go  access.go  errors.go  ...
  agentcore/  cli/  tools/
  skills/                            # 原 skills/embed.go 与 simplebase/ 文档整体迁入

internal/                           # 根模块：SimpleBase 宿主与适配器
  app/                               # 装配 Runtime、store、日志、CLI、委托与沙盒
  api/                               # HTTP、LLM、数据库/对象访问及类型映射
  agentjob/                          # 定时任务调用方与请求 DTO 映射
  ...                                # auth/systemdb/sqlguard/sbcli/llmgateway 原职责不变
```

依赖方向：`internal/{app,api,agentjob} → gocloudagent → {gosandbox/host, eino, ...}`。`agentcore` 不反向 import gocloudagent 根包；`cli`、`tools` 继续复用 `agentcore`，不新增平级独立模块。CLI 仍通过根包 `RunCLI`、`SetCLIVersion` 访问子包，不通过旧 `internal/gocloudagent` 转发。

### 3.1 模块关系与构建边界

- 根 `go.mod` 增 `require github.com/linkxzhou/SimpleBase/gocloudagent v0.0.0` 和 `replace github.com/linkxzhou/SimpleBase/gocloudagent => ./gocloudagent`，保留现有 `gosandbox` 的 require/replace。
- `gocloudagent/go.mod` 使用当前 `go 1.25.0`，并声明 `require github.com/linkxzhou/SimpleBase/gosandbox v0.0.0`、`replace github.com/linkxzhou/SimpleBase/gosandbox => ../gosandbox`。
- **replace 不传递**：根模块不会继承依赖模块的 replace，新模块独立构建也不会继承根模块的 replace。两边都必须声明自身构建所需的本地替换；验收使用 `GOWORK=off`，不通过 `go.work` 掩盖缺项。
- 本次“独立”指有独立依赖清单、可在仓库内单独 build/test，且不依赖根模块；仍需要兄弟目录 `gosandbox/`。暂不承诺单独复制 `gocloudagent/`、`go install ...@version` 或发布到外部仓库即可使用；这些需要后续确定可解析的 gosandbox 发布版本。
- 依赖沿用现有选定版本（包括 `eino v0.9.19`、`litellm v1.5.8`、`uuid v1.6.0`、`zap v1.27.0`），按实际 import 生成完整清单。根模块仍消费的依赖不得机械删除；两个模块 tidy 后都检查版本差异。

### 3.2 源码目录与产物冲突（迁移前处理）

`build.sh` 当前输出 `$ROOT_DIR/gocloudagent` 或 `$(dirname "$OUTPUT_BIN")/gocloudagent`，与新源码目录冲突。**不再要求保持原产物路径**，只保持二进制名称与命令行为：

1. gocloudagent CLI 统一输出到仓库 `build/bin/gocloudagent`；Linux helper 输出到 `build/bin/gosandbox-inner`。`simplebase` 的 `OUTPUT_BIN` 语义保持原样，CLI 不再随其父目录变化。
2. 若根 `gocloudagent` 已是普通文件，先核对是否为旧产物并经用户确认迁到新的产物目录；不是明确产物时停止，禁止直接删除、覆盖或清空同名路径。脚本必须拒绝二进制输出路径为目录，不能利用 `go build -o <目录>` 隐式把产物放进源码树。
3. 删除 `.gitignore` 的 `/gocloudagent` 规则，保留现有 `/build/`；修正“根路径锚定只匹配二进制”的注释：`/gocloudagent` 同样匹配根目录下的源码文件夹。
4. 更新 `examples/run_agent.sh` 的 CLI 默认查找路径，判定同时检查普通文件和可执行权限（只检查 `-x` 也可能命中目录）；枚举其他仍使用根二进制路径的活跃脚本/文档。Linux helper 仍与 CLI 同目录，默认定位行为不变。
5. `cmd/gocloudagent/main.go` 的版本变量仍是 `main.version`，不会仅因移动包路径而改变。核对真实发布命令再调整 `-ldflags -X`，不要误改该符号；CLI 配置来源、`~/.gocloudagent/` 会话格式/位置均不变。

### 3.3 技能资源归属

仅迁 `skills/embed.go` 和 `skills/simplebase/` 至 `gocloudagent/skills/`；**根 `skills/` 内的其他技能、评测和用户文件不动**。新 embed 仍使用相对路径 `simplebase`，`SkillPrompt` 正文及索引路径不因迁移改变，不新增运行时文件读取或网络下载。

唯一 Go importer 不等于唯一消费者：同步检查 `simplebase/references/authoring.md` 等维护文档的路径，以及前端 `ui/src/utils/agent-skills.ts`、示例 `examples/capabilities/agent/run.mjs` 的技能名称/别名一致性。技能内容不变则不改前端业务代码；历史 plan 中的旧路径作为历史事实保留，不进行全仓盲替换。

## 4. 拆耦方案

| 当前跨界依赖 | 处理方式与所有权 | 必须保证 |
|---|---|---|
| `auth.Principal` / `auth.Permission` | 采用模块自有的**无损、无密钥身份 DTO**，不采用仅保留模块直接消费字段的最小 DTO；由宿主集中双向映射，详见 §4.1 | 保留角色、租户、项目与权限语义；只读降权必须复制权限集合，不能污染调用方；鉴权和签发仍归宿主 |
| `systemdb.CloudAgent` / `AgentMessage` | 在 `gocloudagent` 定义运行所需的最小 `Agent` / `Message` DTO，宿主在读库之后转换；历史消息字段及工具摘要语义不变 | 不把 `systemdb.Store`、SQL 行或存储层对象暴露给独立模块；确认 Agent 字段完整映射，避免 writable/tool_ids 丢失 |
| `systemdb.LogEvent`、`LogLevelCount`、`LLMSettings` | 云助手侧定义只读结果 DTO 与 `LogAccess`、`SettingsAccess` 接口；原 `store_access.go` 适配实现移至根模块装配/适配层 | 保持 admin 项目日志查询和默认模型的现有语义；访问仍只读、无密钥 |
| `observability.Logger` | 定义 gocloudagent 所需的最小日志接口（与 `zap.Field` 方法签名兼容时由现有 logger 实现），日志实例仅由宿主创建并注入 | `trace_log` 默认关闭；关闭时只记摘要，开启时仍须先脱敏再截断，永不写系统库 logs |
| `llmgateway` 错误哨兵 | gocloudagent 只保留本地运行错误、`context`/网络/供应商安全分类；宿主对 `llmgateway` 哨兵做额外分类，集中在根模块的调用/HTTP 错误映射侧。核对 `ClassifyError` 当前所有调用并统一走宿主分类入口 | 保持现有 `llm_model_not_allowed`、`llm_not_configured`、限流/鉴权/超时映射及脱敏消息；不可直接透传上游原始错误 |
| `sbcli.CommandReadOnlyWithArgv` | 通过单一 `CommandPolicy` 接口返回命令只读/破坏性分类，由宿主调用 `sbcli`；不在模块内维护第二份只读允许表 | 策略未注入或分类失败时无条件拒绝执行，不允许 writable、确认或 headless 绕过；详见 §4.2 |
| `database/sqlguard` | 同一个宿主 `CommandPolicy` 负责现有 `destructiveArgv/sqlDestructive` 的分类，继续使用 `sqlguard`；模块保留工具编排，不接收分散的 SQL 解析回调 | 首关键字提取等剩余 `sqlguard` 使用也经宿主适配；展示参数的截断/脱敏不得重新引入 SQL 解析器；执行侧 SQL 边界仍在 `database/sqlguard` |
| `sbcli.RunIO` | `InProcessCLI` 迁到根模块测试/装配侧，实现 gocloudagent 的 `CLIRunner`；独立模块仅保留 `SubprocessCLI` 和 `CLIRunner` 接口 | 不让 gocloudagent 链接 SimpleBase 宿主 CLI 或 DuckDB；保留原本 in-process 测试的 HTTP 行为验证 |
| 根模块 `skills` | 迁移 `skills/embed.go` 与 `skills/simplebase/` 至 `gocloudagent/skills/`，调整嵌入路径；模块内技能只读逻辑仍引用这份 embed | `SkillPrompt` 内容、`allow` 与 `readonly` 标注不变；根模块的 `sbcli` 对账测试覆盖文档与命令表一致性 |

### 4.1 身份映射与宿主桥接

新增小范围的 `internal/agentbridge` 包，集中 `Principal`/`Agent`/`Message` 转换、宿主命令策略、日志/设置 store 适配、委托适配、最终错误分类。它可 import gocloudagent、`auth`、`systemdb`、`sbcli`、`sqlguard`、`llmgateway`，**不 import `api`、`app`、`agentjob`**；`app` 是唯一服务装配点，`api` 与 `agentjob` 复用转换/分类，禁止定时任务为了复用代码反向 import `api`。原数据库/对象/LLM 适配器保留在 `internal/api/gocloudagent_access.go`，不顺带重构整套 API 分层。

- 身份 DTO 明确保留 `APIKeyID`、`UserID`、`Username`、`Role`、`SessionID`、`AccessJTI`、`TenantID`、`ProjectIDs`、`Permissions`、`Delegated`。`Role` 用字符串传递但不在模块内实现角色授权；权限为模块自有字符串类型及集合，不 alias `auth` 类型。
- 双向转换对 map/slice 深拷贝，保持 nil 与空集合语义；模块的只读降权只生成副本。DTO 不是 HTTP 请求结构，不接受模型或用户自行提交的角色/权限，不记录完整身份 DTO。
- 数据库适配器转换回完整 `auth.Principal` 再调用 catalog，保留角色、租户、项目授权语义；不以“目标项目 ID 已提供”为理由省略原校验。
- `IssueDelegation` 当前遍历传入权限写 claims，**不会自行与原主体求交集**。因此受信任的请求构造必须从已认证主体复制权限，模块只允许移除权限，适配器不得用缓存的原始全权限主体替换降权结果。测试需实际解析委托 claims 并断言权限子集和 project/run 固定，而非只验证传参。
- `UserID` / `apikey:<APIKeyID>` 确认主体优先级保持不变；`SessionID/AccessJTI/Delegated` 保留到宿主，不声称它们都是委托 JWT 必填。`RevokeRun`、thread 锁、cancel、token 清理与确认预登记顺序不变，不引入全局“当前用户”。
- `Agent` 初始字段至少为当前执行消费的 `ID/Name/Description/Module/SystemPrompt/ModelOverride/ToolIDs/Writable`；`Message` 包括 `Role/Content/ToolCallsJSON`。M0 与真实字段使用对照后补全；settings 仅返回所需默认模型，logs DTO 保持当前工具输出 JSON 的字段名和时间格式，不能因丢失 tags 改变结果。

### 4.2 命令策略端口与拒绝语义

采用一个 `CommandPolicy`，拟定契约为 `Classify(argv []string, stdin string) (CommandDecision, error)`，`CommandDecision` 包含 `ReadOnly`、`Destructive`；另提供 `RedactArgv(argv []string) []string` 保持现有确认展示（承接 `redactStatement` 的首关键字依赖）。实现位于 `agentbridge`，策略只分类/脱敏，禁止执行命令、读取宿主文件或访问数据库。

- `ReadOnly` 委托 `sbcli.CommandReadOnlyWithArgv`；破坏性分类从原 `destructiveArgv/sqlDestructive` 搬迁，SQL 仍调用 `sqlguard`。不把所有非只读命令都改成破坏性，不改变 INSERT/UPDATE 与 DROP/DELETE 等既有分类差异。
- 工具按原顺序执行参数/文件路径校验、skill 允许表校验，然后分类、不可写判定、确认、执行；`AllowArgv` 是技能授权表，不替代宿主命令只读表。
- 缺策略或分类报错：返回稳定安全错误（拟定 `command_policy_unavailable`），不调用 `Confirm`、不发 `tool_start`、不调用 `CLIRunner`。这是新端口的装配错误处理，**不提供“确认后仍执行”的兜底**；本地 CLI 的文件/shell 工具不依赖该 SimpleBase 命令策略。
- 不可写且非只读：维持 `write_not_allowed`，在请求确认前拒绝。破坏性且 headless：维持 `confirmation_unavailable`；交互模式仍须确认通过，确认不能提升权限。策略错误正文不能直接写日志或返回模型。

### 4.3 LLM 错误：保留内部语义，只在最终出口分类

`agentcore.Model.fallback` 依赖供应商类型、400/422 和原错误的 tool/function 关键词；Stream 是否再调用 Chat 也依赖错误类型及 context 哨兵。**不得在 `Chat`、`Stream`、`Next`、`NextDelta` 适配器中提前把原错误压成 `RunError`**；仅增加 `Unwrap` 也不足以保持关键词行为。

共享宿主出口 `agentbridge.ClassifyError` 先匹配 `llmgateway` 哨兵，再委托 gocloudagent 的安全分类；API HTTP/SSE、`run_events`、`agentjob` 的最终失败落库统一使用它。API 的其他领域错误映射不受影响。原始错误只保留在运行内部，不进入响应、事件、任务记录或新增日志。

已发现 `agentcore/model.go` 的回退 warn 含 `err.Error()`：这是现有日志风险，不可把迁移验收描述为其已修复。M0 单列基线；若同批处理，以独立的脱敏修改和敏感值回归测试验收，不改变用于内部回退判断的原始错误。

### 4.4 测试拆分与 nil 行为

- gocloudagent 模块中保留 Runtime、gate、prompt、skills 解析、agentcore、CLI、tools 与 subprocess 单元测试；使用模块自有 DTO、fake 策略、fake 日志。不在测试中反向引入根模块。
- 与真实 `sbcli/sqlguard` 的策略和技能对账、真实 `llmgateway` 哨兵分类、store 与 auth 适配测试放 `internal/agentbridge`；API 全链路测试保留 `internal/api`；定时任务测试保留 `internal/agentjob`。
- `InProcessCLI` 移到可共享的根模块测试辅助包（沿用 `internal/testutil`），供 API 多个测试包使用；保留输出截断和 HTTP 调用语义。不仅改测试 import：原 `skills_test.go` 访问的 `skillPrefixes/readSkill/skillDocPath` 是私有符号，宿主对账应通过已有 `EmbeddedSkillAllows`、`EmbeddedSkillReadOnly`、`SkillPrefixes` 及迁移后的 embed FS 进行，不为迁测试公开所有内部函数。
- 日志接口确定为 `Info/Warn/Debug(msg string, fields ...zap.Field)`，保持 zap 版本；装配侧 nil 指针不得装入非 nil interface，新增 nil/no-op 回归。日志、settings、logs 为 nil 的行为保持现状，只有必需的命令安全策略缺失才按 §4.2 拒绝。
- 迁移已有同包私有测试时逐项决定保留、拆分或移到宿主；不得删除断言来换取独立模块编译通过。

## 5. 实施顺序

| 阶段 | 工作 | 阶段验收 |
|---|---|---|
| M0 基线与目录准备 | 记录 Git 状态和定向测试基线；清点所有活跃 import/脚本/资源引用；按 §3.2 先调整产物输出与 ignore，并经确认处理同名旧二进制；旧入口此时仍可构建 | 无源码/产物同名冲突；新模块不会被 ignore；基线失败单独记录，不覆盖用户文件 |
| M1 原位去耦 | 在旧包原位引入 DTO/端口，建立 `internal/agentbridge` 和测试辅助；完成调用方身份映射、命令策略注入与最终错误分类；迁移跨层测试 | 根模块定向测试通过；除待搬迁技能 embed 外，旧 gocloudagent 生产代码/测试无根模块依赖；不引入 bridge → api/app/agentjob 或 gocloudagent → bridge 反向依赖 |
| M2 原子迁移与构建接线 | 同一变更迁移模块、CLI 入口、指定技能资源，配置两侧 require/replace，改所有活跃 import；同步改 `build.sh`、Docker manifest COPY、测试脚本的新入口，不留一轮构建断档 | 根模块与 gocloudagent 在 `GOWORK=off` 下 build/test；独立模块依赖图无根模块；脚本不会引用已删入口 |
| M3 工程集成与文档 | 完善 `scripts/test.sh` 的包数组和参数传递、`scripts/smoke.sh` 的模块覆盖；更新示例、维护文档及根/后端 `AGENTS.md` 的规则；核对容器上下文与 CLI/helper 打包 | 定向脚本、CLI help/version、依赖边界检查通过；`./build.sh --skip-ui` 满足现有 UI dist 前提时通过；容器检查区分本次故障与已有 CGO 问题 |
| M4 完整回归 | 执行 §6 测试矩阵和 race/跨平台适用项，确认没有删除原断言或遗留转发层 | 原 HTTP、工具结果 JSON、SSE 顺序、任务权限、CLI 会话与隔离行为保持；测试通过与环境跳过分别记录 |

每阶段保持可编译，不保留旧 `internal/gocloudagent` 兼容别名/转发层，不保留根 `cmd/gocloudagent` 双入口。回退应撤销相应迁移变更，不通过复制两份实现、硬重置工作区或删除用户文件实现。迁移不触及数据库 schema；不运行 `init_db`、DDL 或启动连接用户运行库的服务来验证。

工程集成额外检查：

- `scripts/test.sh` 当前默认包列表在单个数组元素内；改为真正的多元素数组，保留显式包参数行为，新增模块调用正确传递 `-race` 和 timeout，不能因 M2 删除旧包而静默跳过测试。
- `Dockerfile` 在 `go mod download` 前复制 gocloudagent、gosandbox、gofunction 三组 manifest，随后 COPY 源码；镜像仍不包含本地 CLI/helper。当前未发现根 `.dockerignore`，不把它写成已存在；如后续补充规则，不得屏蔽新模块和服务 embed 内容，也不因此迁移或删除 `.codebuddy`。
- 当前 Dockerfile 使用 `CGO_ENABLED=0`，与根服务 DuckDB 的 CGO 要求存在基线冲突；先记录实际构建结果，不能将根服务改成关闭 CGO 以宣称迁移通过。若需修 Docker 运行镜像/动态库，作为单独批准的构建修复处理。
- 更新 `docs/workbench/agent-e2e.md` 等活跃运行文档；历史计划中的旧路径不纳入“零旧引用”门禁。不顺带升级前端依赖、改变前端样式或重写历史计划。

## 6. 验收命令与不变量

以下为迁移完成后的验收，不是本次改计划已执行的命令。在仓库根目录、测试使用 fake/隔离临时数据且不存在指向用户运行库的配置时执行：

```bash
export GOWORK=off
CGO_ENABLED=1 go build ./internal/... ./cmd/...
CGO_ENABLED=1 go test ./...
go vet ./internal/agentbridge ./internal/api ./internal/agentjob
(cd gocloudagent && go build ./... && go vet ./... && go test ./...)
(cd gosandbox && go build ./... && go vet ./... && go test ./...)
(cd gofunction && go test ./... -short)
./scripts/test.sh -race
(cd gocloudagent && go test -race ./...)
./scripts/smoke.sh
```

根目录 `./...` 不包含三个嵌套模块；根模块需要 CGO，不把根服务的要求误套成“gocloudagent 一定必须/一定不必 CGO”。Linux 真隔离、macOS Seatbelt 分平台验证；缺 bwrap/helper/对应平台的测试明确记为跳过，不等于通过。

### 6.1 独立性与工程验收

1. 在 `gocloudagent/` 用 `GOWORK=off go list -m -json all` 检查没有模块路径**恰等于** `github.com/linkxzhou/SimpleBase` 的条目；不能按前缀误伤合法的 gocloudagent/gosandbox。
2. 用 `go list -deps -test -json ./...` 检查生产与测试的包依赖，无根模块包、无 DuckDB 链接路径；配合静态 import 扫描覆盖当前平台未启用的 Go 文件。检查命令必须保留退出码，不能只根据过滤器无输出就判成功。
3. 活跃 Go 源码、构建/发布脚本、运行文档中无旧 `internal/gocloudagent` 和根 `cmd/gocloudagent` 入口；历史 plan 不纳入该门禁。不存在旧兼容层和双入口。
4. `git check-ignore gocloudagent/go.mod gocloudagent/cmd/gocloudagent/main.go` 应无匹配（退出码 1 在此表示预期），`build/bin` 产物仍被忽略。确认 manifest/技能资源可以进入版本控制和 Docker 构建上下文。
5. 从新模块构建到 `build/bin/gocloudagent`，执行现有 help/version 命令；示例能找到普通可执行文件，Linux helper 同目录。测试不写用户真实 `~/.gocloudagent`，使用临时 HOME/config/session。

### 6.2 行为回归矩阵

| 测试组 / 所在层 | 必须断言 |
|---|---|
| 身份往返 / `agentbridge` | 全部身份字段、nil/空 map、角色/租户/项目保留；修改转换后权限不改变源对象；API key 与登录主体的确认 ID 不变 |
| 委托与并发 / `agentbridge` + API | claims 的权限为源权限子集，不可写助手无写/admin；不同 run/project 不串身份；结束/失败/取消均撤销令牌，确认预登记与 thread 锁生命周期不变 |
| 命令策略 / bridge + gocloudagent | 注释/引号/多语句、`--statement`/stdin/batch、缺参数与未知命令；无策略/策略失败且 writable=true 也不能执行；不可写非只读不弹确认，headless 破坏性命令不执行 |
| LLM / agentcore + 宿主出口 | 400/422 tool 不支持时 auto 回退次数不变；401/403/429、context 取消/超时不新增 Chat 请求；终端错误码/文案一致，流 Next/NextDelta 的错误同样安全分类 |
| 技能 / 模块 + bridge | embed 索引与引用路径有效、提示词内容不变；公开技能前缀及 readonly 标注与真实 sbcli 命令双向对账，不复制私有表 |
| API / `internal/api` | SSE tool_call/确认/tool_start/tool_result/终态顺序、工具结果 JSON 与默认模型不变；系统库保护、项目隔离保持；所有 fake 接口同步 |
| 定时任务 / `agentjob` | 使用同一身份映射、策略及最终分类；headless 不绕过确认，任务记录不含原始错误/正文泄漏 |
| trace / 模块 + 装配 | nil 和 typed-nil logger 无崩溃；关闭仅元数据，开启脱敏/截断；已知回退日志风险按 §4.3 单列，不伪报已解决 |
| CLI / tools / gosandbox | help/version、临时会话恢复、HTTP 客户端、native/text 工具协议保持；隔离不可用 fail-closed，Yolo 不绕过隔离，Cloud 不回退本机 |

每阶段交付实际命令、退出状态、已有失败与环境跳过列表。只更新文档时不执行这些构建/测试，也不宣称行为已验证。

## 7. 风险及处理

1. **DTO 字段遗漏**：迁出前列全字段并做宿主到 gocloudagent 的单元测试，尤其权限集合、`Agent.Writable`、工具列表、消息 `ToolCallsJSON` 和默认模型；不可直接用不安全的结构转换。
2. **独立模块测试仍偷偷依赖宿主**：测试 fake 替代根 `auth/systemdb/observability`；与真实 `sbcli/sqlguard/llmgateway` 的跨层对账和 E2E 放根模块侧测试，而非在 gocloudagent 的 `go.mod` 中反向依赖根模块。
3. **命令分类双事实来源**：只读判定保留在 `sbcli`、SQL 边界保留在 `sqlguard`，gocloudagent 只通过注入端口调用；端口缺失时 fail-closed，避免默认允许。
4. **构建产物改变**：新目录与旧根二进制冲突，路径明确改为 `build/bin/gocloudagent`；同步示例和发布脚本，保持二进制名称、`main.version` 和 helper 定位。不能将源码目录当可执行文件，不能用保留旧二进制掩盖构建失败。
5. **模块依赖版本漂移**：从既有锁定版本建立独立 `go.mod/go.sum`，检查 `go mod tidy` 的变动；不借迁移机会升级依赖，`gosandbox` 继续使用当前独立模块版本与本地引用策略。
