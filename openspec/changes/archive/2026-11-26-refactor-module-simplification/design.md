# Design

依据：`skills/simplify-code/SKILL.md`（Refactor 模式）及 `references/refactoring.md`、`references/assessment.md`。评估基于 2026-11-26 工作区（HEAD `fe37cfb` 加上未提交改动），用只读方式采样：按文件行数、超过 90 行的函数、跨包同名私有函数三条线筛选。这是采样结论，不代表全仓库已经没有其他问题。

## 冻结契约（Frozen surface）

- 导出符号、签名、类型别名的可见性；`gofunction` 门面（`gofunction.go` / `jsonrun.go`）保持原样。
- HTTP 路径、错误码与文案、SSE 帧、`/runs` 与群轮次的状态迁移（唯一权威在 `gocloudagent/group/state.go`）。
- `gosandbox/proto` 的帧字节布局、字段顺序、长度上限，以及解码失败时的错误文本。
- `internal/config` 的覆盖顺序（默认值 → YAML → `SIMPLEBASE_*` env）、键名、拒绝密钥落 YAML 的行为、`Redacted()` 输出的 map 结构。
- 依赖方向：R1–R10 规则；四个嵌套模块只能 import 自身（`gocloudagent` 另外允许 `gosandbox`）。`internal/utils` 只能 import 标准库。

## 可变内部面（Free surface）

私有函数、同包文件划分、私有 helper 边界。改动前先确认没有反射、`go:linkname`、字符串引用，也不在 build tag 分支里。

## 候选清单

优先级定义见 `references/assessment.md`：P1 表示维护负担大、收益明确；P2 表示收益有界的局部收敛；P3 是可选项。

| ID | P | 位置 | 问题 → 方案 | 风险 | 分类 |
| --- | --- | --- | --- | --- | --- |
| A-01 | P1 | `internal/api/{data,file,function,sandbox}_compat.go` | 21 个迁移期别名，仓外调用只剩 `app/assemble_api.go` 的 `api.FunctionTriggerHandler`。方案：调用方和同包测试直接引用 `<context>http`，删除 compat 文件；`sandboxDeps` 保留并移到 `router.go` | 中：会改 `TestKernelResidueFiles` 清单，`internal/api` 的导出别名会减少 | apply（`internal/` 不对外发布，调用方都已知；同步门禁清单和文档） |
| A-02 | P2 | `internal/config/env.go` `applyEnv`（174 行） | 按配置域拆成 `applyEnvHTTP/Backup/Sandbox/Agent…`，写法和 YAML 侧已有的 `applyYAMLHTTP` 一致；赋值顺序不变 | 低 | apply |
| A-03 | P2 | `internal/config/yaml.go` `applyYAML`（147 行） | 把 instance / storage / database / auth 段抽成 `applyYAML<Domain>`，密钥探测仍汇总到 `secrets` | 低 | apply |
| A-04 | P3 | `internal/config/{config.go defaults, redact.go Redacted}` | 按域拆出私有构造函数，map 键和值保持不变 | 低 | apply（可选） |
| C-01 | P1 | `gocloudagent/group/engine.go`（1407 行） | 在同包内按职责拆文件：`engine.go`（构造 / Submit / launch）、`engine_exec.go`（execute / runOne / settle / replan / summarize）、`engine_control.go`（ConfirmPlan / Cancel / RetryTask / ReplyHuman / Resume / Shutdown）、`engine_state.go`（transition* / revive* / failTurn / load*）、`engine_frames.go`（`*JSON` 帧载荷）。只移动代码，不改逻辑 | 中：状态迁移和锁不改，只搬位置 | apply（纯搬移检查点） |
| C-02 | P2 | `gocloudagent/group/engine.go` `execute`（119 行） | 把单轮循环体抽成 `step(...) (done bool)`；`defer` 释放的作用域留在 `execute` | 中：涉及早退出、`b.finish` 次数、ctx 取消顺序 | apply（需要先补交错测试，见 tasks 3.2） |
| C-03 | P3 | `gocloudagent/group/engine.go` `truncate`、`cloudagent/prompt.go` `truncate` | 一行转发，可以直接调用 `agentcore.Truncate`；group 的 2000 上限提成具名常量 | 低 | apply |
| C-04 | P2 | `gocloudagent/plan/validate.go` `Validate`（122 行） | 按校验阶段（结构 / 成员 / 依赖图）抽函数，错误产生的优先级不变 | 中：校验顺序决定哪个错误先返回 | apply（先补多错误输入的特征测试） |
| C-05 | P2 | `gocloudagent/cli/group_runtime.go`（815 行） | 把 `groupRunner` / `groupMembers` / `lockedWriter` 移到 `group_runner.go`；交互提示（prompt*、readLine）移到 `group_prompt.go` | 低 | apply（纯搬移） |
| S-01 | P1 | `gosandbox/proto/codec.go` `Encode` / `Decode`（219 / 269 行 switch） | 每个 `Kind` 拆成成对的私有 `encodeX` / `decodeX`，`switch` 只负责分发；同一个 Kind 的编码和解码放在相邻位置，方便对照 | 中：wire 协议 | apply（`codec_contract_test.go` 已有；再补全 Kind 往返和字节快照用例后执行） |
| S-02 | P2 | `gosandbox/agent/pty_linux.go` 与 `gosandbox/host/pty_{linux,darwin}.go` | `openPTY` 收敛到 `gosandbox/internal/ptyx`（`Open` / `SetWinsize`，分 linux / darwin / 其他平台三份实现）。窗口大小策略留给调用方：agent 只在 rows、cols 都大于 0 时设置，失败忽略；host 总是设置，失败就关闭两端并返回错误 | 中 | apply（2026-11-26 已批准，结论见下文「已批准项结论」） |
| S-03 | P3 | `gosandbox/host/pty_linux.go` `itoa`、`gocloudagent/group/scheduler.go` `itoa`、`internal/api/sandboxhttp/e2b_envd_handler.go` `itoa` | 手写整数转字符串，换成 `strconv.Itoa`；host 侧的 `itoa` 随 S-02 一起删除 | 低 | apply（host 侧 2026-11-26 已批准） |
| S-04 | P2 | `gosandbox/host/bwrap_args.go` `BwrapArgs`（126 行） | 按挂载 / 网络 / 环境 / 命令几段抽私有函数，参数顺序必须逐字节一致 | 中：参数顺序就是行为 | apply（先补 golden argv 用例） |
| F-01 | P3 | `gofunction/internal/interp/{eval.go binop, compilefast.go binopSpecialized}` | 两个大 switch 结构相似，但一个是热路径特化，一个是通用兜底 | 高：性能敏感 | skip（重复是性能特化有意为之；没有基准支持就不合并） |
| X-01 | P3 | `gocloudagent/cli/config.go` 与 `internal/sbcli/config.go` 的 `envGet` | 实现完全相同，但两者分属不同模块，R9 禁止共享 | — | skip（少量独立重复比跨模块耦合更合适） |
| X-02 | P3 | `internal/objectstore/keys.go` `joinKey` 与 `godatabase/blob/keys.go` `joinKey` | 两份实现逐字相同，objectstore 也已经依赖 blob。方案：blob 导出 `JoinKey`，删除 objectstore 的副本 | 中：给 `godatabase` 新增公共 API（只增不改） | apply（2026-11-26 已批准） |

