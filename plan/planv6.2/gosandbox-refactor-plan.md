# gosandbox 独立与本机命令隔离实施计划（planv6.2）

> 依据：`plan/planv6.2/sandbox-research.md`（以下简称「调研」，§5 为其本机隔离层草案）。
> 本文是计划，不是已落地代码。与调研 §5 不一致处，以本文为准，差异见 §9。

## 1. 目标与已定结论

1. Linux 自托管时，gocloudagent 本机 `shell` 工具在**原有破坏性命令确认之后**，改走 `gosandbox/host`。`Yolo` 只跳过确认，不跳过隔离。
2. 隔离路线与 Codex CLI、Anthropic sandbox-runtime 一致，不起 VM，只是 fork+exec，调研实测 bwrap 增量约 2 ms：
   - Linux：bubblewrap 负责挂载，并隔离 user / pid / ipc / net；沙盒内先 Landlock（`go-landlock` v0.10.1），再 seccomp（`elastic/go-seccomp-bpf` v1.6.0）。
   - macOS：系统自带 `/usr/bin/sandbox-exec`（只认这个绝对路径）。
3. 权限三档，与 Codex 对齐：`read-only`、`workspace-write`、`danger-full-access`。前两档默认不能上网。
4. 配置在 `agent.shell_isolation`，**默认关闭**。cgroup 只在系统允许时才限制内存 / CPU / 进程数，否则结果里明确标「未限制」。
5. 这一版不让本机隔离实现 `Driver`；Cloud 沙盒失败仍不退回本机（`app.go` 现有 `no local fallback` 日志保持）。
6. 把沙盒执行层独立成库 `gosandbox`；`internal/sandbox` 缩成 SimpleBase 的业务层（项目配额、元数据、reaper）。

不做：网络 allowlist / 代理、`sandbox.backend: host`、wazero / goja、microsandbox 本地 libkrun 后端、多租户不可信代码（仍走 Cloud microVM）。

## 2. 现状（已读代码）

`internal/sandbox` 共 2191 行：

| 文件 | 行数 | 性质 |
|---|---|---|
| `driver.go` | 80 | `Driver` 接口 + `Spec/Target/ExecSpec/ExecResult/FileEntry`，通用 |
| `driver_cloud.go` | 238 | microsandbox SDK 封装；依赖 `internal/config`、`internal/observability`；用 `os.Setenv` 写 `MSB_*`（进程全局） |
| `driver_fake.go` | 354 | 内存假驱动，仅测试 |
| `errors.go` | 23 | 驱动与 Manager 的错误混在一起 |
| `paths.go` | 88 | `cleanPath/validateEnv/truncateUTF8/validName` 属 Manager 校验；`CloudName` 属命名（`ThreadCloudName` 已随 v3 兼容移除） |
| `manager.go` | 934 | 元数据、配额、锁、幂等、reaper、thread 沙盒；依赖 `config`、`Store` |

调用方：`internal/app/app.go`（装配 + `sandboxAdapter`）、`internal/app/sandbox_store.go`、`internal/api/sandbox_handler.go`、`internal/api/error.go`（按 `errors.Is` 映射 HTTP）、`internal/config`。

gocloudagent 本机 `shell`（`internal/gocloudagent/tools/tools.go` 第 115–162 行）：确认通过后 `exec.CommandContext("/bin/sh","-c")`，`cmd.Env = sanitizedEnv(...)`，`CombinedOutput`。`resolvePath` 只约束 `cwd` 参数。装配点是 `internal/gocloudagent/cli/repl.go:154` 的 `&tools.Workspace{Root, Confirm, Yolo}`。

根 `go.mod` 里 `microsandbox/sdk/go v0.7.4` 是 indirect；`golang.org/x/sys v0.40.0` 是 indirect；没有 landlock / seccomp。

## 3. 目标架构

### 3.1 模块与目录

`gosandbox/` 做成**嵌套 Go 模块**：模块路径 `github.com/linkxzhou/SimpleBase/gosandbox`，自带 `go.mod` / `go.sum`，根 `go.mod` 用 `replace ... => ./gosandbox` 引用。库内**不得 import** SimpleBase 的任何 `internal/...`，也不 import 根模块。

