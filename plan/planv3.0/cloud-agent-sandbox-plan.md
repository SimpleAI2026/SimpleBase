# 云 Agent 云沙盒：microsandbox Cloud

> **仓库**：SimpleBase（https://github.com/linkxzhou/SimpleBase）
> **状态**：plan-only。本文件只定方案，不改产品代码。
> **日期**：2026-09-27
> **SDK**：`github.com/superradcompany/microsandbox/sdk/go`（核对日 pkg.go.dev 最新 tag 为 **v0.7.3**；仓库 `main` 的 `sdk/go/setup.go` 中 `sdkVersion` 为 **0.7.4**）。实现时 `go get` 锁定一个已发布的 `v0.7.x`，并把该版本写进 `go.mod`。
> **文档**：https://docs.microsandbox.dev （Cloud、Local or cloud、Go SDK Sandbox / Execution / Filesystem / Networking）
> **关联**：`plan/planv2.0/cloud-agent-plan.md`、`internal/cloudagent`、`internal/AGENTS.md`、`ui/src/pages/AgentManager.vue`

用户已明确要求引入该 SDK。这是对 `internal/AGENTS.md`「不新增第三方依赖」的例外。

---

## 0. 目标

给云 Agent 加一块**跑在 microsandbox Cloud 上的隔离环境**。模型可以在对话里执行命令、读写沙盒文件；这些动作发生在托管 microVM 里，不发生在 SimpleBase 进程、用户 DuckLake 或项目 S3 上。

同一套 Go API 也能起本地 microVM。本方案**只走 Cloud**：

- 启动时用 SimpleBase 配置显式选择 cloud backend，并核对 `DefaultBackendInfo().Kind == cloud`。
- 核对失败则沙盒工具不可用。进程不得退回本地 KVM / Apple Virtualization。
- 不调用 `EnsureRuntime`。Cloud 不需要本机虚拟化；`msb doctor` 只约束本地 runtime。

现有 database / s3 / logs 只读工具保持不变。

---

## 1. 本版范围

| 做 | 放到以后 |
|---|---|
| 配置开关 + Cloud API key（生产只来自环境变量） | 本地 backend、`EnsureRuntime`、本机 `~/.microsandbox` runtime |
| 每个 agent thread 一个具名 Cloud 沙盒，跨轮次复用 | 每项目一个长期沙盒、快照 / fork、命名卷 |
| 四个工具：`sandbox_exec`、`sandbox_shell`、`sandbox_read_file`、`sandbox_write_file` | 交互 TTY、stdin 管道、SSH、端口发布、`Modify` |
| 默认无出网；可选 public egress | 自定义域名白名单 UI、hostname-scoped secrets |
| 新模块 `sandbox`，控制台可选用 | 沙盒状态页、用量账单、文件浏览器 |
| 工具实现与 SDK 之间用接口，单测不连 Cloud | 带真实 key 的集成测试进默认 `go test` |

---

## 2. 现状（要对齐的代码）

云 Agent 是 eino `ChatModelAgent` + 只读工具，跑在 `internal/cloudagent`。

- 工具目录与模块模板：`internal/cloudagent/modules.go`（`database` / `s3` / `logs` / `general`）。`KnownTool` 只认只读 id。
- 工具装配：`internal/cloudagent/tools.go` 的 `buildTools`，经 `utils.InferTool` 挂到 eino。
- 运行时：`internal/cloudagent/runtime.go` 的 `Runtime.StartRun`，`toolDeps` 只有 DB / Obj / Logs。
- 装配：`internal/app/app.go` 的 `cloudAgentRuntime()`。
- 对话入口：`POST /v1/projects/:projectID/agent-threads/:threadID/runs`，权限 `database:read`，并 `CheckQuota(..., "llm")`（`internal/api/router.go`、`internal/api/agent_handler.go`）。
- 控制台：`ui/src/pages/AgentManager.vue` 按模块默认工具画 badge，文案仍是「工具只读」。
- 约束：`internal/AGENTS.md` 写明 `cloudagent`「禁止提供写工具」。实现时改成：禁止写 DuckLake / S3 / 系统库；允许只在 Cloud 沙盒内执行。

