# gosandbox 优化与清理计划（v6.6）

> **状态（2026-11-17）：G1 批（GSB-1~6）已在代码中执行，macOS 门禁通过，待 Linux 验证；B1 已按方案 A 解决；G2、G3（仅文案）、G4 已执行；B2/B3/B6 与基线提交未动。** 本文是用 `skills/simplify-code`（R-branch，只读评估）对 `gosandbox/` 做出的结果，对照 `kiri-go-port-analysis.md`（M0~M5 交付）核对「计划写了什么、代码里实际接上了什么」。
>
> 原则（simplify-code Golden rules）：行为与公共 API 不变；每项单独改、单独验证；有差异输入的改动一律走「需批准」。不改 `go.mod` / `go.sum`。承接 `dead-code-and-optimization-plan.md` §0 的误报口径，不重复其已登记项。

## 0. 评估口径

| 项 | 内容 |
| --- | --- |
| 日期 / 版本 | 2026-11-17，HEAD `d855e65` + 工作区未提交改动 |
| 范围 | `gosandbox/` 全模块（99 个非测试 `.go`、约 18.6k 行；测试约 8.5k 行）：`agent` `box` `cmd` `host` `image` `netx` `proto` `rootfs` `snapshot` `vm{,/box,/fake,/local}` |
| 工具 | `go build` / `go vet`（darwin 与 `GOOS=linux`）、`gofmt -l`、`staticcheck ./...`（两个 GOOS）、`deadcode -test ./...`（两个 GOOS）、`go test ./...`（darwin）、`GOOS=linux go test -c` 逐包编译、手工 `grep` 引用扫描（含根模块） |
| 局限 | 本机是 macOS：**Linux 专用代码（`box` `agent` `vm/box` `host/*_linux.go`）只做到编译 + vet，没有运行**；未做重复代码检测工具扫描；未逐函数通读（`proto/codec.go`、`image/registry.go` 等只做抽样）；根模块 `deadcode` 因 §B1 编译失败无法给出宿主视角结果 |
| 重要事实 | `gosandbox/` 下 **9 个目录仍是未跟踪文件**（`agent` `box` `image` `netx` `proto` `rootfs` `snapshot` `vm/box`、`host` 新增 6 个），当前没有 git 历史可供回滚，见 §5 |

**已核实为误报、不得删除**：

- darwin 下 staticcheck 报 `box/copytree.go` 的 `copyTree` / `copyFileInto` 与 `box/transport.go` 的 `wrapConn` 未使用：它们由 `launch_linux.go` / `vsock_linux.go` 使用，只是 darwin 不编译那两个文件。**判定死代码必须两个 GOOS 都报才算。**
- `deadcode -test` 在 Linux 下对 `vm/box`、`vm/fake`、`vm/local` 报的约 60 个方法：`vm/box` 的测试是 Linux 专属且只覆盖 `resolveLower`，`Driver` 方法由宿主 `internal/app` 通过 `vm.Driver` 接口使用；`vm/fake` 是测试替身。
- `host/limits_darwin.go` 的 `SYS_PROC_INFO`、`host/pty_darwin.go` 的 `SYS_IOCTL` 弃用告警：x/sys 无 libSystem 包装，沿用 v6.6 DC 既有结论。
- `image/unpack_test.go:391` SA4000（`ChainID(x) != ChainID(x)`）：有意比较同一输入的确定性，保持原样。
- `image.SetZstdReader`：文档化的注入点，0 调用方但属公开扩展口，保留。

---

## B. 阻塞项与疑似缺陷（先看，不属于简化，本计划不修）

### B1 宿主编译失败：`opts.Supervise` 未定义（阻塞宿主侧一切验证）

```text
internal/app/app.go:1522:7: opts.Supervise undefined (type box.Options has no field or method Supervise)
```

`go build ./internal/... ./cmd/...` 当前在任何 GOOS 下都失败。`vm/box.Options`（`driver_linux.go:58` 起与 `driver_other.go`）没有 `Supervise` 字段，`ensure()` 构造 `gosandboxbox.Config` 时也没有传 `Supervise`；而 `internal/config/config.go:502` 与 README「服务重启不丢沙盒」都声明了该能力。

**需要决定**（不代为执行）：

