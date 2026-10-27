# gosandbox

SimpleBase 的沙盒执行层，独立 Go 模块 `github.com/linkxzhou/SimpleBase/gosandbox`（根 `go.mod` 用 `replace => ./gosandbox` 引用）。**不得 import 根模块任何包**（含 `internal/...`）。

```text
host/                  本机进程隔离（gocloudagent CLI 的本机 shell 用）
  inner/               沙盒内加载 Landlock + seccomp（仅 linux）
cmd/gosandbox-inner/   沙盒内 helper 二进制，纯 Go，不链接 DuckDB
vm/                    云沙盒的执行后端：Driver 接口，以及 local（本机隔离）与 fake（内存假后端，仅测试）
  local/               Driver 的本机实现：每个沙盒是一个工作区目录，命令经 host 隔离执行
  fake/
```

依赖方向只有 `vm/local → host`；`host` 不 import `vm`。`vm/local` 在隔离不可用时返回 `ErrUnavailable` / `ErrBackend`，**不会退回裸执行**。云沙盒不依赖任何第三方沙盒服务或 SDK（不再有 microsandbox）。

## host：本机命令隔离

| 平台 | 方式 |
|---|---|
| Linux | bubblewrap 负责挂载并隔离 user / pid / ipc / net；沙盒内先 Landlock（go-landlock v0.10.1，严格 V3，无 BestEffort），再 seccomp（elastic/go-seccomp-bpf v1.6.0），最后 exec 命令 |
| macOS | 系统自带 `/usr/bin/sandbox-exec`（只认这个绝对路径；其 man page 标注 deprecated） |
| 其他 | `ErrIsolationUnavailable`（fail-closed） |

### 三档权限

| 模式 | 文件系统 | 网络 |
|---|---|---|
| `read-only` | 根只读，`/tmp` 为 tmpfs | 无 |
| `workspace-write` | 根只读；工作区与 `writable_roots` 可写；工作区内 `.gocloudagent` 与强制 deny 表（见下）再盖成只读 | 无 |
| `danger-full-access` | 不启动沙盒（等同关闭隔离） | 不限制 |

网络没有「按域名放行」：放开网络只有 `danger-full-access`。

### 强制 deny 表（planv6.3）

`workspace-write` 下，下面这些路径在**生成命令时已存在**的前提下再盖成只读，即使它们落在可写区内。它们会被沙盒之外的程序执行或读取，沙盒内写入不会立刻越界，但会在用户之后于宿主侧操作该目录时被触发：

| 路径（相对工作区与每个 `writable_roots`） | 谁会用到 |
|---|---|
| `.git/hooks`、`.git/config`、`.git/info/attributes` | git 的 hook、`core.fsmonitor` / `alias` / `filter.*` |
| `.gitmodules` | 子模块 URL 与更新命令 |
| `.mcp.json` | MCP 客户端 |
| `.vscode`、`.idea` | IDE 打开项目即触发的任务与运行配置 |
| `~/.bashrc` `~/.bash_profile` `~/.zshrc` `~/.zprofile` `~/.profile` `~/.gitconfig` | **仅当某个可写根覆盖了 HOME 时**追加；根本来就只读，否则不需要 |

- `Policy.DisableForceDeny=true` 可关闭；零值即开启。关闭时 `Result.Warnings` 如实提示，shell 工具的返回里带 `warnings`。
- `read_only_subpaths` 与强制表取并集后去重。显式写空数组只关闭前者，不关闭强制表。
- 默认的 `read_only_subpaths` 是 `[".gocloudagent"]`，**不含整个 `.git`**，所以沙盒内 `git init` / `add` / `commit` 可用。想要旧行为（整个 `.git` 只读，`git commit` 失败）就显式写 `[".git", ".gocloudagent"]`。
- **只保护已存在的路径。** 对尚不存在的 `.git/hooks` 加 deny 会让沙盒内 `git init` 直接失败（macOS 真机实测）。所以工作区还没有 `.git` 时不做保护，`git init` 之后的**下一条命令**起即受保护；**同一条命令里「`git init` 后写入 hook」的窗口无法关闭**。

### 失败语义

- `Enabled=false` 或 `danger-full-access` → `ErrNotIsolated`，调用方自行裸执行。
- 其他任何建立不了隔离的情况 → `ErrIsolationUnavailable`，**命令不会被执行**，不退回裸执行。
- 命令自己的非零退出码是正常结果。helper 装载失败要求「退出码 71 且输出含固定前缀」同时命中，用户命令自己 `exit 71` 不会被误判。

### Linux 要求与探测（`Probe`，结果进程内缓存）

- bwrap：绝对路径、非 setuid（CVE-2020-5291）、`--help` 含 `--disable-userns`（bubblewrap >= 0.8.0），不会选中当前目录里的同名文件。
- 用户命名空间可用。Ubuntu 的 AppArmor 限制会让探测失败，错误信息指向 <https://bugs.launchpad.net/bugs/2046477>；不要为此把 sysctl 改成 0。
- Landlock ABI >= 3（V3 起含 truncate）；seccomp 可用。
- helper `gosandbox-inner`：显式 `helper_path`，否则取调用方可执行文件同目录。`./build.sh` 在 Linux 上会把它构建到 `gocloudagent` 旁边。

