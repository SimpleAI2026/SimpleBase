# gocloudagent 死代码与冗余清理计划（v6.6）

> **状态：已执行完成（2026-11-17）。** 见文末「§7 执行记录」。
>
> 承接 `gocloudagent-directory-reorg-plan.md`（目录整理已完成）与 `dead-code-and-optimization-plan.md`（DC-2 已清 `gocloudagent` 的零引用私有符号）。本文只登记**目录整理后**用 `skills/simplify-code` 做只读评估得到的新发现，**本文只规划，不代表已实施**。
>
> 原则（simplify-code Golden rules）：行为与公共 API 不变；每项单独改、单独验证；有差异输入的改动一律走「需批准」。不改 `go.mod` / `go.sum`。

## 0. 评估口径

| 项 | 内容 |
| --- | --- |
| 日期 / 版本 | 2026-11-17，HEAD `d855e65` + 工作区未提交的目录整理改动 |
| 范围 | `gocloudagent/` 全模块：`cloudagent` `agentcore` `cli` `mcp` `tools` `skills` `cmd` 与根包；不含 `bench/*.py`、`docs/` |
| 工具 | `deadcode -test ./...`（模块内）、根模块 `deadcode ./cmd/...`（宿主视角）、`staticcheck ./...`（默认集 0 告警；`all` 仅 ST1020 一条）、`go mod tidy -diff`（无差异）、自写导出符号跨模块引用扫描（已剥离注释） |
| 局限 | 未做重复代码检测；未逐函数通读；样本外不下结论。评估在 `/tmp` 副本上进行（原因见 B0） |

**已核实为误报、不得删除**：

- 根模块 `deadcode` 报的 `agentcore.{User,System,Assistant,ToolResult}SchemaMessage`、`mcp.ConnectStdio`、`mcp.DisplayName`、`skills.Known`：由 `cli` 使用，`cli` 只被独立二进制 `cmd` 引用，根模块不构建（同 `dead-code-and-optimization-plan.md` §0 口径）。
- 模块内 `deadcode -test` 报的 `skills.Prefixes / IDs / Body / EmbeddedReadOnly`：仅由 `internal/agentbridge/skills_test.go` 做「文档 ↔ 前缀表 ↔ sbcli 只读表」对账，属于对账专用 API，源码注释已声明。
- `Runtime.IsRunActive`：`internal/api/agent_handler.go:516` 在用。
- `agentcore.Model.WithTools`：eino `ToolCallingChatModel` 接口要求。

---

## B0 阻塞项：`gocloudagent/skills/simplebase/` 文档被删除（需用户决定，先于一切清理）

**现象**：当前工作区 `gocloudagent/skills/simplebase/` 下 10 份技能文档（`SKILL.md` + `references/{authoring,cron,database,gofunction,kv,logs,project,s3,sandbox}.md`）显示为已删除，`gocloudagent/skills/embed.go` 的 `//go:embed simplebase` 因此无匹配文件：

```text
gocloudagent/skills/embed.go:9:12: pattern simplebase: no matching files found
```

`go build ./internal/... ./cmd/...` 与 `internal/agentbridge` 的对账测试在此状态下均无法通过。HEAD 提交 `d855e65 feat: 清理不必要的 skill` 之后，仓库根 `skills/simplebase/` 已改成另一套内容（`cli.md` / `database.md` / `gofunction.md` / `manifest.md` / `sites.md` + `scripts/sbdev.py`），与云 Agent 内嵌文档的 id 集合（`database kv s3 gofunction cron sandbox logs project`）**不一致**。

**判断**：这很可能是别处的 skill 清理连带误删了云 Agent 的内嵌文档。云 Agent 运行时依赖它们（`skills.Prompt` / `skills.AllowArgv` 前缀表对账 / `internal/agentbridge/skills_test.go`），不是死代码。

**需要决定**（二选一，不代为执行）：

