# 云沙盒 v4：独立资源 + 侧栏入口 + 端到端 API

> **仓库**：SimpleBase
> **状态**：实施中（2026-10-05）。S1–S8 已实现并通过 fake e2e、SDK、UI 覆盖率门槛；唯一未关闭项是真实 Cloud 冒烟（待凭据，未运行）。详见 §16。
> **日期**：2026-10-04
> **前置**：`plan/planv3.0/cloud-agent-sandbox-plan.md`（下称 **v3**，已落地：`internal/sandbox/client.go`、`cloudagent` 四个沙盒工具、`config.SandboxConfig`、`AgentManager` Sandbox 模块）
> **SDK**：沿用 `github.com/superradcompany/microsandbox/sdk/go v0.7.4`（`go.mod` 已有，**不新增依赖**；实现时把 `// indirect` 去掉即可）

---

## 0. 一句话

把「云沙盒」从**云 Agent 的附属物**升级为**项目级一等资源**：侧栏「自动化」里有独立页面，HTTP API 覆盖从创建、执行、读写文件到销毁的完整生命周期（API Key 可直接调用，SDK / 云函数 / 定时任务 / Agent 都走同一套能力），运行形态保持轻量：按需起、空闲回收、无常驻进程池。

---

## 1. 目标与非目标

### 1.1 目标

| # | 需求 | 本版落点 |
|---|---|---|
| G1 | 侧边栏「自动化」增加云沙盒 | `NavMenu.vue` 的 `automation` 组追加 `sandboxes`；新页 `/console/sandboxes` |
| G2 | 沙盒尽量轻量 | 懒创建、小规格默认值、短空闲回收、单表元数据、不存执行记录正文、无连接池、无新依赖（§3） |
| G3 | 支持端到端（e2e）接口 | `/v1/projects/:projectID/sandboxes/**`：资源 CRUD + exec + files + 一次性 `run`；幂等创建；稳定错误码；`fake` 驱动让 e2e 不依赖 Cloud（§6、§10） |
| G4 | 其他配套功能 | 镜像预设白名单、项目并发上限、自动过期清理、审计/用量、JS/Go SDK、文档、Agent 复用同一 Manager（§8、§9、§11） |

### 1.2 非目标（以后再说）

- 交互式 TTY / WebSocket 终端、端口暴露、SSH。
- 快照 / fork / 命名卷 / 跨沙盒共享目录。
- 本地 microVM backend（仍然**只走 Cloud**，v3 §0 的约束不变）。
- 多实例分布式锁（随 `multi-instance-consistency-plan.md` 落地）。
- 浏览器端 e2e 框架（Playwright 等）：不新增前端依赖，UI 只做 vitest 组件测试 + mock。

---

## 2. 现状（v3 已有，v4 要改造的点）

| 位置 | 现状 | v4 处理 |
|---|---|---|
| `internal/sandbox/client.go` | `Client` 直接调 SDK，按 `threadID` 命名 `sb-{hex}`，无元数据 | 拆成 `Driver`（SDK 适配）+ `Manager`（业务）；`Client` 删除或保留为 Manager 的薄壳 |
| `internal/cloudagent/access.go` | `Sandbox` 接口按 `(projectID, threadID)` 寻址 | 接口**不变**；app 层 adapter 改为调 `Manager.EnsureForThread` |
| `internal/app/app.go` `sandboxClient()` | 只在 cloudAgentRuntime 内创建 | 提升为 `assembleDeps` 内的独立模块 `a.sandboxMgr`，同时注入 api 和 cloudagent |
| `internal/api/agent_handler.go` | thread 删除时 `ReleaseThread` | 保留；改为 Manager 删除对应资源行 |
| `internal/config` `SandboxConfig` | enabled / key / image / 规格 / 超时 / network | 新增 `backend`、`images`、`max_per_project`、`exec_timeout_max`、`reap_interval`（§5） |
| `ui` | 仅 `AgentManager.vue` 有 Sandbox 模块 | 新增 `Sandboxes.vue` 页 + 侧栏入口 |
| `internal/AGENTS.md` | `cloudagent` 职责「禁止提供写工具」；无 `sandbox` 行 | 增加 `sandbox` 行；cloudagent 行改成「禁止写 DuckLake/S3/系统库；沙盒执行仅经 sandbox.Manager」 |

沙盒不是对 SimpleBase 数据面的写操作：不经 `sqlguard`、不开 registry、不碰 DuckLake / S3。

---

## 3. 轻量化原则（设计约束，评审时逐条核对）

1. **懒创建**：`POST /sandboxes` 默认只写一行元数据（`status=pending`），**第一次 exec / 写文件时**才在 Cloud 上 `ConnectOrCreate`。传 `"start": true` 可预热。
2. **小默认规格**：`1 vCPU / 256 MiB`，`idle_timeout=5m`，`max_duration=30m`。上限仍受配置约束（cpu ≤ 4，mem ≤ 4096）。
3. **不保活**：每次调用 open → 操作 → `Close` 句柄，不调用 `Stop`；VM 由 Cloud 侧 idle / max duration 回收（同 v3 §3.2）。
4. **单表元数据**：只新增 `sys_sandboxes` 一张表。**不建执行记录表**：命令原文、stdout 不落库；执行次数和耗时只进审计/用量。
5. **无进程池、无常驻 goroutine 池**：唯一的后台协程是一个低频 reaper（默认 5 分钟），只做过期清理（§7.3）。
6. **状态懒对账**：不轮询 Cloud。`GET /sandboxes/:id` 带 `?refresh=1` 时才调 `GetSandbox` 刷状态；其余时候读本地行。
7. **单包接入 SDK**：只有 `internal/sandbox/driver_cloud.go` import microsandbox；其余全部走接口，默认 `go test` 不 `dlopen`。
8. **前端零新依赖**：输出区用 `<pre>`，文件编辑用已有 `Textarea`（不引入 xterm / Monaco 新实例；GoMonacoEditor 只用于 Go）。