### 资源限额

`memory_max_bytes` / `cpu_max` / `pids_max` 仅当 cgroup v2 已把 `memory cpu pids` 委派给当前子树时才写入。否则**不生效**，`Result.Limits` 报告 `Configured=true, Enforced=false`，shell 工具的返回里带 `"limits":"unenforced"`。macOS 没有 cgroup，恒为未生效。本仓库没有在「已委派」的环境上实测过 `memory.max` 读回。

### 已知限制

- 共享内核：不承诺挡住 Dirty Pipe、CVE-2024-1086 等内核提权；多租户 / 不可信代码不应交给本层；需要 VM 级隔离时应另行实现 `vm.Driver`。
- 挡不住工作区**内部**的误删，所以 `rm -rf` 等的确认保留；`--yolo` 只跳过确认，不跳过隔离。
- 文件工具（`read_file` 等）在父进程里运行，不在沙盒里；`deny_read` 由 `host.DeniesRead` 同时约束它们。
- 默认 `.git` 里只有 hooks / config / info/attributes 只读，其余可写，所以沙盒内 `git commit` 可用。代价（接受的残余风险）：`HEAD`、`refs`、`objects` 可写，沙盒内命令能改写分支指向、植入对象。它们不直接导致宿主侧代码执行，但会影响用户之后检出 / 合并的内容。需要更严时显式配 `read_only_subpaths: [".git", ".gocloudagent"]`，此时 `git commit` 会失败。
- 强制 deny 表只覆盖已存在的路径：同一条命令里先 `git init` 再写 `.git/hooks/*` 会成功（见上文）。
- `deny_read` 支持 glob（`*` 不跨段，`**` 跨段，`?`，`[abc]`；无转义字符，拼不出集合的方括号如 `[user-id]` 按字面量；glob 不得以 `/` 结尾；`literal:` 前缀强制字面量）。两个平台语义不同：Linux 在生成命令时展开成当下存在的路径（快照，之后新建的匹配文件不受掩蔽，单个 glob 最多 1024 条，超限报错而不是截断）；macOS 转成 SBPL 正则，新建文件也被拒。`host.DeniesRead` 始终按完整 glob 判断，所以文件工具比 Linux 的 shell 更严。
- seccomp 默认拒绝新建 `AF_UNIX` / `AF_VSOCK` socket，依赖本地 socket 的工具可能受影响。

## vm/local：云沙盒的本机后端

`vm/local` 把 `host` 包成 `vm.Driver`，供服务端的云沙盒使用：

- 每个沙盒是 `Root/<vm 名>/ws` 目录；`Ensure` 懒创建，`Stop` 只写标记，`Remove` 删目录，`Status` 由目录与标记推导。没有常驻进程。
- `New` 会做一次真实的隔离冒烟测试（跑 `true`），不通过就返回 `vm.ErrUnavailable`。
- 命令固定为 `workspace-write` 档；网络恒为关闭，`Spec.Network=public` 被拒绝。
- 服务自己的数据通过 `Options.ProtectDirs` / `ProtectFiles` 以 `DenyRead` 遮住（`data_dir`、缓存、配置、`.env`），工作区之外的其他沙盒目录也被遮住。
- 沙盒进程的环境变量**不继承**服务进程，只有最小集合加调用方传入的 `Env`。
- 文件读写走 `os.Root`，沙盒里放的符号链接无法把父进程的读写带出工作区；`cwd` 也会校验解析后仍在工作区内。
- 输出受 `Options.MaxOutputBytes` 约束（超限丢弃，不会撑大服务内存）；stdout 与 stderr 合并后放在 `ExecResult.Stdout`。
- API 层的路径是 `Spec.Workdir`（默认 `/workspace`）下的逻辑路径，驱动映射到真实工作区；命令的当前目录就是真实工作区，命令里请用相对路径，字面量 `/workspace` 在沙盒里不存在。
- 资源限额沿用 `host` 的语义：cgroup 不可用时不限制。

### 性能

下面是写计划时在**一台 Linux VM**上的实测（`/bin/true`，5 次热身后 40 次，丢弃输出），仅说明量级，不是 SLO：

| 命令 | median |
|---|---|
| `/bin/true` | 0.277 ms |
| `bwrap --new-session --die-with-parent --unshare-user --unshare-pid --unshare-ipc --unshare-net --ro-bind / / --dev /dev --proc /proc --cap-drop ALL -- /bin/true` | 2.313 ms |

「bwrap + gosandbox-inner + `/bin/true`」的数字**尚未测量**（开发机无 bwrap），在有 bwrap 的机器上补测后再并列写在这里。

## 开发

独立模块，根目录的 `go test ./...` 不含它：

```bash
cd gosandbox
go build ./... && go vet ./... && go test ./...
GOOS=linux go vet ./host/... ./cmd/...        # 交叉检查 linux 专用代码
```

涉及真 bwrap 的用例（`host/run_linux_test.go`）在没有 bwrap、用户命名空间被禁或 helper 构建失败时自动 `Skip`；macOS 上的 `run_darwin_test.go` 会真的调用 `sandbox-exec`。