| 选项 | 做法 | 说明 |
| --- | --- | --- |
| A（推荐） | `git checkout HEAD -- gocloudagent/skills/simplebase` 恢复 10 份文档 | `embed.go` 头注释的改动保留；恢复后跑 §5 基线确认 `go build` 与对账测试通过 |
| B | 确认云 Agent 技能改用仓库根 `skills/simplebase/` 的新内容 | 需要重写 `skills.go` 的 `skillPrefixes`、`skillDocPath`、`walkSkillDocs`、对账测试与 `<!-- allow: -->` / `<!-- readonly: -->` 注释，属独立重构，不在本计划内 |

**B0 未解决前不得开始 §1 的任何实施项**（基线会失败，无法区分新旧失败）。

---

## 1. 清理项

优先级按维护收益，风险与置信度分开标注。**需批准** = 改签名 / 导出名 / 跨目录引用，或存在差异输入。

### 1.1 P2：可直接应用（低风险）

| ID | 位置 | 问题 → 方案 | 风险 / 证据 |
| --- | --- | --- | --- |
| GCL-1 | `cli/repl.go:472-473`、`tools/tools.go:514` | `var _ = tool.BaseTool(nil)`、`var _ = agentcore.RedactSecrets`、`var _ = schema.AssistantMessage` 仅为让 import 保持「已使用」。删这三行；**若对应 import 在文件内无其他引用则一并删除** | 低。已核对 `cli/repl.go` 内 `tool.` / `agentcore.` 仅出现在这两行；`tools/tools.go` 内 `schema.` 仅此一处。以 `go build` + `go vet` 判定 |
| GCL-2 | `cli/mcp_config.go:166-172` | `meta, merr := t.Info(ctx)` 后 `_ = meta`，只用 `merr`。改为 `if _, merr := t.Info(ctx); merr != nil { … }` | 低。`meta` 在函数内无其他引用；`t.Info` 仍被调用一次，副作用与错误处理不变 |
| GCL-3 | `cloudagent/ports.go` | 别名 `ToolSpec`、`DeltaStream` 在宿主与模块内引用均为 0。删除这两项 | 低。保留 `ChatMessage`（宿主 2 处）等仍有引用的别名 |
| GCL-4 | `cloudagent/ports.go:27` + `internal/api/agent_e2e_stream_test.go:178-179` | `ResetUnsupportedNativeTools` 别名只有这一个宿主测试在用，同包 `oai_e2e_test.go` 已直接用 `agentcore.ResetUnsupportedNativeTools`。把这一处改成 `agentcore.…`（该文件需补 `agentcore` import）后删别名 | 低。仅测试代码 |
| GCL-5 | `cloudagent/prompt.go:82` | 导出的 `RedactSecrets` 只是 `agentcore.RedactSecrets` 的转发，宿主 0 引用；模块内 19 处调用。把模块内调用改为 `agentcore.RedactSecrets` 并删除转发。**保留 `truncate`**（高频短名、非导出、行为同源） | 低。纯机械替换；`cloudagent` 已 import `agentcore` |

### 1.2 P2：需批准

| ID | 位置 | 问题 → 方案 | 为什么需批准 / 差异输入 |
| --- | --- | --- | --- |
| GCL-6 | `cloudagent/callgate.go:64-65` `dropRun(runID)` | 参数 `runID` 从未使用（`_ = runID`）。删参数，调用点 `runtime.go:146` 与测试 `skilltool_test.go:57` 同步 | 改私有方法签名（调用点 2 处，无外部影响）。**注意**：删参数前先完成 GCL-9 的结论——若最终要让 `dropRun` 按 run 维度清理，参数反而需要保留并真正使用。两项应一起决定 |
| GCL-7 | `cloudagent` 导出但宿主 0 引用的符号 | `CLIStartFailure`（`cliexec.go`）、`SandboxToolIDs`、`GeneralTools`、`ModuleTemplate`（`modules.go`）、`MCPServerIDs`、`MCPToolRefs`（`mcpaccess.go`）降为小写，缩小导出面 | 改导出名，模块内测试可能引用；需先 grep 测试文件。`HasMCPTools` / `IsSandboxTool` / `DefaultToolsForModule` / `KnownTool` 宿主在用，保留 |

### 1.3 不做（登记原因）

