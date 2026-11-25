# gosandbox 重构、精简与清理计划（planv6.8）

> 日期：2026-11-23。方法：用户指定的 `skills/simplify-code/SKILL.md`（v2.2.0），Assess / R-branch。
> **状态：✅ 代码项已完成并关闭（2026-11-24 复核）。** §7 激进批次与 §9 的 U-1 均已实施；S-4 / B-3 为有意 skip（§7.1）。2026-11-24 门禁复核：`GOWORK=off go build / vet / test ./...`（darwin）通过。开口验证项不变（§8.3）：Linux 真机运行、Go 1.25 最低版本、`TestAgentStdin` 偶发收尾超时根因、macOS 4 条既有 SA1019。
> **已按用户后续授权执行激进策略（2026-11-23）；完成结果与平台验证缺口见 §7，实施后的第二轮评估见 §8，第二轮候选 U-1 的执行结果见 §9。下文评估记录保留为实施依据，冲突处以本段及 §7 / §8 / §9 为准。**
> 授权差异：协议升 v3，删除 Exited/Stats/RelayClose 旧短帧容错；`box.Session.DialPort` 改为 `(ctx, port)`，不留旧签名包装；B-1/B-2 纳入修复，关闭允许丢弃尚未交付的 watch 事件。宿主/helper 必须同步更新，旧常驻 agent/supervisor 不自动迁移。
> 目标：合并真正重复的规则、移除可证明的冗余分支、明确协议与资源生命周期职责。不以删行数为目标，不引入通用框架或新功能。
> `internal-simplify-refactor-plan.md` 的激进策略与审批不自动适用于本独立模块。

## 0. 范围、证据与基线

### 0.1 范围

- 修订：HEAD `dead32f`；检查时 `gosandbox/` 工作区干净。现有 `internal-simplify-refactor-plan.md` 已修改、`ui-simplify-refactor-plan.md` 未跟踪，本轮均不修改。
- 模块：`github.com/linkxzhou/SimpleBase/gosandbox`，`go.mod` 声明 Go 1.25.0；本机实际 Go 1.26.1、darwin/arm64。
- 体量：生产 Go 约 15,363 行，测试约 9,061 行。统计跨平台文件总和，不等于单平台编译规模。
- 重点已读：`proto/codec.go`、`vm/driver.go`、`vm/local/{local,exec,files,metrics}.go`、`vm/box/{exec_linux,files_linux,capabilities,metrics_linux,logdir,imagesource}.go`，以及 `host` 进程/PTY、`box` 启动/监听、`image` 公共接口的相关片段。
- 调用方核对：`internal/app/assemble_sandbox.go`、`internal/sandbox`、`internal/api`、`gocloudagent/{cli,tools}` 的导入/相关符号；**没有把所有调用方逐函数审计**。
- 约束：遵循根 `AGENTS.md`、`gosandbox/README.md`；模块内未发现额外 `AGENTS.md` / `CLAUDE.md`。不得反向依赖 SimpleBase 根模块，不新增依赖、不改模块配置。
- 排除：平台业务、数据库与 DDL、UI、真实部署、镜像仓库接入、跨平台功能扩展。`agent`、`image`、`snapshot`、`netx`、`stream` 的非重点部分只有规模/热点扫描，不作全模块缺陷清零承诺。

### 0.2 已实际执行的基线

以下命令在 `gosandbox/` 执行；续跑阶段重新执行核心基线并直接检查退出状态，未以 `tail` / `head` 的退出码代替命令结果。

| 检查 | 结果及边界 |
| --- | --- |
| `GOWORK=off go build ./...` | 通过，darwin/arm64 |
| `GOWORK=off go vet ./...` | 通过，darwin/arm64 |
| `GOWORK=off go test ./... -timeout 120s` | 通过；`cmd`、`host/spec`、`vm/box` 在本平台显示无测试文件。不能据此认定 Linux 路径已测试 |
| `GOOS=linux GOWORK=off go build ./...` | 交叉构建通过；GOARCH 沿用 arm64 |
| `GOOS=linux GOWORK=off go vet ./...` | 通过，不代表 Linux 运行测试通过 |
| `gofmt -l .` | 无输出 |
| 前序 `staticcheck -checks 'all,-ST*' ./...` | 输出 4 条既有 SA1019：`host/limits_darwin.go:58,83` 的 `SYS_PROC_INFO`，`host/pty_darwin.go:28,50` 的 `SYS_IOCTL`；不是零告警 |
| 前序 `GOOS=linux staticcheck -checks 'all,-ST*' ./...` | 未见诊断；原调用经过输出过滤，后续验收须直接运行并保留退出状态 |
| 前序 `GOOS=linux deadcode -test ./...` | 报告多项不可达，包括整个 `vm/box` 驱动入口；仅作分析线索，不能据此删除公共 API |