---

## 4. 架构

```text
ui (Sandboxes.vue)         packages/js-sdk · go-sdk         外部 e2e 脚本 / curl
        │                              │                              │
        └──────────── HTTP /v1/projects/:projectID/sandboxes/** ──────┘
                                       │
internal/api   SandboxHandler ── SandboxService 接口（api 内声明）
                                       │  app 层 adapter
internal/sandbox   Manager ──┬── Store 接口（systemdb 实现，app 缝合）
                             └── Driver 接口
                                   ├── cloudDriver（唯一 import microsandbox）
                                   └── fakeDriver（仅 dev_mode，内存实现）
internal/cloudagent  Sandbox 接口（不变）── app adapter → Manager.EnsureForThread
```

依赖方向：`app → api / sandbox / cloudagent / systemdb`；`sandbox` 只依赖 `config`、`observability`；`api`、`cloudagent` 不 import `sandbox`（经各自声明的接口 + app adapter），保持 `internal/AGENTS.md` 的分层约定。

### 4.1 包内文件

```text
internal/sandbox/
  driver.go         Driver 接口、DriverStatus、ExecSpec/ExecResult
  driver_cloud.go   microsandbox Cloud 实现（v3 client.go 的 SDK 调用迁过来）
  driver_fake.go    内存实现：shell 只支持 echo/cat/ls/pwd/exit N/sleep，文件存 map
  manager.go        Manager：校验、限额、锁、懒创建、截断、状态机
  paths.go          /workspace 路径校验（v3 validPath 迁移）
  reaper.go         过期清理
  errors.go         领域错误（供 api/error.go 映射）
  *_test.go
```

### 4.2 Driver 接口

```go
type Driver interface {
    Kind() string // "cloud" | "fake"
    Ensure(ctx context.Context, name string, spec Spec) error           // ConnectOrCreate，返回前 Close 句柄
    Exec(ctx context.Context, name string, req ExecSpec) (ExecResult, error)
    ReadFile(ctx context.Context, name, path string) ([]byte, error)
    WriteFile(ctx context.Context, name, path string, data []byte) error
    ListDir(ctx context.Context, name, path string) ([]FileEntry, error)
    Status(ctx context.Context, name string) (DriverStatus, error)        // running/stopped/absent
    Stop(ctx context.Context, name string) error
    Remove(ctx context.Context, name string) error                         // Stop + Remove，不存在视为成功
}

type Spec struct {
    Image string; CPUs, MemoryMiB int; Network string
    IdleTimeout, MaxDuration time.Duration
    Labels map[string]string
}

type ExecSpec struct {
    Cmd   string   // 与 Shell 二选一
    Args  []string
    Shell string   // /bin/sh -c
    Env   map[string]string
    Cwd   string   // 已校验，位于 /workspace 下
    Timeout time.Duration
}

type ExecResult struct {
    Stdout, Stderr []byte
    ExitCode int
    TimedOut bool
    Duration time.Duration
}
```

`ListDir`：所锁定 SDK tag 的 `FS()` 若有可用于 Cloud 的列目录 API 则用之；否则在 cloudDriver 内部用 `find <dir> -maxdepth 1 -mindepth 1 -printf '%y\t%s\t%T@\t%f\n'` 实现，对外语义一致。

### 4.3 Manager 对外方法

```go
type Manager struct { /* cfg, store, driver, locks, now */ }

func (m *Manager) Available() bool
func (m *Manager) Capabilities() Capabilities
func (m *Manager) Create(ctx, projectID string, in CreateInput, actor string) (Sandbox, error)
func (m *Manager) List(ctx, projectID, status, source, cursor string, limit int) ([]Sandbox, next string, error)
func (m *Manager) Get(ctx, projectID, id string, refresh bool) (Sandbox, error)
func (m *Manager) Update(ctx, projectID, id string, in UpdateInput) (Sandbox, error)
func (m *Manager) Start(ctx, projectID, id string) (Sandbox, error)
func (m *Manager) Stop(ctx, projectID, id string) (Sandbox, error)
func (m *Manager) Delete(ctx, projectID, id string) error
func (m *Manager) Exec(ctx, projectID, id string, in ExecInput) (ExecOutput, error)
func (m *Manager) ReadFile(ctx, projectID, id, path string) (FileContent, error)
func (m *Manager) WriteFile(ctx, projectID, id, path string, data []byte) error
func (m *Manager) DeleteFile(ctx, projectID, id, path string) error
func (m *Manager) ListDir(ctx, projectID, id, path string) ([]FileEntry, error)
func (m *Manager) RunOnce(ctx, projectID string, in RunInput, actor string) (ExecOutput, error)
// 供云 Agent：按 thread 找或建 source=agent 的资源行
func (m *Manager) EnsureForThread(ctx, projectID, threadID string) (Sandbox, error)
func (m *Manager) ReleaseThread(ctx, projectID, threadID string) error
```

每个 Cloud 名一把进程内 `sync.Mutex`（沿用 v3），同一沙盒的操作串行；不同沙盒并行。

---

## 5. 配置

