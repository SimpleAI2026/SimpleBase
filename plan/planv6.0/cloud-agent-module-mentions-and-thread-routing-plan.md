# 云助手模块覆盖、@ 点名与会话路由优化计划

> 日期：2026-10-12  
> 状态：请求契约/切换引导、统计模块、沙盒按可用性播种、已知默认名双语展示及关闭能力门禁已实施；`@` 技能/助手分组菜单、同名/空格名候选 ID 绑定、KV 兼容别名及调度 thread 归属防护已实施；通用助手跨助手交接、用户助手仍待安全契约；未操作运行库或迁移  
> 范围：`ui/src/pages/AgentManager.vue`、`ui/src/components/ai/AiChatComposer.vue`、`internal/gocloudagent/`、`internal/api/agent_handler.go`、`internal/systemdb/agents.go` 及测试/文案  
> 关联：[`cloud-assistant-chat-ux-plan.md`](./cloud-assistant-chat-ux-plan.md)、[`cloud-assistant-skills-cli-v2-plan.md`](./cloud-assistant-skills-cli-v2-plan.md)、[`bugfix-cloud-agent-cli-chain.md`](./bugfix-cloud-agent-cli-chain.md)  
> 约束：不新增依赖；不得绕过项目、角色、系统库及工具权限；不得由代理运行 `init_db`、ALTER/DDL；对象形状使用 TypeScript `interface`；中英文文案同时更新。

## 1. 现状和根因

1. 修复前 `defaultCloudAgentSpecs` 只播种通用、Database、S3、Logs 四个助手；现在 CLI 技能启用时幂等追加 KV、云函数、定时三种助手及独立模块/默认技能；未启用时不追加，也不把不存在的只读工具填入其 `ToolIDs`。Sandbox 在云沙盒实际可用时补种内置助手，关闭后保留已有记录但禁止运行；**模块目录、内置助手、CLI skill 是三种不同对象**。`users` CLI 技能已下线，顶层 `SKILL.md` 对 `@用户` 的错误宣传已移除。
2. `AiChatComposer.vue` 已分别列出八种既有技能和当前项目助手，`@数据库` 保持 skill、`@Database` 保持原助手语义；关闭 CLI 后技能候选禁用，沙盒未就绪时沙盒技能禁用。用户显式选中助手时按草稿区间保留稳定 ID，改动该 token 后绑定失效；同名未选候选不猜第一项。含空格名称、歧义引导和自由文本的结构化绑定仍待完善。
3. 初始问题经隔离契约测试澄清：真实输入 `@数据库` 在通用助手 thread 发出 `mentions:[{agent_id:通用助手ID}]`、`skills:['database']`，不会触发 thread 归属报错；真正失败的是点名另一个 Agent（例如 `@Database` 或专用助手里的 `@KV助手`）。已修复的路径：专用助手发送前引导切换并保留草稿；后端仍拒绝跨助手请求，但返回 409 `agent_switch_required`，未点名时按已绑定 thread 的助手执行。**未复现用户当时的线上原始请求，不能断定其 `@数据库` 报错仅由该点名引起；须在环境可用时核对实际 thread ID、agent_id 与请求体，不运行迁移。**
4. 后端仍保留旧英文默认名 `Database` / `S3` / `Logs`，现 UI 仅对与原始默认名精确一致的内置记录显示中英文翻译，不修改记录或历史 ID；已改名/自定义记录保留原文。`auth.RoleAdmin` 是**只读管理员**，`RoleSuperAdmin` 才能管理用户；`project.isAdmin` 表示系统项目，**不是**用户角色。`/v1/users` 和助手 CLI 的委托令牌边界不能因为增加“用户助手”而放开。
5. v47 已把 thread 绑定到 Agent；不能为消除报错而删除归属校验、悄悄修改 `agent_id`，或使普通专用助手任意执行别的助手。

## 2. 目标与语义决策