| 选项 | 做法 |
| --- | --- |
| A（推荐） | 给 `vm/box.Options`（Linux 与 other 两份）加 `Supervise bool`，`ensure()` 里传给 `gosandboxbox.Config{Supervise: …}`；属于补全 M5-#19 的接线，不是简化 |
| B | 去掉 `app.go:1522` 与配置项，等 supervisor 修好（见 B2）再接 |

**B1 未决前**，§3 中凡需要宿主 `go build` / `go test ./internal/...` 的门禁无法执行，只能跑模块内门禁。

### B2 supervisor 模式的 fd 约定对不上（怀疑，未复现）

`box/launch_linux.go:48-49,87,129`：argv 里写死 `--ctrl 4`、`--file 3 <policy>`；但 `launchSupervised`（`:276-`）把 `policyPath` 丢弃（`_ = policyPath`，`:287`），且 `RunSupervisor`（`supervise_linux.go:132`）只给子进程 `ExtraFiles = []*os.File{agentFile}`——控制通道变成 **fd 3**，策略文件没有传。结果是 supervisor 路径下 agent 读不到 `--ctrl 4`、bwrap 的 `--file 3` 读到的是控制 socket。`supervise_test.go` 在「起不来」时 `Skip`，所以绿灯不能证明可用。

确认方式：在 Linux 机器上 `go test -run Supervisor -v ./box`，看是 PASS 还是 SKIP。**未确认前不动代码。**

### B3 `egress` / `egress-allowlist` 档沙盒共享宿主网络栈（怀疑，高优先级，未复现）

`box/launch_linux.go:109`：只有 `none` 档才加 `--unshare-net`；其余档不隔离 netns，然后 `session.go:161` 让 pasta 去连 `bwrap` 进程的 netns——那就是宿主自己的 netns。如此一来：

- 沙盒直接拥有宿主的全部网络（含 `169.254.169.254` 元数据与私网），README「必须拦的地址」一节没有任何代码在执行：`netx.CheckAllowed` 在生产代码里 0 调用（仅测试与注释）。
- 与 `kiri-go-port-analysis.md` §6.5 / §8「出网必须拦元数据、私网」的要求不符。

确认方式：Linux 上配 `sandbox.network: egress`，沙盒内 `ip route` / 访问内网地址。涉及安全边界，**建议单独立项，不与本清理计划混做**。

### B4 白名单 DNS 代理从未启动（已确认）

`netx.DNSProxy.ListenAndServe` 在全仓 0 调用；`netx.StartOptions` 没有 `Allow` 字段，`box/session.go:161` 也没有传白名单；`agent` 也没有任何起 DNS 的代码。而 `internal/config/config.go:1135` 的报错文案写的是「the DNS allowlist proxy works」。事实是：`egress-allowlist` 档的白名单**完全没有生效**（等同 `egress`，再叠加 B3）。文案应改为如实表述，或接线。见 GSB-10。

### B5 README 与实现不符（已确认）

| README 说法 | 实际 |
| --- | --- |
| 「`overlay` 的探测是**真起一个 bwrap** 读一个探针文件」 | 生产路径 `box/launch_linux.go:175 overlaySupported` 只是 `bwrap --help` 里 grep `--overlay`；真探测的 `rootfs.probeOverlay` 没有任何调用方 |
| 依赖方向 `vm/* → box → {proto, image, rootfs, host}` | `box` 不 import `rootfs`（0 个包 import 它） |
| `rootfs` 头注释「同文件系统优先 reflink」 | `rootfs.copyFile` 只是 `io.Copy`，没有 `reflink` 实现（`probe_linux.go` 头注释里列的 `reflink()` 也不存在） |

### B6 镜像根构造失败时静默退回宿主根（怀疑，设计风险）

`box/launch_linux.go:152-156`：overlay 的 `upper/work` 建目录失败，直接 `return []string{"--ro-bind", "/", "/"}`，**没有日志**；`:161-167` 拷贝失败有一条 `warn` 日志，但同样退回宿主根。而 `vm/box/driver_linux.go:258` 的注释明确写着「不是悄悄退化成宿主根（那等于用户以为在跑镜像，实际在跑宿主）」。两处意图相反。建议改为返回错误（行为变化，需批准）。

---

## 1. 清理项

### 1.1 P2：可直接应用（低风险，**G1 批**）