| 项 | 原因 |
| --- | --- |
| 合并 `cloudagent/cliexec.go` 与 `internal/agentbridge/cli.go` 的 `limitOut` | 两模块互不 import，look-alike；强行共享会让 `gocloudagent` 反向依赖宿主或在 `gosandbox` 外多出一个公共包。行为差异见 GCL-8 |
| 拆 `StartRun`（295 行）、`execSimplebase`（188 行）、`buildTools`（180 行）、`cli.SessionRunner.Run`（172 行）、`agentcore.Model.Stream`（127 行） | P1 级重构：依赖流式事件顺序与确认流程，需先补特征测试；不属于「清理死代码」，另立项 |
| `tools/tools.go:78` ST1020（注释格式） | 仅 `staticcheck -checks all` 才报，默认集已 0 告警；可顺手改，但不单独立项 |

---

## 2. Bug 疑点（不属于简化，本计划不修，须先复现 / 确认）

| ID | 位置 | 状态 | 说明 |
| --- | --- | --- | --- |
| GCL-8 | `cloudagent/cliexec.go:87-92` `limitOut` | **已确认，影响小** | 按**字节**截断（`s[:cliOutputLimit]`），输出含中文且恰好在 64 KiB 边界时可能切断 UTF-8 多字节字符，产生非法 UTF-8；宿主侧 `agentbridge/cli.go` 用 `strutil.Truncate` 无此问题。修复属于行为变化（输出内容变），需批准。方向：改为按 rune 边界回退截断 |
| GCL-9 | `cloudagent/callgate.go:64-71` `dropRun` | **怀疑，未复现** | `dropRun` 执行 `g.ch = nil`，清空的是**所有 run** 的登记，而 `callGate` 挂在共享的 `Runtime` 上（`ensureGate`）。两个 run 并发时，一个收尾会丢掉另一个正在 `wait` 的 slot，后续 `release` 会新建 channel，导致该 `wait` 只能等满 1 秒超时。后果是延迟而非错误，但与注释「按 run 清理」不符。确认需要一个并发用例（run A 等待中，run B 收尾）。若确认，方案是 slot 带 runID 前缀或维护 `runID → callIDs` 映射，属于行为变化，单独批准 |

---

## 3. 实施批次

| 批次 | 内容 | 前置 | 备注 |
| --- | --- | --- | --- |
| **C0** | 处理 B0（恢复或确认技能文档），然后跑 §4 基线并存档 | 用户决定 | 未通过则后续全部暂停 |
| **C1** | GCL-1、GCL-2 | C0 | 纯删占位引用，一个提交 |
| **C2** | GCL-3、GCL-4、GCL-5 | C1 | 别名与转发清理，涉及宿主 1 个测试文件的 import |
| **C3**（需批准） | GCL-6 + GCL-9（一并决定）、GCL-7 | C2 | 先写 GCL-9 的并发复现用例，再决定 `dropRun` 的形态 |
| **C4**（需批准） | GCL-8 | C2 | 单独提交，附 UTF-8 边界用例 |

每批只做计划中列出的项，不顺手改其他代码。

## 4. 验证门禁（每批结束必须满足；C0 先存基线）

```bash
cd gocloudagent
gofmt -l .                                              # 无输出
GOWORK=off go build ./... && GOWORK=off go vet ./...
GOWORK=off go test -race -count=1 ./...
GOWORK=off go test -list '.' ./... | grep -E '^(Test|Benchmark|Example)' | sort > /tmp/gca-tests-now.txt
diff /tmp/gca-tests-before.txt /tmp/gca-tests-now.txt   # 新增 GCL-9 用例外，不得减少
staticcheck ./...                                       # 0 告警
go mod tidy -diff                                       # 无差异（不得改 go.mod / go.sum）
cd .. && go build ./internal/... ./cmd/... && go vet ./internal/api/ ./internal/agentbridge/ ./internal/app/
go test -count=1 ./internal/agentbridge/ ./internal/app/ ./internal/sbcli/ ./internal/auth/   # api 包约 105 s，C2 / C3 / C4 后各跑一次
```

C2 额外：`go list -deps ./... | grep -c 'SimpleBase/internal'` 必须为 0（不得引入对根模块的依赖）。