在 v3 `SandboxConfig` 上**追加字段**，旧字段语义不变；`rejectYAMLSecrets` 对 `api_key` 的生产拒绝保持。

```yaml
sandbox:
  enabled: false
  backend: cloud              # 新增：cloud | fake。fake 仅 dev_mode=true 时允许（e2e / 本地开发）
  api_url: ""
  api_key: ""                 # 生产只来自 SIMPLEBASE_SANDBOX_API_KEY
  image: "python:3.12-slim"   # 默认镜像（改为 slim，更轻）
  images:                     # 新增：可选镜像白名单（UI 下拉 + API 校验），空 = 仅 image
    - "python:3.12-slim"
    - "node:22-alpine"
    - "alpine:3.20"
  cpus: 1
  memory_mib: 256             # 默认从 512 降到 256
  max_duration: 30m
  idle_timeout: 5m            # 默认从 10m 降到 5m
  exec_timeout: 30s           # 单次命令默认时限
  exec_timeout_max: 300s      # 新增：API 允许调用方自定的上限
  max_output_bytes: 65536
  max_file_bytes: 1048576     # 1 MiB（HTTP 上传文件需要比 Agent 更宽）
  max_per_project: 5          # 新增：项目内非 deleted 的沙盒上限（含 agent 来源）
  reap_interval: 5m           # 新增：过期清理周期；0 = 关闭 reaper
  network: none               # none | public
  workdir: /workspace
```

新增环境变量：`SIMPLEBASE_SANDBOX_BACKEND`、`SIMPLEBASE_SANDBOX_IMAGES`（逗号分隔）、`SIMPLEBASE_SANDBOX_MAX_PER_PROJECT`。

`Validate` 追加：

- `backend` 只接受 `cloud` / `fake`；`fake` 且 `dev_mode=false` → 失败。
- `backend=cloud` 且 `enabled` → `api_key` 必填（v3 规则）；`backend=fake` 不要求 key。
- `images` 非空时必须包含 `image`。
- `exec_timeout ≤ exec_timeout_max`；`max_per_project` 1–100。

`backend=cloud` 核对失败（`DefaultBackendInfo().Kind != cloud`）：Manager `Available()=false`，**不**退回 fake，**不**退回本地 microVM。

---

## 6. 数据模型

迁移 `version: 38`（当前最大 37），新增一张表：

```sql
CREATE TABLE IF NOT EXISTS sys_sandboxes (
    id            VARCHAR PRIMARY KEY,        -- uuid
    project_id    VARCHAR NOT NULL,
    name          VARCHAR NOT NULL,           -- 用户可读名，项目内唯一（未删除范围）
    cloud_name    VARCHAR NOT NULL,           -- Cloud 侧名：sbx-{id 去连字符}
    source        VARCHAR NOT NULL,           -- console | api | agent | run
    thread_id     VARCHAR,                    -- source=agent 时非空
    image         VARCHAR NOT NULL,
    cpus          INTEGER NOT NULL,
    memory_mib    INTEGER NOT NULL,
    network       VARCHAR NOT NULL,           -- none | public
    idle_timeout_s INTEGER NOT NULL,
    max_duration_s INTEGER NOT NULL,
    status        VARCHAR NOT NULL,           -- pending | running | stopped | expired | error | deleted
    last_error    VARCHAR,                    -- 已脱敏，<=512 字节
    created_by    VARCHAR,                    -- user id 或 api key id
    created_at    TIMESTAMP NOT NULL,
    started_at    TIMESTAMP,
    last_active_at TIMESTAMP,
    expires_at    TIMESTAMP,                  -- started_at + max_duration
    deleted_at    TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sys_sandboxes_project ON sys_sandboxes(project_id, status);
CREATE INDEX IF NOT EXISTS idx_sys_sandboxes_thread  ON sys_sandboxes(thread_id);
```

- 命名：`cloud_name = "sbx-" + hex(id)`，与 v3 的 `sb-{threadHex}` 前缀区分。迁移期 agent 来源沿用旧名（见 §9.2），避免已存在的 Cloud VM 被孤立。
- `name` 规则：`^[a-z0-9][a-z0-9-]{0,62}$`；未传时生成 `sbx-` + 6 位随机。
- `last_active_at` 写回节流：同一沙盒 30 秒内最多写一次，避免高频 exec 打系统库（经 `systemdb` 现有 async flush 路径更好，实现时二选一）。
- 不建执行记录表（§3.4）。

### 6.1 状态机

```text
           create                 首次 exec/write/start
  (none) ─────────▶ pending ────────────────────────▶ running
                       │                                │ ▲
                       │ delete                  stop   │ │ exec/start（Cloud 侧 ConnectOrStart）
                       ▼                                ▼ │
                    deleted ◀──── delete ──────────── stopped
                       ▲                                │
                       │            now > expires_at    ▼
                       └────────── delete / reaper ── expired
  任何状态 driver 报不可恢复错误 → error（可 delete；可 start 重试）
```

- Cloud 侧 idle 回收后，本地行仍为 `running`；下次操作时 `ConnectOrCreate` 透明恢复，`refresh=1` 时对账为 `stopped`。
- `expired`：`max_duration` 已到，文件不保证存在；再次 `start` 视为重建（新 `started_at`），UI 明确提示「工作区已重置」。

---

## 7. HTTP API（e2e 接口）

前缀：`/v1/projects/:projectID`，全部经 `projectContextMiddlewareEcho`；`deps.Sandbox == nil` 时整组不挂载（与 KV / LLM 一致）。实例 `writable=false` 时写类接口返回 503（`ErrWriterUnavailable`），与云函数一致。