| 输入/场景 | 预期行为 |
| --- | --- |
| 通用助手 thread：`@数据库`、`@云函数`、`@定时任务`、`@键值`、`@对象存储`、`@日志`、`@沙盒` | 作为**模块技能**加载给通用助手执行；thread 始终归通用助手。调用已有受限 CLI/API 或只读/沙盒工具，不凭空扩权。能力缺失时说明需要配置/启用，不伪装为成功。 |
| 通用助手 thread：`@数据库助手`、`@键值助手` 等内置/自定义助手 | **待决策，不随修复报错自动实施**：候选方案是在通用 thread 单轮交接，run/message 记录执行者、thread 保持归通用助手；实施前须解决权限交集、有效技能快照与重试、SSE/历史执行者、调度归属及存量 thread 兼容。现阶段沿用拒绝跨助手点名并引导切换，不经 HTTP Agent 路由递归调用。 |
| 专用助手 thread：`@其他助手`（例如数据库助手点名键值助手/“KV助手”） | 发送前给出“请切换到 {目标助手} 后执行”的可点击引导；点击后切换到目标助手的 thread 并**保留草稿**，用户再次确认发送。不在原 thread 偷换 Agent/自动转发。直接调用 API 时返回稳定错误码，不执行 run；目前错误体不包含目标 ID。 |
| 专用助手 thread：`@本助手` 或没有 Agent 点名 | 沿用原有执行方式；显式点名技能仍遵守已有 `NormalizeSkills`、工具/委托权限及沙盒可用性边界。 |
| 管理员与 `@用户` | “用户管理”不作为普通项目可执行技能；仅有管理用户权限的 `superadminl1` 可见/使用**用户助手**，只做已有授权的用户信息查询与操作指引，不得由 LLM/委托 CLI 自动调用 `/v1/users` 写入、重置密码或读取凭据。只读 `admin` 不等同有用户管理权限；若希望它可看用户数据，需单独审议只读入口。 |

**交互规格（部分已实施）**：每轮至多一个不同的 Agent ID；前端提示移除多余点名，后端直调返回 400 `multiple_agents_not_supported`，不会只执行第一个；重复同一 ID 可继续。多处技能点名可组合，仍须依现有委托权限执行。无点名请求按绑定 thread 的助手解析；旧未绑定 thread 保留兼容。跨助手交接的重试必须复用原 run 的执行助手、有效技能及用户消息，属于后续数据契约工作：现有 run 未存技能快照，禁止把这点当作已实现。

**关于通用助手执行边界（待决策）**：现阶段通用助手可 `@` 所有**已授权技能**，不等于跨 Agent 调用；跨 Agent 点名依然引导切换。若后续决定支持单轮交接，不能把所有工具一次授给通用助手：须明确调用者权限、目标 Agent 配置、技能及 CLI 委托路由的交集，并先解决 run 的有效技能持久化、重试、SSE 和历史执行者标注；专用助手独立 thread 不因此改变。

## 3. 模块、名称与权限设计

### 3.1 统一清单（覆盖统计网站场景）

| 稳定 ID | 中文显示 / 英文显示 | 作用与边界 |
| --- | --- | --- |
| `general` | 通用助手 / General Assistant | 按明确的点名执行已有能力；无点名不默认获得全部 CLI 写权限。 |
| `database` | 数据库助手 / Database Assistant | 存统计明细、维度和聚合结果；写入仍走已授权 CLI/API，系统库只读。 |
| `gofunction` | 云函数助手 / Cloud Function Assistant | 提供后端查询、过滤、聚合 API；保留调用权限和函数执行门禁。 |
| `cron` | 定时任务助手 / Scheduled Task Assistant | 创建/查看按小时、按天的离线汇总任务；创建、启停需写权限。 |
| `kv` | 键值助手 / Key-Value Assistant | 缓存看板/排行榜、计数器；只在项目存在 KV 存储且具备权限时执行。 |
| `s3` | 对象存储助手 / Object Storage Assistant | 管理 CSV/PDF 报表、图片及静态资源；文件上传不得读取宿主机文件。 |
| `logs` | 日志助手 / Logs Assistant | 项目内访问与运行日志检索、指标汇总；不得越项目或改日志保留策略。 |
| `sandbox` | 沙盒助手 / Sandbox Assistant | 隔离脚本调试/口径验证；仅沙盒启用且 backend 可用时提供，不注入 SimpleBase 凭据。 |
| `users`（**受限**） | 用户助手 / Users Assistant | 仅有用户管理权限的管理员可见；**不是已下线 `users` CLI skill 的复活**，本阶段仅在允许的上下文提供说明与控制台导航，不提供模型用户 CRUD 工具。 |