验证缺口：未运行 Linux 真机测试、race、bench、Go 1.25 最低版本测试及宿主集成回归；本机测试未逐项盘点 Skip。前序试探 Windows 构建出现 `syscall.Credential` / `SysProcAttr` 字段编译错误，**不宣称支持 Windows，也不把修复它纳入精简批次**。

前序辅助统计脚本曾发生递归溢出，部分 shell 批次因 Zsh 通配/等号展开失败；不将这些失败调用当作有效验证。函数规模仅作为热点线索，实施前以源码与测试重核。

## 1. 不得改变的契约

1. **公共 API**：保留 `host.Run/Start/Probe`、`vm.Driver` 与全部可选能力接口、`proto.Msg/Encode/Decode/ReadFrame/Send`、镜像/快照公开入口。独立库有仓库外调用方，“本仓库没调用”不等于可删除。
2. **隔离边界**：驱动隔离不可用时不得退回裸执行；不得为去重合并两套 bwrap 参数策略。`box` 底层已有的显式开发/测试路径也不能在等价重构中暗改。
3. **协议**：帧编号、字段顺序、大小端、长度上限、错误值/文本、部分解码结果、旧帧兼容与缓冲所有权不变；单 writer 串行写帧不变。
4. **进程生命周期**：`Run` 和 `Start` 的超时、输出限额与背压语义不同；保留杀进程组、资源关闭顺序、PTY 与信号行为。非零退出码不擅自改成 error。
5. **文件/镜像/快照**：保持 `os.Root` 边界、符号链接处理、whiteout 与解包限额、摘要校验、原子落盘和仅文件系统快照语义。
6. **错误和空值**：保留 sentinel、包装链、日志脱敏、nil/空切片区别；`local` 的日志与 `box` 的错误包装不强行统一。
7. **平台与依赖**：保持 build tags；`host` 不因共享工具而反向依赖 `vm` 或 `box`；不得将通用模块改成依赖宿主的工具集合。

## 2. 等价精简与结构重构候选

以下 `apply` 表示适合后续等价实施，**不是本轮已应用**；缺少特征测试时先补测试。优先级代表维护收益，风险单列。

### S-1 同包文件类型映射去重

- **位置**：`vm/box/capabilities.go:142–152 fileKindName`、`vm/box/files_linux.go:102–112 kindOf`。
- **证据**：两者均接收 `proto.FileKind`，四种输出和未知值回退完全相同；分别被 watch 转换和目录列表调用，两文件均为 Linux 分支。
- **方案**：保留一个包内映射，迁移两个调用点；不增加公共导出或新工具包。
- **保护**：未知枚举仍返回 `other`。`vm/local/files.go` 的 `kindOf(fs.FileMode)` 输入语义不同，不合并。
- **验收**：表驱动覆盖 regular/dir/symlink/other/未知值，覆盖列表与 watch 转换调用点。
- **分类**：apply；P2；低风险；源码已核实，Linux 运行验证待补。

### S-2 删除 Start 的不可达错误分支

- **位置**：`vm/box/exec_linux.go:65–69 resolveArgv`、`:81–113 Start`。
- **证据**：`resolveArgv` 返回 false 当且仅当 `Shell == "" && Cmd == ""`；`Start` 的 `!ok` 内再次判断该条件，其 else 不可达。
- **方案**：`!ok` 时直接设置交互 shell 参数，删除重复条件和不可达 error。
- **保护**：Shell 优先；空 Shell/Cmd 的 Start 仍忽略 Args 并使用 `/bin/sh -i`；Exec 的空命令仍返回 127。保留 `ensure`、路径解析、环境构造与启动顺序。
- **验收**：补 Shell/Cmd/Args 组合与空命令回退特征测试；现有 `driver_test.go:159–168` 只验证解析器基本输入，不能代替 Start 流程测试。
- **分类**：apply；P2；低风险；源码已核实。