### 7.1 路由表

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/sandboxes/capabilities` | `database:read` | `{available, backend, images[], default_image, limits{cpus_max, memory_mib_max, exec_timeout_max_s, max_file_bytes, max_output_bytes, max_per_project}, network_options[]}`。未启用时仍挂此路由返回 `available=false`，UI 用于空态 |
| GET | `/sandboxes` | `database:read` | 列表；`?status=`、`?source=`、`?limit=50&cursor=` |
| POST | `/sandboxes` | `database:write` | 创建；支持 `Idempotency-Key` 头 |
| GET | `/sandboxes/:sandboxID` | `database:read` | 详情；`?refresh=1` 与 Cloud 对账 |
| PATCH | `/sandboxes/:sandboxID` | `database:write` | 仅改 `name`、`idle_timeout`（规格/镜像创建后不可改） |
| DELETE | `/sandboxes/:sandboxID` | `database:write` | Cloud Stop+Remove，本地行软删；Cloud 失败记 `last_error` 但仍 204 |
| POST | `/sandboxes/:sandboxID/start` | `database:write` | 预热 / 从 stopped、expired、error 恢复 |
| POST | `/sandboxes/:sandboxID/stop` | `database:write` | Cloud Stop，保留行，文件随 VM 保留到 Cloud 回收 |
| POST | `/sandboxes/:sandboxID/exec` | `database:write` | 执行命令（同步返回） |
| GET | `/sandboxes/:sandboxID/files?path=/workspace` | `database:read` | 列目录（单层，最多 500 项） |
| GET | `/sandboxes/:sandboxID/files/content?path=` | `database:read` | 读文件；`Accept: application/octet-stream` 返回原始字节，否则 JSON（文本 / base64） |
| PUT | `/sandboxes/:sandboxID/files/content?path=` | `database:write` | 写文件；body 为原始字节或 JSON `{content, encoding}`；`BodyLimit(max_file_bytes)` |
| DELETE | `/sandboxes/:sandboxID/files/content?path=` | `database:write` | 删文件（driver 内 `rm -f --`，路径已校验） |
| POST | `/sandboxes/run` | `database:write` | **一次性执行**：建临时沙盒 → 写入 files → exec → 返回 → 删除。最适合 e2e/CI |

> 路径参数用 query 而不是 `*` 通配，避免 Echo 通配与 URL 编码在 `/workspace/a b.txt` 上的歧义。

### 7.2 请求 / 响应

**创建**

```http
POST /v1/projects/p1/sandboxes
Idempotency-Key: 7c1e...           # 可选；24h 内同 key 同项目返回同一资源（200 而非 201）
{
  "name": "etl-check",              // 可选
  "image": "python:3.12-slim",      // 可选，必须在 images 白名单
  "cpus": 1, "memory_mib": 256,     // 可选，受上限约束
  "network": "none",                // 可选，none|public；public 需配置允许
  "idle_timeout_s": 300,            // 可选，≤ 配置 max_duration
  "start": false                    // 可选，true 时同步 Ensure
}
→ 201
{
  "id": "…", "name": "etl-check", "status": "pending", "image": "python:3.12-slim",
  "cpus": 1, "memory_mib": 256, "network": "none", "source": "api",
  "created_at": "…", "started_at": null, "last_active_at": null, "expires_at": null
}
```

幂等实现：`Idempotency-Key` **只存进程内存** LRU（项目+key → id，24h TTL，容量 1024），重启即失效，**不落库、不新增表**（§14.1）。

**执行**

```http
POST /v1/projects/p1/sandboxes/{id}/exec
{
  "command": "python -c 'print(1+1)'",   // 与 cmd 二选一：走 /bin/sh -c
  "cmd": "python", "args": ["-c","print(1)"],
  "cwd": "/workspace",                     // 可选，必须在 workdir 下
  "env": {"FOO":"bar"},                    // 可选，≤32 项，key ^[A-Z_][A-Z0-9_]*$，值 ≤4KiB
  "timeout_s": 30                          // 可选，≤ exec_timeout_max
}
→ 200
{
  "exit_code": 0, "stdout": "2\n", "stderr": "",
  "stdout_truncated": false, "stderr_truncated": false,
  "timed_out": false, "duration_ms": 412, "status": "running"
}
```

- **非零退出码是 200**，由 `exit_code` 表达；**超时也是 200**，`timed_out=true`、`exit_code=-1`。这样 e2e 脚本只需判断 HTTP 2xx 即可区分「平台故障」与「被测程序失败」。
- stdout/stderr 按 UTF-8 截断；非 UTF-8 字节以 `\uFFFD` 替换（需要原始字节的用文件接口）。

**一次性执行（e2e 首选）**

```http
POST /v1/projects/p1/sandboxes/run
{
  "image": "python:3.12-slim",
  "files": [ {"path": "/workspace/test.py", "content": "assert 1+1==2\nprint('ok')"} ],
  "command": "python test.py",
  "timeout_s": 60,
  "keep": false                          // true 时不删除，返回体带 sandbox_id 便于排查
}
→ 200 { ...exec 响应..., "sandbox_id": "…"(仅 keep=true) }
```

`run` 总时长上限 = `exec_timeout_max + 60s`（含冷启动）。无论成功失败都在 `defer` 中删除（`keep=false`），删除失败交给 reaper。`files` 总大小 ≤ `max_file_bytes`，条数 ≤ 20。

### 7.3 错误码

沿用 `WriteError` + `error.go` 映射，`internal/sandbox/errors.go` 定义领域错误：

| 领域错误 | HTTP | `code` |
|---|---|---|
| `ErrUnavailable` | 503 | `sandbox_unavailable` |
| `ErrNotFound` | 404 | `sandbox_not_found` |
| `ErrNameConflict` | 409 | `sandbox_name_conflict` |
| `ErrLimitExceeded`（项目上限） | 429 | `sandbox_limit_exceeded` |
| `ErrInvalidSpec`（镜像不在白名单、规格越界、env 非法） | 400 | `sandbox_invalid_spec` |
| `ErrInvalidPath` | 400 | `sandbox_invalid_path` |
| `ErrFileTooLarge` | 413 | `sandbox_file_too_large` |
| `ErrBusy`（同一沙盒锁等待超 5s） | 409 | `sandbox_busy` |
| `ErrDeleted` / `ErrExpired`（exec 时） | 410 | `sandbox_gone` |
| driver 传输错误 | 502 | `sandbox_backend_error`（message 固定句子，不透传 SDK 内部路径） |

### 7.4 Reaper（§3.5）

`reap_interval` 周期执行，单次最多处理 100 行：

1. `status IN (running, pending) AND expires_at < now` → 标 `expired`（不调 Cloud，Cloud 已自行回收）。
2. `source=run AND status != deleted AND created_at < now - (exec_timeout_max + 10m)` → driver `Remove` + 软删（兜底 `run` 删除失败）。
3. `status=deleted AND deleted_at < now - 7d` → 物理删行。

Reaper 随 `App.Close` 停止，复用 `cronjob.Scheduler` 的 Start/Stop 模式，不引入新调度库。

---

## 8. 安全、权限、审计、用量

### 8.1 权限

- 读（列表、详情、列目录、读文件、capabilities）：`database:read`。
- 写（创建、删除、启停、exec、写/删文件、run）：`database:write`。与 v3 Agent 沙盒工具门槛一致。
- 跨项目：Manager 所有方法以 `(projectID, id)` 查行，`project_id` 不匹配一律 `ErrNotFound`（不暴露存在性）。
- `source=agent` 的沙盒在页面可见、可查看文件、可删除；**不可 exec**（避免与进行中的 Agent run 抢锁干扰），API 返回 409 `sandbox_busy` 附说明。

### 8.2 隔离与凭据

- 不向沙盒注入任何 SimpleBase 凭据：`env` 入参拒绝 key 前缀 `SIMPLEBASE_`、`MSB_`、`AWS_`；不用 `WithSecrets`。
- 网络默认 `none`。`network=public` 需配置 `sandbox.network: public`（作为「允许上限」）；配置为 `none` 时请求 `public` 返回 400。
- 路径约束沿用 v3：绝对路径、`filepath.Clean` 后位于 `workdir` 下、拒绝 `..`。应用层约束，不是 chroot；隔离边界是 microVM。
- 四层限制：单次 exec 时限、输出截断、文件大小、VM idle / max duration；新增第五层：项目沙盒数上限。

### 8.3 审计

经现有 `audit` 服务，事件：`sandbox.create / delete / start / stop / exec / file.write / file.delete / run`。
字段只含：`project_id`、`sandbox_id`、`source`、`actor`、`exit_code`、`duration_ms`、`timed_out`、`truncated`、`bytes`（文件）。
**不记命令原文、env 值、stdout、文件内容**；命令只记 SHA-256（与 SQL 审计同尺度）。

### 8.4 用量与配额

- `usage` 新增 kind `sandbox`：每次 exec / run 记一条 `{count:1, duration_ms}`。
- **本版只记录、不拦截**（§14.1）：不新增 `project_quotas` 列，沙盒路由不调用 `CheckQuota`。
- Dashboard 指标不在本版加卡片（轻量）。

---

## 9. 与其它模块集成

### 9.1 侧边栏与路由（G1）

- `ui/src/router/index.ts`：在 `cron-jobs` 与 `agents` 之间新增
  `{ path: 'sandboxes', name: 'sandboxes', component: () => import('../pages/Sandboxes.vue'), meta: { title: '云沙盒' } }`；`legacyConsolePaths` 追加 `'sandboxes'`。
- `NavMenu.vue`：`iconMap` 增加 `sandboxes: BoxIcon`（`@lucide/vue` 已有，无新依赖）；`GROUP_DEFS` 自动化组改为
  `['gofunctions', 'cron-jobs', 'sandboxes', 'agents']`。
- 沙盒未启用时**仍显示入口**，页面展示空态说明「未配置云沙盒」及配置示例（与 Agent 模块一致的处理方式，避免功能不可发现）。

### 9.2 云 Agent（复用同一 Manager）

- `cloudagent.Sandbox` 接口与四个工具**不变**；app 层 `sandboxAdapter` 改为：
  `EnsureForThread(projectID, threadID)` → 拿到资源行 → 调 `Manager.Exec/ReadFile/WriteFile`。
- `EnsureForThread`：按 `thread_id` 查 `source=agent` 未删除行；没有则插入一行，`cloud_name` **沿用 v3 的 `sb-{threadHex}`**，保证已在 Cloud 上的 VM 能接回。
- agent 来源计入 `max_per_project`；超限时工具返回明确错误「项目云沙盒数量已达上限」。
- thread 删除继续调 `ReleaseThread` → `Manager.Delete`。
- `AgentManager.vue` 沙盒工具卡片里显示「在云沙盒页查看」链接（跳 `/console/sandboxes?id=`）。

### 9.3 云函数 / 定时任务

- 本版**不新增**定时任务类型。定时跑沙盒的方式：云函数里用 Go SDK 调 `POST /sandboxes/run`，再由定时任务触发云函数。文档给出示例。
- P2 视需求再加 `cron_jobs.kind=sandbox_run`。

### 9.4 SDK

- `packages/js-sdk/src/sandboxes.ts`：`client.sandboxes.{capabilities,list,create,get,update,delete,start,stop,exec,run,files.{list,read,write,remove}}`；导出类型；vitest 用 mock fetch 覆盖。
- `packages/go-sdk/sandboxes.go`：同名方法；`client_test.go` 用 `httptest.Server`。
- 两边 README 各加一段「在 CI 里跑 e2e」示例（`run` 一次性执行 + 判 `exit_code`）。

### 9.5 文档

- `docs/sandbox/`：`index.md`（概念、轻量模型、生命周期）、`api.md`（§7 全量）、`e2e.md`（curl / JS / Go 三种 e2e 写法 + `fake` 后端本地联调）。
- `docs/_meta.json` 注册模块；`docs/getting-started/index.md` 的 Agent「只读工具」措辞同步。
- `config.example.yaml` 补全 §5 新字段与注释。
- `internal/AGENTS.md` 职责表加 `sandbox` 行：「microsandbox Cloud 唯一接入；Driver/Manager；禁止读写 DuckLake/S3/系统库凭据、禁止本地 microVM 回退」。

---

## 10. 前端（`ui/src/pages/Sandboxes.vue`）

布局沿用 `CronJobs.vue` / `GoFunctions.vue` 的「列表 + 详情抽屉」，组件全部来自现有 `components/ui`。

### 10.1 列表页

- 顶部：标题「云沙盒」+ 说明一行（「按需启动的隔离 Linux 环境，空闲 N 分钟自动回收」）+「新建沙盒」按钮。
- capabilities `available=false`：整页空态 + 配置片段（`sandbox.enabled / SIMPLEBASE_SANDBOX_API_KEY`），按钮禁用。
- 表格列：名称、状态 badge（pending 灰 / running 绿 / stopped 黄 / expired 灰删除线 / error 红）、镜像、规格（`1C/256M`）、来源（控制台 / API / Agent / 一次性）、最近活跃（相对时间）、到期、操作（打开 / 停止 / 删除）。
- 过滤：状态、来源。默认隐藏 `deleted`。

### 10.2 新建弹窗 `SandboxCreateModal.vue`

字段：名称、镜像（下拉，来自 `images`）、CPU / 内存（受 limits 约束）、网络（仅在允许 public 时可选）、空闲超时、「立即启动」勾选。提交走 `POST /sandboxes`，带随机 `Idempotency-Key` 防双击。

### 10.3 详情抽屉 `SandboxDrawer.vue`

三个 tab：

1. **终端**（非交互）：命令输入框（回车执行，↑↓ 历史，仅存组件内存）、超时选择、输出区 `<pre>` 分别染色 stdout / stderr，尾部显示 `exit 0 · 412ms`，截断时提示。执行中禁用输入，显示「冷启动中…」（首次 pending → running）。
2. **文件**：面包屑 + 单层目录列表（`/workspace` 起），点击文本文件在 `Textarea` 中查看/编辑并保存（PUT），支持上传本地文件（≤ max_file_bytes）、下载、删除。二进制文件只提供下载。
3. **信息**：id、cloud 名、规格、网络、时间线、`last_error`、「复制 curl 示例」按钮（生成带当前沙盒 id 的 exec curl，API key 用占位符）。

`source=agent` 时终端 tab 只读并提示「由 Agent 会话使用」。

### 10.4 服务层

- `services/types.ts`：`SandboxItem`、`SandboxCapabilities`、`SandboxExecResult`、`SandboxFileEntry`。
- `services/api.ts` 接口 + `http-api.ts` 实现（`sandboxesPath(projectId, id?, suffix?)`，风格同 `cronJobsPath`）+ `mock.js` 内存实现（mock 的 exec 支持 `echo` / `ls` / `cat`，便于离线开发与 vitest）。

---

## 11. 测试

### 11.1 单元测试（默认 `go test ./...`，不访问 Cloud、不 dlopen）

| 包 | 覆盖 |
|---|---|
| `internal/config` | 新字段默认值、env 覆盖、`fake` 在生产被拒、`images` 必含 `image`、`exec_timeout ≤ exec_timeout_max` |
| `internal/sandbox` | Manager + fakeDriver：懒创建（create 后 driver 未被调用）、状态机全部边、项目上限、名称冲突、幂等、路径校验、截断、超时 → `timed_out`、env 黑名单、`run` 必删（含 exec 失败 / panic 路径）、reaper 三条规则（注入 `now`）、锁等待超时 → `ErrBusy` |
| `internal/systemdb` | 迁移 38 幂等；CRUD；`thread_id` 查询；软删过滤 |
| `internal/api` | `sandbox_handler_test.go`：fake service 覆盖每条路由的 2xx 与 §7.3 每个错误码；只读 key 写接口 403；跨项目 404；`writable=false` 写接口 503；`deps.Sandbox=nil` 时仅 capabilities 可用 |
| `internal/cloudagent` | 现有 `sandbox_tools_test.go` 不改断言即通过（接口未变） |
| `internal/app` | adapter：`EnsureForThread` 沿用 `sb-` 旧名 |

`driver_cloud.go` 不加 build tag。默认测试只要不构造 cloudDriver 就不会触发 FFI 加载；cloudDriver 只在 `app` 中构造，并且要求 `backend=cloud && enabled`。

### 11.2 HTTP e2e（新增，默认 CI 可跑）

`internal/api/e2e_sandbox_test.go`（或 `test/e2e/sandbox_test.go`，看现有约定放置）：

- 用 `app` 真实装配 + `sandbox.backend=fake` + `dev_mode=true` + 临时系统库目录，起 `httptest.Server`，用 **packages/go-sdk** 作为客户端走完整链路：
  1. capabilities → `available=true, backend=fake`
  2. create（带 Idempotency-Key，重复请求得同一 id）
  3. PUT 文件 → GET 目录 → GET 内容一致
  4. exec `cat` 读回 → `exit_code=0`；exec `exit 3` → 200 且 `exit_code=3`
  5. exec 超时 → `timed_out=true`
  6. 越权路径 → 400 `sandbox_invalid_path`
  7. 超限创建 → 429
  8. `run` 一次性 → 返回后列表中不存在该沙盒
  9. delete → GET 404 / exec 410
- JS SDK：`packages/js-sdk/src/__tests__/sandboxes.test.ts`（mock fetch，校验路径、头、错误映射）。

### 11.3 真实 Cloud 冒烟（不进默认 CI）

`//go:build sandbox_integration`，需 `SIMPLEBASE_SANDBOX_API_KEY`：create → exec `python -c 'print(1)'` → 写读文件 → stop → start（文件仍在）→ delete → `GetSandbox` 不存在。另测 `network=none` 时 `curl` 失败。