### 已批准项结论（2026-11-26）

- **`O_CLOEXEC` 差异无实际效果**：Go 的 `os.OpenFile` 在 unix 上总会追加 `O_CLOEXEC`（`os/file_unix.go` 的 `openFileNolog`），所以 host 显式加不加这个 flag，结果都一样。子进程的 stdio 由 `exec.Cmd` 经 dup2 继承，不受影响。
- **在 master 上还是 slave 上设置 `TIOCSWINSZ` 等价**：两者指向同一个 tty。`ptyx.SetWinsize` 对两边都适用。
- **接受的可观察差异**（随本次批准一起接受）：
  1. host 打开 PTY 失败时，错误多一段阶段前缀，例如 `open pty: unlock pty: …`（原来是 `open pty: …`），错误类别仍是 `unavailable`。
  2. agent 在非 Linux 平台上的桩错误文案改由 `ptyx` 统一给出。agent 只在 Linux 沙盒里运行，这条路径实际不可达。
- `JoinKey` 是新增导出函数，不改变已有符号；objectstore 测试里对 `joinKey` 的断言移到 blob 包，断言内容不变。

### 明确不在本变更内

- `godatabase/**`：由 `add-godatabase-runtime-refactor` 负责（F/D/K/C/R 系列）。
- `internal/api/perfbench`（`dbCases` 290 行等）：K3 计划把整个包外移，外移之前不拆。
- `gosandbox/vm/fake`、`gofunction/bench`：测试 / 基准辅助代码，收益低。
- 跨模块共享 helper：会违反 R9，或者需要新增公共 API。

## 迁移策略

- `internal/` 内部的调用方都已知，A-01 用原子迁移：同一个检查点里改调用方、删除 compat 文件、更新门禁清单，不保留兼容别名。
- 嵌套模块的导出符号可能有未知外部使用者，一律不删不改，只调整私有结构。
- 每个检查点只搬移一项职责。纯搬移（C-01、C-05）和逻辑抽取（C-02、C-04、S-01、S-04）分成不同提交，方便按 rename 对比 diff。

## 验证

| 范围 | 命令 |
| --- | --- |
| 根模块 | `CGO_ENABLED=1 go build ./internal/... ./cmd/... && go vet ./internal/... ./cmd/...`；`go test ./internal/config/ ./internal/api/... ./internal/app/ ./internal/agentbridge/` |
| 门禁 | `go test ./internal/app -run TestArchitectureGate`；`go test ./internal/api -run TestKernelResidueFiles` |
| gocloudagent | `(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...)`；group 改动另跑 `-race ./group/...` |
| gosandbox | `(cd gosandbox && go build ./... && go vet ./... && go test ./...)`；Linux 分支（`*_linux.go`）需要 `GOOS=linux go vet ./...`，或在 Linux CI 上验证 |
| 格式 | `gofmt -l` 无输出 |
| Agent 协作 | 改动 group 后跑 `go test -race ./internal/agentrun/ ./internal/api/ ./internal/app/` |