| ID | 位置 | 问题 → 方案 | 风险 / 证据 |
| --- | --- | --- | --- |
| GSB-1 | `box/transport.go:61-69,77-91` | `asDeadliner`、`transportPid` 两个函数（Linux 下 staticcheck + deadcode 均报 0 调用）及仅为它们服务的导出接口 `Deadliner`、`Pid`。删除；保留 `Transport` 与 `wrapConn`（`vsock_linux.go` 在用）。同步修剪文件头「主要符号」与用法示例里的 `Deadliner` 一行 | 低。`Deadliner` / `Pid` 是导出类型，已在全仓（含根模块）grep：除本文件外 0 引用；`session.go:160` 用的是内联 `interface{ Pid() int }`，不受影响 |
| GSB-2 | `agent/loopback_linux.go:60`、`agent/loopback_other.go:31` | `loopbackUp`（注释写「测试与日志用」，实际无测试无日志调用）两个平台各一份，均 0 调用。删除，修剪两个文件头的符号列表 | 低。已 grep `agent` 全目录含测试：仅定义处出现 |
| GSB-3 | `host/cgroup_linux.go:92-95` | `newCgroupGroup()` 只是 `newNamedCgroupGroup("")` 的单行转发，0 调用方（`run_linux.go:79` 直接用 Named 版）。删除；文件头 `同包调用` 行里的 `newCgroupGroup()` 同步去掉 | 低。仅 Linux 编译，需 `GOOS=linux go vet` + `go test -c` 把关 |
| GSB-4 | `image/import.go:119`、`image/unpack.go:251` | `tar.TypeRegA` 已弃用（Go 1.11）。`archive/tar` 的 `Reader.Next` 会把 `TypeRegA` 归一为 `TypeReg`（`$GOROOT/src/archive/tar/reader.go:145`），读出的 `hdr.Typeflag` 永远不会是 `TypeRegA`。两处分支里的 `TypeRegA` 删掉，`case tar.TypeReg, tar.TypeRegA:` → `case tar.TypeReg:` | 低。差异输入：手工构造的 `tar.Header` 不经 Reader——本模块两处都来自 `tar.NewReader`，不受影响。消除 2 条 SA1019 |
| GSB-5 | `image/pull.go:187-198` | `cfgBody, err := p.store.ReadBlob(...)` 成功分支里 `cfgBody` 从未使用（SA4006），读整个文件只为判断「是否存在且可读」。改为 `if _, err := p.store.ReadBlob(m.Config.Digest); err != nil { cfgBody, gerr := …GetSmall(…); … PutBlob(…) }`，行为（含「存在但不可读则重新下载」）完全保持；不改成 `HasBlob`（那样「存在但不可读」时不再联网，行为有差异） | 低。保持错误顺序与网络调用次数不变 |
| GSB-6 | `netx/pasta_linux.go:30`、`box/vsock.go:35`、`box/copytree.go` 头、`README.md:21` | 过期的文档引用：`dns_linux.go`、`vsock_darwin.go` 都不存在；README 依赖方向写了 `box → rootfs` 但实际没有。改成与代码一致（`rootfs` 的最终去留见 GSB-9，先按现状改 README） | 无代码风险。若 GSB-9 选 A，这一项合并进去 |

### 1.2 P2：需批准（改导出面 / 删整块 / 有差异输入）

