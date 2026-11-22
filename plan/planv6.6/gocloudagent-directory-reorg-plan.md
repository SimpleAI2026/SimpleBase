# gocloudagent 目录整理计划（v6.6）

> **状态：已执行完成（2026-11-17）。** 见文末「§9 执行记录」。
>
> **决策已确认（D-1 拆子包 / D-2 包名 `cloudagent` / D-3 宿主 import 全部迁移，不保留门面），按本文执行。** 下文中的 `cloudagent/`、`package cloud` 均已改为 `cloudagent/`、`package cloudagent`。目标：把 `gocloudagent/` 模块根目录下平铺的 38 个 Go 文件、两份 3700 行的参考文档、命名撞车的 `cli_*` / `mcp_*` 文件理顺。**不改任何行为、协议、权限与安全边界，不动 `go.mod` 依赖。**

## 1. 现状问题（2026-11-17 扫描）

| 编号 | 问题 | 证据 |
| --- | --- | --- |
| P-1 | 模块根包是「云端 Runtime」，却和模块入口门面、文档、bench 混在同一层；根目录 38 个 `.go`（19 个生产 + 19 个测试）+ 6 个杂项文件 | `runtime.go` 644 行、`tools.go` 479、`cli_tool.go` 456、`coverage_test.go` 575 |
| P-2 | **命名撞车**：根 `cli_tool.go`（云端 simplebase CLI 工具）、`cli_runner.go`（CLIRunner 端口）、`cli_entry.go`（本地 CLI 门面）与子包 `cli/`（本地命令行）语义完全不同；根 `mcp_tool.go` / `mcp_access.go` 与子包 `mcp/`、`cli/mcp_tool.go` 同名 | 看文件名无法分辨属于云端还是本地 |
| P-3 | 单文件职责过多：`runtime.go` 含 Runtime 装配、线程占用、取消、历史转换、流消费；`cli_tool.go` 含三个接口、`callGate`、令牌、权限降权、确认文案；`access.go` 把 `agentcore` 别名与数据端口、沙盒端口放一起；`identity.go` 同时放 Principal、DTO、Logger、CommandPolicy | 见各文件 `type`/`func` 清单 |
| P-4 | 测试文件按「迭代编号/来源」而非「被测对象」命名：`c3_startrun_events_test.go`、`cli_tool_planv6_test.go`、`coverage_test.go`、`optimization_test.go`、`tools_more_test.go`；`policy_test.go` 无 `Test*`（只放辅助）；fake（`fakeDB`、`skillRecorder`）定义在无关测试里被跨文件使用 | 同上 |
| P-5 | 参考文档 `eino-readme.md`（2311 行）、`pi-readme.md`（1439 行）放在模块根，且大量仍写 `internal/gocloudagent`（已迁出）、`internal/gocloudagent/model.go`（已不存在，现为 `agentcore/model.go`）；相对链接 `../AGENTS.md` 语义已过期 | `grep internal/gocloudagent` |
| P-6 | 文件头「文件说明 / 调用架构」注释是自动生成，上游写 `internal/agentbridge, internal/api`，部分内容已过时（如 `access.go` 还写 `ResetUnsupportedNativeTools` 是 `var`，实为别名转发） | `access.go`、`cmd/gocloudagent/main.go` |
| P-7 | `skills.go`（265 行）既是技能解析（frontmatter / allow / readonly）又在根包导出 `AllowArgv` 等，和 `skills/` 内嵌资源包分离，路径上看不出关联 | `skills.go` vs `skills/embed.go` |
| P-8 | `cli/` 25 个文件平铺，MCP（`mcp.go` / `mcp_command.go` / `mcp_tool.go`）、会话（`session.go` / `compaction.go`）、REPL 混居；`cli/mcp_tool.go` 与根 `mcp_tool.go` 各自实现一套「分类 + 确认 + 调用」 | 目录列表 |

宿主侧依赖面（决定改动半径）：`internal/` 下 33 个文件 `import ".../gocloudagent"`（约 60 个符号：`Runtime`、`Event`、`Principal`、`StreamDelta`、`ClassifyError`、`ModuleXxx`、`AllowArgv` 等），另有 `gocloudagent/agentcore`（1）、`gocloudagent/cli`（1）、`gocloudagent/mcp`（3）。**因此根包 import 路径必须保持不变**，否则一次改动波及整个 `internal/api` 与 `internal/agentbridge`。

## 2. 目标结构