基线（2026-11-26 实测）：四个嵌套模块 `go build ./...` 通过；根模块 `go build ./internal/... ./cmd/...` 通过；`TestArchitectureGate` 和 `TestKernelResidueFiles` 通过。全量 `go test` 基线在实施第 0 批时补记录。

另外：A-01 实施前 `internal/api` 有 29 个顶层生产文件，删除四个 compat 文件后为 25 个；以 `kernel_residue_test.go` 为准，并同步文档中的旧计数与转发层描述。

## 回滚

只撤销当前候选自己的改动；不整文件 restore，不 `reset --hard`。纯搬移出现回归时，先确认是否混进了逻辑改动。

## 实施记录

### 1.1 基线（2026-11-26）

- 工作区基线为 HEAD `fe37cfb` 加既有未提交改动：`git status --short` 显示大量 DDD 迁移中的修改、删除与新增文件，本变更在其上继续实施，不覆盖或回退这些改动。
- `CGO_ENABLED=1 go build ./internal/... ./cmd/... && CGO_ENABLED=1 go vet ./internal/... ./cmd/...`：通过。
- `CGO_ENABLED=1 go test ./internal/config/ ./internal/api/... ./internal/app/ ./internal/agentbridge/`：已有失败 `TestNewLocalStorageAppUsesSystemDatabase`（`app_test.go:147: catalog must not live in cache_dir`）；其余所列包通过。
- `CGO_ENABLED=1 go test ./internal/app -run TestArchitectureGate`、`CGO_ENABLED=1 go test ./internal/api -run TestKernelResidueFiles`：通过。
- `(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...)`：通过。
- `(cd gosandbox && go build ./... && go vet ./... && go test ./...)`：通过。
- `(cd godatabase && GOWORK=off go build ./...)`、`(cd gofunction && go build ./...)`：通过。
- 对本次涉及目录执行 `gofmt -l`：无输出。

### 3.4–3.6 gocloudagent 收尾（2026-11-26）

- **3.4（C-04）** `plan.Validate`：结构检查（action / update·complete 不得改 dispatch / goal / 空 / 数量上限）留在 `Validate`，按原顺序调用 `validateMembers`（返回 `refIdx`）→ `validateDependencyGraph` → `validateSteps` → `validateComplexSteps`。各阶段的 `return err` 只改成 `return nil, err`，错误优先级不变；1.4 的 `TestValidateFirstErrorPrecedence` 10 条通过。
- **3.5（C-05）** `cli/group_runtime.go`（815 行）拆成三个文件：`group_prompt.go`（`promptPlanConfirm` / `promptEdits` / `promptHumanReply` / `printWaitingHuman` / `readLine`）、`group_runner.go`（`groupMembers`、`lockedWriter`、`groupRunner`、`memberTools`、`instruction`、`sessionRequestsReplan`），其余留在 `group_runtime.go`。
  - 按行号切片搬移，并以 `git show HEAD:gocloudagent/cli/group_runtime.go` 为基准逐行比对三个文件的正文：723 个非空行逐行一致、每个恰好出现一次，只有 2 个分隔空行（原 555 / 656 行）被相邻空行吸收。
  - `group_runtime.go` 相对 HEAD 的新增行只有文件头注释，另去掉随 `lockedWriter` 搬走的 `sync` import。用法块里的签名原来与实际不符（缺 `yes`、`reply` 参数、`status(w)`），重写文件头时对齐为 `submit(ctx, text, interactive, yes)` / `reply(ctx, outcome, text)` / `status()`。
  - design 未点名的两个小辅助 `osUser`（`turnInput` 与 `groupMembers.Group` 共用）、`shortID`（只有 `status` 用）留在 `group_runtime.go`；`printWaitingHuman` 是人工待办的终端提示且被 `promptHumanReply` 调用，随提示代码放到 `group_prompt.go`（`follow` 与 `repl.go` 的调用不受影响）。
  - 观察到的遗留（纯搬移不改）：`groupRunner.in` 只在 `newGroupRuntime` 里赋值，没有任何读取；读取走的是 `lineReader`。
- **命令与结果**
  - `(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./... -count=1)`：3.4 后、3.5 后各跑一次，均通过。
  - `gofmt -l`（`gocloudagent/{plan,group,cli,cloudagent}`）：无输出。
  - 3.6 `CGO_ENABLED=1 go test -race -count=1 ./internal/agentrun/ ./internal/api/ ./internal/app/`：`agentrun`、`api` 通过，0 个 data race；`app` 只有基线已有失败 `TestNewLocalStorageAppUsesSystemDatabase`（`app_test.go:147: catalog must not live in cache_dir`），与 1.1 记录一致。

### 4.1–4.4 gosandbox（2026-11-26）