```text
gosandbox/
  go.mod  go.sum  README.md
  host/                      # 本机进程隔离（本计划新增）
    policy.go                # Mode / Policy / Validate / DeniesRead
    probe.go                 # Caps / Probe（bwrap、userns、Landlock ABI、seccomp、cgroup）
    bwrap_args.go            # 纯函数，全平台可测
    run.go                   # Run(ctx, Command) → Result；ErrNotIsolated
    run_linux.go             # 启动 bwrap、超时杀进程组
    run_darwin.go            # sandbox-exec
    seatbelt.go              # SBPL 生成（纯函数）
    cgroup_linux.go
    inner/                   # 沙盒内加载 Landlock + seccomp（仅 linux）
  vm/                        # 由 internal/sandbox 迁入的「VM 执行后端」
    driver.go errors.go      # Driver 接口、Spec/Target/ExecSpec/ExecResult/FileEntry、驱动错误
    cloud/                   # microsandbox Cloud 驱动（独立 Options，不依赖 SimpleBase 配置）
    fake/                    # 内存假驱动
  cmd/gosandbox-inner/       # 沙盒内 helper：只 import host/inner，不链接 DuckDB
```

依赖方向：`internal/gocloudagent/tools → gosandbox/host`；`internal/sandbox → gosandbox/vm`；`internal/app → gosandbox/vm/{cloud,fake}`。`host` 与 `vm` 互不 import。

`internal/sandbox` 保留：`Manager`、`Store`、`Sandbox/CreateInput/...` 业务类型、`paths.go` 里的校验与命名、业务错误。`config.SandboxConfig` 留在 `internal/config`（配置只来自 `config.yaml` + `SIMPLEBASE_*`），由 `app` 转成 `cloud.Options`。

### 3.2 `host` 公共 API（要新写）

```go
type Mode string // "read-only" | "workspace-write" | "danger-full-access"

type Policy struct {
    Enabled          bool
    Mode             Mode
    Workspace        string
    WritableRoots    []string
    ReadOnlySubpaths []string      // 相对 Workspace 或绝对；默认 .git .gocloudagent
    DenyRead         []string      // 已展开 ~ 的绝对路径
    MemoryMaxBytes   int64         // 0 = 不限
    CPUMax           string        // cgroup v2 cpu.max 原文，空 = 不限
    PidsMax          int64         // 0 = 不限
    Timeout          time.Duration
    BwrapPath        string        // 空 = PATH 查找，结果必须是绝对路径
    HelperPath       string        // 空 = 与 os.Executable() 同目录的 gosandbox-inner
}

type Command struct {
    Argv []string; Dir string; Env []string
}

type Limits struct {
    Enforced bool     // 内存/CPU/进程数是否真的生效
    Backend  string   // "cgroup" | "systemd" | "none"
}

type Result struct {
    Output   []byte   // stdout+stderr 合并（shell 工具现状如此）
    ExitCode int
    TimedOut bool
    Limits   Limits   // 配置了限额但未生效时 Enforced=false，调用方必须回显「未限制」
}

var ErrNotIsolated = errors.New(...) // Enabled=false 或 danger-full-access：调用方走原裸执行
var ErrIsolationUnavailable = ...    // 其他一切失败：不执行命令（fail-closed）

func Validate(Policy) error
func Probe(ctx, Policy) (Caps, error)
func DeniesRead(Policy, absPath string) bool
func Run(ctx, Policy, Command) (Result, error)
```

调研草案的五返回值 `Launch` 改为 `Run` + `Result`，便于以后加字段而不破坏签名。

### 3.3 Linux 启动链

```text
父进程(gocloudagent)
  resolvePath → approve(破坏性确认) → Probe
  → [cgroup 子组，可选] → exec bwrap …
        bwrap：只读根 → 可写 bind → 只读子路径/deny_read 覆盖 → /dev /proc → unshare user/pid/ipc/net → cap-drop ALL
          → gosandbox-inner
                1. 读 /run/gosandbox/policy.json（父进程经 --file 3 写入，策略不进 argv / environ）
                2. Landlock：RODirs("/") + RWDirs(workspace, writable_roots)
                3. seccomp：默认允许 + 拒绝清单
                4. syscall.Exec /bin/sh -c <command>
```