沙盒命令不是对 SimpleBase 的写操作。它不经过 `sqlguard`，也不打开 registry。

---

## 3. SDK 事实（实现时按此调用）

模块路径：

```go
microsandbox "github.com/superradcompany/microsandbox/sdk/go"
```

### 3.1 选择 Cloud

Cloud 必须显式选择。只设置 `MSB_API_KEY` 时默认仍是 **local**；`MSB_BACKEND=cloud` 但没有可用 key 时返回配置错误，**不会**退回本地（upstream `#1259`）。

Go SDK 公开面目前只有诊断函数，没有 TypeScript / Rust 那种 `setDefaultBackend`：

```go
info, err := microsandbox.DefaultBackendInfo()
// info.Kind == microsandbox.BackendCloud ("cloud")
// info.APIURL、info.Source、info.Profile 不含 API key
```

因此 SimpleBase 在**第一次调用 SDK 之前**写入进程环境，然后立刻核对：

| 变量 | 值 |
|---|---|
| `MSB_BACKEND` | `cloud` |
| `MSB_API_KEY` | 配置里的 key |
| `MSB_API_URL` | 仅当配置覆盖了默认端点时设置。默认 `https://api.microsandbox.dev` |
| `MSB_HOME` | `<database.cache_dir>/microsandbox` |

`MSB_HOME` 只给 SDK 解压内嵌 FFI 库（`sdk/go/setup.go` 的 `installDir`）。Cloud 路径不下载 `msb` / `libkrunfw`。

核对 `Kind != cloud` 或 key 为空：沙盒功能保持关闭，打一条不含 key 的错误日志，进程继续启动。其余 Agent 模块不受影响。

Go SDK 若在所锁定 tag 里提供了进程内 backend setter，改为调用 setter，不再 `os.Setenv`。环境变量方案是当前公开 API 下的做法。

### 3.2 生命周期与执行

沙盒名最长 **128** 个 UTF-8 字节（Go SDK Sandbox 文档）。

```go
sb, err := microsandbox.ConnectOrCreateSandbox(ctx, name,
    microsandbox.WithImage(image),          // 默认 python:3.12
    microsandbox.WithCPUs(1),
    microsandbox.WithMemory(512),           // MiB
    microsandbox.WithWorkdir("/workspace"),
    microsandbox.WithLabels(labels),
    microsandbox.WithMaxDuration(maxDuration),
    microsandbox.WithIdleTimeout(idleTimeout),
    microsandbox.WithNetwork(network),
)
defer sb.Close() // 释放客户端句柄；不调用 Stop

out, err := sb.Exec(ctx, cmd, args, microsandbox.WithExecTimeout(execTimeout), microsandbox.WithExecCwd("/workspace"))
// 或 sb.Shell(ctx, command, microsandbox.WithExecTimeout(...))
// 非零退出码在 out.ExitCode()，不是 Go error
_ = out.Stdout()
_ = out.Stderr()
```

所锁定 tag 若没有 `ConnectOrCreateSandbox`：`GetSandbox` → 不存在则 `CreateSandbox`；已停止则 `ConnectOrStart`。已有沙盒的配置以 Cloud 上那份为准，选项只在创建时生效。

文件（Cloud 上常用路径操作可用；底层 open-handle 仅本地）：

```go
fs := sb.FS()
err = fs.WriteString(ctx, path, content)
text, err := fs.ReadString(ctx, path)
```

命令跑完后 **Close 句柄，不 Stop**。VM 留在 Cloud 上，靠 `WithIdleTimeout` / `WithMaxDuration` 回收。下次工具调用按同一个名字接回去，工作目录里的文件还在。

删除 thread 时：`Stop`，等到 stopped，再 `RemoveSandbox`。失败记日志，不挡住 thread 的软删。

### 3.3 Cloud 上不要用的 API