- **4.1（S-01）** `proto/codec.go`：`Encode` / `Decode` 只按 Kind 分发；每种布局一对 `encodeX(m *Msg) ([]byte, error)` / `decodeX(kind Kind, payload []byte) (Msg, error)`，成对相邻，分组与顺序同 `proto.go` 的 Kind（会话 / 进程 / 文件 / 目录监听 / 观测 / 中继）。同布局的帧共用一对：Ping / Pong、Ack / FsWriteAck，以及原先靠 `if m.Kind == …` 在一个分支里分叉的请求组——拆成 `request`（Seq + ID：CloseStdin / StatsQuery / TopQuery / ListenQuery）、`pathRequest`（再加 Path：FsStat / FsList / FsRemove / FsMkdir）、`fsRead`（再加 Offset + Size）。Shutdown 无字段，保留为分发里的空分支。
  - 等价性以 `git show HEAD:` 为基准机械比对：27 对单布局函数的解码体逐字等于原 case 分支，编码体逐字等于原分支（1 条注释留在原位，6 条原样上移为函数文档）；请求组按原分支的 if 条件逐 Kind 推出字段序列，与新函数一致。
  - 刻意保留的行为：`Decode` 出错时返回已解出的字段、未知 Kind 返回带 Kind 的 `Msg`；不检查尾部多余字节；`Encode(Shutdown)` 返回非 nil 空切片（golden 用 `reflect.DeepEqual` 比对 `hex.DecodeString("")` 的结果，不能改成 `nil`）。
  - 文件头重写（原「同包」清单缺 codec_golden_test.go，符号清单已过期）。文件 786 → 1281 行，增量是各对函数的前置（构造 writer / reader、`var err error`）与收尾。
  - 结果：proto 92 个子测试通过（39 个 Kind 的 golden 字节一致、契约测试、错误文本）。
- **4.2（S-04）** `host/bwrap_args.go`：`BwrapArgs` 只做校验、解析 `exists` / `isDir` 探针，再按原顺序拼接五段：`policyMountArgs`（/tmp tmpfs → 可写绑定 → 只读路径 → deny_read，唯一会出错的段）→ `systemMountArgs`（/dev /proc → /dev/shm → masked / readonly）→ `namespaceArgs` → `innerPolicyArgs`（cap-drop + 策略文件）→ `commandArgs`（chdir + helper + argv）。`mode` / `fd` / `home` 是无副作用的计算，分别挪进用到它们的段。
  - 除 1.3 的 golden 外做了一次**不入库**的差分验证：把 HEAD 的旧函数原样拷进临时测试，对 40059 个输入比较新旧输出——38715 个有效输出的 argv 逐字节一致、1344 个错误的文本一致，覆盖 16253 种不同 argv；临时文件已删除。
  - 结果：host 包 18 个 BwrapArgs 相关测试（含 golden）通过。
- **4.3（S-03）** `gocloudagent/group/scheduler.go` 的 `Progress` 与 `internal/api/sandboxhttp/e2b_envd_handler.go` 的 `h.record` 改用 `strconv.Itoa`，删除这两份手写 `itoa`；入参分别是 `done` / `len(tasks)` / `len(data)`，均为非负，输出逐值相同。
  - host 侧手写原因已查清：`itoa` 由 2026-10-20 的提交 `7fc4715` 引入，没有注释和提交说明；只在 `openPTY` 里拼一次 `/dev/pts/<n>`，不是热路径；同模块的 `agent/pty_linux.go` 做同一件事用的是 `fmt.Sprintf`，所以不是包内约定；与 `strconv.Itoa` 的差别只在 `n < 0`（返回空串），而 `n` 来自 `TIOCGPTN` 不可能为负。结论：没有需要保留的理由，按 5.1 / 5.3 随 PTY 收敛一起删除。另确认 `golang.org/x/sys v0.40.0` 没有 `unix.PtsName`，没有现成库函数能替掉整段 `openPTY`。
  - 门禁：`TestArchitectureGate`、`TestKernelResidueFiles` 通过。
- **4.4 Linux 分支（首轮，macOS 交叉）**：`GOOS=linux go build ./...`、`GOOS=linux go vet ./...`、`GOOS=linux go test -c`（host 与其余含测试的包）通过。真实 bwrap 的对照验证见下节「4.4 补做」。
  - **未验证**：需要真实 bwrap 的 `host/run_linux_test.go`、`host/start_linux_test.go`、`host/cgroup_linux_test.go`。本机是 macOS，没有 bwrap 也没有容器运行时；仓库现有的两个 GitHub Actions 工作流（`build1.25.yml`、`build-image.yml`）都不运行 gosandbox 模块的测试，所以「在 Linux CI 上验证」目前没有可用入口——需要有 bwrap 的 Linux 机器上跑 `cd gosandbox && go test ./host`，或另开变更补 CI。
  - 其它平台：`GOOS=windows go build ./...` 在未改动的 `agent/proc.go` 上失败（`syscall.Credential`、`Setpgid` 等 POSIX 专用字段），属既有状态；README 的构建说明只覆盖 Linux 与 macOS。`GOOS=freebsd` 可构建，但未运行测试。

### 4.4 补做：真实 Linux + bwrap 上的对照验证（2026-11-26）

仓库没有会跑 gosandbox 的 Linux CI，开发机上也没有容器 / 虚拟机工具。为把这一项做完，在开发机上 `brew install lima`（2.3.0，单个 bottle，无 qemu 依赖）起了一台一次性虚拟机：Ubuntu 24.04.5 LTS、vz、arm64、内核 6.8.0-147，`apt install bubblewrap`（0.9.0，非 setuid，支持 `--disable-userns`），Go 1.26.1 linux/arm64（与开发机同版本），`git`、`python3` 齐备，cgroup v2。验证完实例已删除（`limactl delete -f`），本机只留 lima 二进制。

