# gosandbox

SimpleBase 的沙盒执行层，独立 Go 模块 `github.com/linkxzhou/SimpleBase/gosandbox`（根 `go.mod` 用 `replace => ./gosandbox` 引用）。**不得 import 根模块任何包**（含 `internal/...`）。

```text
host/                  本机进程隔离（gocloudagent CLI 的本机 shell 用）
  inner/               沙盒内加载 Landlock + seccomp（仅 linux）
  spec/                父进程与 helper 之间的协议（策略 JSON、失败退出码）
cmd/                   沙盒内 helper 二进制（产物名 gosandbox-inner，仅 linux），纯 Go，不链接 DuckDB
vm/                    云沙盒的执行后端：Driver 接口，以及 local / box / fake
  local/               Driver 的本机实现：每个沙盒是一个工作区目录，命令经 host 隔离执行
  box/                 常驻 agent 后端：每沙盒一个 bwrap + 一个 PID 1 agent（后台进程存活、热 exec）
  fake/                内存假后端（仅测试）
proto/                 宿主 ↔ 沙盒 agent 的帧协议（len|kind|payload，块数据走裸字节）
agent/                 沙盒内 PID 1：进程表、PTY、信号、rlimits、文件服务、观测、入站中继
box/                   宿主侧：拉起 agent、单写者帧通道、进程句柄、日志落盘
image/                 镜像：引用解析、registry 拉取、blob store、层解包（压平成只读根）
netx/                  出网档位（none / egress）与 pasta 用户态出网
snapshot/              快照与 fork：文件系统归档（内容寻址）
```

依赖方向只有 `vm/* → box → {proto, netx, host}`，`vm/box` 另直接用 `image`、`snapshot`；`host` 不 import 其它新包。`vm/local` 与 `vm/box` 在隔离不可用时都返回 `ErrUnavailable` / `ErrBackend`，**不会退回裸执行**。云沙盒不依赖任何第三方沙盒服务或 SDK；镜像拉取直接实现 registry 协议，不引入 `go-containerregistry`。

## vm/box：常驻 agent 后端

`vm/local` 每条命令新起一个 bwrap，命令结束沙盒就不存在；`vm/box` 里 bwrap 常驻、`gosandbox-inner --agent` 是沙盒的 PID 1：

| | vm/local | vm/box |
|---|---|---|
| 沙盒形态 | 每条命令一个 bwrap | 一个常驻 bwrap + 一个 agent |
| 后台进程 | 随命令结束 | 跨请求存活 |
| exec 成本 | bwrap 启动 + 挂载（~ms） | 一帧往返（~0.1ms） |
| 文件操作 | 宿主侧 `os.Root(ws)` | 沙盒内 `os.Root("/")`（视角一致） |
| 暂停 | 只写标记 | `cgroup.freeze`，进程保留 |
| 根 | 宿主根只读 | 镜像解包出的 lower（overlay 或拷贝） |
| 平台 | Linux / macOS（Seatbelt） | 仅 Linux |

```go
d, err := box.New(ctx, box.Options{
	Root:   "/var/lib/simplebase/sandboxes",
	Helper: "/usr/libexec/gosandbox-inner",
	Bwrap:  "/usr/bin/bwrap",
	Seccomp: "default",          // 沙盒里宿主根不可见，允许本地 socket
	Images: box.NewImageSource(image.NewPuller(store)), // 可选
})
res, err := d.Exec(ctx, vm.Target{Name: "sb1", Spec: vm.Spec{Image: "python:3.12-slim"}}, vm.ExecSpec{Cmd: "python3", Args: []string{"-V"}})
```

`image=host`（或空）时不经过镜像，沙盒以宿主根只读运行——与 `vm/local` 的行为一致。

## image：镜像拉取与解包