| API | 原因 |
|---|---|
| `EnsureRuntime` / 本地镜像缓存 | Cloud 由平台拉 OCI 镜像 |
| `WithPorts`、自定义 DNS、TLS 拦截、`Modify` | Cloud create 不接受，或仅本地 |
| `Metrics` | Cloud 上资源指标是 local-only |
| `WithSecrets` | v1 不把任何 SimpleBase 凭据送进沙盒 |
| `WithDetached` 作为保活手段 | Cloud VM 不跟 Go 进程生死绑定；保活靠不调用 `Stop` |

平台是 private beta，key 从 microsandbox dashboard 申请。没有 key 时功能关闭，控制台给出「未配置云沙盒」而不是尝试创建。

### 3.4 构建约束

- Go SDK 通过 **CGO + 内嵌 FFI** 工作：首次调用时把动态库解压到 `MSB_HOME/lib/v<sdkVersion>/` 再 `dlopen`。要求 `CGO_ENABLED=1` 和本机 C 工具链。SimpleBase 已因 DuckDB 使用 CGO。
- 预编译 FFI 覆盖：macOS arm64，Linux amd64 / arm64。在与目标相同的 OS/arch 上构建。从 macOS 交叉编译 Linux 二进制是否带对 Linux `.so`，实现时先用一次 `go build` 验证；失败则 CI 在 Linux 上编 server。
- 二进制会变大（内嵌 FFI）。可接受。
- `go test ./...` 不得访问 Cloud，也不得在默认测试里 `dlopen`。见 §8。

---

## 4. 代码放哪

```text
internal/config          sandbox 配置段（无密钥逻辑以外的字段）
internal/sandbox         唯一 import microsandbox 的包：backend 核对、命名、路径约束、Client
internal/cloudagent      新工具 id、模块模板、buildTools 调用 Sandbox 接口
internal/app             assembleDeps / cloudAgentRuntime 注入 Client；关闭时不扫全量删除
internal/api             无新路由。modules 响应多一个 sandbox_available
ui                       Agent 模块与工具 badge、composer 文案
```

依赖方向：`app → sandbox` 与 `app → cloudagent`；`cloudagent` 只依赖自己声明的 `Sandbox` 接口，不 import `microsandbox`。`api` 不直接调用 SDK。

`cloudagent.Runtime` 增加字段：

```go
Sandbox Sandbox // nil 表示未启用
```

```go
type Sandbox interface {
    Available() bool
    Exec(ctx context.Context, projectID, threadID string, cmd string, args []string) (Output, error)
    Shell(ctx context.Context, projectID, threadID string, command string) (Output, error)
    ReadFile(ctx context.Context, projectID, threadID, path string) (string, error)
    WriteFile(ctx context.Context, projectID, threadID, path, content string) error
    ReleaseThread(ctx context.Context, projectID, threadID string) error
}
```

`Output` 含 `Stdout`、`Stderr`、`ExitCode`，已截断、已脱敏。

进程内按沙盒名一把 `sync.Mutex`，同一 thread 的工具调用串行。SimpleBase 当前是单实例（见 `multi-instance-consistency-plan.md`）。两个副本打到同一个 Cloud 沙盒会竞态；在多实例方案落地前不为此加分布式锁。

---

## 5. 配置

`config.yaml` / `SIMPLEBASE_` 环境变量，沿用 `internal/config` 的加载顺序。生产（`dev_mode=false`）拒绝 YAML 里的非空 `sandbox.api_key`，与 LLM provider key 同一条 `rejectYAMLSecrets` 路径。

```yaml
sandbox:
  enabled: false
  api_url: ""                 # 空 = https://api.microsandbox.dev
  api_key: ""                 # 生产必须来自 SIMPLEBASE_SANDBOX_API_KEY
  image: "python:3.12"
  cpus: 1                     # 1–4
  memory_mib: 512             # 128–4096
  max_duration: 30m           # Cloud WithMaxDuration
  idle_timeout: 10m           # Cloud WithIdleTimeout
  exec_timeout: 30s           # 单次命令 WithExecTimeout，至少 1s
  max_output_bytes: 32768
  max_file_bytes: 262144
  network: none               # none | public
  workdir: /workspace
```