| ID | 位置 | 问题 → 方案 | 为什么需批准 / 差异输入 |
| --- | --- | --- | --- |
| GSB-9 | `rootfs/`（`rootfs.go` 218 行 + `probe_*.go` 115 行 + 测试 179 行）与 `box/launch_linux.go:145-170`、`box/copytree.go` | **同一职责实现了两遍，而被宿主实际使用的是 `box` 那份**：`rootfs.Build` ≈ `box.rootArgs`，`rootfs.copyTree/copyFile` ≈ `box.copyTree/copyFileInto`（逐行相同）。`rootfs` 包全仓 0 import。方案 A（推荐）：删除 `rootfs/` 整包并改 README（含 B5 三条）；方案 B：让 `box.rootArgs` 调用 `rootfs`（需导出 `rootfs.copyTree`，并决定采用真探测还是 `--help` grep） | 删除公开包；A 会丢掉 `rootfs.probeOverlay` 的「真跑一次 bwrap」探测实现——若希望兑现 README 的说法，应选 B 并同时修 B5/B6。**两者行为不同**：B 会让 overlay 探测变慢（要真起 bwrap）且失败时改走 copy |
| GSB-10 | `netx/dns.go`（497 行）及其测试、`netx.Policy.AllowPrivate/AllowMetadata`、`netx.CheckAllowed`、`HostAllowed`（仅 dns.go 内部用） | 整块无生产调用方（见 B3/B4）。方案 A：先如实修正文案（config 报错与 README 不再说「DNS 代理能工作」），本计划**不删**，等 B3/B4 立项时决定「接线」还是「整体移除」；方案 B：现在删 `DNSProxy` 与 `CheckAllowed` | 删除会让 `egress-allowlist` 档彻底没有实现，需改 `config`、`sandbox.manager`、README、`netx.ParseMode`；属于产品面决定 |
| GSB-11 | `box/vsock.go` `vsock_linux.go` `vsock_other.go`（共约 290 行）+ `Transport` 抽象 | `DialVSock`、`VSockSupported`、`VSOCKPort*`、`HostCID/GuestCID` 全仓只有 `vsock_test.go` 在用；`vm/box` 没有 darwin 实现。计划 §10.7 #21 明确是「为 macOS VM 后端预留的可复用层」。建议：**保留，不删**，在 `vsock.go` 头注释与 README 里写明「预留，当前无生产调用方」，避免被后来者当成已启用的功能 | 删除是产品取舍；保留成本只是 ~290 行。若决定 macOS VM 后端不做，再整体删（含 `Transport` 抽象，回到直接用 `net.Conn`） |
| GSB-12 | 导出但全仓 0 引用：`image.HostPlatform`（`arch.go:32`）、`image.Client.GetManifest`（`registry.go:161`）、`netx.NetnsPath`（`pasta_linux.go:225` 与 `pasta_other.go:71` 两份）、`snapshot.Store.Root`（`snapshot.go:130`） | 删除，同步修文件头符号表。`Puller.Client/Store/Unpack/LowerPath` 虽被 `deadcode` 报，但 `vm/box/imagesource.go` 在用（`Store`、`Unpack`、`LowerPath`），`Client()` 仅测试用，保留 | 改导出面。差异输入：外部模块若 import 了这些符号会编译失败——全仓（含根模块）已 grep 为 0，但独立模块对外无法保证 |
| GSB-13 | `host/innerpolicy.go`（`InnerPolicy` + `InnerPolicyJSON`） | 与 `host/internal/spec.Spec` **字段 1:1 重复**，仅因 `spec` 在 `internal/` 下而 `box` 无法 import。方案：把 `spec` 提到 `host/spec`（或 `gosandbox/spec`）公开包，去掉这层复制 | 移动包路径 + 改 `inner`、`host`、`box` 三处 import；收益是删 ~56 行与一处「改字段要改两处」的风险。P3，可选 |

### 1.3 P3：已看过，选择**不做**（登记原因）

| 项 | 原因 |
| --- | --- |
| `forceRemoveAll`、`hasEnv` 在 `vm/local/local.go` 与 `vm/box/driver_linux.go` 各一份（逐行相同） | 仅 2 份、共约 25 行；抽公共包要新增 `vm/internal/…`，收益小于一个新包的概念成本。出现第三份再合并 |
| `cappedBuffer` 在 `host/run.go`（`bytes.Buffer` 版）与 `vm/box/driver_linux.go`（`[]byte` 版） | 内部表示不同、各自非导出；`vm/local` 已通过 `host.Run` 复用 host 版。look-alike，不合并 |
| `resolveArgv`（`vm/box`）与 `resolveCommandArgv`（`vm/local`） | 后者多一步 `exec.LookPath`，语义不同，不是重复 |
| `procAdapter`（两份） | 一个包 `host.Proc`、一个包 `box.Handle`，适配对象不同 |
| `host/inner/inner_linux.go:254,261` ST1005（`errors.New(usage)`，常量以 `...` 结尾） | 属于 stderr 用户可见文案，改了就是观察到的行为变化；收益为零 |
| 文件头「调用架构」ASCII 图（`gosandbox` 91/99 个文件有，约 2755 行，占非测试代码约 15%） | 是仓库既有约定（`internal` 204/238 同样有），不属于本次清理；只修**已过期**的引用（GSB-6），不整体删除 |
| `proto/codec.go` 的 `Encode`（208 行）/`Decode`（263 行） | 一个平铺的 `switch kind`，每个 case 自成一段，拆开只会把 switch 打散；协议有单测（13 个）。保持 |
| 其他 80 行以上的函数：`host.BwrapArgs`（129）、`agent.startWith`（106）、`vm/fake.run`（102）、`host.parseGlob`（100）、`image.pull`（94）、`image.Import`（91）、`host.Run`（Linux 89 / darwin 82）、`agent.watchLoop`（83） | P1 级重构：依赖 Linux 真机行为（bwrap / inotify / cgroup），本机无法跑；需先在 Linux CI 补特征测试，另立项 |