```go
store, _ := image.Open("/var/lib/simplebase")
p := image.NewPuller(store)
img, err := p.Pull(ctx, "alpine:3.19", image.PullOptions{Arch: "arm64"})
chainID, err := p.Unpack(ctx, img)   // 压平成只读根
```

- **引用**：`alpine` → `docker.io/library/alpine:latest`；支持 `@sha256:`、`localhost:5000/x`（端口不会被当成 tag）。
- **拉取**：匿名 → 401 + `WWW-Authenticate` → 换 token → 重试；token 按 host+scope 缓存；scope 缩成只 `pull`。凭据只发给 HTTPS 或 loopback。
- **校验**：manifest 与每层都按内容算 sha256，与声明不符直接拒绝（`ErrDigestMismatch`），不留半成品。
- **并发**：同一引用同一平台的并发拉取合并，后来者跟随。
- **导入**：`docker save` 的 tar（`manifest.json` + config + 层文件），离线分发用。

### 解包为什么压平

rootless 下建不了 whiteout 需要的 0:0 字符设备，overlayfs 就认不出删除标记。所以按镜像链把各层依次解到**同一个**只读根，解的时候就执行 `.wh.` 与 `.wh..wh..opq` 的语义。代价是不同镜像不共享层；收益是 lower 只有一层、不需要任何特权。

安全约束（每条都有测试）：

- 所有写入走 `os.Root`（底层 `openat2(RESOLVE_IN_ROOT)`）：绝对路径与 `..` 被限制在 lower 内，**中间段是符号链接也不能逃逸**；
- 解压膨胀上限 8 GiB、条目数 50 万、单文件 4 GiB；
- 设备节点 / FIFO 跳过（不失败）；setuid / setgid 位清掉；
- `<chainID>.done` 标记出现才代表完整，没有标记的目录被重建。

## 沙盒根（`box/launch_linux.go` 的 `rootArgs`）

| 方式 | 条件 | 创建成本 |
|---|---|---|
| overlay | bwrap 支持 `--overlay` | ~ms |
| 拷贝 | bwrap 不支持 `--overlay` | 与镜像大小成正比（逐文件 `io.Copy`，没有 reflink） |
| 宿主根只读 | `image=host`（或空） | 0 |

是否支持 overlay 只看 `bwrap --help` 的输出里有没有 `--overlay`（进程内缓存，每个进程只探一次）。**不会**真起一个 bwrap 验证：版本对但内核没开 overlay、AppArmor 拦了 userns 里的 mount 时，探测会误判为支持。

## 日志与目录监听

进程输出在 `Handle` 转发给调用方的同时落盘（`<root>/<name>/logs/<proc>.log`）：
进程退出后仍能回放，这是 E2B `logs` 的前提。

```go
s, _ := box.Start(ctx, box.Config{LogDir: "/var/lib/sb/sb1/logs", …})
recs, _ := s.Logs(nil, box.TailOptions{Lines: 100})   // 全部进程，最后 100 条
w, _ := s.Watch(ctx, "/workspace", box.WatchOptions{Recursive: true})
for ev := range w.Events() { … }
```

监听走 inotify：200ms 成批合并、按路径去重；递归时运行中新建的子目录也会被登记。
事件通道满时**丢事件并置 `Overflowed()`**——让 agent 阻塞在写帧上会拖住整个沙盒的事件循环。

## 快照与 fork

快照只保存**文件系统**：镜像的写时复制层（`upper/`）与工作区（`ws/`）。
归档按内容寻址，同一状态拍两次只占一份空间。

```go
st, _ := snapshot.Open("/var/lib/sb")
info, _ := st.Create(ctx, "/var/lib/sb/sb1", "sb1", "before-upgrade")
st.Restore(ctx, info.ID, "/var/lib/sb/sb2")   // fork 到新名字
```

**fork 出来的沙盒没有进程**：内存快照需要 CRIU，与 seccomp 冲突，代价不划算。
API 响应里 `processes=false` 明说这一点。

## 入站端口是隧道，不是一次性转发