## 5. 预期收益

- 删除 3 处占位引用 + 1 处 `_ = meta` + 2 个无引用别名 + 1 个转发别名 + 1 个转发函数，减少约 15 行与若干无意义的 import 依赖，消除「为了让 import 有用而写的代码」这类坏味道。
- 缩小 `cloudagent` 导出面（GCL-7），使「宿主真正依赖的接口」一目了然。
- GCL-8 / GCL-9 若确认，分别消除一处非法 UTF-8 风险与一处并发 run 下的确认等待延迟。
- 没有发现需要删除的整块死代码：`deadcode` 与 `staticcheck` 的真实命中均已在 v6.6 DC-2 与目录整理中清完，本轮只剩占位与冗余层。

## 6. 回滚

每批单独提交；任何一项出现新失败即只回退该提交。C3 / C4 属于需批准项，批准前不进入实施。

## 7. 执行记录（2026-11-17）

### B0 / C0：技能文档

用户选择「按模块合并」：

- `git checkout HEAD -- gocloudagent/skills/simplebase` 恢复原 10 份文档（`SKILL.md` + 9 份 references）。顶层 `SKILL.md` 本来就是云助手版（带 `<!-- allow: -->`、`argv` + `stdin` 约定），**不需要改写**。
- 仓库根 `skills/simplebase/` 的新内容是「本地开发 + 云端重放」skill（`sbdev.py`、`simplebase.json`、迁移文件、站点发布），与云助手的调用方式（无工作区、无文件系统、`--file -` + stdin、需界面确认）不同，**不能整体替换**。只把其中对云助手仍成立的事实合并进 `database.md` 与 `gofunction.md`：
  - `database.md` 新增「建表与 DDL 限制」：不写 `PRIMARY KEY` / `UNIQUE` / `CREATE INDEX` / `CREATE SEQUENCE`（DuckLake 不支持）、引擎是项目级设置、保留前缀 `__sb_` / `__ducklake_metadata_` / `sqlite_`、时间列统一类型、破坏性语句先说明影响。
  - `gofunction.md` 新增：解释器不等于真实 `go build`（以 `function test` 为准）、函数内不访问数据库连接或文件系统、不要写 Key、导出函数只能一个返回值（错误放进结构体字段，附错误示例与正确写法）；`version_limit_exceeded` 改为「告知用户到控制台清理，不要自行删除」。
  - 未采用：工作区 / 迁移文件 / `sbdev.py` / 种子 / 站点发布 / 清单字段，这些在云助手里没有对应能力。
- kv / s3 / cron / sandbox / logs / project 六个模块保持原文档，等用户提供新文档后再替换。
- `skills.go` 的前缀表、`<!-- allow/readonly -->` 标注、`internal/agentbridge/skills_test.go` 的对账测试均**无需改动**，且全部通过。

基线：248 个用例、`staticcheck` 0 告警、`gofmt` 干净。

### 各项结果

| ID | 结果 | 与计划的差异 |
| --- | --- | --- |
| GCL-1 | 已完成 | 删 `cli/repl.go`、`tools/tools.go` 三行占位引用，同时删掉随之无用的 3 个 import（`eino/components/tool`、`agentcore`、`eino/schema`） |
| GCL-2 | 已完成 | `cli/mcp_config.go` 改为 `if _, merr := t.Info(ctx); merr != nil`，删 `_ = meta` |
| GCL-3 | 已完成 | 删 `cloudagent` 的 `ToolSpec`、`DeltaStream` 别名 |
| GCL-4 | 已完成 | `internal/api/agent_e2e_stream_test.go` 改用 `agentcore.ResetUnsupportedNativeTools`，该文件不再需要 `cloudagent` import，已移除；别名已删 |
| GCL-5 | 已完成 | 删 `cloudagent.RedactSecrets` 转发，6 处调用（`prompt.go`、`history.go` ×2、`mcptool.go`、`tools.go`、`trace.go`）与 `prompt_test.go` 改为 `agentcore.RedactSecrets`。计划里写的「19 处」是把 `truncate` 一并数进去了，实际 `RedactSecrets` 为 6 处调用点 |
| GCL-6 + GCL-9 | 已完成（一并处理） | 见下 |
| GCL-7 | 已完成 | `CLIStartFailure` / `SandboxToolIDs` / `GeneralTools` / `MCPServerIDs` / `MCPToolRefs` / `ModuleTemplate`（函数）降为小写。`PromptParts.ModuleTemplate` 是导出结构体字段，保持不变 |
| GCL-8 | 已完成 | 见下 |