要点（沿用调研 §5.6，已按 Codex / sandbox-runtime 源码核对的部分不再重复论证）：

- **顺序**：先 Landlock 后 seccomp（seccomp 先装会拒绝 Landlock 需要的 syscall）。
- **bwrap 参数**固定顺序：`--new-session --die-with-parent --ro-bind / /` → `--bind` 工作区与 `writable_roots` → `--ro-bind` 只读子路径 → `--ro-bind /dev/null`（deny_read 文件）/ `--tmpfs`（deny_read 目录）→ `--dev /dev --proc /proc` → `--unshare-user --unshare-pid --unshare-ipc --unshare-net` → `--cap-drop ALL`。不用 `--unshare-all`；`--proc` 失败即失败，不保留宿主 `/proc`；不加 `--as-pid-1`。`read-only` 档不 bind 工作区，`/tmp` 用 tmpfs。
- **seccomp 拒绝清单**（默认允许 + `EPERM`）：`io_uring_*`、`ptrace`、`process_vm_readv/writev`；`socket(AF_UNIX|AF_VSOCK)`；网络 none 时再拒 `connect/accept/accept4/bind/listen/sendto/sendmmsg`。不拒 `socketpair`、`recvfrom`（Codex 注释：cargo / clippy 子进程需要）。条件字段按库里的拼写 `NamesWithCondtions`。
- **超时**：`Setpgid` + 取消时 `SIGKILL` 进程组再 `Wait`，不依赖 `CommandContext` 只杀直接子进程的行为。
- **bwrap 可信性**：拒绝相对路径与 cwd 下的同名文件；拒绝 setuid 的 bwrap（CVE-2020-5291）；`--help` 无 `--disable-userns`（< 0.8.0）则拒绝。探测命令 `bwrap --unshare-user --ro-bind / / --dev /dev --proc /proc --die-with-parent -- /bin/true` 失败（如 Ubuntu AppArmor 限制 userns）→ `ErrIsolationUnavailable`，错误信息指向 Launchpad #2046477，不建议改 sysctl。结果在进程内缓存。
- **helper 定位**：`gosandbox-inner` 是单独的小二进制（纯 Go，`CGO_ENABLED=0` 可构建），以保证启动快、不拖进 gocloudagent 的依赖。找不到则 fail-closed。

### 3.4 cgroup（可选，明确降级）

父进程按顺序尝试：① 当前 cgroup v2 子树已委派 `memory cpu pids` → 建 `sb-<id>` 写 `memory.max/cpu.max/pids.max` 并把 bwrap pid 写入 `cgroup.procs`；② `systemd-run --scope`（`MemoryMax/CPUQuota/TasksMax`）；③ 都不行 → `Limits{Enforced:false, Backend:"none"}`，命令照常在文件系统 + seccomp 隔离下运行。

不引入 `containerd/cgroups/v3`。`cpu.max` 原样写入，非法内容让该次启动失败。调研 §5.9 实测：该 VM 环境下子树打不开且无 systemd 总线，因此本机验证只能覆盖第③条路径；①② 需要在委派环境或裸机上补测。

`Limits.Enforced=false` 且策略配置了任一限额时，shell 工具必须在返回给模型 / 日志的元数据里带 `"limits":"unenforced"`，不静默。

### 3.5 macOS

只用 `/usr/bin/sandbox-exec -p <SBPL> -Dkey=path -- <cmd>`。SBPL 以 `(deny default)` 开头，允许 `process-exec/process-fork`，`file-read*` 放开（deny_read 路径除外），`file-write*` 只给工作区与 `writable_roots`，默认无网络。`sandbox-exec` 在 man page 标注 deprecated，文档照实写。无 bwrap 的 macOS 不尝试 Linux 路径；生成失败 → `ErrIsolationUnavailable`。macOS 没有 cgroup，`Limits` 恒为未限制。