`Session.DialPort` 返回的是全双工隧道（`net.Conn`）：

```go
conn, _ := sess.DialPort(8080)
go io.Copy(conn, body)
conn.(interface{ CloseWrite() error }).CloseWrite()  // 请求体发完
io.Copy(w, conn)                                     // 读响应直到 EOF
```

半关闭（`CloseWrite`）让沙盒内的 HTTP 服务端知道请求结束——keep-alive、
流式响应、TLS close_notify 都靠它。旧的「一次请求一次响应」做不到这些。

## 网络：默认没有出网

沙盒的 netns 里默认只有 `lo`（bwrap `--unshare-net`）。入站不受影响：
`getHost(port)` 走 agent 中继，不在宿主上开任何监听。

两档（`sandbox.network`）：

| 档位 | 出站 | 依赖 |
|---|---|---|
| `none`（默认） | 无 | — |
| `egress` | pasta 用户态出网 | `pasta`（passt） |

> **两个档位都 `--unshare-net`**：沙盒有自己的 network namespace，看不到宿主网卡。
>
> `egress` 档的出站由 pasta 在**沙盒自己的 netns** 里配地址与默认路由（`--config-net`），
> 再由用户态转发到宿主网络。agent 启动时会在沙盒 netns 里为云元数据
> （`169.254.0.0/16`）、私网（`10/8`、`172.16/12`、`192.168/16`）、CGNAT（`100.64/10`）
> 与回环（`127/8`）加**黑洞路由**，需要 `CAP_NET_ADMIN`（只在沙盒 userns 内有效，配完即丢）。
> 不做域名级白名单：可达的只有公网地址。
>
> pasta 不可用、或无法进入沙盒 netns 时，`egress` 档**没有出网**（沙盒没有 default route），
> 日志里有 `sandbox egress is unavailable` 警告。这是 fail-closed：不会退回共享宿主网络。
>
> 曾有的 `egress-allowlist`（域名白名单，`DNSProxy` + `CheckAllowed`）从未接入生产路径，
> 已于 planv6.6 GSB-10 整体移除；`sandbox.network` 填它会在启动校验时报错。

> 隔离后 `lo` 是 DOWN 的，agent 启动时会把它拉起——否则入站中继失效。

## 磁盘限额

rootless 下没有文件系统配额，靠宿主侧 `du`（`upper` + `ws`，10s 缓存）：
超限**冻结**沙盒（`cgroup.freeze`）而不是杀，用户还能读日志、导出文件、拍快照。

```yaml
sandbox:
  disk_limit_mib: 2048
```

## 多用户镜像（多段 uid 映射）

镜像里以 999 之类的用户运行的服务，在单段映射下起不来（沙盒内只有 uid 0 是真的）。
装上 `uidmap` 并把 subuid 配额给当前用户之后自动启用：

```bash
sudo usermod --add-subuids 100000-165535 --add-subgids 100000-165535 $USER
```

沙盒内可用的 id 从 1000 起（避开 1–999 的系统用户段），总量上限 4096。
缺任意一样（`/etc/subuid`、`newuidmap`）就退回单段映射，并把原因写进日志——不静默降级。

```
ExecRequest{User: "999:999"}   // 无法映射时退出码 126 + 说明，不会静默按 0 运行
```

## 服务重启不丢沙盒（supervisor）

```yaml
sandbox:
  supervise: true
```

```
服务进程 ──unix socket──► supervisor ──socketpair──► bwrap + agent
```

服务进程升级或崩溃后重新连上同一个 socket 即可接管，沙盒里的进程继续跑。
**代价**：断开那一刻 agent 正在发但还没被读走的帧会丢，调用方需要能重试。

## proto：宿主 ↔ agent

帧 `len u32 LE | kind u16 LE | payload`，上限 1 MiB。大块数据（文件写入）跟在帧后以裸字节发送，不复制进帧。