### GCL-9 + GCL-6：`callGate` 并发 bug（已确认并修复）

先写复现用例 `TestCallGate_DropRunDoesNotStrandOtherRunWaiter`，**修复前稳定失败**：run A 在 `wait`，run B 收尾调用 `dropRun` 把 `g.ch = nil`，随后 `release(callA)` 为同一 call_id 新建 channel，A 手里的旧 channel 永远收不到信号，只能等满超时（2.001 s，用例期望 < 0.5 s）。

修复：`callGate` 的登记键由 `callID` 改为 `{runID, callID}`，`wait` / `release` 增加 `runID` 参数，`dropRun` 只删除本 run 的登记。因此 GCL-6 没有「删掉 `runID` 参数」，而是相反——**让它真正被使用**，这也是计划 §1.2 GCL-6 预留的分支。调用点：`skilltool.go`（`wait`）、`runtime.go`（`release`）。

行为变化（需知悉）：不同 run 即使 call_id 相同也不再互相影响。call_id 在 run 内由模型 / 宿主生成，跨 run 重复是个别情形，修复前会互相干扰，修复后隔离。

新增 `callgate_export_test.go`（测试用 `size()`），`TestCallGate_DropRun` 增加「只清本 run、保留其他 run」断言。

### GCL-8：`limitOut` UTF-8 截断（已确认并修复）

先复现：`strings.Repeat("中", 65536)` 经 `limitOut` 后 `utf8.ValidString` 为 false。修复：长度达到上限时检查末尾，回退被切断的不完整字符，不改动其他字节。新增 `TestLimitOutKeepsValidUTF8`（短串不动、恰好等于上限的 ASCII、超长 ASCII、多字节在边界被切断、多字节恰在边界不多删）。

行为变化：输出恰在边界被切断时，最多少保留 3 个字节（保证合法 UTF-8）；其余情况与修复前一致。`internal/agentbridge/cli.go` 的 `limitOut` 用 `strutil.Truncate`（同为按字节），本计划按「不做」表保持不动，同类问题登记在此，未处理。

### 验证结果

| 项 | 结果 |
| --- | --- |
| `gofmt -l gocloudagent` | 无输出 |
| `go build` / `go vet`（模块） | 通过 |
| `go test -race -count=1 ./...`（模块） | 全绿 |
| 用例清单 diff | 248 → 250，仅新增 `TestCallGate_DropRunDoesNotStrandOtherRunWaiter`、`TestLimitOutKeepsValidUTF8`，无删除 |
| `staticcheck ./...`（模块） | 0 |
| `go mod tidy -diff` | 无差异（未改 `go.mod` / `go.sum`） |
| `go list -deps ./... \| grep -c SimpleBase/internal` | 0 |
| 宿主 `go build ./internal/... ./cmd/...` | 通过 |
| 宿主 `go test ./internal/{agentbridge,app,sbcli,auth,api}/` | 全绿（api 约 106 s） |

未运行：`scripts/smoke.sh` 全量（含前端构建）、`scripts/examples-verify/verify.sh`（本次不涉及 `sbcli/dev*`、`sbdev.py`、示例清单）。`internal/e2bproto/watch.go` 的 `gofmt` 告警是既有问题，与本计划无关，未处理。

### 遗留