```text
gocloudagent/
  go.mod  go.sum  README.md
  gocloudagent.go          # 门面：对 cloud 包导出符号做 type 别名 / 常量 / 函数转发（宿主 import 路径不变）
  cli_entry.go             # RunCLI / SetCLIVersion（保持）
  cmd/                    # 本地 CLI 入口（不变，仍只 import 根包）
  cloudagent/                   # 云端 Runtime（原根包实现，package cloud）
    runtime.go             #   Runtime / RunRequest / RunResult / StartRun
    runthread.go           #   ClaimThread / ReleaseThread / IsRunActive / CancelRun
    history.go             #   historyToSchema / toolHistorySummary / buildSnapshot
    stream.go              #   consumeAssistant / 事件映射
    types.go               #   Principal / Agent / Message / LogEvent / LLMSettings / CommandDecision
    ports.go               #   DatabaseAccess / ObjectAccess / LogAccess / Sandbox / SettingsAccess / Logger / CommandPolicy / MCPConnector / MCPPolicy
    tools.go               #   buildTools / wrapToolErrors（只读数据工具）
    tools_sandbox.go       #   buildSandboxTools
    skilltool.go           #   buildSkillTools / execSimplebase / writableFor / stripWritePermissions
    callgate.go            #   callGate（串行化确认）
    delegation.go          #   DelegationIssuer / ConfirmationWaiter / 令牌与 TTL
    cliexec.go             #   CLIRunner / SubprocessCLI / ValidateCLIPath（原 cli_runner.go）
    mcptool.go             #   云端 MCP 工具（原 mcp_tool.go）
    mcpaccess.go           #   MCPToolRef 等（原 mcp_access.go）
    modules.go  prompt.go  errors.go  trace.go
  agentcore/               # 不变（与运行时无关的模型协议 / 流 / 脱敏）
  mcp/                     # 不变（Session / 传输 / 命名）
  tools/                   # 不变（本机工具）
  cli/                     # 本地命令行；内部仅做文件重命名（§4 P3）
  skills/                  # 内嵌文档 + 技能解析（原根 skills.go 迁入，§4 P2）
  docs/
    eino-guide.md  pi-guide.md   # 原 eino-readme.md / pi-readme.md，修正过期路径
  bench/                   # 不变（Python 评测 harness，与 Go 代码解耦）
```

依赖方向（保持单向，无新增跨模块依赖）：

```text
cmd/gocloudagent → gocloudagent(门面) → cloud ─┬→ agentcore
                                        │      ├→ mcp
                                        │      └→ skills
                                        └→ cli ─┬→ agentcore ├→ tools ├→ mcp └→ gosandbox/host
```

约束：`cloud` 不得 import 根包 `gocloudagent`（会成环）；`cli` 不得 import `cloud`（本地 CLI 与云端 Runtime 互不依赖，只共享 `agentcore` / `mcp` / `tools`）；`agentcore` 不 import 上层。

### 决策点（请确认）

- **D-1 是否拆出 `cloudagent/` 子包**：推荐「是」，门面兜底，宿主 33 个文件零改动。保守替代方案是只在根包内重命名 / 拆文件（即只做 P1、P4、P5），代价是 P-1、P-2 只缓解不根治。
- **D-2 `cloud` 包名**：候选 `cloud` / `cloudagent` / `runtime`（后者与标准库 `runtime` 冲突，不推荐）。
- **D-3 门面的存续**：本轮只加别名；后续是否把宿主 import 逐步迁到 `cloud` 另行评估，不在本计划内。

## 3. 重命名对照（根包 → 目标）

| 现文件 | 目标 | 说明 |
| --- | --- | --- |
| `runtime.go` | `cloudagent/runtime.go` + `runthread.go` + `history.go` + `stream.go` | 按职责切四份，每份 < 250 行 |
| `tools.go` | `cloudagent/tools.go` + `tools_sandbox.go` | 沙盒工具独立 |
| `cli_tool.go` | `cloudagent/skilltool.go` + `callgate.go` + `delegation.go` | 消除与 `cli/` 的撞名 |
| `cli_runner.go` | `cloudagent/cliexec.go` | 同上 |
| `cli_entry.go` | 保持在根 | 唯一保留在根的生产实现 |
| `mcp_tool.go` / `mcp_access.go` | `cloudagent/mcptool.go` / `mcpaccess.go` | 去下划线，区别于 `mcp/` 子包 |
| `identity.go` | `cloudagent/types.go` + `ports.go`（`Logger` / `CommandPolicy` / `SettingsAccess` 部分） | |
| `access.go` | `cloudagent/ports.go`；`agentcore` 别名组移入门面 | 别名只在一处维护 |
| `modules.go` `prompt.go` `errors.go` `trace.go` | 同名迁入 `cloudagent/` | |
| `skills.go` | `skills/` 包（`skills.AllowArgv` 等），`cloud` 与门面转发 | 需先核实 `SkillsForModule` 对 `ModuleXxx` 常量的依赖，必要时改为入参传字符串，避免 `skills → cloud` 成环 |
| `eino-readme.md` / `pi-readme.md` | `docs/eino-guide.md` / `docs/pi-guide.md` | |

