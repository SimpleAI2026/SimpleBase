# 云助手 Skill + CLI 收口（v2）

> **仓库**：SimpleBase  
> **日期**：2026-10-11  
> **核对基线**：`main` @ `5a03d47` + 工作区未提交改动（含 backend-optimization-v3 的 SEC/LIFE/WR 修复）  
> **前版**：[`plan/planv5.0/cloud-assistant-skills-cli-plan.md`](../planv5.0/cloud-assistant-skills-cli-plan.md)（下称 v5）  
> **约束**：[`internal/AGENTS.md`](../../internal/AGENTS.md)，gocloudagent 不得直写存储，不得向沙盒注入凭据

## 0. 结论：v5 完成了吗

**骨架齐了，但没有完成。** v5 的五个阶段（A–E）都有代码和测试，`TestAgentSkillsE2E` 的 15 条样例也都能通过。问题在于这 15 条样例打的是**测试桩 HTTP 服务**（`skillData`），不是真实的 `api.NewRouter`。所以凡是「委托令牌 → 真实鉴权中间件 → 真实路由」这一段出的错，CI 都发现不了。按真实路由核对后，有以下问题：

| 级别 | 问题 | 影响 |
| --- | --- | --- |
| **P0** | 委托白名单 `delegationAllowedResources` 用的资源名和真实路由对不上（`functions`/`cronjobs`/`objects`，而真实路由是 `gofunctions`/`cron-jobs`/`s3`）；`/go/...` 也不在 `/v1/projects/` 前缀下。**这是 backend-optimization-v3 SEC-03 修复（黑名单改白名单）引入的回归**，SEC-03 的测试用的是虚构路径，所以没发现 | 生产环境里 `@云函数`、`@定时任务`、`@对象存储` 的**全部命令**都返回 403；`function invoke` 也是 403 |
| **P0** | 白名单放行了 `agent-threads`、`agent-runs`、`agent-schedules`、`agents` | 违反 v5 §3.6「助手 runs 不做成 CLI 命令，防止递归」：委托令牌可以直接调 HTTP 发起新 run，或者确认别的 run |
| **P0** | 助手模式下，CLI 的 `--file` / `--init-sql` / `sql batch --file` / `function create --file` / `object upload --file` 会直接读 **simplebased 宿主机**上的任意路径；`destructiveArgv` 判定时也会读宿主机文件 | 模型可以把 `config.yaml`、`/etc/passwd` 之类的文件当成云函数源码或对象上传，泄漏宿主机文件 |
| P1 | 白名单拦掉了 `/v1/users`、`/v1/projects`、`api-keys`、`settings`、`quota` | `users`、`project` 两个 skill 实际上整体不可用，skill 文本却说可以用 |
| P1 | `ConfirmRun` 只要求写权限，不校验确认人是不是这个 run 的发起人；`AgentRun` 也没有记录发起人 | v5 §4.5 第 3 步「同一 Principal」没有落地 |
| P1 | `callGate` 的 channel 从来不删；`wait` 固定等 5 秒，期间无人 `release` 就白等 | 长时间运行后内存只增不减；参数增量缺失时每次调用都多等 5 秒 |
| P2 | `redactArgv` 只是原样复制，没有做脱敏 | 确认弹窗和 SSE 里会出现 argv 原文（例如 `kv SET` 的值） |
| P2 | `sqlDestructive` 用 `strings.Split(";")` 拆分语句 | 字符串字面量里有 `;` 时会误拆，可能把非破坏性语句判成破坏性（方向偏保守，不算漏判） |
| P2 | 每个 skill 正文只有 127–637 字节，没有写 flag 和示例 | 模型只能猜 flag，容易拿到 `usage` 错误后反复重试 |
| P2 | `docs/cli.md` 只有 30 行；`sbcli` 只有 3 个测试函数 | 文档和单测都明显不足 |

完成度（按 v5 的验收总表 §10 逐项对照）：

| v5 §10 | 状态 | 说明 |
| --- | --- | --- |
| 1 CLI 覆盖 §3.5、JSON 信封、错误码透传 | 部分完成 | 命令表齐全（`internal/sbcli` 共 72 个命令）；`users`/`project` 被白名单拦截 |
| 2 skill 隔离 | 完成 | `AllowArgv` + `skill_not_loaded`；样例 13 覆盖 |
| 3 破坏性操作先确认 | 部分完成 | 协议通了；缺少「确认人等于发起人」的校验 |
| 4 系统库 / admin / 只读实例拒绝 | 完成 | WR-01 写门禁已统一 |
| 5 流式（增量、执行中、结果、续传） | 完成 | `tool_call_delta`/`tool_start`/`is_error`/`id:`/`events?after=`/三次重连都在 |
| 6 CI | 有缺口 | 样例集测的是桩服务器，P0 问题因此漏过 |