这里的“管理员”按 `auth.Role.CanManageUsers()` / 前端 `auth.canManageUsers` 定义为 `superadminl1`；`admin` 是可查看用户的只读角色，不应被误当作可管理用户的管理员。系统项目 `sb-admin` 另有系统库保护，不能因为处于该项目就扩大用户助手权限。**后续用户助手的先决条件**：决定是按主体计算的虚拟目录项还是持久 Agent；如为持久 Agent，创建/修改、列表/详情、建 thread、run 解析与 API Key 直调都必须在后端逐请求校验，不能仅按项目播种或前端隐藏。若产品确需让只读 `admin` 看用户信息，再添加与 `CanViewUsers` 一致的专门只读工具及独立安全评审。

### 3.2 命名与兼容

- 模块 ID、`builtin_key`、skill ID 固定英文，不随语言切换；界面显示名、说明、占位文案、引导和错误提示由 `zh-CN.ts` / `en-US.ts` 的同构 key 生成，跟随语言设置。自定义助手名称保持用户原文，不做机器翻译。
- 内置助手以 `builtin_key` 作稳定身份，已有记录及历史 thread **保留原 ID**。现已对 `Name` 与原始默认名精确一致的内置记录使用前端双语展示名，不写回 API/数据库；明确改名的记录及自定义助手保留原文。旧记录如果由用户改成恰好等于原始默认名，现有字段无法区别，只能默认按内置名翻译；要完全排除这种边界情况需改名标记和迁移。新播种仍尊重幂等和 `dismissed`。
- `@数据库` 等短名保留为**技能/模块**，`@数据库助手` 等完整名用于**助手**；英文界面提供 `@database` 等技能别名和明确的助手标签。考虑现有 `Database` / `S3` / `Logs` 自定义与旧文本，定义双语别名兼容表，解析为稳定 ID；显式候选选择保存目标 ID，不依赖显示文字反解析。技能与助手撞名时弹出“技能 / 助手”两个明确条目，发送协议携带区分后的结构化 ID；服务端不信任用户可改写的裸 `mentions`/`skills` 数据。
- 已保留旧内置原名 `@Database` / `@S3` / `@Logs` 作为助手点名（只在对应 Agent 存在时）；`@数据库` 仍是 skill。手输 `@KV助手` 在未改名的内置 KV 助手存在时可解析为其稳定 ID；若有同名自定义助手则提示选择候选。含空格名由用户选候选绑定 ID，手输完整名称则提示选择候选，不猜 ID。

## 4. 分阶段实施

### A. 请求契约与确证问题（已完成隔离验证，现场待核）

- `ui/tests/AgentManager.test.ts` 用真实 `AgentComposer` 验证 `@数据库` → 通用助手 ID + `database` 技能；用专用助手的 `@KV助手` 验证旧逻辑错误发到数据库 thread；`internal/api/agent_handler_test.go` 验证隔离内存库中的归属冲突。测试没有使用用户运行库。
- 已修复专用助手点名另一助手的发送前引导与 409 `agent_switch_required`；不取消服务器 thread 归属保护；未点名按绑定的助手执行。**未复现线上原始请求，不宣称 `@数据库` 线上报错已根治。** 后续结合用户现场请求核对 thread/mentions/skills；如确认是旧客户端或会话状态问题，再补针对性测试。
- 若决定支持跨 Agent 交接，先核对 `internal/gocloudagent/runtime.go`、`scheduler.go`、`systemdb/agents.go` 的 run/历史/调度数据归属；现有调度 thread 并非一律绑定 schedule Agent，不能把“调度始终按绑定助手执行”当成现状。