### 3.6 三档语义

| 模式 | 文件系统 | 网络 |
|---|---|---|
| `read-only` | 根只读，`/tmp` tmpfs | 无 |
| `workspace-write` | 根只读；工作区与 `writable_roots` 可写；工作区内 `.git`、`.gocloudagent` 再盖只读 | 无 |
| `danger-full-access` | 不启动沙盒，保持今天的裸执行 | 不限制 |

`enabled=false` 与 `danger-full-access` 都返回 `ErrNotIsolated`。`Yolo` 不会变成 `danger-full-access`。默认模式（开启后未指定时）取 `workspace-write`。

## 4. 配置

### 4.1 服务器 `config.yaml`（`agent.shell_isolation`，默认关闭）

```yaml
agent:
  shell_isolation:
    enabled: false
    mode: workspace-write          # read-only | workspace-write | danger-full-access
    writable_roots: []             # 额外可写绝对路径，必须已存在
    read_only_subpaths: [".git", ".gocloudagent"]
    deny_read: ["~/.ssh", "~/.aws", "~/.config/gcloud"]
    memory_max_bytes: 0            # 0 = 不限；cgroup 不可用时标「未限制」
    cpu_max: ""                    # cgroup v2 cpu.max 原文，如 "100000 100000"
    pids_max: 0
    timeout: 120s
    bwrap_path: ""
    helper_path: ""
```

- 环境变量（沿用 `SIMPLEBASE_AGENT_*` 风格）：`..._SHELL_ISOLATION_ENABLED`、`..._MODE`、`..._TIMEOUT`。列表字段只放 YAML。
- `internal/config` 增 `ShellIsolationConfig` 与校验：未知 mode、不存在的 `writable_roots`、非法 `cpu_max`、负数限额均报错。不含任何密钥字段。
- 配置层把它转成 `host.Policy`（`~` 展开、相对路径基于 Workspace），`config` 包本身不 import `gosandbox`，转换放在 gocloudagent 装配侧。

### 4.2 CLI

CLI 不读服务器 `config.yaml`，沿用 `~/.gocloudagent/config.json`（0600），增同形字段 `shell_isolation`；命令行 `--shell-isolation <mode>` 覆盖；缺省用内置默认（`enabled:false`）。`buildRunner` 把结果填进 `tools.Workspace.Isolation`，工具里不读任何配置文件。

## 5. 对现有代码的修改

### 5.1 `internal/gocloudagent/tools/tools.go`

- `Workspace` 增 `Isolation host.Policy`。
- `shellTool`：确认通过后调 `host.Run`；`ErrNotIsolated` → 走原 `exec.CommandContext`；其他错误 → 返回 `ErrEnvelope("isolation_unavailable", …)`，不执行命令。超时仍返回 `timeout`。
- `resolvePath` 调用方（read / write / edit / glob / grep）在 `host.DeniesRead` 为真时返回 `path_not_allowed`，否则模型可用 `read_file` 读到 shell 里已盖住的 token。
- `sanitizedEnv`、`IsDangerousShell`、`approve` 不改。
- `enabled=false` 时现有测试（如 `echo hello-from-shell`）输出必须不变。

### 5.2 `internal/sandbox` 重构

按下表移动 / 改造，**HTTP API、前端、`sys_sandboxes` 表结构与 SDK 均不变**（本计划不涉及任何数据库结构变更）。

| 现有 | 去向 |
|---|---|
| `driver.go` | 整文件删除：类型别名与 `DriverCloud`/`DriverFake` 再导出全部取消；`api` / `app` / `manager` 直接 import `gosandbox/vm` |
| `driver_cloud.go` + `driver_cloud_integration_test.go` | → `gosandbox/vm/cloud`；入参改 `cloud.Options{APIKey, APIURL, Home}`，去掉对 `config`、`observability` 的依赖，日志改由 `app` 打 |
| `driver_fake.go` | → `gosandbox/vm/fake`（`fake.New()`） |
| `errors.go` | 驱动侧错误（`ErrBackend`、`ErrUnavailable`、`ErrFileNotFound`）→ `vm`，**不再再导出**；`api/error.go` 直接引用 `vm.ErrX`。业务错误（`ErrNotFound`、`ErrInvalidSpec` 等）留 `sandbox` |
| `manager.go`、`paths.go`、测试 | 留在 `internal/sandbox`，只改 import 与 `NewFakeDriver`→`fake.New()` |