### 11.4 UI

`ui/tests/Sandboxes.test.ts`、`SandboxCreateModal.test.ts`、`SandboxDrawer.test.ts`：不可用空态、列表渲染与过滤、新建表单约束、执行输出渲染（含截断/超时/非零退出）、agent 来源只读；`NavMenu` 测试断言自动化组包含「云沙盒」且顺序为 云函数 / 定时任务 / 云沙盒 / 云 Agent。

---

## 12. 实施顺序

每步结束保持 `go build ./cmd/... ./internal/...`、`go test ./...`、`cd ui && npm run build && npm test` 通过。

| 步骤 | 内容 | 产出 |
|---|---|---|
| S1 配置与文档边界 | §5 新字段、Validate、env；`config.example.yaml`；`internal/AGENTS.md` | config 测试 |
| S2 sandbox 包重构 | Driver 接口；v3 `client.go` 迁成 `driver_cloud.go`；`driver_fake.go`；`paths.go`、`errors.go` | 单测（fake） |
| S3 系统表 + Manager | 迁移 38、`systemdb` 仓库、Manager 全部方法、reaper | Manager 单测 |
| S4 app 装配 + Agent 迁移 | `a.sandboxMgr` 独立装配、关闭顺序；cloudagent adapter 改走 Manager | 现有 Agent 测试全绿 |
| S5 HTTP API | `sandbox_handler.go`、路由、错误映射、审计、用量 | handler 测试 + §11.2 e2e |
| S6 SDK | js-sdk / go-sdk 方法与测试 | SDK 测试 |
| S7 控制台 | 路由、侧栏、页面、弹窗、抽屉、mock、服务层 | UI 测试 |
| S8 文档与收尾 | `docs/sandbox/*`、getting-started 措辞、`plan/planv4.0/IMPLEMENTATION_SUMMARY` 追加一节 | — |