**对照方法**：把变更起点 `fe37cfb` 的 gosandbox（`git archive`）与工作区的 gosandbox 各拷一份进虚拟机，在同一环境里跑 `go build ./... && go vet ./...` 与 `go test ./... -json`，逐顶层用例比较 pass / fail / skip。两棵树的全部包都能编译并运行测试，且**任何配置下逐用例结果都没有差别**（Linux 文件集的 vet 以 macOS 上的 `GOOS=linux go vet ./...` 为准：虚拟机里 vet 与 test 串在同一条命令里，vet 的输出没有单独留存核对）；work 只多出本变更新增的 5 个通过用例（`TestBwrapArgsGolden`、`TestWireGoldenRoundTrip`、`TestFieldTooLongErrors`、`TestOpenPair`、`TestSetWinsize`）。

| 配置 | base | work |
| --- | --- | --- |
| 默认 TMPDIR，代码未改 | host 20 个用例 FAIL（`bwrap: Can't create file at /proc/scsi: Is a directory`）；`box` 包在全部用例 PASS 后被判 FAIL（`Test I/O incomplete`）；9 SKIP；312 PASS | 同左，另加 5 个新用例 PASS |
| ＋目录型 masked 路径改盖空 tmpfs（实验补丁，两棵树相同） | host 20 FAIL（`bwrap: execvp /tmp/…/gosandbox-inner: No such file or directory`） | 同左 |
| ＋`TMPDIR` 指到 `/tmp` 之外 | host 82 PASS / 2 FAIL（`gosandbox-inner: setup failed: controlling tty: operation not permitted`） | 83 PASS / 2 FAIL |
| ＋helper 忽略 `TIOCSCTTY` 的 EPERM（实验补丁，两棵树相同） | `./host ./vm/...` 132 PASS / 0 FAIL / 5 SKIP | 133 PASS / 0 FAIL / 5 SKIP |
| `vm/local` 的 `TestRealIsolation*`（把 `gosandbox-inner` 放到测试二进制旁） | 5 PASS | 5 PASS |

两处实验补丁只打在虚拟机里的两份副本上，用来绕开下面这些与本变更无关的既有问题；仓库代码没有因此改动。

**由此得到的验证结论**

- `BwrapArgs` 重构后的完整 argv 在真实 bwrap 上跑通：17 个 `TestLinux*`（读写边界、逃逸、只读子路径、git hooks、deny_read、断网、超时杀进程树、限额未强制、helper 失败路径、退出码 71）与 `TestLinuxStartPipe`、`TestLinuxStartCloseKillsTree` 实际执行并通过。
- `ptyx` 在 Linux 上：`internal/ptyx` 单测通过；`TestLinuxStartPTY`、`TestLinuxStartPTYIsolated` 通过（PTY 打开、初始窗口、`Resize`、输出回流；唯一被实验绕过的是 helper 获取控制终端这一步，它不属于 ptyx）。agent 在 Linux 上的 PTY 运行时没有单独的用例，未验证。
- `vm/local` 经 host 的真实隔离用例 5/5 通过；其余包全部通过（`box` 包的用例全部 PASS，但包被下面第 5 条判为 FAIL）。

**本变更范围外的既有问题（base 同样存在，未修改，建议另开变更）**

1. 出厂 Ubuntu 24.04（`kernel.apparmor_restrict_unprivileged_userns=1`）上普通用户用不了 bwrap：带 `--unshare-net` 报 `loopback: Failed RTM_NEWADDR: Operation not permitted`，不带时报 `setting up uid map: Permission denied`，退出码 1；该 `bubblewrap` 包不附带 AppArmor 档案；以 root 运行可用。README 说明 Probe 不真起 bwrap，所以这种主机上要到运行时才暴露。本次验证用 `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0` 放开（CI 上的通行做法）。
2. `MaskedPaths` 一律 `--ro-bind /dev/null`：`/proc/scsi`、`/sys/firmware` 在这台内核上是目录，bwrap 报 `Can't create file at /proc/scsi: Is a directory`，隔离整体不可用。Docker 对目录型的 masked 路径盖的是空 tmpfs。
3. Linux 集成测试把 helper 编到 `t.TempDir()`（即 `/tmp` 下），而沙盒内的 `/tmp` 是 tmpfs，helper 不可见；按默认 TMPDIR 这 20 个用例跑不通。`vm/local` 的 `TestRealIsolation*` 则因为测试二进制旁没有 `gosandbox-inner` 而整组 Skip。
4. `--unshare-pid` 与 `--new-session` 同时使用时，bwrap 在新 PID 命名空间里先起 init，用户命令不是会话首领（最小复现：带 `--unshare-pid` 时 `leader=False`、`TIOCSCTTY` EPERM，不带时成功），helper 的 `setCtty` 因此失败，Linux 上 `host.Start` 的 PTY 会话在 helper 阶段就会失败。`BwrapArgs` 恒带 `--unshare-pid`。
5. `box` 包测试全部通过后，有子进程仍持有测试二进制的输出管道，`go test` 等待 60 秒后报 `Test I/O incomplete … exec: WaitDelay expired before I/O complete` 并把包判为 FAIL。