| 组 | 帧 |
|---|---|
| 会话 | `Hello` `Ping/Pong` `Shutdown` `Error` `Ack` |
| 进程 | `Start` → `Started` → `Output*` → `Exited`；`Input` `CloseStdin` `Resize` `Signal` `InputConsumed` |
| 文件 | `FsStat` `FsList` `FsRead` `FsWrite` `FsRemove` `FsMkdir` `FsRename` + 对应回复 |
| 观测 | `StatsQuery` `TopQuery` `ListenQuery` |
| 中继 | `Relay` `RelayData` `RelayHalfClose` `RelayClose` |
| 监听 | `Watch` `Unwatch` `WatchEvent` |

字段与帧长都有上限，超限在编码侧就拒绝；`ReadFrame` 复用调用方的缓冲。**单写者**是硬约束：宿主与 agent 各有一个 writer goroutine，否则帧会交错。

## 快速开始

### 构建

```bash
# Linux：除调用方外还需要 helper，放在调用方可执行文件同目录（或用 Policy.HelperPath 指定）
cd gosandbox
CGO_ENABLED=0 go build -o /path/to/bin/gosandbox-inner ./cmd

# macOS：不需要 helper，系统自带 /usr/bin/sandbox-exec 即可
```

仓库根目录的 `./build.sh` 在 Linux 上会自动把 helper 构建到 `build/bin/gosandbox-inner`（与 `gocloudagent` 同目录）。

### 一次性执行：`host.Run`

```go
import "github.com/linkxzhou/SimpleBase/gosandbox/host"

p := host.Policy{
	Enabled:        true,
	Mode:           host.ModeWorkspaceWrite, // read-only | workspace-write | danger-full-access
	Workspace:      "/abs/path/to/ws",       // 必须是绝对路径
	DenyRead:       []string{"/abs/secret", "/abs/ws/**/*.pem"},
	MemoryMaxBytes: 512 << 20,               // 0 = 不限
	CPUMax:         "100000 100000",         // cgroup cpu.max 语法，此处为 1 核
	PidsMax:        64,
	Timeout:        30 * time.Second,
	MaxOutputBytes: 1 << 20,                 // 服务端必须设置
}
res, err := host.Run(ctx, p, host.Command{
	Argv: []string{"/bin/sh", "-c", "make test"},
	Dir:  p.Workspace,
	Env:  []string{"PATH=/usr/bin:/bin", "HOME=" + p.Workspace},
})
switch {
case errors.Is(err, host.ErrNotIsolated):          // 未开启隔离：调用方自行裸执行
case errors.Is(err, host.ErrIsolationUnavailable): // 建立不了隔离：命令没有执行
case err != nil:
default:
	// res.ExitCode / res.TimedOut / res.Output / res.Limits / res.LimitExceeded / res.Warnings
}
```

### 长会话与交互终端：`host.Start`

```go
proc, err := host.Start(ctx, p, host.StartSpec{
	Command: host.Command{Argv: []string{"/bin/zsh", "-i"}, Env: env},
	PTY:     &host.PTYSize{Cols: 120, Rows: 40}, // nil = 管道模式（stdout+stderr 合并）
})
defer proc.Close()
go io.Copy(os.Stdout, proc.Output()) // 必须持续读取，否则子进程写入阻塞（背压）
proc.Stdin().Write([]byte("ls\n"))
proc.Resize(host.PTYSize{Cols: 100, Rows: 30})
info := proc.Wait() // ExitCode / Signaled
```

`ctx` 取消时杀整个进程组。`Policy.Timeout` 与 `MaxOutputBytes` 不作用于 `Start`，限额（内存 / CPU / 进程数）照常生效。

### 探测与文件工具