### S-3 移除无意义的错误别名

- **位置**：`vm/box/logdir.go:14 errNotExist`，同文件 `logNamesIn` 与 `capabilities.go:43`。
- **证据**：仅为 `var errNotExist = os.ErrNotExist`；范围内未发现重赋值，两处使用。
- **方案**：直接引用 `os.ErrNotExist`，删除私有变量。
- **保护**：`logNamesIn` 仍把不存在错误归一到 sentinel，不能顺手返回原始 `*os.PathError`；Logs 对不存在目录仍返回 `(nil, nil)`。
- **验收**：日志目录不存在、存在但空、读取失败三种情况，核对错误身份与返回值。
- **分类**：apply；P3；低风险；源码已核实。

### S-4 同一后端的冻结/解冻样板收敛

- **位置**：`vm/local/metrics.go:64–88 Pause/Resume`；`vm/box/metrics_linux.go:44–62 Pause/Resume`。
- **方案**：每个包内分别共享一个私有冻结操作，公共方法传递 op、name、frozen；不跨后端造新的抽象层。若节省有限或新增间接调用反而难读，保留原样。
- **保护**：local 先做 `vmDir` 名称校验并走 `d.fail` 日志；box 当前没有同样的前置流程，不在精简时补入。两者保持不存在视为成功，pause/resume 错误文案各自不变。
- **验收**：不存在、冻结失败、解冻失败、非法 local 名称；对照调用次序和日志次数。须先具备不操作真实 cgroup 的错误路径测试方案。
- **分类**：propose（测试边界就绪后可转 apply）；P3；中风险；不作为首批必做项。

### R-1 协议文件按职责分离，保留显式编解码

- **位置**：`proto/codec.go` 947 行，混合类型声明、标量/集合读写、帧 I/O、Encode 与 Decode。Encode 约 221 行，Decode 约 290 行。
- **方案**：先作纯搬移：消息数据类型与 codec 实现分开，帧 I/O 与 payload 编解码分开；按实际职责选择少量文件（例如 `message.go`、`frame.go` 与保留的 `codec.go`），不预先规定必须拆成多少个文件。
- **第二步**：只提取有完整边界的集合编解码或请求公共字段；保留显式 Kind switch。仅拆文件不算运行优化，不承诺行数下降。
- **禁止**：反射/字段表协议框架、全局 sticky-error 一次性重写、仅为缩短 switch 将一个 case 拆成多个薄包装；不改 `proto.Kind` 的常量顺序。
- **关键陷阱**：Decode 出错时可返回已填充的 Msg；`KindExited`、`KindRelayClose` 的可选尾字段与 `KindStats` 的短帧兼容不同。`reader.bytes` 复制数据，不能改为借用下一帧会覆写的缓冲；空数据的非 nil 切片语义也应保留。
- **验收**：先扩充现有 `proto/codec_test.go`，为每种 Kind 固定 wire 样例；逐字节截断核对错误及部分结果，覆盖字段超限、未知 Kind、旧短帧、额外尾字节与缓冲复用。Encode/Decode round-trip 不能替代 wire golden，二者同时改错也能往返成功。
- **分类**：纯文件搬移 apply；局部编解码算法重写 propose，须经差分证据再决定。P1；中/高风险。

### D-1 修正文档中已失真的契约描述

- **位置**：`vm/driver.go:49–50` 的“原始结果（未截断）”；`:69–70` 的“操作完成即释放句柄，不保活”。
- **证据**：local/box 的 Exec 已有输出限额；`vm/box` 为常驻会话后端，不能用一次性后端生命周期概括所有 Driver。
- **方案**：只更新注释，说明输出是否限额由后端实现决定、长会话由后端管理。保持真正的边界说明，不做全模块删注释运动。
- **分类**：apply；P2；低风险；实现与 README 已核对。