环境变量：

| 变量 | 映射 |
|---|---|
| `SIMPLEBASE_SANDBOX_ENABLED` | `enabled` |
| `SIMPLEBASE_SANDBOX_API_KEY` | `api_key` |
| `SIMPLEBASE_SANDBOX_API_URL` | `api_url` |
| `SIMPLEBASE_SANDBOX_IMAGE` | `image` |
| `SIMPLEBASE_SANDBOX_NETWORK` | `network` |
| `SIMPLEBASE_SANDBOX_EXEC_TIMEOUT` | `exec_timeout` |
| `SIMPLEBASE_SANDBOX_IDLE_TIMEOUT` | `idle_timeout` |
| `SIMPLEBASE_SANDBOX_MAX_DURATION` | `max_duration` |

`enabled=true` 且 key 为空：`Config.Validate` 失败，进程不启动。`enabled=false` 时忽略 key，沙盒工具不注册进可用集合。

`network=none` → `NetworkPolicy.None()`（无出网）。`network=public` → `NetworkPolicy.FromProfiles(NetworkProfilePublic)`。不提供 `AllowAll`，不提供 `NetworkProfilePrivate` / `Host`（那是客户机网段与宿主机网关，Cloud 上没有 SimpleBase 宿主机可连）。

---

## 6. 命名、标签、工作目录

一个 thread 对应一个 Cloud 沙盒。thread id 已是 UUID，名字：

```text
sb-{threadID 去掉连字符}
```

32 位十六进制加前缀，远小于 128 字节。thread 全局唯一，不把 project id 再编进名字。

标签（便于在 Cloud 控制台过滤；值里不放密钥）：

| key | value |
|---|---|
| `simplebase` | `1` |
| `project_id` | 项目 id |
| `thread_id` | thread id |

工作目录固定 `/workspace`。`read_file` / `write_file` 的路径必须落在该目录下：

- 拒绝空路径、相对路径、含 `..` 的路径、非 `/workspace` 前缀。
- 写入用 `filepath.Clean` 后再做前缀判断（`/workspace` 本身与 `/workspace/...`）。
- 单文件不超过 `max_file_bytes`。读出同样截断并在 JSON 里带 `truncated: true`。

命令的 cwd 固定 `/workspace`。工具参数里的 `cwd` 不向模型开放。

---

## 7. 模块与工具

新模块：

| 字段 | 值 |
|---|---|
| id | `sandbox` |
| 名称 | Sandbox |
| 默认工具 | `sandbox_exec`、`sandbox_shell`、`sandbox_read_file`、`sandbox_write_file` |

`GET /agents/modules` 的该条增加 `sandbox_available`（bool）。`enabled` 且 backend 核对为 cloud 时为 true。为 false 时：

- 仍返回模块，便于 UI 显示原因。
- `POST/PATCH /agents` 若 `module=sandbox` 或 `tool_ids` 含任一 sandbox 工具，返回 400，正文说明云沙盒未启用。
- 已存在的 sandbox agent 在功能被关掉之后，工具调用返回明确错误字符串，不创建 VM。

### 7.1 工具入参与返回

返回一律 `marshalToolJSON` + 现有 `RedactSecrets`。stdout/stderr 再截到 `max_output_bytes`。

| 工具 | 入参 | 行为 |
|---|---|---|
| `sandbox_exec` | `cmd`（必填）、`args`（字符串数组，可空） | `Sandbox.Exec`。`cmd` 按字面传给客户机，不经 shell |
| `sandbox_shell` | `command`（必填，最长 4096 字节） | `Sandbox.Shell`，即客户机 `/bin/sh -c` |
| `sandbox_read_file` | `path` | `FS().ReadString`，路径约束见 §6 |
| `sandbox_write_file` | `path`、`content` | `FS().WriteString` |

返回体：