**仍未验证**：其它发行版 / 内核（如 Ubuntu 22.04、Debian 12）与 amd64；以 root 身份运行整套用例（只做了 bwrap 冒烟）；仓库里的 Linux CI 本身（不存在）。

### 5.1 / 5.3 PTY 收敛与 host 侧 itoa（2026-11-26）

- 新建 `gosandbox/internal/ptyx`：`Open() (master, slave *os.File, err error)` 与 `SetWinsize(f *os.File, rows, cols uint16) error`。平台实现按 design 分三份：`ptyx_linux.go`（/dev/ptmx + TIOCSPTLCK + TIOCGPTN）、`ptyx_darwin.go`（/dev/ptmx + TIOCPTYGRANT / TIOCPTYUNLK / TIOCPTYGNAME）、`ptyx_other.go`（返回 `ErrUnsupported`）；linux 与 darwin 的 `SetWinsize` 完全相同，放在共用的 `ptyx_unix.go`。
  - Linux 的 `Open` 取自 agent 的实现（带「unlock pty / pty number / open pty slave」阶段前缀）；darwin 的取自 host 的实现，补上同样的阶段前缀（grant pty / unlock pty / pty name / open pty slave）。两份都不再显式传 `O_CLOEXEC`（`os.OpenFile` 在 unix 上总会追加，已在 Go 源码 `os/file_unix.go` 核对）。
  - 窗口大小策略留在调用方：host 的 `attachStdio` 总是设置、失败关闭两端并归为 `unavailable("open pty: …")`；agent 的 `startWith` 只在 rows、cols 都大于 0 时设从端、失败忽略。`Resize` / `resize` 直接调 `SetWinsize`。
  - 删除 `agent/pty_linux.go`、`agent/pty_other.go`、`host/pty_linux.go`、`host/pty_darwin.go`；host 侧手写 `itoa` 随 `host/pty_linux.go` 一起删除（5.3），全仓 S-03 清单里的三份 `itoa` 至此清零。
  - 可观察差异即 design 批准的两条：host 打开 PTY 失败时错误多了阶段前缀（如 `open pty: unlock pty: …`，错误类别仍是 `unavailable`）；agent 在非 Linux 平台上的桩错误文案改由 `ptyx.ErrUnsupported` 给出（在 darwin 上 agent 现在也能打开 PTY，但 agent 只在 Linux 沙盒里运行，这条路径不可达）。Linux 上 agent 的行为与错误文本不变。
  - 测试：`ptyx_unix_test.go` 验证打开的一对可双向读写、对任一端设置窗口大小另一端读到的也是新值；`ptyx_other_test.go` 验证 `ErrUnsupported`。
  - 结果：darwin 本机 `go test ./internal/ptyx/` 通过；host 的 `TestDarwinStartPTY`、`TestDarwinStartPTYIsolated`（真实 sandbox-exec + PTY）通过；gosandbox 全量通过。交叉：`GOOS=linux`、`GOOS=freebsd` 的 build 与 vet 通过，`GOOS=linux go test -c ./internal/ptyx` 通过。根模块与 gocloudagent 仍可构建。
  - **未验证**：Linux 上 `ptyx` 以及 host / agent 的 PTY 运行时行为（本机无 Linux）；`ptyx_other_test.go` 只做了交叉编译，没有实际运行过。→ Linux 部分随后在「4.4 补做」里补齐：`ptyx` 单测与 `host.Start` 的两个 PTY 用例均通过；agent 侧仍无单独用例。
  - `gosandbox/README.md` 同步：目录表加 `internal/ptyx/`；依赖说明改为「`host` 与 `stream` 除 `internal/` 外不 import 模块内其它包」，并说明 ptyx 是 `internal/` 里唯一带平台分支的包（原「internal 包不 import 任何平台包」对 ptyx 不成立）。实际依赖：`host → internal/executil, internal/ptyx`；`agent → internal/ptyx, proto`。
  - 观察到的遗留（未改）：`ptyx_darwin.go` 的 `unix.SYS_IOCTL` 被 staticcheck（2026.1）标为 deprecated（SA1019，原 `host/pty_darwin.go` 就有，随代码原样搬入；staticcheck 不在项目门禁内）。另有一份不在 S-03 清单里的手写 `itoa` 在 `gocloudagent/tools/todo_test.go`（测试辅助），未动。

### 5.2 导出 blob.JoinKey（2026-11-26）