- `host.Probe(ctx, p)`：只探测、不执行，结果是 `Caps`（`OS`、`BwrapPath`、`LandlockABI`、`Cgroup`、`SandboxExec` …）。Linux 上的结果在进程内缓存，`ResetProbeCache()` 可清除。
- `host.DeniesRead(p, absPath)`：在父进程里运行的文件工具（`read_file` 等）用它判断路径是否被禁读，与 shell 里实际被遮住的路径保持一致（macOS 包含默认凭据表）。

### 云沙盒后端：`vm/local`

```go
d, err := local.New(ctx, local.Options{
	Root:           "/var/lib/simplebase/sandboxes", // 所有工作区的父目录
	MaxOutputBytes: 1 << 20,
	PidsMax:        128,
	ProtectDirs:    []string{dataDir, cacheDir},
	ProtectFiles:   []string{configPath, envPath},
})
// d 实现 vm.Driver：Ensure / Exec / Start / ReadFile / WriteFile / RemoveFile / ListDir / Status / Stop / Remove
```

`New` 会实际跑一次 `true` 做冒烟测试，不通过返回 `vm.ErrUnavailable`。

## host：本机命令隔离

| 平台 | 方式 |
|---|---|
| Linux | bubblewrap 负责挂载，并隔离 user / pid / ipc / net 命名空间；沙盒内先加载 Landlock（go-landlock v0.10.1，严格 V3，不用 BestEffort），再加载 seccomp（elastic/go-seccomp-bpf v1.6.0），最后 exec 命令 |
| macOS | 系统自带 `/usr/bin/sandbox-exec`（只认这个绝对路径；man page 标注为 deprecated）；资源限额由父进程采样监控执行 |
| 其他 | 返回 `ErrIsolationUnavailable`（fail-closed） |

### 三档权限

| 模式 | 文件系统 | 网络 |
|---|---|---|
| `read-only` | 根只读，`/tmp` 为 tmpfs（macOS 为 `/private/tmp`、`/private/var/folders` 可写） | 无 |
| `workspace-write` | 根只读；工作区与 `writable_roots` 可写；工作区内的 `.gocloudagent` 和强制 deny 表（见下）再盖成只读 | 无 |
| `danger-full-access` | 不启动沙盒（等同关闭隔离） | 不限制 |

网络不支持按域名放行，要放开网络只能用 `danger-full-access`。

### 强制 deny 表（planv6.3）

`workspace-write` 下，下面这些路径如果在**生成命令时已存在**，就再盖成只读，即使它们位于可写区内。这些文件会被沙盒之外的程序执行或读取：沙盒内写入不会立刻越界，但用户之后在宿主侧操作该目录时会被触发：

| 路径（相对工作区与每个 `writable_roots`） | 谁会用到 |
|---|---|
| `.git/hooks`、`.git/config`、`.git/info/attributes` | git 的 hook、`core.fsmonitor` / `alias` / `filter.*` |
| `.gitmodules` | 子模块 URL 与更新命令 |
| `.mcp.json` | MCP 客户端 |
| `.vscode`、`.idea` | IDE 打开项目即触发的任务与运行配置 |
| `~/.bashrc` `~/.bash_profile` `~/.zshrc` `~/.zprofile` `~/.profile` `~/.gitconfig` | **仅当某个可写根覆盖了 HOME 时**才追加；否则根本来就只读，不需要 |

- `Policy.DisableForceDeny=true` 可关闭；零值即开启。关闭时 `Result.Warnings` 会如实提示，shell 工具的返回里带 `warnings`。
- `read_only_subpaths` 与强制表合并后去重。把 `read_only_subpaths` 显式写成空数组只会关掉它自己，不影响强制表。
- `read_only_subpaths` 默认是 `[".gocloudagent"]`，**不含整个 `.git`**，所以沙盒内 `git init` / `add` / `commit` 可用。需要旧行为（整个 `.git` 只读，`git commit` 会失败）时显式写 `[".git", ".gocloudagent"]`。
- **只保护已存在的路径。** 对尚不存在的 `.git/hooks` 加 deny 会让沙盒内 `git init` 直接失败（macOS 真机实测）。因此工作区还没有 `.git` 时不做保护，`git init` 之后的**下一条命令**起才受保护；**同一条命令里「先 `git init`、再写入 hook」的窗口无法关闭**。