## 1. 目标

1. 修复 P0：生产环境里每个 skill 至少有一条读命令和一条写命令能走通真实路由。
2. 把「样例集打桩服务器」换成「样例集打真实 router」，让鉴权链路的回归能在 CI 里暴露。
3. 宿主机文件不再进入模型可控的输入面。
4. 补齐 v5 承诺但没落地的项：确认人校验、argv 脱敏、gate 回收。
5. 精简：`users` / `project` 这类与委托边界冲突的 skill，收缩为只读，或者直接下线。

不在本计划内：换传输层（继续用 SSE）、引入 CLI 框架、补 JS SDK、多 Agent 协作。

## 2. 决策

| # | 决策 | 理由 |
| --- | --- | --- |
| D1 | 委托白名单从「资源名字符串」改为**路由表驱动**：在 `router.go` 挂路由时声明 `delegable`，白名单由 Echo 的 `c.Path()` 模板查表 | 字符串表和路由各维护一份，迟早会漂移，这次就是这样。改成路由表驱动后只剩一个事实来源 |
| D2 | 助手相关路由（`agents`/`agent-threads`/`agent-runs`/`agent-schedules`/`agent-modules`）一律**不可委托** | 防止递归调用和自我确认 |
| D3 | 助手模式下禁止读宿主机文件。需要文件内容的参数改为**内联**：`--source -` / `--statement` / `--content-base64`，由宿主通过 stdin 传入，大小上限沿用 handler 的上限 | 模型手里本来就没有文件，`--file` 只对人类操作者有意义 |
| D4 | `users` skill 下线；`project` skill 只保留 `quota get`、`audit list` | 与 SEC-03「委托令牌不能做身份和凭据管理」一致。用户管理与 API Key 管理继续走控制台 |
| D5 | `AgentRun` 增加 `initiator_id` 字段（UserID，或 `apikey:<id>`）。确认和取消都要求 `initiator_id` 相同，或者调用者带有 `project:admin` | 落地 v5 §4.5；同时收口 SEC-04 遗留的「项目内互相操作」 |
| D6 | 样例集保留，fake LLM 不变；数据面改为 `api.NewRouter` + 内存 SQLite 系统库 + fake catalog/fake S3 | 能测到鉴权，又不依赖 DuckDB 或真实 S3 |

## 3. 改动清单

### 3.1 P0-1 委托白名单改为路由表驱动（D1、D2）

- `internal/api/router.go`：新增 `delegable := delegationMark()` 中间件，加到 skill 需要的路由上。中间件只做一件事：`c.Set("delegable", true)`。
- `internal/auth/middleware.go`：组级 authMW 校验委托令牌后，只注入 Principal 并标记 `Principal.Delegated=true`，不再做路径判断。Echo 的路由级中间件在组级之后执行，所以拒绝放在路由级：`require(...)` 遇到 `Delegated && !delegable` 时返回 403。未标记 `delegable` 的路由默认拒绝委托令牌。
- 删除 `delegationAllowedResources` 和 `delegationPathAllowed` 里的字符串表；`delegation_allowlist_test.go` 改为对真实 router 做表驱动。
- 可委托的路由（与 skill 前缀表一一对应）：

| skill | 可委托的路由（相对 `/v1/projects/:projectID`，另注明者除外） |
| --- | --- |
| database | `databases*`、`databases/:id/{query,execute,batch,schema*,data/collections*,documents*}` |
| kv | `POST kv` |
| s3 | `s3/objects`（GET/POST/DELETE）、`GET s3/presign` |
| gofunction | `gofunctions*`，以及 `POST /go/:projectID/:name/:functionName`（`/go` 组单独加 `delegable`；项目一致性由 `projectContextMiddleware` 保证） |
| cron | `cron-jobs*` |
| sandbox | `sandboxes*` |
| logs | `GET logs`；`logs/retention` **不可委托**（v5 原本需要确认，现收紧为直接拒绝） |
| project | `GET quota`、`GET audit` |