S2–S4 可在一个 PR；S5–S6 一个 PR；S7–S8 一个 PR。

### 12.1 实际实施进度（2026-10-05）

| 阶段 | 状态 | 已完成 / 待补齐 |
|---|---|---|
| S1 配置 | 已实现 | `SandboxConfig`、YAML/env/Validate、`config.example.yaml`、边界文档与配置测试 |
| S2–S3 Driver、Manager、系统表 | 已实现 | Cloud/fake 驱动、v38/v39、懒启动、内存幂等、reaper；已测 Manager 生命周期、同 VM 串行/取消、删除失败重试、项目上限和 Agent 兼容；锁等待改为可注入的 `lockWait`（默认 5s），`manager_lock_test.go` 覆盖超时 → `ErrBusy`、超时后可再次获取、不同 VM 互不阻塞 |
| S4 app / Agent | 已实现 | 单一 Manager 注入 API 与 Agent；原 `sb-{threadHex}` VM 命名兼容；原 Agent 测试通过 |
| S5 HTTP API | 已实现 | CRUD / exec / files / run、审计与只记录不拦截的用量；全部领域错误码、系统项目/只读实例、真实只读 Key 已测；`sandbox_cross_project_test.go` 覆盖 11 条资源路由 × 三种越权方式的跨项目矩阵；列表支持 `cursor` 分页并返回 `next_cursor` |
| S6 SDK | 已实现 | JS/Go SDK 接口与测试通过；新增 `listPage` / `ListSandboxesPage`；真实 app + fake VM 的 Go SDK `/run` 已跑通 |
| S7 控制台 | 已实现 | 侧栏、列表、弹窗、终端、文件读写、二进制下载上传及 mock；`yarn test` 覆盖率门槛通过（未调整阈值） |
| S8 文档 | 已实现 | `docs/sandbox/*`（含分页与隔离语义）、SDK README、入门文案及本实施状态 |