## 3. 功能/可靠性问题：与精简分开决策

### B-1 Watch 适配层转发生命周期（优先核实）

- **位置**：`vm/box/capabilities.go:118–139 fileWatch.Events/Close`；底层 `box/watch.go:83,92–123`。
- **源码推理，尚未运行复现**：每次 Events 调用创建独立容量 64 的输出通道与转发 goroutine，多个转发者竞争同一个底层源；发送 `out <- event` 不可取消。调用方停止读取、适配层缓冲填满后，即使底层 Close，已阻塞发送的 goroutine 仍不能退出。
- **现有测试边界**：`box/log_watch_test.go` 覆盖底层 watch，而不是这一 VM 适配层；未见重复 Events 与满缓冲关闭测试。
- **建议方案**：每个适配器一个稳定输出通道、一次转发、明确的停止信号及单一关闭所有者。Close/context/源结束都应能终止阻塞转发；仅加 `sync.Once` 不足以修复满缓冲阻塞。
- **差异必须明确**：重复 Events 不再创建竞争消费管道；关闭时可能放弃未交付事件，需明确尾部事件与 overflow 的语义，不得偷偷改变背压/丢事件策略。
- **验收场景**：重复与并发 Events、超过 64 条后停止消费再 Close、context 取消、源结束、重复 Close；用完成信号和有界等待证明退出，避免仅用 sleep 或 goroutine 总数判断。
- **分类**：propose，独立可靠性修复；高风险。得到修复范围确认并补出失败测试后，才进入 optimize 批次。

### B-2 DialPort 取消监听可能晚于连接释放

- **位置**：`vm/box/capabilities.go:90–110 DialPort`。
- **疑似问题**：取消监听 goroutine 只等 ctx 或 Session 结束；调用方提前 Close 连接而两者长期存活时，监听者可能长期保留。
- **下一步**：核实 relay 是否已有可复用的 Done 信号及实际调用方 context 生命周期；用“连接已关闭但 session/context 仍存活”的用例验证，再决定是否绑定到连接生命周期。
- **分类**：propose；中风险；尚未复现，不先引入新的通用连接包装器。

### B-3 macOS 已废弃 syscall 的替换

- **位置**：基线中的 4 条 SA1019。
- **建议**：先检查已锁定 `x/sys` 的可用封装；只有签名、返回数据、errno、PTY 行为均能验证时才替换。无等价封装则保持现状，单列后续平台维护任务。
- **分类**：propose；高风险；不升级依赖、不加 lint suppress、不声称只是机械替换。

## 4. 明确不做的“精简”

- **不按 deadcode 输出删除导出 API**：模块测试入口无法代表宿主和外部使用者。`MetricsFor`、`FreezeCgroup`、`Puller.Store/Unpack/LowerPath` 等已有跨包使用；`SetZstdReader` 是扩展入口。
- **仅有测试调用也不自动删除**：`ResetProbeCache`、`WaitTimeout`、`DecodeConfig`、`EncodeAuth`、`Reference.Repo`、`Store.GC` 等属于公共面；`BlobSize/CopyBlob/ProcIDs` 本仓库未见生产调用不证明无外部使用。删除须独立 API 破坏性审批，不计入本计划收益。
- **不强行合并 PTY 实现**：agent/host 的 Linux 实现有 flags、错误包装与尺寸参数差异，macOS 更不同；两份相似代码不足以支持抽新包。
- **不合并 local/box 的命令解析**：local 在宿主 `LookPath`，box 在镜像环境执行；同名命令的可用性可以不同。
- **不把各层 Metrics/Stats 改成一个公共类型**：它们分别承担 host 数据、wire 数据与 vm 契约，字段名和含义不完全相同。
- **不重写成熟的小工具**：`stream.Hub`、`vm.CleanPath/ValidateEnv/ClampPTYDim/TruncateUTF8`、`internal/executil` 已有明确边界；没有新的证据就保留。
- **不为长函数机械切碎**：`host.BwrapArgs`、`agent.startWith`、`image.pull/Import`、`snapshot.walkPrefixed` 是后续审查线索，不仅凭行数批准修改。挂载/清理顺序与归档内容都是契约。
- **不新增缓存/池化/并发下载**：当前无性能基线，不声称可获得多少百分比加速；暂不采用 zero-copy ReadFrame 等会影响缓冲所有权的改写。