### macOS 专有的收紧

**mach-lookup 白名单**（`host.SeatbeltMachServices`）。macOS 上进程间通信大多走 Mach 服务，原先的无条件 `(allow mach-lookup)` 会让命令触达剪贴板、LaunchServices（`open` 启动 App）、`launchctl`、钥匙串等用户会话服务。现在只放行以下服务：

| 服务 | 用途 |
|---|---|
| `com.apple.system.opendirectoryd.libinfo` / `.membership` | `getpwuid`、`whoami`、组成员查询 |
| `com.apple.system.logger`、`com.apple.logd`、`com.apple.diagnosticd` | syslog / os_log |
| `com.apple.system.notification_center` | `notify_*`（时区、语言变化） |
| `com.apple.cfprefsd.daemon` / `.agent` | 偏好读取（策略另有 `user-preference-read`，没有写权限） |
| `com.apple.bsd.dirhelper` | 用户临时目录 |
| `com.apple.trustd.agent` | 证书校验 |
| `com.apple.FSEvents` | 文件监听（`fs.watch`、各类 watch 模式） |

真机实测（macOS 27）：`git init/add/commit`、`clang`、`go run`、`python3 multiprocessing`、`node fs.watch` 均正常；`pbcopy`、`open -a`、`launchctl list`、`defaults write` 被拒绝。如果某个工具因此失败，在 `SeatbeltMachServices` 里补上对应服务名即可（服务名是常量，直接写进策略；路径类的用户输入仍然只经 `-D` 参数传入）。

**默认凭据禁读表**（`host.DefaultDenyReadHomePaths`）。Seatbelt 是「根可读」模型，没有挂载命名空间，HOME 下的凭据默认对命令可见。macOS 上在用户的 `DenyRead` 之后自动追加以下条目：

```text
~/.ssh  ~/.aws  ~/.azure  ~/.gnupg  ~/.kube  ~/.netrc  ~/.git-credentials
~/.docker/config.json  ~/.config/gh  ~/.config/gcloud
~/Library/Keychains  ~/Library/Cookies
```

- 不存在的路径同样生效（覆盖之后新建的文件）。
- 工作区或可写根落在某个条目之内时（例如工作区就是 `~/.aws/proj`），跳过该条目。
- `host.DeniesRead` 同步生效，所以文件工具也读不到这些路径。
- `Policy.DisableDefaultDenyRead=true` 可关闭。Linux 不使用这张表，行为不变。

### 资源限额

| 字段 | Linux（cgroup v2 已委派） | macOS（采样监控） |
|---|---|---|
| `MemoryMaxBytes` | `memory.max`，超限由内核 OOM 处理 | 进程树常驻内存（RSS）之和超限 → SIGKILL 整棵树 |
| `PidsMax` | `pids.max`，超限后 fork 失败 | 进程数超限 → SIGKILL 整棵树（macOS 做不到让 fork 失败） |
| `CPUMax` | `cpu.max` | 按 `quota/period` 折算核数，用令牌桶核算 CPU 时间；超额时 SIGSTOP 整棵树，额度恢复后 SIGCONT |