- 测试：`TestDelegation_RouteTable` 遍历 `e.Routes()`，断言「可委托集合」恰好等于上表；新增路由默认不可委托，没有显式标记的路由，委托令牌访问一律 403。

### 3.2 P0-2 宿主机文件隔离（D3）

- `internal/sbcli`：`loadedConfig.Agent == true` 时，`readFileOrDash` 只接受 `-`（stdin）；传入其他路径一律返回 `usage` / `file_not_allowed`。人类操作者的行为不变。
- `gocloudagent.execSimplebase`：
  - 工具参数从 `{argv}` 扩展为 `{argv, stdin?}`，`stdin` 是字符串，上限 256KiB（`function create` / `sql batch` / `object upload` 用 base64 的场景，CLI 侧解码）。
  - `destructiveArgv` 不再读文件，改为分析 `stdin` 的内容。argv 里出现 `--file`/`--init-sql` 且值不是 `-` 时，直接返回 `file_not_allowed`，不启动 CLI。
  - `SubprocessCLI` 设 `cmd.Dir` 为空的临时目录，进一步缩小攻击面（防御纵深，不作为唯一边界）。
- skill 文本：把 `--file PATH` 改写成 `--file -` 加 `stdin`，并各给一条示例。
- 测试：助手模式下 `--file /etc/hosts` 返回 `file_not_allowed`，HTTP 调用次数为 0；人类模式读文件的行为不变。

### 3.3 P1 发起人校验（D5）

- 迁移：`sys_agent_runs` 增加 `initiator_id TEXT NOT NULL DEFAULT ''`，历史数据保持空串。
- `CreateRun` 写入 `initiator_id`；`ConfirmRun` / `CancelRun` 的判定为：`initiator_id == 当前主体`，或者主体有 `project:admin`，或者 `initiator_id == ''`（兼容历史数据，后者仍要求写权限）。
- `confirmHub.Resolve` 改为接收 `principalKey`，并与 `Wait` 时登记的值比对，做二次校验。
- 测试：A 发起的 run，B（同项目、有写权限、没有 `project:admin`）确认 → 403；A 确认 → 200；项目管理员确认 → 200。

### 3.4 P1 gate 回收与等待

- `callGate`：`wait` 结束后（无论 release 还是超时）`delete(g.ch, id)`；run 结束时调用 `gate.dropRun(runID)` 清理残留。
- 等待时长从固定 5 秒改为「收到该 `call_id` 的 `tool_call` 收束就立即放行，最多等 1 秒」。增量不可用的供应商（text 协议）直接跳过 gate。
- 测试：1000 次调用后 `len(g.ch)==0`；text 协议下单次工具调用的额外延迟小于 50ms。

### 3.5 P2 脱敏与 SQL 分类

- `redactArgv`：`--statement` 的值只保留首关键字和前 200 字符；`kv exec` 的 `--arg` 除第一个（key）以外全部替换为 `***`；匹配 `sensitiveKey` 的 flag 值替换为 `***`。
- `sqlDestructive`：改用 `sqlguard` 已有的语句切分（`sqlguard.Split`，如果没有就在 sqlguard 内补一个能识别引号和注释的切分），不在 gocloudagent 里再写一套。
- 测试：含 `';'` 字面量的 INSERT 不判为破坏性；`DELETE` 藏在第二条语句里仍判为破坏性。

### 3.6 skill 精简与补全（D4）

- 删除 `skills/simplebase/users/`；同步删除 `skillPrefixes["users"]`、别名 `@用户`/`@users`，以及前端 `agent-skills.ts` 里的对应条目。
- `project` 前缀收缩为 `{"quota", "audit"}`；`sbcli` 里的 `user *`、`project create`、`apikey *`、`settings set` **保留**给人类操作者，只是不再出现在任何 skill 中。
- 其余 7 个 skill 正文每份补到 1–3KiB：每条命令写清必填 flag、一条最小示例、需确认标记、常见错误码及处理方式。仍受 `maxSkillChars=8192` 约束。
- `skills_test`：除了 allow 注释与前缀表对账，再加一条：skill 里出现的每个 `simplebase <r> <a>` 都必须能被 `sbcli.dispatch` 解析。

### 3.7 样例集改打真实 router（D6）