测试迁移（随被测对象同目录，`package cloud`）：

| 现测试 | 目标 |
| --- | --- |
| `runtime_test.go` `run_limits_test.go` `c3_startrun_events_test.go` `optimization_test.go` `model_native_test.go` | 合并为 `runtime_test.go`（生命周期）、`runtime_limits_test.go`、`runtime_stream_test.go`（按 `Test*` 主题归并，不丢用例） |
| `coverage_test.go`（575 行，混 Modules / Prompt / Gateway / Runtime） | 拆回 `modules_test.go` `prompt_test.go` `runtime_history_test.go` `model_test.go`，原文件删除 |
| `cli_tool_planv6_test.go` `failclosed_test.go` `writable_test.go` `policy_test.go` | `skilltool_test.go` `callgate_test.go`、`failclosed_test.go`、`writable_test.go`；`policy_test.go` 的辅助挪到 `testutil_test.go` |
| `tools_test.go` `tools_more_test.go` `sandbox_tools_test.go` | `tools_test.go` `tools_sandbox_test.go` |
| `mcp_tool_test.go` `skills_test.go` `trace_test.go` `errors_test.go` `prompt_test.go` `cli_entry_test.go` | 随源文件走；`cli_entry_test.go` 留根 |
| 跨文件复用的 fake（`fakeDB` `skillRecorder` `c3ErrDB` `c3NoUsageLLM`…） | 集中到 `cloudagent/testutil_test.go` |

## 4. 分阶段实施

每阶段单独可提交、可回滚；阶段结束必须满足 §5 验收。

### P0 基线（零改动）

1. 记录基线：`GOWORK=off go test -race ./...`（gocloudagent）与 `go build ./internal/... ./cmd/...` + `go test ./internal/api/ ./internal/agentbridge/ ./internal/app/` 全绿，保存用例清单：`go test -list '.' ./... > /tmp/gca-tests-before.txt`，用于 P1/P2 后核对**用例数不减**。
2. 导出符号清单：`go doc -all .`（根包）存档，门面完成后逐项比对。

### P1 文档与注释（纯文本，零代码风险）

1. `eino-readme.md` / `pi-readme.md` → `docs/`；修正 `internal/gocloudagent/...` 为 `gocloudagent/...` 与 `agentcore/model.go`，修正指向 `AGENTS.md` 的相对链接。
2. 更新 `README.md` 的目录结构、模块边界、`docs/` 入口；更新仓库根 `AGENTS.md`「代码结构」段、`.gitignore` 的注释（若有路径变化）。
3. 文件头自动生成的「调用架构」注释与迁移后真实位置对齐（随各阶段同步，不单独大面积重写）。

### P2 根包拆子包（核心）

1. 新建 `cloudagent/`，用 `git mv` 保留历史，逐文件迁移，每迁一批即 `go build` 通过；先迁叶子（`errors` `trace` `modules` `prompt`），再 `types/ports`，最后 `runtime/tools/skilltool`。
2. 按 §3 拆 `runtime.go`、`cli_tool.go`、`tools.go`、`identity.go`、`access.go`；**只搬移与重命名文件，不改函数体**；可见性不变（私有符号保持私有）。
3. 根包 `gocloudagent.go` 写门面：对外导出的 ~60 个符号（类型用 `type X = cloud.X`，常量 / 变量 / 函数显式转发，包括 `ModuleXxx`、`ErrThreadBusy`、`AllowArgv`、`ClassifyError`、`ChatXxx`、`StreamDelta`、`ResetUnsupportedNativeTools`）。以 §P0 的导出清单为准，做到一一对应。
4. `skills.go` 迁入 `skills/`：保持 `go:embed` 路径不变；`EmbeddedSkillAllows` / `EmbeddedSkillReadOnly` 的行为与 `internal/agentbridge` 的对账测试不变。
5. 测试按 §3 迁移与归并，集中 fake。
6. 宿主侧**不改 import**；若门面缺符号编译会失败，据此补全。

### P3 `cli/` 内部整理（仅文件重命名，不拆包）