`Setenv` 说明：调研建议“改为通过 SDK option 传入”，但我在 SDK v0.7.4 源码里只检索到 `DefaultBackendInfo()` 与读取 `MSB_*` 环境变量的实现，**没有找到 option 入口**。因此 `cloud.New` 仍需要写进程环境；改为只在 `New` 内集中设置并用 `sync.Once` 保证一次，注释写明限制。若升级 SDK 后出现 option 再改。

### 5.3 清理项

确定要做：
1. `driver_cloud.go` 对 `internal/config` / `internal/observability` 的耦合（随迁移消失）。
2. `Setenv` 散落与重复设置（见上）。
3. 根 `go.mod` 中 microsandbox 的直接引用迁入 `gosandbox/go.mod`。

已定论并执行（本项目为新项目，不做兼容层）：
- **别名层整体删除**：`internal/sandbox/driver.go` 移除，`errors.go` 的 3 个再导出删除，`api` / `app` / `manager` 直接依赖 `gosandbox/vm`。
- **死代码删除**：`vm.DriverCloud` / `vm.DriverFake` 常量与 `Driver.Kind()` 方法（全仓零调用）从 `vm`、`cloud`、`fake` 一并删除。
- **v3 兼容移除**：`ThreadCloudName` 与 `sb-` 前缀删除；agent thread 沙盒统一用 `CloudName`（`sbx-` + UUID）；`EnsureForThread` / `ReleaseThread` 不再为「无元数据的旧 VM」走 driver 的兜底删除（没有元数据行即视为无沙盒）。
- 保留：`RunOnce`、幂等表 `idem`（HTTP 路由与 e2e 在用）、`SIMPLEBASE_SANDBOX_ALLOW_FAKE` 路径（e2e 在用）。

### 5.4 依赖与构建

- 只新增两个依赖，版本钉死：`github.com/landlock-lsm/go-landlock v0.10.1`、`github.com/elastic/go-seccomp-bpf v1.6.0`（写在 `gosandbox/go.mod`）。不升级 `golang.org/x/sys`；若它们要求更高版本，停下来与你确认，不自行升级。
- 根 `go.mod` 增 `require github.com/linkxzhou/SimpleBase/gosandbox v0.0.0` + `replace => ./gosandbox`。
- **嵌套模块不会被根目录 `go test ./...` / `go build ./...` 覆盖**：新增命令 `(cd gosandbox && go build ./... && go vet ./... && go test ./...)`，并写入 `scripts/test.sh`、`scripts/smoke.sh`、`AGENTS.md` 的命令表。
- `Dockerfile`：在 `go mod download` 之前补 `COPY gosandbox/go.mod gosandbox/go.sum gosandbox/`，否则 `replace` 目标缺失导致构建失败。服务镜像仍是 distroless，**不含 bwrap 与 helper**，本机隔离只用于 gocloudagent CLI。
- `build.sh`：gocloudagent 的构建产物旁同步产出 `gosandbox-inner`。
- 现有 `AGENTS.md` 写的是“单 go.mod，gofunction 无独立模块”，需改为“`gosandbox` 为嵌套模块”。

## 6. 里程碑