- `godatabase/blob/keys.go`：私有 `joinKey` 改名导出为 `JoinKey`，实现不变，包内 3 处调用改名。导出后的文档按实际情况写：只负责拼接（去每段首尾 `/`、跳过空段、以单个 `/` 连接，段内字符原样保留），不做校验，`..` / 路径穿越的拦截靠入参已经过 `ValidateProjectID` / `validateID`——原注释写成 joinKey 自己「保证不出现 `..`」，并不准确。
- `godatabase/blob/blob_test.go` 新增 `TestJoinKey`，其中 `{"", "/a/", "", "b"} → "a/b"` 就是 objectstore `branches_test.go` 里移过来的那条断言（入参与期望不变），另加 5 行钉住「只拼接、不校验」的实际行为。
- `internal/objectstore/keys.go`：删除逐字相同的副本，`SystemPrefix` / `SystemKey` / `CatalogPrefix` / `ResetLockKey` 改调 `blob.JoinKey`；`branches_test.go` 里的 `joinKey` 断言随之移走。
- key 布局没有变化：`internal/objectstore/testdata/keys.golden` 改前改后 SHA-256 都是 `070ca941e90f40ae2811ca6e4e482efc961c6d134ae5973704dc380e77e40bac`。
- 按 `godatabase/AGENTS.md`「修改后的验证」：`(cd godatabase && GOWORK=off go build ./... && GOWORK=off go vet ./...)` 通过；`GOWORK=off go test ./... -timeout 900s` 全部通过；根模块 `go build ./internal/... ./cmd/...`、`go test ./internal/dbbridge ./internal/catalog ./internal/systemdb`、`go test ./internal/objectstore` 通过；`TestArchitectureGate`、`TestKernelResidueFiles` 通过。

### 6.1–6.3 收尾（2026-11-26）

**6.1 在最终状态复跑第 0 批命令**

| 命令 | 结果 | 与基线对比 |
| --- | --- | --- |
| `CGO_ENABLED=1 go build ./internal/... ./cmd/... && CGO_ENABLED=1 go vet ./internal/... ./cmd/...` | 通过 | 一致 |
| `CGO_ENABLED=1 go test ./internal/config/ ./internal/api/... ./internal/app/ ./internal/agentbridge/` | 只有 `TestNewLocalStorageAppUsesSystemDatabase` 失败（`app_test.go:147: catalog must not live in cache_dir`），其余 9 个包通过 | 一致：基线已有的失败 |
| `TestArchitectureGate`、`TestKernelResidueFiles` | 通过 | 一致 |
| `(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...)` | 通过 | 一致 |
| `(cd gosandbox && go build ./... && go vet ./... && go test ./...)` | 通过 | 一致 |
| `(cd godatabase && GOWORK=off go build ./...)`、`(cd gofunction && go build ./...)` | 通过 | 一致 |
| `gofmt -l`（本变更涉及的目录） | 无输出 | 一致 |

另跑的非基线项：`(cd godatabase && GOWORK=off go test ./... -timeout 900s)` 全部通过；`go test ./internal/dbbridge ./internal/catalog ./internal/systemdb` 通过；3.6 的 `go test -race` 见上文。

**已有失败 / 既有问题（都不在本变更范围，未处理）**

- `internal/app` 的 `TestNewLocalStorageAppUsesSystemDatabase`：基线即失败，文案与断言行未变。
- `gofmt -l` 在 `godatabase/cache/evict.go`、`godatabase/filedb/remote.go`、`godatabase/lease/lease.go`、`godatabase/registry/handle.go` 有输出。这 4 个文件与 HEAD 逐字节一致，HEAD 版本在当前 gofmt 下本就不合格；design 已把 `godatabase/**` 划在变更之外。基线只对本变更涉及的目录跑了 gofmt，所以当时没有列出。
- `gosandbox` 的 `GOOS=windows go build ./...` 在未改动的 `agent/proc.go` 上失败（见 4.4）。

**6.2 最终 diff 检查**

- 导出面：apidiff 未安装，也没有为此新增工具；改用标准库 `go/parser` 写了一次性脚本，比较「导出符号清单」（包级导出声明 + 导出类型的导出方法，所有 .go 文件按文本解析，不受 build tag 影响）。HEAD 快照用 `git archive` 导出到仓库外，不动工作区。相对 HEAD `c7203ba` 与相对变更起点 `fe37cfb` 结论相同：`gocloudagent`（1305 个）与 `gofunction`（375 个）完全一致；`gosandbox` 只多 `internal/ptyx` 下的符号（`internal/` 包在模块外不可 import）；`godatabase` 只多 `blob.JoinKey`。没有任何导出符号被删除或改签名。
- 残留：build / vet 通过，没有多余 import；本变更新建或重写的文件里没有 TODO / 占位；`objectstore` 的 `joinKey` 副本、`cloudagent` / `group` 的 `truncate` 转发、S-03 清单里的三份手写 `itoa`、四份 `openPTY` / `setWinsize` 均已删除，没有留下包装函数。
- 无关改动：按修改时间列出 3.4 之后变动的文件，与预期清单一致——`gocloudagent/{plan/validate.go, cli/group_runtime.go, cli/group_prompt.go, cli/group_runner.go, group/scheduler.go}`、`gosandbox/{proto/codec.go, host/bwrap_args.go, host/start_posix.go, agent/proc.go, internal/ptyx/*, README.md}` 与四个被删的 pty 文件、`godatabase/blob/{keys.go, blob_test.go}`、`internal/{objectstore/keys.go, objectstore/branches_test.go, api/sandboxhttp/e2b_envd_handler.go}`、本变更的 `tasks.md` / `design.md`。`ui/dist`、`ui/coverage` 在 16:05 被重新生成：不是本会话的命令触发的（会话内没有执行任何 yarn / node 命令，Go 测试里也没有 shell 出去调 yarn 的地方），且都在 `.gitignore` 里。
- 提交情况：除 C-01（`c7203ba`）外全部还在工作区，本会话没有提交。design「迁移策略」要求纯搬移与逻辑抽取分开提交，建议按下列互不相交的文件集分组：C-02 + C-03（`group/engine_{control,exec,frames,state}.go`、`group/engine_exec_test.go`、`cloudagent/{history,mcptool,prompt,runtime,runtime_events,trace}.go`）；C-04（`plan/validate.go`、`validate_test.go`）；C-05（`cli/group_{runtime,prompt,runner}.go`）；S-01（`proto/codec.go` 与 1.2 的 golden）；S-04（`host/bwrap_args.go` 与 1.3 的 golden）；S-02（`internal/ptyx/*`、`agent/proc.go`、`host/start_posix.go`、四个删除、`gosandbox/README.md`）；X-02（`godatabase/blob/{keys.go, blob_test.go}`、`internal/objectstore/{keys.go, branches_test.go}`）。注意 `internal/api/sandboxhttp/e2b_envd_handler.go` 是 DDD 迁移里尚未入库的新文件，S-03 的这一处只能随迁移一起提交。