### B. 模块与默认助手（统计模块、沙盒可用时播种已实施；用户助手待单列）

- 已实施 `kv`、`gofunction`、`cron` 稳定模块、独立提示词及 `NormalizeSkills` 默认 skill 映射。三者无新增内置工具，需 `SkillsCLI=true`；关闭时模块目录不展示、服务端拒绝新建、播种不追加；先前已存在的记录和 thread 保留，前端提示需启用 CLI，后端运行与重试请求返回 503 `cli_skills_unavailable`。
- CLI 启用时为一般项目幂等播种上述三个内置助手，旧 `general`/`database`/`s3`/`logs` 的 ID 和配置不变，`dismissed` 仍有效；沙盒可用时补种内置助手（含四项沙盒工具）；关闭/重开不删除或覆盖旧记录，`dismissed` 不补回；运行时若沙盒关闭返回 503 `sandbox_unavailable`。用户助手不得被全项目公开播种。已删顶层 `SKILL.md` 中失效的 `@用户` 宣传；不把 `/v1/users` 标为 `delegable`。

### C. 后端路由与安全校验（基础修复已完成；交接待决策）

- 基础修复：`CreateRun` 未点名时使用绑定 thread 的助手，显式跨助手点名返回 **409 `agent_switch_required`**，保留项目与会话归属校验；目前错误不返回目标 ID，前端只对本地已知的目标提供切换按钮。原先未绑定的存量 thread 维持兼容。
- 待决策：如果确需通用 thread 中单轮交接，才扩展 `CreateRun` 解析全部 Agent 点名；必须先落实调用者权限与目标助手、技能的交集、run 技能快照/旧 run 重试安全退化、SSE/历史执行者、调度 thread 与存量空归属 thread 的兼容。多 Agent 点名不可只取第一个；仍校验项目归属、`ClaimThread`、写门禁、配额和委托边界。
- 调度归属收口已实施：创建/修改 schedule 时若目标 thread 有别的助手归属返回 409 `agent_switch_required`；自动创建及归档后重建的 thread 绑定 schedule Agent；执行前再防御性拒绝存量异主 thread 并记录失败。v47 之前空归属历史 thread 暂保持兼容，未自动回填。若决定交接：明确 thread 现有 `AgentID` 作为 owner、run/message 的 `AgentID` 作为执行者，SSE 增执行者字段，历史上下文需保留执行者，不能只靠客户端猜测头像；run 当前无有效技能字段，必须先定义快照及旧 run 重试安全退化。`RetryOfRunID` 不接受客户端借重试传新点名切换或读取别的 thread。
- 后端返回登记错误码，前端对 `agent_switch_required` 显示友好引导；CLI/沙盒失效时既有助手保留但运行前分别拒绝并提示 `cli_skills_unavailable` / `sandbox_unavailable`；未知错误保持原有兜底。API 直调、伪造 `mentions`/`skills`、无权限用户助手访问均由服务端拒绝，**不以 UI 隐藏代替授权**。不把助手 HTTP 路由纳入委托令牌可调用集。

### D. 前端 @ 选择与切换体验

- `AiChatComposer.vue` 已显示**技能**（含只读项目技能）与**助手**两组；CLI 关闭及云沙盒不可用时禁用相应技能候选，中文/英文候选采用可解析别名。选中助手按草稿区间记住稳定 ID、编辑点名后清除绑定；未选同名候选不猜首项；`@用户` 不进入技能菜单。含空格名称从候选选择后按绑定 ID 解析；手输同名或完整含空格 Agent 名但未选候选时阻止发送并提示选择候选。语言切换后，未改名内置 Agent 的跨语言点名仍根据稳定 `builtin_key` 解析原 ID，切换引导保留；已选同名自定义 Agent 若仅靠内存区间绑定则切换组件/刷新后需要重选候选。技能候选现会显示需启用 CLI 或云沙盒不可用的具体原因；未来用户助手候选仍需由后端按主体权限过滤。
- `AgentManager.vue` 已实施：专用及通用 thread 点名其他助手时均不清空草稿、不发 run；展示切换按钮，按现有 `selectAgent` 载入目标 thread 并由用户再次发送；后端 409 竞态路径会在本地已知目标时恢复草稿。发送前核对已加载 thread 归属，项目/会话切换清除引导。通用 thread 跨 Agent 单轮交接仍待决策。
- 对于未来的受限用户助手，候选源必须使用后端按主体过滤的可见列表，而非仅由 UI 隐藏；现已按“内置 key + 原始默认名精确匹配”本地化显示，其余自定义名称保持原样。若新增结构化候选/错误契约，同步 `ui/src/services/types.ts`、`http-api.ts`；新增对象形状只用 `interface`，中英文文案及测试同时更新。