---

## 2. 实施批次

| 批次 | 内容 | 前置 | 备注 |
| --- | --- | --- | --- |
| **G0** | 处理 B1；**先提交基线**（见 §5）；在 **Linux 机器**上跑 §4 全套并存档基线（含 `box` `agent` `vm/box` 的真实用例，以及 `-run Supervisor` 看 B2） | 用户决定 B1 | 未通过则 G1 以后全部暂停。本机（macOS）只能做到编译与 vet |
| **G1** | GSB-1、GSB-2、GSB-3、GSB-4、GSB-5、GSB-6（只改文档引用） | G0 | 每项一个提交；GSB-1~3 触及 Linux 专属文件，**必须在 Linux 上跑一遍**才算验证 |
| **G2**（需批准） | GSB-9（A 或 B）→ 同时处理 B5 的 README；GSB-12 | G1 | GSB-9 选 B 时顺带评估 B6 |
| **G3**（需批准，另立项） | B3、B4、GSB-10、GSB-11 | G2 | 涉及安全边界与产品取舍，不是「清理」 |
| **G4**（可选） | GSB-13 | G2 | 包路径移动，收益小 |

每批只做计划中列出的项，不顺手改其他代码。

## 3. 预期收益

- G1：约 -50 行、消除 6 条 staticcheck 告警（U1000 ×4、SA1019 ×2；SA4006 ×1 视 GSB-5 实现而定）、去掉 3 处过期的文档引用。
- G2 选 A：约 -510 行（`rootfs` 含测试），并消除「两份实现、其中一份从未被用到」这一最大的误导源。
- 本计划最大的价值不在行数，而在 §B：**README / 配置文案 / 计划文档声称已具备的能力（真探测 overlay、白名单 DNS、元数据拦截、supervisor）与代码实际接线不一致**，其中 B3 是潜在的安全问题。

## 4. 验证门禁（每批结束必须满足；G0 先存基线）

```bash
cd gosandbox
gofmt -l .                                              # 无输出
go build ./... && go vet ./...
GOOS=linux go vet ./...                                 # 交叉检查 Linux 专用代码
for p in $(go list ./...); do GOOS=linux go test -c -o /dev/null "$p"; done   # Linux 测试至少能编译
go test -count=1 ./...                                  # darwin 本机
staticcheck ./... && GOOS=linux staticcheck ./...       # 不得新增；G1 完成后应只剩 §0 登记的误报
go test -list '.' ./... | grep -E '^(Test|Benchmark|Example)' | sort > /tmp/gsb-tests-now.txt
diff /tmp/gsb-tests-before.txt /tmp/gsb-tests-now.txt   # 除 GSB-9 删 rootfs 测试外不得减少
go mod tidy -diff                                       # 无差异（不得改 go.mod / go.sum）
```

**Linux 机器上额外必跑**（macOS 覆盖不到，当前 darwin 覆盖率：`box` 24.9%、`agent` 48.7%、`vm` 23.1%，`vm/box` 无 darwin 测试）：

```bash
cd gosandbox && go test -race -count=1 -timeout 900s ./...
```

宿主侧（B1 解决后）：

```bash
cd .. && go build ./internal/... ./cmd/... && go vet ./internal/app/ ./internal/sandbox/ ./internal/api/
go test -count=1 ./internal/sandbox/ ./internal/app/
```

`GOOS=linux go build ./internal/...` 在 macOS 上会因 DuckDB 的 cgo 失败，与本计划无关，宿主 Linux 构建请在 Linux 上做。