- `Result.Limits`：`Configured` 表示设置了限额，`Enforced` 表示实际生效，`Backend` 为 `cgroup` / `systemd` / `monitor` / `none`。Linux 上只有 cgroup v2 把 `memory cpu pids` 委派给当前子树时才写入，否则为 `Configured=true, Enforced=false`，shell 工具的返回里带 `"limits":"unenforced"`。本仓库还没有在「已委派」的环境上实测读回 `memory.max`。
- macOS 因超限被杀时：`Result.LimitExceeded` 为 `memory` 或 `pids`，`ExitCode = 137`，`Warnings` 里追加 `killed: <name> limit exceeded`。
- macOS 监控的精度限制（实现见 `host/limits_darwin.go`，采样间隔 100ms）：
  - 采样间隔内的瞬时内存峰值可能漏检；
  - 统计范围是「同进程组 ∪ 子孙进程」，被 launchd 收养且换了进程组的孙进程不在统计内；
  - 已退出进程的 CPU 时间不再计入，所以「反复 fork 很快退出的子进程」这类负载基本不受 CPU 配额约束（单进程或长驻多进程的忙循环会被有效节流）。

### 失败语义

- `Enabled=false` 或 `danger-full-access` → `ErrNotIsolated`，由调用方自行裸执行。
- 其他任何建立不了隔离的情况 → `ErrIsolationUnavailable`，**命令不会被执行**，不退回裸执行。
- 命令自己的非零退出码是正常结果。判定 helper 装载失败要求「退出码 71 且输出含固定前缀」同时满足，用户命令自己 `exit 71` 不会被误判。macOS 上 `sandbox-exec` 无法加载策略时输出以 `sandbox-exec:` 开头，同样判为 `ErrIsolationUnavailable`。

### Linux 要求与探测（`Probe`，结果进程内缓存）

- bwrap：必须是绝对路径、非 setuid（CVE-2020-5291）、`--help` 输出含 `--disable-userns`（bubblewrap >= 0.8.0），不会选中当前目录里的同名文件。
- 用户命名空间可用。Ubuntu 的 AppArmor 限制会导致探测失败，错误信息会指向 <https://bugs.launchpad.net/bugs/2046477>；不要为此把 sysctl 改成 0。
- Landlock ABI >= 3（V3 起包含 truncate）；seccomp 可用。
- helper `gosandbox-inner`（源码在 `cmd/`）：优先用显式配置的 `helper_path`，否则取调用方可执行文件同目录。

### 已知限制

- 共享内核：不承诺挡住 Dirty Pipe、CVE-2024-1086 等内核提权；多租户、不可信代码不应交给本层；需要 VM 级隔离时应另行实现 `vm.Driver`（macOS 可选 Virtualization.framework，在虚拟机内复用 Linux 路径）。
- 挡不住工作区**内部**的误删，所以 `rm -rf` 等命令的确认保留；`--yolo` 只跳过确认，不跳过隔离。
- 文件工具（`read_file` 等）在父进程里运行，不在沙盒里；`deny_read` 通过 `host.DeniesRead` 同时约束它们。
- 默认 `.git` 里只有 hooks / config / info/attributes 只读，其余可写，所以沙盒内 `git commit` 可用。代价（已接受的残余风险）：`HEAD`、`refs`、`objects` 可写，沙盒内命令能改写分支指向、植入对象。它们不直接导致宿主侧代码执行，但会影响用户之后检出、合并的内容。需要更严时显式配置 `read_only_subpaths: [".git", ".gocloudagent"]`，此时 `git commit` 会失败。
- 强制 deny 表只覆盖已存在的路径：同一条命令里先 `git init` 再写 `.git/hooks/*` 会成功（见上文）。
- `deny_read` 支持 glob（`*` 不跨段，`**` 跨段，`?`，`[abc]`；没有转义字符，拼不出集合的方括号如 `[user-id]` 按字面量处理；glob 不得以 `/` 结尾；`literal:` 前缀强制按字面量）。两个平台语义不同：
  - Linux 在生成命令时把 glob 展开成当下已存在的路径（快照语义，之后新建的匹配文件不受遮蔽；单个 glob 最多 1024 条，超限直接报错而不是截断）；
  - macOS 把 glob 转成 SBPL 正则，之后新建的文件同样被拒。
  - `host.DeniesRead` 始终按完整 glob 判断，所以在 Linux 上文件工具比 shell 更严。