---

## 13. 验收

1. `sandbox.enabled=false`：服务正常启动；侧栏「自动化」下可见「云沙盒」，页面显示未配置空态；`GET /sandboxes/capabilities` 返回 `available=false`，其它沙盒路由不挂载（404）。
2. `backend=fake, dev_mode=true`：§11.2 全部 e2e 通过，无任何外网请求。
3. `backend=cloud` + 有效 key：控制台新建沙盒 → 状态 `pending`（Cloud 上尚无 VM）→ 终端执行 `python -V` 后变 `running` → 文件 tab 上传/编辑/下载可用 → 空闲超过 `idle_timeout` 后再执行仍成功（透明恢复）。
4. `curl` 用 `database:write` API key 调 `POST /sandboxes/run` 完成一次性执行，返回后 Cloud 上无残留（或 reaper 在一个周期内清理）。
5. 只读 key 调写接口 403；跨项目访问 404；越界路径 400；超大文件 413；超过项目上限 429；超时为 200 + `timed_out=true`。
6. 云 Agent：原有对话中的沙盒工具行为不变；v3 时期已有 thread 能接回原 VM（`sb-` 旧名）；会话产生的沙盒出现在云沙盒页、来源为 Agent、终端只读。
7. 审计、日志、SSE、响应体中均不出现 Cloud API key、env 值或 SimpleBase 凭据；审计只记命令 SHA-256，不记原文。
8. 后端新增常驻协程只有 reaper 一个；默认规格 `1C/256M`、idle 5m。