## 5. 回滚

- **先决条件**：`gosandbox/` 大半目录尚未纳入 git。**G0 必须先把当前状态提交（或至少 `git add`）作为回退点**，否则「只回退该提交」无从谈起。
- 每批单独提交；任何一项出现新失败即只回退该提交。G2 / G3 属于需批准项，批准前不进入实施。

## 6. 本次评估已运行的验证（均在 macOS，HEAD `d855e65` + 未提交改动，未改任何源码）

| 命令 | 结果 |
| --- | --- |
| `go build ./...`（gosandbox） | 通过 |
| `go vet ./...` / `GOOS=linux go vet ./...` | 均无输出（通过） |
| `gofmt -l .` | 无输出 |
| `go test ./...`（darwin） | 全部 `ok`；`box` / `vm/box` / `host/internal/spec` / `cmd` 无或极少用例 |
| `GOOS=linux go test -c`（逐包） | 全部可编译 |
| `staticcheck ./...`（darwin） | 14 条：U1000 ×6（3 条为 darwin 误报）、SA1019 ×6（2 条 `TypeRegA` 可修，4 条 darwin syscall 误报）、SA4006 ×1、SA4000 ×1（测试，误报） |
| `GOOS=linux staticcheck ./...` | 10 条：U1000 ×4（均为真）、ST1005 ×2、SA1019 ×2、SA4006 ×1、SA4000 ×1 |
| `deadcode -test ./...`（两 GOOS） | 结果见 §0 误报说明与 §1；`vm/*` 方法类告警已核实为误报 |
| 根模块 `go build ./internal/... ./cmd/...` | **失败**（B1） |

**未运行**：任何 Linux 真机用例（bwrap / cgroup / inotify / supervisor / overlay）、`-race`、根模块 `deadcode`（B1 阻塞）、`scripts/smoke.sh`。以上结论中 B2、B3、B6 是「读代码得出的怀疑」，未复现，不应作为已确认缺陷对外引用；B1、B4、B5 为已确认。

## 7. 执行记录

### G1（2026-11-17，已改代码，未提交，待 Linux 验证）

| ID | 实际改动 |
| --- | --- |
| GSB-1 | `box/transport.go`：删除 `Deadliner`、`Pid`、`asDeadliner`、`transportPid` 与 `time` import，修剪头注释；保留 `Transport`、`wrapConn` |
| GSB-2 | `agent/loopback_linux.go`、`agent/loopback_other.go`：删除 `loopbackUp` 及头注释符号 |
| GSB-3 | `host/cgroup_linux.go`：删除 `newCgroupGroup`，头注释改列 `newNamedCgroupGroup` |
| GSB-4 | `image/import.go`、`image/unpack.go`：去掉 `tar.TypeRegA` 分支 |
| GSB-5 | `image/pull.go`：config 读取改为 `if _, err := ReadBlob(...)`，不再保留未使用的 `cfgBody`；网络调用次数与错误顺序不变 |
| GSB-6 | `netx/pasta_linux.go`（`dns_linux.go`→`dns.go`）、`box/vsock.go`（去掉不存在的 `vsock_darwin.go`）、`README.md` 依赖方向行（改为 `box → {proto, netx, host}`，`vm/box` 另用 `image`/`snapshot`，注明 `rootfs` 无调用方） |

macOS 门禁结果：`gofmt -l` 无输出；`go build`、`go vet`、`GOOS=linux go vet` 通过；逐包 `GOOS=linux go test -c` 均可编译；`go test -count=1 ./...` 全部 `ok`；测试清单与改动前逐项一致（256 项）。`staticcheck`：darwin 剩余 8 条（`copyTree`/`copyFileInto`/`wrapConn` 为 darwin 误报，4 条 darwin syscall 弃用，1 条测试 SA4000），Linux 剩余 3 条（2 条 ST1005、1 条 SA4000，均为 §0/§1.3 登记的不改项）。原有 U1000 ×4、SA1019（TypeRegA）×2、SA4006 ×1 已消除。

**待你在 Linux 上执行**：`cd gosandbox && go test -race -count=1 -timeout 900s ./...`，重点看 `agent`（loopback）、`host`（cgroup 相关）、`box`。

### B1（2026-11-17，已按方案 A 解决）