```json
{
  "stdout": "",
  "stderr": "",
  "exit_code": 0,
  "truncated": false
}
```

读文件把内容放在 `stdout`，`exit_code` 恒为 0。传输失败（超时、沙盒不存在、SDK error）作为工具 error 返回给模型，让它解释错误，不把传输失败伪装成 `exit_code != 0`。

`WithExecTimeout` 触发时 SDK 返回 `ErrExecTimeout`。工具错误文案使用固定句子「命令超过执行时限」，不附带内部路径。

### 7.2 谁可以调用

`CreateRun` 保持 `database:read`，这样只读 Agent 的对话权限不变。

沙盒工具在执行前检查 `RunContext.Principal`：

- 拥有 `database:write` 或 `database:admin` 才执行。
- 只有 `database:read` 的 API key：工具返回错误「沙盒执行需要 database:write」。同一次 run 里的只读工具仍可调用。

控制台登录用户若已能编辑 Agent，即已具备 write，对话里可以跑沙盒工具。

### 7.3 Prompt

`modules.go` 增加 `sandbox` 模板，叠在现有 platform base 之后：

- 命令与文件只存在于当前 thread 的云沙盒，工作目录 `/workspace`。
- 空闲超过 `idle_timeout` 或总寿命达到 `max_duration` 后环境会被回收，文件不保证还在。
- 默认无出网。实例若打开 public egress，模板里写明「可以访问公网，不要外传项目数据」。
- 查看本项目库表、对象、日志仍用只读工具；沙盒里没有 SimpleBase 凭据，也连不上系统库。
- 不要索要或打印 S3 key、provider key、DSN、token。

platform base 里「Tools are readonly」对 sandbox 模块改成：只读工具保持只读；沙盒工具只允许动 Cloud VM 内的 `/workspace`。

---

## 8. 安全

- API key 只出现在进程环境 `MSB_API_KEY` 与配置结构体中。日志、审计、SSE、工具 JSON、`DefaultBackendInfo` 都不打印 key。审计只记 `tool=sandbox_exec`、`project_id`、`thread_id`、退出码、耗时、是否截断。不记命令原文与 stdout（与 SQL 审计只记哈希的尺度一致：命令可能含用户贴进对话的数据）。
- 不把 `S3`、`LLM`、`auth.api_key_hash_secret` 写进 `WithEnv` 或 `WithSecrets`。
- 沙盒网络默认拒绝出网，避免模型把项目数据送到任意主机。
- 路径限制在 `/workspace`。这是应用层约束，VM 内的 shell 仍能读写镜像里的其他路径。接受这一点：隔离边界是 microVM，不是 chroot。工具描述里告诉模型使用 `/workspace`；`sandbox_shell` 无法从应用层禁止 `cat /etc/passwd`。
- 单次命令超时、输出上限、文件上限、VM 的 idle / max duration 四层限制同时生效。
- 删除 thread 时释放 Cloud 沙盒，避免闲置 VM 一直计费到 max duration。
- admin 项目与普通项目使用同一 Cloud 组织（同一把 key）时，名字按 thread id 隔离。沙盒内仍看不到系统库 DSN。

---

## 9. 前端

`AgentManager.vue`：

- 模块下拉出现 Sandbox。`sandbox_available=false` 时该项禁用，旁注「未配置云沙盒」。
- 工具 badge 使用模块返回的 `default_tools`，Sandbox 模块显示四个沙盒工具。
- composer 旁的「工具只读」在当前 Agent 含沙盒工具时改为「沙盒命令在云端隔离环境执行」。
- 空态文案补一句：Sandbox Agent 可在云端环境里运行代码。

`services/types.ts` 的 `AgentModuleInfo` 增加 `sandbox_available?: boolean`。`http-api.ts` 与 `mock.js` 一起补上。对话仍走现有 `agent-threads/.../runs` SSE，工具卡片沿用 `tool_call` / `tool_result`，不新增页面。

---

## 10. 测试