1. MCP 相关三件 `mcp.go` `mcp_command.go` `mcp_tool.go` 重命名为 `mcp_config.go` `mcp_cmd.go` `mcp_adapter.go`（含测试），文件头标明「本地 CLI 的 MCP 装配」，与根云端实现区分。
2. `runner.go`（本地 agent 循环）重命名为 `agent_loop.go`，避免与 `cloudagent/cliexec.go` 的 `CLIRunner` 混淆。
3. 调研（只出结论，不在本阶段改）：`cli/mcp_tool.go` 与 `cloudagent/mcptool.go` 的「分类→确认→调用」是否可下沉为 `mcp/` 的共享片段；若收益小则登记为不做。

### P4 外围脚本与引用

核对并必要时更新：`build.sh`（`gocloudagent/cmd/gocloudagent` 路径不变，预期无需改）、`Dockerfile`（只复制 `go.mod/go.sum`，不变）、`scripts/test.sh` / `scripts/smoke.sh`（`(cd gocloudagent && ...)` 不变；`scripts/test.sh:21` 注释里的 `internal/gocloudagent` 过期字样清理）、`bench/run_smoke.py` 对 `gocloudagent/cmd` 的构建路径（不变）。`plan/` 历史文档里的旧路径**不追改**，仅在本文登记。

## 5. 验收标准

```bash
cd gocloudagent
gofmt -l .                                                     # 无输出
GOWORK=off go build ./... && GOWORK=off go vet ./...
GOWORK=off go test -race ./...                                 # 用例数 >= P0 基线
go list -deps ./cloud | grep -c 'SimpleBase/internal'          # 0（不得依赖根模块）
go list -deps ./cli   | grep 'gocloudagent/cloud'              # 无输出（cli 不依赖 cloud）
cd .. && go build ./internal/... ./cmd/... && go test ./internal/api/ ./internal/agentbridge/ ./internal/app/
./scripts/test.sh && ./scripts/smoke.sh
python3 internal/agentbridge/bench/run_smoke.py --self-check
```

附加：根目录 `.go` 文件 ≤ 3 个（`gocloudagent.go`、`cli_entry.go`、`cli_entry_test.go`）；`cloudagent/` 内单个源文件 ≤ 300 行；`docs/` 内无 `internal/gocloudagent` 字样；`internal/` 的 import 行 `git diff` 为 0。

## 6. 风险与对策

| 风险 | 对策 |
| --- | --- |
| 门面漏符号，宿主编译失败 | P0 导出清单逐项比对；P2 末尾以宿主编译为判据 |
| 类型别名改变反射 / `%T` 输出或日志类型名 | 别名不改变类型身份，仅 `%T` 里的包名由 `gocloudagent` 变 `cloud`；全仓 grep 无依赖 `%T` 的断言，已在 P2 中复核 |
| `skills` 迁移引入循环依赖 | 先核实 `SkillsForModule` 对模块常量的使用；必要时让入参改为字符串，常量留在 `cloud` |
| 测试合并丢失用例 | P0/P2 对比 `go test -list` 清单 |
| `git mv` 与重命名同时发生导致历史难追 | 重命名与拆分分提交：先纯 `git mv`，再拆文件 |
| 与 `agentbridge`、`internal/api` 并行改动冲突 | 本计划保证宿主 import 零改动；P2 期间避免并行改动 `gocloudagent/` |
| 触碰 `AGENTS.md`「依赖方向」约束 | P1 同步更新 `AGENTS.md` 与 `gocloudagent/README.md`，并保留「不得 import 根模块」「`cli` / `tools` 挂在 gocloudagent 下」两条 |

## 7. 不做的事

- 不改 `agentcore` / `mcp` / `tools` 的包边界与对外 API。
- 不改 `bench/`（Python 与题库保持原位，只核对路径）。
- 不新增 / 升级依赖，不改 `go.mod` / `go.sum`。
- 不改 HTTP 协议、CLI 命令行为、`~/.gocloudagent/` 会话格式、fail-closed 与权限降权语义。
- 不把宿主 `internal/` 的 import 改到 `cloud`（见 D-3）。

## 8. 工作量与顺序

P0（0.5h）→ P1（1h）→ P2（4–6h，最大头）→ P3（1h）→ P4（0.5h）。P1 可独立先行合入；P2 之后必须一次性通过 §5 全部验收再合入。

## 9. 执行记录（2026-11-17）

与计划的差异已在此登记，正文保持原计划口径。