- Linux 的 seccomp 默认拒绝新建 `AF_UNIX` / `AF_VSOCK` socket，依赖本地 socket 的工具可能受影响。macOS 不拦截本地 socket，但网络规则仍是全拒绝。
- macOS 的 `sandbox-exec` 已被标注为 deprecated，目前仍可用（Codex、Claude Code 的 sandbox-runtime 也在用）；如果 Apple 移除它，`Probe` 会返回 `ErrIsolationUnavailable`，命令不会以无隔离方式执行。

## vm/local：云沙盒的本机后端

`vm/local` 把 `host` 包装成 `vm.Driver`，供服务端的云沙盒使用：

- 每个沙盒对应 `Root/<vm 名>/ws` 目录；`Ensure` 懒创建，`Stop` 只写标记，`Remove` 删除目录，`Status` 由目录与标记推导。没有常驻进程。
- `New` 会做一次真实的隔离冒烟测试（跑 `true`），不通过返回 `vm.ErrUnavailable`。
- 命令固定使用 `workspace-write` 档；网络恒为关闭，`Spec.Network=public` 会被拒绝。
- 服务自己的数据通过 `Options.ProtectDirs` / `ProtectFiles` 以 `DenyRead` 遮住（`data_dir`、缓存、配置、`.env`），工作区之外的其他沙盒目录也被遮住。
- 沙盒进程的环境变量**不继承**服务进程，只有最小集合加调用方传入的 `Env`。
- 文件读写走 `os.Root`，沙盒里放置的符号链接无法把父进程的读写带出工作区；`cwd` 也会校验解析后仍在工作区内。
- 输出受 `Options.MaxOutputBytes` 约束（超限部分丢弃，不会撑大服务内存）；stdout 与 stderr 合并后放在 `ExecResult.Stdout`。
- API 层的路径是 `Spec.Workdir`（默认 `/workspace`）下的逻辑路径，由驱动映射到真实工作区。命令的当前目录就是真实工作区，请在命令里使用相对路径；字面量 `/workspace` 在沙盒里不存在。
- 资源限额沿用 `host` 的语义：Linux 上 cgroup 不可用时不限制，macOS 由采样监控执行。

### 性能

下面是写计划时在**一台 Linux VM** 上的实测（`/bin/true`，5 次热身后跑 40 次，丢弃输出），仅说明量级，不是 SLO：

| 命令 | median |
|---|---|
| `/bin/true` | 0.277 ms |
| `bwrap --new-session --die-with-parent --unshare-user --unshare-pid --unshare-ipc --unshare-net --ro-bind / / --dev /dev --proc /proc --cap-drop ALL -- /bin/true` | 2.313 ms |

「bwrap + gosandbox-inner + `/bin/true`」的耗时**尚未测量**（开发机没有 bwrap），之后在有 bwrap 的机器上补测再并列写在这里。macOS 的采样监控只在配置了限额时启动，每 100ms 做一次 `proc_info` 枚举，开销与进程数成正比。

## 开发

本模块是独立模块，根目录的 `go test ./...` 不包含它：

```bash
cd gosandbox
go build ./... && go vet ./... && go test ./...
GOOS=linux go vet ./host/... ./cmd/...        # 交叉检查 linux 专用代码
GOOS=linux go test -c -o /dev/null ./host     # 交叉编译 linux 测试
```

- 涉及真实 bwrap 的用例（`host/run_linux_test.go`，会现场 `go build ./cmd` 得到 helper）在没有 bwrap、用户命名空间被禁或 helper 构建失败时自动 `Skip`。
- macOS 上的 `host/run_darwin_test.go` 会真的调用 `sandbox-exec`，覆盖：写入边界、强制 deny 表、`deny_read` 与 glob、网络拒绝、超时杀进程树、mach-lookup 白名单、默认凭据禁读，以及内存 / 进程数 / CPU 三种限额（部分用例需要 `python3`，缺失时 `Skip`）。