| 层 | 覆盖 |
|---|---|
| `internal/config` | YAML 生产拒绝 `sandbox.api_key`；env 覆盖；`enabled` 无 key 时 Validate 失败；`network` 只接受 `none`/`public` |
| `internal/sandbox` | 名字生成、`/workspace` 路径接受与拒绝、输出截断。用假的执行后端，不 import 真实调用 |
| `internal/cloudagent` | `buildTools` 在 Sandbox 为 nil 时返回明确错误；fake Sandbox 断言 exec 参数与脱敏；只读 principal 被拒绝；write principal 放行 |
| `internal/api` | 沙盒未启用时创建 `module=sandbox` 的 agent 返回 400；modules JSON 带 `sandbox_available` |
| UI | `AgentManager` 在 `sandbox_available=false` 时不能选中 Sandbox 模块 |

默认 `go test ./...` 不设置 `MSB_API_KEY`，不访问 `api.microsandbox.dev`。

可选、不进默认 CI：`//go:build sandbox_integration` 的测试，要求环境里已有 key，创建沙盒、`echo ok`、读回文件、`RemoveSandbox`。

---

## 11. 实现顺序

1. **配置与边界文档**：`SandboxConfig`、env、生产密钥拒绝；改 `internal/AGENTS.md` 的 cloudagent 行；`docs/getting-started/index.md` 的「只读工具」一句改为只读工具 + 可选云沙盒。
2. **`internal/sandbox`**：backend 核对、`Client` 实现 §3.2、路径与截断。`app.assembleDeps` 在 LLM runtime 之前完成核对并注入。失败只关闭沙盒。
3. **工具与模块**：`modules.go` / `tools.go` / `runtime.go` / prompt 测试。
4. **API**：modules 字段与创建校验；thread 删除调用 `ReleaseThread`。
5. **控制台**：模块、badge、文案、mock 与 `AgentManager` 测试。

每步保持 `go test ./...` 与 `ui` 下 `npm run build` 通过。

---

## 12. 验收

1. `sandbox.enabled=false` 时服务照常启动，现有四个模块与只读工具行为不变。
2. `enabled=true` 且 key 有效时，`DefaultBackendInfo` 为 cloud。Sandbox 模块可选，对话里 `@` 该 Agent 后能执行 `python -c` 并在下一轮读到上一轮写入的 `/workspace` 文件。
3. `network=none` 时沙盒内访问公网失败；`network=public` 时 HTTPS 出网可用。
4. 只持有 `database:read` 的调用方跑沙盒工具得到权限错误，只读 SQL 工具仍可用。
5. 命令超时、超长输出被截断、`../` 路径被拒绝，且这些结果里没有 API key。
6. 删除 thread 后 Cloud 上该名字的沙盒被停止并删除（或删除请求已发出且错误被记录）。
7. 配置成 cloud 但 key 无效时，进程在启动核对阶段关闭沙盒功能并留下不含 key 的日志；不在本机拉起 microVM。

---

## 13. 参考调用（实现时照此收口，不直接贴进业务包）

```go
func (c *Client) open(ctx context.Context, projectID, threadID string) (*microsandbox.Sandbox, error) {
    name := sandboxName(threadID) // sb- + 32 hex
    net := microsandbox.NetworkPolicy.None()
    if c.cfg.Network == "public" {
        net = microsandbox.NetworkPolicy.FromProfiles(microsandbox.NetworkProfilePublic)
    }
    return microsandbox.ConnectOrCreateSandbox(ctx, name,
        microsandbox.WithImage(c.cfg.Image),
        microsandbox.WithCPUs(c.cfg.CPUs),
        microsandbox.WithMemory(c.cfg.MemoryMiB),
        microsandbox.WithWorkdir(c.cfg.Workdir),
        microsandbox.WithMaxDuration(c.cfg.MaxDuration),
        microsandbox.WithIdleTimeout(c.cfg.IdleTimeout),
        microsandbox.WithNetwork(net),
        microsandbox.WithLabels(map[string]string{
            "simplebase": "1",
            "project_id": projectID,
            "thread_id":  threadID,
        }),
    )
}
```