- 新建 `internal/api/agent_skills_router_e2e_test.go`：`NewRouter(Dependencies{...})` 使用真实的 `AuthMiddlewareWithDelegation`、`writeGate`、`projectContextMiddleware`；数据面依赖用现有的 fake（`fakeCatalog`、内存对象存储、`gofunction` 真解释器）。
- v5 的 15 条样例全部迁移过来；新增：
  - 16：`@云函数` create → invoke，HTTP 走真实 `/gofunctions` 和 `/go/...`，返回 200（锁住 P0-1）
  - 17：委托令牌直接 `POST agent-threads/:id/runs` → 403（锁住 D2）
  - 18：`function create --file /etc/hosts` → `file_not_allowed`，HTTP 0 次（锁住 P0-2）
  - 19：B 确认 A 的 run → 403（锁住 D5）
  - 20：`@用户` 被解析成普通文本，不产生 skill；请求体传 `skills:["users"]` → 400 `unknown_skill`
- 原来打桩服务器的 `agent_skills_e2e_test.go` 保留流式协议相关的子用例（replay、comments、confirmation timeout），删掉与真实 router 版本重复的样例。
- `scripts/smoke.sh` 的 `-run` 改为 `TestAgentSkills`，同时匹配两个文件。

### 3.8 文档

- `docs/cli.md` 扩写：安装、配置优先级、退出码表、助手模式与人类模式的区别（文件读取、secret、presign）、「哪些命令不能被助手调用」一节。
- `plan/planv5.0/cloud-assistant-skills-cli-plan.md` 的文件头加一行「已由 planv6.0 修订」，不改正文。

## 4. 分阶段

| 阶段 | 内容 | 门槛 |
| --- | --- | --- |
| A | 3.7 的真实 router 测试框架 + 样例 16、17、18 **先写成失败用例** | 在当前代码上 16、17、18 失败，且失败原因与第 0 节一致 |
| B | 3.1 + 3.2 | A 中的失败用例全部通过；`go test -race ./internal/auth ./internal/api ./internal/gocloudagent ./internal/sbcli -count=3` |
| C | 3.3 + 3.4 + 3.5，样例 19 | 迁移在空库和已有数据的库上都能应用；gate 计数测试通过 |
| D | 3.6 + 样例 20 + 前端别名同步 | `cd ui && yarn test && yarn build`，覆盖率阈值不下降 |
| E | 3.8 文档；全量验收 | 见第 6 节 |

A→B 的顺序不能对调：先证明 P0 存在，再修。

## 5. 风险

| 风险 | 处理 |
| --- | --- |
| 路由表驱动之后，新增业务路由忘了标 `delegable`，助手功能悄悄失效 | 失效方向是安全的（403）；`TestDelegation_RouteTable` 把可委托集合写死，变更必须同时改测试 |
| 去掉 `--file` 后，大源码通过 stdin 传入会撑大工具参数 | 上限 256KiB，与 handler 一致；超过上限返回 `usage`，由模型分段处理或提示用户去控制台 |
| `initiator_id` 迁移碰到旧数据 | 空串走兼容分支，不阻断历史 run 的取消 |
| 下线 `users` skill 改变了对外宣传的能力 | v5 的这项能力在生产中本来就被 SEC-03 拦着，实际不可用，下线只是把文档和实现对齐 |
| 真实 router 测试变慢 | 复用 `agent_e2e_harness_test.go` 的内存 SQLite；目标是这组测试不超过 10 秒 |

## 6. 验收

```sh
go build ./internal/... ./cmd/... ./gofunction/...
go vet ./internal/... ./cmd/...
go test -short -count=1 ./internal/... ./cmd/... ./packages/go-sdk
go test -race -short -count=3 ./internal/auth ./internal/api ./internal/gocloudagent ./internal/sbcli
go test -count=1 -run TestAgentSkills ./internal/api
cd ui && yarn test && yarn build
```

完成定义：

1. 第 0 节的每个 P0/P1 都有「修复前失败、修复后通过」的回归测试。
2. 7 个保留的 skill，每个至少有一条读命令和一条写命令在真实 router 上返回 2xx。
3. 委托令牌访问任何助手路由、用户路由、凭据路由都返回 403。
4. 助手模式下，CLI 进程不打开任何宿主机文件（测试中用 `--file` 加不存在的路径和真实路径各验证一次）。
5. 确认和取消只对发起人或项目管理员生效。
6. `docs/cli.md` 与 skill 文本中出现的命令，都能被 `sbcli.dispatch` 解析。