## 5. 实施顺序与验收门槛

### P0：锁定特征测试

- [x] 重查工作区状态、适用规范和本计划涉及文件；保留任何在途改动。（实施前 `gosandbox/` 工作区干净）
- [ ] Linux 环境直接运行模块 build/vet/test，记录 bwrap、userns、cgroup、pasta 相关 Skip，不改宿主隔离策略来强行通过。**未做**：只有 linux/arm64 交叉 build / vet / `go test -c`，见 §7.3。
- [x] 补 S-1/S-2/S-3 和协议的特征测试，先在旧实现上通过。（`proto/codec_contract_test.go`、`vm/box/contracts_linux_test.go`，§7.2）
- [x] 若确认修 B-1/B-2，另补旧实现失败的最小复现；不要把它们计作等价测试。（v3 协议的截断测试先在旧实现上失败再改实现，§7.1；B-1 / B-2 的 `box/lifecycle_test.go`、`box/backpressure_test.go` 只记录了新实现通过，未记录「先在旧实现上失败」，§7.2）

### P1：低风险清理

- [x] 依次执行 S-1、S-2、S-3、D-1；每个逻辑变更单独检查，不夹带格式化其他文件。（§7.1）
- [ ] 验收 Linux `vm/box` 测试及模块 build/vet，确认错误、默认命令、映射回退未变。**部分**：模块 build / vet 通过；`vm/box/exec_linux_test.go`、`watch_linux_test.go`、`contracts_linux_test.go` 仅交叉编译，未在 Linux 执行（§7.2）

### P2：协议结构

- [x] 先搬移 R-1，再考虑小范围公共字段去重；保留公开入口。（搬移到 `proto/message.go` / `proto/frame.go`；公共字段去重经 §8.2 评估不做）
- [x] wire golden、截断差分、旧帧、buffer 所有权、agent/box 协议交互测试全部通过。（§7.2 / §7.3；按授权删掉了旧短帧兼容，对应断言改为截断错误）
- [x] 若改变算法或分配策略，先建立原版 `ns/op`、`B/op`、`allocs/op` 基线；没有可重复收益便不保留额外复杂度。（未改算法与分配策略，无需基线）

### P3：可选项与可靠性

- [x] S-4 只有明显改善且错误路径可测才实施，否则 skip。（= skip，§7.1）
- [x] B-1/B-2/B-3 独立确认、独立修改、独立验收；本计划不授权顺带修复。（B-1 / B-2 已修；B-3 = skip，§7.1）

### 最终回归（已执行；逐条结果见 §7.3，未闭合的在此标注）

状态一览：1 ✅（darwin）　2 ◐ macOS 已跑、Linux 未运行　3 ✅（`-race -count=3` 时 `TestAgentStdin` 一次收尾超时，根因未定）　4 ✅（macOS 退出 1，仅 4 条既有 SA1019；Linux 退出 0）　5 ✅（宿主只验证编译与 fake 驱动测试）　6 ✅（`gocloudagent` 编译）　7 ✅

1. 在 `gosandbox/`：`GOWORK=off go build ./...`、`GOWORK=off go vet ./...`、`GOWORK=off go test ./... -timeout 120s`；真实集成用例需要时合理放宽超时并记录。
2. Linux 与 macOS 分别运行相关测试；交叉 build/vet 只能补充，不能替代平台运行验证。复查 Go 1.25 兼容性。
3. 并发改动后运行相关包 `go test -race`；协议改动后运行 agent/box 交互测试，核对各平台 Skip。
4. 直接运行两平台 staticcheck 并记录退出码，既有 SA1019 单列；`gofmt -l` 必须无新增输出。
5. 根模块先编译调用方：`go build ./internal/... ./cmd/...`；执行 fake 驱动的相关宿主测试。若扩大到根模块全测，先确认 fixture 不连接运行库、不触发真实 DDL；不启动服务、不操作现存沙盒。
6. `gocloudagent/` 独立模块做 build 与 host 使用方的相关测试；不因根模块测试通过而省略嵌套模块。
7. 复查 diff：无公共 API/协议/config/依赖漂移，无测试断言弱化，无无用兼容层；按变更批次记录实际结果，不用“全部通过”概括未测平台。