| 里程碑 | 内容 | 验收 |
|---|---|---|
| **M0 迁移（纯搬家，无行为变化）** | 建 `gosandbox` 模块、`replace`、Dockerfile；迁 `vm/` `vm/cloud` `vm/fake`；`internal/sandbox` / `app` 改 import；错误再导出 | 根与 gosandbox 两个模块 build / vet / test 全过；`internal/api`、`internal/app` 沙盒相关测试不改断言即通过；Cloud 失败仍打 `sandbox disabled: cloud backend unavailable (no local fallback)` |
| **M1 清理** | §5.3「确定要做」三项；`cloud.Options` 去耦合 | 同 M0；`internal/sandbox` 无 microsandbox import（`go list -deps` 验证） |
| **M2 host：策略、探测、参数** | `Policy/Validate/DeniesRead/Probe/BwrapArgs`；`config.ShellIsolationConfig` + yaml/env + `config.example.yaml` | 黄金文件 `TestBwrapArgsOrder`；`Validate` 拒绝非法输入；`TestRejectsRelativeBwrap`、`TestSetuidBwrapRejected`；`go test ./internal/config/` |
| **M3 host：Linux 运行 + helper** | `Run`、`gosandbox-inner`（Landlock V3 严格 + seccomp）、进程组超时；tools 集成；CLI 配置 | 见 §7 逃逸表在有 bwrap 的 Linux 上通过；无 bwrap 环境自动 `t.Skip`；`enabled=false` 时 `go test ./internal/gocloudagent/...` 与现状一致 |
| **M4 deny_read 与文件工具** | `resolvePath` 调用方接 `DeniesRead` | `TestDenyReadToken` |
| **M5 cgroup 或明确降级** | `cgroup_linux.go`、`Limits` | 本机（无委派）断言 `Enforced=false` 且命令仍隔离运行；委派环境补测 `memory.max` 读回 |
| **M6 macOS Seatbelt** | `seatbelt.go`、`run_darwin.go` | SBPL 生成黄金文件（全平台）；macOS 上手动跑一遍写 / 读 / 断网用例 |
| **M7 文档与边界** | 更新 `internal/AGENTS.md`、根 `AGENTS.md`、`docs/automation/sandbox.md`、`gosandbox/README.md`、`config.example.yaml` | 文档与代码一致；见 §8 |

M0、M1 可独立合并，不依赖后续里程碑。

## 7. 测试

`//go:build linux` 的用例才调用真 bwrap；其他平台只测参数生成与 `Validate`。

| 用例 | 断言 |
|---|---|
| `TestEscapeEtc` | 沙盒内 `touch /etc/…` 失败，宿主无此文件 |
| `TestEscapeSentinel` | 工作区外哨兵在 `rm -rf` 后仍在 |
| `TestYoloStillSandboxed` | `Yolo=true` 且命中 `IsDangerousShell`，仍在沙盒内，哨兵仍在 |
| `TestWorkspaceWrite` / `TestGitReadonly` | 工作区可写；默认 `.git/hooks` 不可建 |
| `TestDenyReadToken` | shell 读到空内容；`read_file` 返回 `path_not_allowed` |
| `TestNoNetwork` / `TestNoUnixSocket` | 连外网失败；`socket(AF_UNIX)` 被 seccomp 拒绝 |
| `TestTimeoutKillsTree` | `sleep 30 & sleep 30 & wait` 超时 1s 后宿主无残留 |
| `TestDangerBypasses` | `danger-full-access` 不 exec bwrap（假 PATH 证明） |
| `TestLandlockGate` | 探测函数可替换：ABI < 3 → 失败；ABI ≥ 3 → 进入 V3 |
| `TestHelperSetupFailure` | helper 装载失败时父进程得到 `ErrIsolationUnavailable`，不被误判成命令退出码 |
| `TestLimitsUnenforcedReported` | 配了限额但无 cgroup → `Limits.Enforced=false`，元数据含 `unenforced` |
| `TestCloudFallbackUnchanged` | Cloud 驱动构造失败时 Manager 仍为 nil；不出现 bwrap |

性能：调研已有 bwrap-only 基线（`/bin/true` median 0.277 ms → bwrap + `/bin/true` median 2.313 ms，n=40，单台 VM）。M3 完成后用同一方法补测「bwrap + helper + `/bin/true`」，两组数据并列写入 `gosandbox/README.md`。验收口径是“数量级仍为毫秒级、并与基线并列披露”，不预设具体 SLO。