**6.3 逐项状态**

「命令」一栏：1.1 与 3.4 之后的各项见上文各节；1.2–3.3 当时没有留下命令记录，这里只写当前可核对的依据，最终结果以 6.1 的复跑为准。

| 任务 | 状态 | 依据 / 未验证的边界 |
| --- | --- | --- |
| 1.1 基线 | 完成 | 见「1.1 基线」 |
| 1.2 proto golden | 完成 | `codec_golden_test.go`：39 个 Kind 的字节快照 + 8 条编码、6 条解码错误文本 |
| 1.3 BwrapArgs golden | 完成 | `bwrap_args_golden_test.go`：3 个场景 |
| 1.4 Validate 多错误 | 完成 | `TestValidateFirstErrorPrecedence`：10 条 |
| 1.5 config 用例 | 完成 | `config_contract_test.go`：`TestEnvRemainingDomains`、`TestYAMLRemainingDomains`、`TestRedactedDefaultsSnapshot` |
| 2.1 A-01 | 完成 | 无 `*_compat.go`；`sandboxDeps` 在 `router.go`；两个门禁通过 |
| 2.2 文档同步 | 完成 | 根与 `internal/` 的 AGENTS 写明 `internal/api` 为 25 个顶层生产文件；`internal/README.md` 与平台架构 spec 改为「不保留 `*_compat.go` 转发层」；四份文档里都没有「29 个」的旧计数 |
| 2.3 A-02 | 完成 | `applyEnv` 拆成 16 个 `applyEnv<Domain>` |
| 2.4 A-03 | 完成 | `applyYAML` 拆成 `applyYAMLInstance / Storage / Database / Auth / HTTP / Limits / Sandbox / Observability / S3 / DuckLake` |
| 2.5 A-04 | 完成 | `Redacted` 拆成 11 个 `redacted<Domain>`；`defaults()` 只有 17 行，无需再拆 |
| 3.1 C-01 | 完成 | 五个 engine 文件；已单独提交 `c7203ba` |
| 3.2 C-02 | 完成 | `Engine.step`；`engine_exec_test.go` 的取消、存储失败、`awaiting_plan_confirm` 暂停三个交错用例；3.6 的 `-race` 通过 |
| 3.3 C-03 | 完成 | `truncate` 转发已删；`maxGroupContentChars = 2000` |
| 3.4 C-04 | 完成 | 见「3.4–3.6」 |
| 3.5 C-05 | 完成 | 见「3.4–3.6」 |
| 3.6 宿主 `-race` | 完成 | 0 个 data race；`app` 只有基线已有失败 |
| 4.1 S-01 | 完成 | 见「4.1–4.4」 |
| 4.2 S-04 | 完成 | 见「4.1–4.4」 |
| 4.3 S-03 | 完成 | `group` 与 `sandboxhttp` 两处已换；host 侧随 5.3 删除 |
| 4.4 Linux 分支 | 完成 | 仓库没有会跑 gosandbox 的 Linux CI，改在本机 Linux 虚拟机 + 真实 bwrap 上做 base / work 对照验证，逐用例无差别；发现 5 个范围外的既有问题，见「4.4 补做」 |
| 5.1 S-02 | 完成 | darwin 用真实 PTY 端到端跑过；Linux 上 `ptyx` 单测与 host 的两个 PTY 用例在「4.4 补做」里通过，agent 侧无单独用例 |
| 5.2 X-02 | 完成 | `keys.golden` 哈希不变；godatabase 全量通过 |
| 5.3 S-03 host | 完成 | 随 `host/pty_linux.go` 删除 |
| 6.1 复跑 | 完成 | 见上表 |
| 6.2 diff 检查 | 完成 | 见上 |
| 6.3 回填 | 完成 | 本表 |
| 6.4 validate / archive | 完成 | `openspec validate refactor-module-simplification --strict` 通过，随后 `openspec archive` |