## 6. 完成定义

- 同一协议文件类型映射只有一处实现；Start 无不可达错误分支；日志缺失错误不再多一层私有别名。
- 协议文件职责可定位，wire 与错误行为保持原状；是否减少行数不是验收门槛。
- 若批准可靠性修复，停止消费并关闭 watch 后转发能退出，重复 Events 的契约有明确测试。
- 所有声明的优化都有场景或测量支撑；公共能力与平台差异被保留；不以删除安全检查、隐藏诊断或缩小测试范围换取“精简”。

## 7. 激进实施结果（2026-11-23）

### 7.1 完成项与有意变化

- [x] **S-1/S-2/S-3/D-1**：watch/list 复用 `kindOf`；Start 默认 shell 分支去掉不可达错误；删除 `errNotExist`；修正输出限额与常驻会话注释。
- [x] **R-1**：新增 `proto/message.go`、`proto/frame.go`，定义原样迁移；纯搬移阶段 `go test ./proto ./agent ./box` 通过后才改协议语义。
- [x] **v3 严格协议**：删除 Exited/Stats/RelayClose 的三处短帧兼容，版本从 2 升至 3；固定完整 payload 布局不变，未擅自拒绝额外尾字节。先运行新截断测试确认旧实现失败，再修改实现使其通过。
- [x] **B-1**：底层 Watch 的 deliver/Close 共用锁，增加 Done；ctx/session 终止会关闭监听。适配器构造时启动唯一转发者，Events 返回稳定通道，满输出缓冲也可退出。关闭可以丢弃尚未交付的事件。
- [x] **B-2**：`Session.DialPort(ctx, port)` 统一拨号、连接取消与资源回收；不保留旧签名。连接自己的 done 唤醒取消监听与阻塞写；读方向半关闭不触发全关闭，远端全关闭和失败删除连接登记。
- [x] **相邻生命周期收尾**：session 结束统一关闭底层 conn，writer 等 done 而不是关闭多发送者 out；满控制队列时关闭消息保留在同一 writer 的清理队列中，先发送已经排队的创建/数据，再发关闭，不新增阻塞发送 goroutine。Watch 创建与拨号入队都支持取消。
- [x] **调用点与说明**：仓库内 Session.DialPort 调用、测试和 `README.md` 同步更新。宿主 `vm.PortDialer` 接口没有变化。
- [x] **S-4 = skip**：两包原实现很短且校验/日志不同；为了注入 cgroup 故障增加框架不划算，不触碰真实 cgroup。
- [x] **B-3 = skip**：只确认了部分 ioctl 整数封装，未获得 PTY 名称/进程采样的完整等价替代证据。保留原实现及四条 SA1019，不升级依赖、不加 suppress。

### 7.2 新增/调整的证据

- `proto/codec_contract_test.go`：39 种 Kind 的固定零值 wire 长度/内容、非零字段字节样例、逐字节截断、额外尾字节、部分 Msg、空数据非 nil 和 payload 所有权；旧实现先通过基础特征测试。
- `box/version_test.go`：net.Pipe 发送实际 Hello，验证版本 0/2/未来版本被拒绝。
- `vm/box/contracts_linux_test.go`：文件枚举与未知值、Shell 优先/空命令、日志目录缺失/空/读取失败；旧实现与新实现均完成交叉测试编译。
- `vm/box/exec_linux_test.go`：临时目录 + 测试二进制 helper，检验默认交互 shell 与 Exec 空命令 127；不接触现存沙盒。**仅编译，尚未 Linux 执行。**
- `vm/box/watch_linux_test.go`：稳定通道、第 65 次输入完成但输出不消费时 Close 的转发退出、源结束。**仅编译，尚未 Linux 执行。**
- `box/lifecycle_test.go`、`box/backpressure_test.go`：deliver/Close 竞争、ctx/session 收尾、半关闭与全关闭、取消拨号、连接关闭后监听退出、满队列清理帧顺序及阻塞 I/O 退出；本机 race 通过。
- `box/tunnel_test.go` 辅助收集函数增加清理队列采集，原关闭帧断言保留；旧帧成功断言改成截断错误是明确授权的 v3 行为变化，不是假称等价。