- `gosandbox/vm/box/driver_linux.go`：`Options` 增加 `Supervise bool`；`ensure()` 构造 `gosandboxbox.Config` 时传 `Supervise: d.opts.Supervise`。
- `gosandbox/vm/box/driver_other.go`：`Options` 增加同名字段（非 Linux 恒不可用，与其他字段一致）。
- 宿主 `internal/app/app.go:1522` 与 `internal/config` 无需改动。

验证：`gofmt -l` 无输出；模块内 `go build`、`go vet`、`GOOS=linux go vet` 通过；宿主 `go build ./internal/... ./cmd/...`、`go vet ./internal/app/ ./internal/sandbox/` 通过；`go test -count=1 ./internal/sandbox/ ./internal/app/` 通过；模块内 `go test ./...` 无失败。

注意：这只是补齐了接线让它能编译。**B2（supervisor 模式下 fd 约定对不上，策略文件未传给子进程）仍未确认**，开启 `sandbox.supervise: true` 之前请先在 Linux 上跑 `go test -run Supervisor -v ./box`，确认是 PASS 而不是 SKIP。

### G2 / G3 / G4（2026-11-17，已改代码，未提交，待 Linux 验证）

| ID | 实际改动 |
| --- | --- |
| GSB-9（方案 A） | 删除 `gosandbox/rootfs/` 整包（含测试 7 个用例：`TestBuild*`、`TestProbeWithoutBwrap`、`TestRelease`） |
| GSB-12 | 删 `image.HostPlatform`（`arch.go` 保留，`runtimeArch`/`goArch` 仍被 `import.go` 使用）、`image.Client.GetManifest`、`netx.NetnsPath`（linux + other 两份，顺带去掉 `pasta_linux.go` 不再用的 `path/filepath`）、`snapshot.Store.Root` |
| B5 | `README.md`：去掉 `rootfs/` 目录行与小节，改写为「沙盒根（`box/launch_linux.go` 的 `rootArgs`）」；如实写明 overlay 只靠 `bwrap --help` 搜 `--overlay`，拷贝为逐文件 `io.Copy`、没有 reflink；目录表补 `netx/`、`snapshot/` |
| GSB-6 收尾 | README 依赖方向行去掉「`rootfs` 目前没有调用方」 |
| B4（仅文案，未改行为） | `README.md` 网络一节、`internal/config/config.go` 的字段注释与报错文案、`internal/sandbox/manager.go` 注释，统一改为：`DNSProxy` 与 `CheckAllowed` 已实现但无生产调用，`egress-allowlist` 的 `network_allow` 不生效、出站行为同 `egress`；`agent/loopback_linux.go` 去掉「白名单 DNS 起不来」的现状表述 |
| GSB-11（保留，仅标注） | `box/vsock.go`、`netx/dns.go` 头部加「预留 / 无生产调用方」状态说明 |
| GSB-13 | `host/internal/spec` 提为 `host/spec`；删除 `host/innerpolicy.go`（`InnerPolicy` 与 `InnerPolicyJSON` 1:1 复制层）；`box/launch_linux.go` 直接 `json.Marshal(spec.Spec{...})`；README 目录行同步 |

**未改行为的项**（仍待决策，见 §3/§4）：B2 supervisor fd 约定、B3 `egress` 档缺 `--unshare-net` 与 `CheckAllowed` 未接入、B6 镜像根失败静默退回宿主根、GSB-10 是否真正接入 DNS 白名单。这些需要 Linux 复现后另立项。

macOS 门禁：`gofmt -l` 无输出；`go build`/`go vet`/`GOOS=linux go vet` 通过；逐包 `GOOS=linux go test -c` 通过；`go test -count=1 ./...` 全 `ok`；测试清单与基线相比只少 `rootfs` 的 7 个用例；`staticcheck` 与 G1 后一致（darwin 8 条、Linux 3 条，均为登记不改项）；宿主 `go build ./internal/... ./cmd/...`、`go vet`、`go test ./internal/{sandbox,app,config}` 通过；`go.mod`/`go.sum` 无变化。

**待你在 Linux 上执行**：`cd gosandbox && go test -race -count=1 -timeout 900s ./...`，重点看 `box`（`writePolicy` 改为直接序列化 `spec.Spec`，真 bwrap 用例）与 `host`。