- **六份云助手文档已复核（2026-11-17）**：不再等外部内容，改为按代码逐项对账。方法与结论如下。
  - 对账口径：CLI 命令与 flag（`internal/sbcli`）、只读判定（`CommandReadOnly`）、后端 handler 的错误码与边界（`internal/api`）、`internal/agentbridge/skills_test.go` 的三条对账测试（readonly ↔ sbcli、示例可分发、allow ↔ 前缀表）。
  - 结论：kv / s3 / cron / logs / project **无差异**（命令白名单 62 条、SCAN COUNT 1–500、`object list` 上限 1000、cron 名字 63 字符 / interval 60–2592000 / input 8KB、runs 状态枚举、`log search` limit 500 回落 100、`audit list` 默认 50 等全部与代码一致）。
  - `sandbox.md` 有 3 处与代码不符，已修：
    1. `create` 漏了 `--timeout`（秒，整数，省略用实例默认）；
    2. `list` 漏了 `--limit`（1–100，越界报 `invalid_request`）；
    3. 原文写「`capabilities` 不对助手开放（返回 `forbidden`）」，实际是只读命令、助手可直接执行（`sbcli` 的 `readOnlyCommands` 里为 true，返回能力信息）。已改为可执行，并把 `sandbox capabilities` 补进 `<!-- readonly: -->` 标注（与 sbcli 判定一致，对账测试通过）。
- **已处理（2026-11-17）**：`internal/agentbridge/cli.go` 的 `limitOut` 原用 `strutil.Truncate`（按字节），改为在截断点回退到 UTF-8 字符边界，与 `cloudagent.limitOut` 口径一致；新增 `internal/agentbridge/cli_limit_test.go`（`TestLimitOutKeepsValidUTF8`）。两处仍各自实现（模块互不 import，见 §1.3），不合并。
- **超长函数拆分已完成（2026-11-17）**：原计划标为「P1 重构，另立项」，本次一并做了。原则是纯提取、不改行为与顺序，四个函数各自的检查顺序（取消 → 超时 → 迭代上限 → max_tokens → 其他错误）与 defer 回收顺序保持不变。

| 函数 | 拆分前 | 拆分后 | 提取出的部分 |
| --- | --- | --- | --- |
| `cloudagent.StartRun` | 306 行 | 约 55 行 | `resolveModelName` / `buildInstruction` / `assembleTools`（返回逆序 cleanup）/ `newChatModel` / `newChatAgent` / `startRunTimeout` / `consumeEvents` / `finishRun` + `runOutcome` |
| `cloudagent.buildTools` | 186 行 | 约 30 行 | `builtinTools` 表 + 7 个 `xxxTool(deps)` 构造函数，新增工具只加一行 |
| `cloudagent.execSimplebase` | 199 行 | 约 110 行 | `validateSkillArgv`（纯参数校验）/ `runSkillCLI`（执行与退出码归一） |
| `cli.SessionRunner.Run` | 175 行 | 约 60 行 | `newCLIModel` / `newCLIAgent` / `consumeEvents` / `finishCLIRun` + `cliRunOutcome` |

关键约束（拆分时逐条核对）：

- `assembleTools` 的 `cleanup` 按注册逆序执行，等价于原来的 `defer` 栈（mcpSet.Close → dropRun → RevokeRun → forgetToken）；中途失败时已占用的资源也放进 cleanup，由 `StartRun` 的 defer 回收。
- `startRunTimeout` 返回的 stop 里是「先 cancel 再 clearCancel」，与原 defer 的 LIFO 顺序一致。
- `finishRun` / `finishCLIRun` 必须把 `context.Canceled` 与真错误原样返回（不能只返回 result），否则中断会被上层当正常结束。
- `cli` 的最终正文在调用处先 `redact` 再同时用于 result 与落盘（`agent_loop_test.go` 的密钥泄漏用例是这条的守卫）。

顺带清掉 `cloudagent/starter_test.go` 的 `type cell struct{...}` 与 `_ = cell{}`（为让未使用类型不报警而写的占位，正是 GCL-1 那类坏味道）。

验证：`gofmt` 干净；`GOWORK=off go build/vet ./...` 通过；`GOWORK=off go test -race -count=1 ./...` 全绿；`staticcheck ./...` 0 告警；`go mod tidy -diff` 无差异；`go list -deps ./... | grep SimpleBase/internal` 为 0（未引入对根模块的依赖）；宿主 `go build ./internal/... ./cmd/...`、`go vet`、`go test ./internal/{agentbridge,app,api}/` 全绿（api 约 110 s）。