### 7.3 实际验证与缺口

环境仍为 Go 1.26.1 / darwin arm64；未安装工具、未启动数据库或正式服务、未改依赖。

- `GOWORK=off go build ./...`、`GOWORK=off go vet ./...`、`GOWORK=off go test ./... -count=1 -timeout 120s`：最终状态通过，退出 0。
- `go test -json ./... -count=1 -timeout 120s`：通过。5 个 skip 事件中，3 个是无测试的平台包（cmd、host/spec、vm/box），2 个是 `host.TestSetuidBwrapRejected`、`host.TestProbeLinuxGates`。
- `go test -race ./box ./proto ./agent ./stream -count=1 -timeout 120s`：通过。后续 `-count=3` 时 agent 的 `TestAgentStdin` 有一次 `agent did not stop`（5 秒收尾超时），非 race detector 报告；不能称全部重复 race 通过。
- 定位复跑：`go test -race ./agent -run '^TestAgentStdin$' -count=10 -timeout 120s` 通过；`go test -race ./box ./proto ./stream -count=5 -timeout 120s` 通过。单次超时根因未确认，保留未解决项，不通过加大超时/改断言隐藏它。
- `GOOS=linux GOWORK=off go build ./...`、`go vet ./...`：通过；`go test -c -o /dev/null ./box` 与 `./vm/box`：通过，均为 linux/arm64 交叉检查。
- Linux staticcheck 最终退出 0。中途新增的 SA4000 测试表达式已修正；macOS staticcheck 退出 1，仅原四条 SA1019，未宣称零告警。
- 根目录 `go build ./internal/... ./cmd/...` 与 `gocloudagent/` 的 `GOWORK=off go build ./...`：通过。
- `gofmt -l .` 无输出；限定 `gosandbox` 和本计划的 `git diff --check` 通过；IDE 指定修改目录无诊断。
- **未做**：Linux 真机运行、Go 1.25 最低版本运行、宿主/Agent 全量测试及生产部署。调用方本轮只验证编译，不假称集成回归完成；无速度提升声明。

### 7.4 部署注意与剩余限制

1. 必须同时更新宿主和 helper；旧 v2 agent 的 Hello 被拒绝。旧 supervisor 托管实例需由用户安排停止/重建，本轮不代为操作。
2. Watch 与 relay 的关闭通知现在异步交给既有 writer；对端永久不读时，实际通知仍受 I/O 背压限制，session 终止会关闭连接释放 writer。清理队列按已关闭对象积累，并非新的硬配额机制。
3. 双向版本握手、Windows 支持、镜像能力扩展、未用公共 API 删除不在本轮实现中。
4. 计划代码项已完成/明确 skip；Linux 运行验证和上述 agent 重复测试偶发超时作为剩余验证事项保留，不勾成通过。

**v3 复核（2026-11-23，未动源码）**：重跑 `GOWORK=off go build ./...`、`go vet ./...`、`go test ./... -count=1 -timeout 180s`，全部退出 0；`go test -json` 的 skip 事件仍是同样 5 个；macOS staticcheck 仍是同样 4 条 SA1019，`GOOS=linux` staticcheck 无诊断；`deadcode -test ./...` 只报各平台 `*_other.go` 桩与被根模块使用的入口（`vm/fake`、`image.Puller` 等），没有可删项。第二轮只发现 1 个值得做的小候选（§8.1）。

## 8. 第二轮评估（实施后，只评估未动代码）

方法与 §0 相同：AST 脚本找「token 归一后结构相同」的函数、控制流嵌套 ≥ 4、> 80 行的函数，再读码确认。缩写：P = 优先级，R = 风险。**本轮只写本节，源码零改动。**

本版本关闭时：§5 仍未勾的两项、§8.1 与 §8.3 并入 `planv6.7/legacy-plan-consolidation-plan.md` §2 的锚点表（同 v6.4 → v6.7 的做法）。

### 8.1 值得做的候选