| 项 | 计划 | 实际 |
| --- | --- | --- |
| 门面 | 根包 `gocloudagent.go` 对外转发约 60 个符号 | **取消**（D-3 改为「全部迁移」）：宿主 33 个文件全部改为 `import ".../gocloudagent/cloudagent"`；根包只剩 `cli_entry.go`（`RunCLI` / `SetCLIVersion`）与其测试 |
| 技能 API | `skills.go` 迁入 `skills/` | 已迁入，并去掉 `Skill` 前缀：`skills.AllowArgv / Prompt / Body / Known / IDs / Prefixes / EmbeddedAllows / EmbeddedReadOnly`，常量 `skills.MaxChars`。`SkillsForModule` 依赖 `ModuleXxx` 常量，**留在 `cloudagent/modules.go`**，避免 `skills → cloudagent` 成环；`truncateSkill` 留在 `cloudagent/prompt.go` |
| `cloudagent` 拆分 | `runtime.go` → 4 份；`cli_tool.go` → 3 份 | `runtime.go` → `run_types.go` / `runtime.go` / `runthread.go` / `history.go` / `stream.go`；`cli_tool.go` → `skilltool.go` / `callgate.go` / `delegation.go`；`tools.go` → `tools.go` / `tools_sandbox.go`；`identity.go` → `types.go` / `ports_host.go`；`access.go` → `ports.go`；`cli_runner.go` → `cliexec.go`；`mcp_tool.go` / `mcp_access.go` → `mcptool.go` / `mcpaccess.go` |
| 测试归并 | 按被测对象重命名 | `coverage_test.go` 拆为 `modules_test.go` / `model_test.go` / `runtime_history_test.go`，`TestPromptRedactAndTruncate` 并入 `prompt_test.go`，fake 与 `testPolicy` 集中到 `testutil_test.go`；`c3_*` → `runtime_stream_test.go`，`cli_tool_planv6_test.go` → `skilltool_test.go`，`optimization_test.go` → `runtime_optimization_test.go`，`run_limits_test.go` → `runtime_limits_test.go`，`sandbox_tools_test.go` → `tools_sandbox_test.go`。原 `TestPromptTemplateExpansionDoesNotGrantSkillPermission` 等 3 个技能用例随 `skills` 迁入 `skills/skills_test.go` |
| `cli/` 重命名 | `mcp*.go` / `runner.go` | `mcp.go`→`mcp_config.go`、`mcp_command.go`→`mcp_cmd.go`、`mcp_tool.go`→`mcp_adapter.go`（含测试）、`runner.go`→`agent_loop.go`（含测试）；P3 第 3 点「共享片段下沉」未做，登记为不做（两边确认 / 事件语义不同，收益小） |
| `docs/` | 修正过期路径 | 已迁并修正 `internal/gocloudagent*` 引用与交叉链接 |
| 跨包路径 | — | `skilltool_test.go` 的 `TestSubprocessCLI_DatabaseListReadOnly` 因目录下沉一层，仓库根相对路径改为 `../..` |

验收结果：

- `gocloudagent`：`gofmt -l` 空、`go build` / `go vet` 通过、`go test -race ./...` 全绿；`go test -list` 用例清单与 P0 基线逐项一致（248 个）。
- 依赖：`go list -deps ./... | grep SimpleBase/internal` = 0；`cli` 不依赖 `cloudagent`；`skills` / `agentcore` 不依赖上层。
- 宿主：`go build ./internal/... ./cmd/...`、`go vet`、`go test ./internal/api/ ./internal/agentbridge/ ./internal/app/ ./internal/sbcli/ ./internal/auth/` 全绿。
- `bench/run_smoke.py --self-check`（`GOCLOUDAGENT_BENCH_RUN_ROOT=/tmp/sbbench`）ok；`cmd/gocloudagent` 可构建并运行 `version`。
- 根目录 `.go` 文件 2 个（`cli_entry.go`、`cli_entry_test.go`）；`cloudagent/` 单个源文件最大 366 行（`runtime.go`），略高于计划的 300 行目标，`StartRun` 单函数约 300 行，继续拆需改函数体，不在「只搬移不改行为」范围内，留作后续优化。
- 未执行：`scripts/smoke.sh` 全量（含前端构建）与 `scripts/examples-verify/verify.sh`，本次改动不涉及 `sbcli/dev*`、`sbdev.py`、示例清单。

追加（2026-11-17）：入口目录由 `cmd/gocloudagent/main.go` 扁平化为 `cmd/main.go`（`package main`，`go build ./cmd`）；`build.sh`、`bench/run_smoke.py`、`bench/README.md`、`README.md`、`AGENTS.md` 及相关代码注释同步。
