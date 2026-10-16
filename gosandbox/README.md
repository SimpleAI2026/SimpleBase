# gosandbox

SimpleBase 的沙盒执行层，独立 Go 模块 `github.com/linkxzhou/SimpleBase/gosandbox`（根 `go.mod` 用 `replace => ./gosandbox` 引用）。**不得 import 根模块任何包**（含 `internal/...`）。

```text
host/                  本机进程隔离（cloudagent CLI 的本机 shell 用）
  inner/               沙盒内加载 Landlock + seccomp（仅 linux）
cmd/gosandbox-inner/   沙盒内 helper 二进制，纯 Go，不链接 DuckDB
vm/                    VM 执行后端：Driver 接口、cloud（microsandbox Cloud）、fake（测试）
```

`host` 与 `vm` 互不 import。本机隔离**不是** Cloud 沙盒失败后的回退，也没有 `Driver` 实现。

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
| `workspace-write` | 根只读；工作区与 `writable_roots` 可写；工作区内 `.git`、`.cloudagent` 再盖成只读 | 无 |
| `danger-full-access` | 不启动沙盒（等同关闭隔离） | 不限制 |

网络没有「按域名放行」：放开网络只有 `danger-full-access`。

### 失败语义

- `Enabled=false` 或 `danger-full-access` → `ErrNotIsolated`，调用方自行裸执行。
- 其他任何建立不了隔离的情况 → `ErrIsolationUnavailable`，**命令不会被执行**，不退回裸执行。
- 命令自己的非零退出码是正常结果。helper 装载失败要求「退出码 71 且输出含固定前缀」同时命中，用户命令自己 `exit 71` 不会被误判。

### Linux 要求与探测（`Probe`，结果进程内缓存）

- bwrap：绝对路径、非 setuid（CVE-2020-5291）、`--help` 含 `--disable-userns`（bubblewrap >= 0.8.0），不会选中当前目录里的同名文件。
- 用户命名空间可用。Ubuntu 的 AppArmor 限制会让探测失败，错误信息指向 <https://bugs.launchpad.net/bugs/2046477>；不要为此把 sysctl 改成 0。
- Landlock ABI >= 3（V3 起含 truncate）；seccomp 可用。
- helper `gosandbox-inner`：显式 `helper_path`，否则取调用方可执行文件同目录。`./build.sh` 在 Linux 上会把它构建到 `cloudagent` 旁边。

### 资源限额

`memory_max_bytes` / `cpu_max` / `pids_max` 仅当 cgroup v2 已把 `memory cpu pids` 委派给当前子树时才写入。否则**不生效**，`Result.Limits` 报告 `Configured=true, Enforced=false`，shell 工具的返回里带 `"limits":"unenforced"`。macOS 没有 cgroup，恒为未生效。本仓库没有在「已委派」的环境上实测过 `memory.max` 读回。

### 已知限制

- 共享内核：不承诺挡住 Dirty Pipe、CVE-2024-1086 等内核提权；多租户 / 不可信代码用 Cloud microVM。
- 挡不住工作区**内部**的误删，所以 `rm -rf` 等的确认保留；`--yolo` 只跳过确认，不跳过隔离。
- 文件工具（`read_file` 等）在父进程里运行，不在沙盒里；`deny_read` 由 `host.DeniesRead` 同时约束它们。
- `.git` 默认只读，所以沙盒内 `git commit` 会失败；要让 agent 提交，把 `.git` 从 `read_only_subpaths` 去掉，并接受 hook 可被改写的风险。
- seccomp 默认拒绝新建 `AF_UNIX` / `AF_VSOCK` socket，依赖本地 socket 的工具可能受影响。

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