---

## 14. 风险与取舍

| 风险 | 取舍 |
|---|---|
| Cloud 冷启动数秒，`exec` 同步接口可能慢 | 提供 `start:true` 预热；UI 显示「冷启动中」；`run` 时限额外 +60s |
| Cloud 自行回收后本地状态滞后 | 懒对账（`refresh=1` / 下次操作）；不轮询，换轻量 |
| SDK 无 Cloud 可用的列目录 API | cloudDriver 用 `find` 兜底，对外语义不变 |
| `sandbox_shell` / exec 可读 VM 内任意路径 | 隔离边界是 microVM，VM 内无 SimpleBase 凭据；文档写明 |

### 14.1 已确认的取舍（2026-10-04 评审结论）

| 项 | 结论 | 落地约束 |
|---|---|---|
| 执行接口形态 | **只做同步**，不做流式输出（不做 SSE / WebSocket / `stream` 参数） | `exec` / `run` 一次请求一次 JSON 响应；长任务由调用方把输出写文件再读回；UI 终端执行完整体渲染 |
| 防重复提交 | **只存内存**：进程内 LRU（项目+key → id，24h TTL，容量 1024），**不落系统库** | 重启后丢失属预期语义，文档写明「尽力而为」；不为此新增表或迁移 |
| 配额 | **只记录、不拦截** | 每次 exec / run 写 `usage` kind=`sandbox`；不调用 `CheckQuota` 拦截、不新增 `project_quotas` 列；唯一硬限制是 `max_per_project` 数量上限 |

---

## 15. 后续候选（本版不做）

- 定时任务 `kind=sandbox_run`。
- 自定义镜像注册、域名白名单出网、hostname-scoped secrets。
- 多实例：沙盒锁迁到系统库租约。

---

## 16. 验证记录与未关闭项（2026-10-05）

**已验证（2026-10-05 重跑）**：`go build ./cmd/... ./internal/...`、`go test ./... -short`、`go test ./internal/app -run TestSandbox`（e2e + 跨项目矩阵 + Manager）、`go test ./packages/go-sdk`、`cd packages/js-sdk && npm run typecheck && npm test`、`cd ui && yarn build`、`cd ui && yarn test`（90 个文件 495 个用例；statements 98.77%、branches 95.31%、functions 95.17%、lines 98.77%，四项均过 95% 门槛，`vite.config.ts` 未改）。fake e2e 使用真实 app 认证/路由/系统库。

**本轮关闭**：

1. UI 覆盖率门槛：补沙盒三个组件的交互/错误路径测试，并补齐此前拉低全局数字的 `mock-kv.js` 边界分支、`Databases.vue` 桌面表格事件、`http-api.ts` 的用户详情 / API Key / 沙盒下载映射。
2. `ErrBusy` 锁等待超时专项测试（`internal/sandbox/manager_lock_test.go`）。
3. 跨项目资源权限矩阵（`internal/app/sandbox_cross_project_test.go`）：B 的 Key 打 A 的项目路径 → 403 `cross_project_denied`；在 B 的路径下用 A 的沙盒 id（B 的 Key 与管理 Key 各一遍）→ 404 `sandbox_not_found`；并断言 A 的沙盒与文件未被改动，列表 / 名称唯一性 / 数量上限按项目隔离。
4. §7.1 列表 `cursor` 分页：按 `(created_at DESC, id DESC)` 的 keyset 游标，响应带 `next_cursor`；未知或跨项目游标 400；JS/Go SDK 新增分页方法，控制台列表自动取完所有页。

**未关闭**：

1. §11.3 真实 Cloud 生命周期和网络策略冒烟需要 `SIMPLEBASE_SANDBOX_API_KEY` 的测试环境；当前环境无该变量，**未运行**。§13 验收第 3、4 条（真实 Cloud）因此未验收，不把 fake 结果等同真实 Cloud。

**已知限制（非缺陷，按 §14.1 取舍）**：进程内幂等不提供跨重启保证；`cd ui && yarn typecheck` 在本轮之前就有大量报错（主要是 `.vue` 模块声明缺失），本轮未处理，构建与测试不依赖它。

> 代码与旧计划 v3 并行存在：旧 `internal/sandbox/client.go` 仅供兼容原单测，运行期只装配 `driver_cloud.go` 的 Manager。默认测试无密钥、不访问 Cloud。