## 5. 验收矩阵

1. 当前已验证：通用助手 thread 的 `@数据库` 是技能；通用或专用助手点名另一 Agent 时引导切换，直调 API 返回 409 `agent_switch_required`。CLI 启用时 KV/云函数/定时模块有默认助手和技能回落；关闭时不新播种，也不展示新建入口；沙盒可用时补种沙盒助手，能力关闭后旧助手和历史 thread 保留，但 UI 与服务端均禁止继续运行。**通用 thread 单轮交接尚未实现**；其余技能的数据面端到端尚未验收，不能在用户运行库做写操作。
2. 数据库助手 thread：输入当前语言可见的目标名称（如 `@键值助手`）后显示切换引导，原 thread 无新 run/消息，切到目标 thread 后保留草稿；直接调用返回稳定错误码而非 `thread agent mismatch`。`@KV助手` 可点名现存未改名的内置 KV 助手；与自定义同名时提示选择候选。同一轮两个不同 Agent ID 点名现在前端提示，后端 400 `multiple_agents_not_supported`，不再默默选第一项。
3. 本轮双语展示覆盖 CLI 可用时七个默认助手及沙盒可用时的沙盒助手，并保持旧 ID；提示词为新增三个模块补齐双语。旧 `@Database`、`@S3`、`@Logs` 在对应记录存在时保持助手点名语义。用户助手权限及自定义同名 Agent 跨组件重建后的绑定仍待完成；CLI/沙盒技能候选已提示具体不可用原因；技能/助手同名 ID 消歧、含空格候选及自由手输同名/含空格引导已覆盖。用户助手需分别验证超管、只读 admin、普通用户、API Key 与系统项目；已下线 `users` skill 和委托 `/v1/users` 继续拒绝。
4. `go build ./internal/... ./cmd/...`、`go test ./...`、`cd ui && yarn test && yarn build` 通过；专测真实 router 的项目/角色/委托边界及 UI 真实输入/切换，不桩掉关键交互语义；不能降低覆盖率门槛。隔离测试不调用生产 DB/S3，也不运行真实写命令。

## 6. 迁移和风险

- 优先复用现有 `builtin_key`、thread `agent_id`、run `agent_id` 字段；跨 Agent 重试要复用有效技能时，目前 run 并无技能快照，必须先确定持久化与旧数据降级方案；默认名是否经用户编辑也无标记，不能直接翻译覆盖。若确需新字段，仅提交迁移定义、兼容读取及回滚说明，**不得由代理运行 `init_db` 或 ALTER/DDL**。目前仓库的 `systemdb.Bootstrap` 会调用 `ApplySystemMigrations`，没有发现源码直接处理 `TIMER_NO_RUN`；如本地通过外部启动脚本跳过初始化，须由维护者先确认实际启动路径和 v47 状态，再决定手动执行或允许启动流程应用迁移。
- 新播种改变左栏条目数量；`dismissed` 尊重用户意愿，沙盒功能开关变化时避免出现“有助手但不可运行”却无提示；用户助手绝不靠全项目公共播种泄露可见性。
- “用户助手”与已下线 `users` skill 冲突，当前方案仅提供授权可见的**咨询/导航**能力；若将来确需模型自动管理用户，需要单独权限模型、安全评审、非委托受控执行面和审计方案，不属于本计划。