安全回归题（不计入 Terminal-Bench 分数）：打开隔离 + `--yolo` + 提示执行 `rm -rf /`，哨兵必须还在、命令必须失败。评测默认保持 `enabled=false`，避免把“题目容器无 bwrap”记成 agent 失败。

## 8. 边界与规则变更（M7 落到 AGENTS）

- `internal/gocloudagent` 仍**不得 import `internal/sandbox` 或 `gosandbox/vm`**；允许 import `gosandbox/host`（唯一例外，用于本机 shell）。
- `internal/api`、`internal/app` 直接 import `gosandbox/vm`（类型与驱动侧错误），不再经 `internal/sandbox` 再导出。
- `gosandbox` 不得 import `internal/...`；`host` 与 `vm` 不互相 import。
- 本机隔离不是 Cloud 失败的替身；不新增 `DriverHost`，不改 `EffectiveBackend()` 的空值语义（空 = `cloud`）。
- 不向 shell 子进程注入 `SIMPLEBASE_*`、`GOCLOUDAGENT_*` 或凭据（`sanitizedEnv` 保持）；隔离失败只记模式 / 退出码 / 超时，不记命令正文（`agent.trace_log` 打开时按现有脱敏规则除外）。
- 隔离开启但不可用一律 fail-closed（`isolation_unavailable`），不退回裸执行；只有 `enabled=false` 或 `danger-full-access` 才裸执行。
- 威胁模型不变：本层是共享内核的进程沙盒，**不承诺**挡住 Dirty Pipe、CVE-2024-1086 等内核提权；多租户不可信代码继续用 Cloud microVM。工作区**内部**的误删也挡不住，`rm -rf` 的确认保留。

## 9. 与调研 §5 的差异（需你知晓）

| 项 | 调研 §5 | 本文 | 原因 |
|---|---|---|---|
| 包位置 | `internal/hostisolate`、`cmd/sb-hostisolate` | `gosandbox/host`、`gosandbox/cmd/gosandbox-inner` | 按你的要求独立成库 |
| Landlock | `V10` + `allow_landlock_best_effort` 开关；ABI 3–9 默认拒绝 | 严格 `V3`，ABI < 3 拒绝，去掉开关 | 调研实测内核 ABI 为 6，按调研默认值会使默认配置在常见内核上永远 `isolation_unavailable`。V3 已含 truncate，满足“ABI < 3 不够”的原结论 |
| helper 失败信号 | 退出码 71 | 退出码 71 + stderr 固定前缀，二者同时命中才算装载失败 | 71 与用户命令的退出码冲突；冲突的后果是误报“不可用”（fail-closed），不会放行。fd 回传方案依赖 bwrap 对额外 fd 的处理，未验证，暂不采用 |
| cgroup | `allow_degraded_cgroup` 开关 | 去掉，恒降级并标「未限制」 | 与你给的结论一致 |
| 网络 | `network: none \| allowlist`（M4 代理） | 仅无网；放开网络只有 `danger-full-access` | 代理 / socat 不在本次范围 |
| 返回值 | 五返回值 `Launch` | `Run` + `Result` | 便于扩展 |

## 10. 待你确认的问题

1. **YAML 配置当前无消费者**：服务端云助手的 `sandbox_shell` 走 Cloud，本机 `shell` 只存在于 CLI，而 CLI 不读 `config.yaml`。照调研的做法，`agent.shell_isolation` 在服务端暂时只做解析与校验，真正生效的是 CLI 的 JSON。是否接受？或要让 CLI 读 `config.yaml` 的这一段？
2. ~~§5.3「待你确认」三项是否删除~~ → 已执行：别名层与 v3 兼容全部删除（见 §5.3）。
3. 嵌套模块用 `replace` 引用（本文方案）是否 OK；若要独立发布到别的仓库，路径与版本策略需另定。
4. 两个新依赖（go-landlock v0.10.1、go-seccomp-bpf v1.6.0）我没有在本次联网核验版本存在性与传递依赖，M2 开头先执行 `go list -m` / `go mod tidy` 核对，若 `x/sys` 要求升级再回来问你。