| ID | P / R | 位置 | 问题 → 方案 | 证据 |
| --- | --- | --- | --- | --- |
| U-1 | P3 / 低 | `agent/user.go` 的 `lookupPasswd`（:136）、`lookupGroup`（:156）、`primaryGID`（:179） | 三份同样的「按 `:` 切列、首列等于 name、取第 N 列转 uint32」循环（各 18 行），差别只有文件、列号、错误文案和「找不到」时的回退（`primaryGID` 回退为 `uid`）。→ 抽纯函数 `lookupColonField(content, name string, col int) (uint32, bool, error)`（列越界或非数字返回 error），三个调用方各自保留现有文案与回退；顺带用夹具字符串给解析补表驱动测试（现有 `user_test.go` 只覆盖纯数字路径，`/etc/passwd` 分支没有用例） | AST 脚本（129 token × 2）+ 读全文；agent 运行在沙盒内，改动只在 Linux 生效，需 Linux 运行验证 |

### 8.2 评估后不做的（下次不必重评）

- `proto/codec.go` 的 `Encode`（220 行）/ `Decode`（270 行）：两边的 switch 已经建在 `w.str` / `w.bytes` / `w.strs` / `w.entries` / `w.rlimits` 与对应的 `r.*` 之上（编码侧 26 / 4 / 3 / 2 / 1 次调用，解码侧次数相同），再抽「公共字段组」只省个位数行，并把 wire 布局藏进间接层。R-1「只拆文件、保留显式 switch」的结论维持；`codec_contract_test.go` 的逐 Kind golden 足以约束将来的改动。
- `writer.bytes` / `writer.str`（各 8 行）：只差长度前缀后的切片 / 字符串类型，合并需要泛型或 `any`，不值得。
- 嵌套 4 的 6 个函数（`agent/watch_linux.go` 的 `watchLoop`、`box/session.go` 的 `writer`、`host/glob.go` 的 `parseGlob`、`host/limits_darwin.go` 的 `listPids`、`host/mandatory.go` 的 `ForceDenyWritePaths`、`vm/fake/fake.go` 的 `run`）：都是「读事件 → 按类型分支」或词法解析的自然形状，没有重复知识，不为指标拆。
- `host.BwrapArgs`（127 行）、`agent.startWith`（106 行）、`image.pull`（93 行）/ `Import`（91 行）：§4 已列为后续审查线索，并说明挂载 / 清理顺序是契约，维持。
- §4 的其余条目（不删公共 API、不合并 PTY / 命令解析 / Metrics 类型、不引入零拷贝）继续有效。

### 8.3 仍然开口的验证事项

与 §7.3 / §7.4 相同，未因第二轮而变化：Linux 真机运行（含 `vm/box` 三个仅编译的测试）、Go 1.25 最低版本、`TestAgentStdin` 在 `-race -count=3` 下的一次收尾超时根因、macOS 的 4 条 SA1019、宿主集成回归（第二轮只重跑了根模块的编译与 fake 驱动测试，以及 `gocloudagent` 的 build / vet / test，见姊妹计划 §8.1）。

## 9. 第二轮候选的执行结果（v4，2026-11-23）

| ID | 状态 | 落点 |
| --- | --- | --- |
| U-1 | ✅ | `agent/user.go` 新增 `colonField(content, name, minFields, col) (uint32, colonLookup)`（`colMissing` / `colOK` / `colBad`），`lookupPasswd` / `lookupGroup` / `primaryGID` 三处循环收敛为「读文件 → 查一列 → 按状态拼原有文案」；`minFields` 保留「字段数不足的行按不存在」的原语义，`primaryGID` 找不到主组仍回退为 `uid`。新增 `TestColonField`（7 种行形态）与 `TestLookupGroupReadsEtcGroup`（宿主 `/etc/group` 没有 root 组时 Skip） |

门禁：`GOWORK=off go build / vet / test ./...`（darwin）通过；`GOOS=linux` build / vet 通过，`go test -c ./agent` 通过；macOS staticcheck 仍只有既有 4 条 SA1019，Linux 0；`gofmt` 无输出。§8.2 / §8.3 的结论与开口项不变（Linux 真机运行仍待补）。
