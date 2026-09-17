# SimpleBase 前后端 HTTP 交互协议（proto-http）

> 版本：v2.0（2026-09-16）
> 基础地址：开发 `http://localhost:8080`（vite 代理 `/v1`、`/health`、`/ws` 到 8080）；生产同源（`internal/web` embed SPA，同端口）
> 认证：所有 `/v1/*` 业务路由要求 `Authorization: Bearer <API_KEY>`
> DevMode 种子凭据：`Authorization: Bearer sb_live_dev_key_12345`，项目 `00000000-0000-0000-0000-000000000002`（展示名「商城后台」），数据库 `default`。系统库 `simplebase-system` 不出现在列表中、不可删除。
> 可执行样例：同目录 `proto.http`（VS Code REST Client / JetBrains HTTP Client 直接运行）
> 依据源码：`internal/api/router.go`、`error.go`、`database_handler.go`、`sql_handler.go`、`sql_types.go`、`data_handler.go`、`s3_handler.go`、`llm_handler.go`、`agent_handler.go`、`quota_handler.go`、`audit_handler.go`、`system_handlers.go`、`internal/systemdb/`、`internal/cloudagent/`

---

## 1. 通用约定

### 1.1 请求

| 项 | 约定 |
|---|---|
| 编码 | JSON `Content-Type: application/json`；上传为 `multipart/form-data` |
| 路径参数 | `projectID`（UUID，DevMode 种子为 `00000000-0000-0000-0000-000000000002`）、`databaseID`（catalog 生成的 UUID）、`collection`（`^[A-Za-z][A-Za-z0-9_]{0,62}$`） |
| 请求追踪 | 可选 `X-Request-ID`（UUID 或 `[a-zA-Z0-9_-]{8,64}`），响应头原样返回；不合法或未传则服务端生成 UUID |
| 体积上限 | `limits.max_request_bytes`（config.yaml 默认 10MB，.env 模板为 1MB） |

### 1.2 命名

后端 DTO 主要使用 **snake_case**（`max_tokens`、`rows_affected`、`duration_ms`、`request_id`、`last_insert_id`、`finish_reason`、`next_cursor`、`created_at`）。

**例外**：S3 接口的 `lastModified` 是 camelCase（见 §3.4）。前端 UI 层统一用 camelCase，转换集中在 `services/http-api.ts`。

### 1.3 成功响应

无统一 envelope，直接返回数据对象/数组。各接口的成功状态码不一致（201/202/204/200 混用），详见各章"成功状态"列——前端不能统一按 200 判断。

### 1.4 错误响应（统一协议）

经 `WriteError` / `errorHandler` 输出的错误，格式恒为：

```json
{
  "error": {
    "code": "invalid_api_key",
    "message": "invalid api key",
    "request_id": "6f96c9f1-..."
  }
}
```

前端 `http.ts` 拦截器统一提取 `error.message` 展示。错误码全集（源自 `internal/api/error.go`）：

| HTTP | code | 场景 |
|---|---|---|
| 400 | `invalid_request` | JSON 格式错误、集合名不合法、文档非对象、创建库含未知字段（`DisallowUnknownFields`）、echo 层 400 |
| 400 | `invalid_database_name` | 库名为空、>63 字节、含 `/` `\` 或控制字符 |
| 400 | `invalid_file_key` | S3 key 违反 `ValidateFileKey` |
| 400 | `empty_sql` | SQL 为空 |
| 400 | `invalid_sql` | SQL 含 NUL 字符 |
| 400 | `multiple_statements` | 单次请求含多条语句 |
| 400 | `sql_not_allowed` | sqlguard 黑名单语句 |
| 400 | `write_in_read_only` | 在 `query`（只读意图）提交写语句 |
| 401 | `unauthenticated` | 缺失/格式错误的 Authorization 头 |
| 401 | `invalid_api_key` | key 错误或哈希不匹配 |
| 401 | `api_key_revoked` | key 已吊销 |
| 403 | `forbidden` | 权限不足（如只读 key 调写接口） |
| 403 | `cross_project_denied` | principal 无该 project 权限 |
| 404 | `database_not_found` | `catalog.ErrNotFound`（**project 不存在也走此码**，见 §2） |
| 404 | `not_found` | 文档不存在、echo 层 404 |
| 400 | `invalid_project_name` | 创建项目时名称为空、超长或含非法字符 |
| 400 | `invalid_project_id` | 创建项目时 `id` 不是合法 UUID，或使用了保留系统项目 ID |
| 409 | `database_already_exists` | 创建重名库 |
| 409 | `project_already_exists` | 创建项目时 id 或同租户 name 冲突 |
| 409 | `invalid_state` | 状态机非法迁移 |
| 409 | `database_deleting` / `database_not_ready` | 库正在删除 / 未就绪 |
| 413 | `request_too_large` | 请求体超 BodyLimit |
| 422 | `row_limit_exceeded` | 查询行数超限 |
| 429 | `query_concurrency_exceeded` | 并发查询槽位耗尽 |
| 500 | `unsupported_value_type` | 列值类型无法序列化 |
| 500 | `internal_error` | **未映射错误兜底**（多处参数校验落在此，见各章标注） |
| 501 | `not_implemented` | 备份/恢复接口 |
| 503 | `writer_unavailable` | 只读实例收到写请求 |
| 503 | `system_store_unavailable` | 系统 DuckLake 不可用 |
| 403 | `system_database_protected` | 试图删除/改写 `kind=system` 的系统库 |
| 503 | `registry_closed` / `descriptor_write_failed` / `migration_failed` | 基础设施异常 |
| 503 | `request_canceled` | `context.Canceled` |
| 504 | `request_timeout` | `context.DeadlineExceeded` |

### 1.5 SPA fallback 注意

未匹配到 API 路由的 GET 请求会返回 **HTML**（`internal/web` 的 SPA fallback），而非 404 JSON。前端 axios 拦截器已将 `text/html` 响应转译为错误「接口不存在或返回了 HTML 页面」；手写 `fetch`（LLM stream）需同样防御。

---

## 2. 认证与项目上下文

- 中间件链：`APIKeyMiddleware`（校验 Bearer key → 注入 Principal）→ `projectContextMiddleware`（`catalog.ResolveProjectTenant` 校验 project 存在 → 注入 ProjectContext）→ `auth.Require(权限)`。
- project 不存在 → `404 database_not_found`（`ResolveProjectTenant` 返回 `catalog.ErrNotFound`，映射表未区分 project/database，前端提示文案需自行兜底）。
- key 与 project 不匹配 → `403 cross_project_denied`。
- 路由挂载是**条件性**的：`deps.Auth` 或 `deps.DatabaseHandler` 为 nil 时整个 `/v1` 组不挂载；`SQLHandler`/`DataHandler`/`LLM`/`Usage`/`Audit`/`S3FileStore` 各自为 nil 时对应子组不挂载 → 请求落到 SPA fallback 返回 HTML。`internal/app/app.go` 默认注入全部依赖。
- DevMode 种子（`internal/systemdb/seed.go`）：tenant `00000000-0000-0000-0000-000000000001` / project `00000000-0000-0000-0000-000000000002`（"商城后台"）/ database `default` / key `sb_live_dev_key_12345`，权限集 `DatabaseRead + DatabaseWrite + DatabaseAdmin + LLMInvoke + ProjectAdmin`。系统库本身 `kind=system`，列表默认隐藏，DELETE 返回 `403 system_database_protected`。
- `GET /v1/projects` 返回当前 Key 可见项目（不含隐藏的系统项目）。
- `POST /v1/projects` 创建项目（`ProjectAdmin`）：body `{ "name", "id?" }`，省略 `id` 时服务端生成 UUID；id/name 冲突 **409** `project_already_exists`。

---

## 3. 接口清单

下表 `:p` = `/v1/projects/:projectID`。权限列指路由声明的 `auth.Require` 权限。

### 3.0 项目（全局切换器）

| Method | Path | 权限 | 成功状态 | 说明 |
|---|---|---|---|---|
| GET | `/v1/projects` | DatabaseRead | 200 | `{ "projects":[{ "id","name","created_at" }] }`；隐藏系统项目 |
| POST | `/v1/projects` | ProjectAdmin | 201 | `{ "name":"...", "id":"<optional UUID>" }` → `{ "id","name","created_at" }`；未知字段 400；冲突 409 |

`ProjectAdmin` 可访问本租户下任意用户项目（不仅是 API key 绑定的那一个）。系统项目 ID `00000000-0000-0000-0000-000000000099` 不可创建、不出现在列表。

### 3.1 数据库管理（Databases 页）

| Method | Path | 权限 | 成功状态 | 说明 |
|---|---|---|---|---|
| POST | `:p/databases` | DatabaseAdmin | 201 | 创建库 `{ "name": "..." }`；body 含未知字段直接 400 |
| GET | `:p/databases?limit=&cursor=` | DatabaseRead | 200 | 分页列表，仅 `kind=user`；`limit` 默认 50、范围 1–200 |
| GET | `:p/databases/:databaseID` | DatabaseRead | 200 | 详情（唯一可能带 `snapshot` 的接口） |
| POST | `:p/databases/:databaseID/open` | DatabaseAdmin | 200 | 预热（取租约后立即释放），返回 DatabaseResponse |
| POST | `:p/databases/:databaseID/close` | DatabaseAdmin | **204 无 body** | 关闭本地连接（不删数据） |
| DELETE | `:p/databases/:databaseID` | DatabaseAdmin | **202** | 软删除并**同步**清理平面 B（DuckLake S3 前缀）；成功后 catalog 为 `deleted`，响应 `status` 为 `deleted` |

> 勘误（2026-09-17）：`POST :p/databases/:databaseID/backups` 与 `POST :p/databases/:databaseID/restore` 的 501 占位已删除，不再挂载。

写类接口（create/open/close/delete）在 `instance.writable = false` 的实例上一律 `503 writer_unavailable`。

`DatabaseResponse`（创建 / 详情 / open）：

```json
{
  "id": "db_xxx",
  "name": "default",
  "status": "creating",
  "created_at": "2026-09-15T12:00:00Z",
  "updated_at": "2026-09-15T12:00:00Z",
  "snapshot": { "last_synced_snapshot": 42, "sync_lag": 0 }
}
```

- `status` 枚举共 9 值：`creating | opening | ready | closing | closed | degraded | deleting | deleted | recovering`。创建在 catalog/descriptor 写成功后**同步转为 `ready`**（`creating` 仅瞬时存在于库内，HTTP 201 响应一般为 `ready`）；前端 tag 仍需覆盖全部取值。
- `snapshot` 仅在 `GET :p/databases/:databaseID` 且服务端注入了 `SnapshotFor` 时出现；列表接口恒无此字段。
- 列表响应：`{ "databases": [...], "next_cursor": "" }`（空 cursor 表示无更多）。
- 删除响应（202）：`{ "database_id": "db_xxx", "status": "deleted" }`（同步清存储后；若中途失败则错误映射，不会假成功）。
- 501 响应体为 `{"error":{"code":"not_implemented","message":"..."}}`，**无 `request_id` 字段**，前端解析需容忍缺失。
- `limit` 非法（非数字 / <1 / >200）时后端返回未映射错误 → `500 internal_error`；前端应在发请求前约束取值。

### 3.2 SQL 执行（SqlConsole 页）

| Method | Path | 权限 | 说明 |
|---|---|---|---|
| POST | `:p/databases/:databaseID/query` | DatabaseRead | 只读查询（`sqlguard.ReadOnly`） |
| POST | `:p/databases/:databaseID/execute` | DatabaseWrite | 单条写语句（`sqlguard.WriteAllowed`） |
| POST | `:p/databases/:databaseID/batch` | DatabaseWrite | 批量，可事务 |

请求体：

```json
// query
{ "sql": "SELECT id, data FROM \"users\" LIMIT ?", "args": [10], "max_rows": 100 }

// execute
{ "sql": "INSERT INTO \"users\" (id, data, created_at) VALUES (?, ?, ?)", "args": ["u1", "{}", "2026-09-15T12:00:00Z"] }

// batch
{ "statements": [ { "sql": "...", "args": [] } ], "transactional": true }
```

约束：
- SQL 必须用参数化占位符 `?`，禁止拼接用户输入；单条 SQL ≤ `max_sql_bytes`(64KB)；批量语句数 ≤ `max_batch_statements`(100)；`query_timeout = 30s`（超时 → 504 `request_timeout`）。
- `query` 中出现写语句 → 400 `write_in_read_only`；sqlguard 还拦截 `empty_sql` / `multiple_statements` / `invalid_sql`(NUL) / `sql_not_allowed`。
- `max_rows` 缺省取 `limits.max_query_rows`(1000)；实际行数超限 → 422 `row_limit_exceeded`。
- 并发槽位满 → 429 `query_concurrency_exceeded`（`max_concurrent_queries = 64`）。
- 只读实例上 `execute`/`batch` → 503 `writer_unavailable`。
- **DuckLake 无 `PRIMARY KEY` / sequence / `last_insert_rowid`**：建表不要写主键约束，主键用应用侧 UUID。

`QueryResponse`：

```json
{
  "columns": ["id", "data"],
  "rows": [["u1", "{\"k\":1}"]],
  "row_count": 1,
  "duration_ms": 3,
  "request_id": "..."
}
```

`rows` 是**二维数组**（不是对象数组），列名需与 `columns` 按下标对应——前端渲染表格时要自行 zip。

`ExecuteResponse`：

```json
{ "rows_affected": 1, "durability": "committed_local", "duration_ms": 5, "request_id": "..." }
```

`durability` 取值 `committed_local | synced_s3`。`last_insert_id` 已废弃（恒省略），需要回传主键用 `RETURNING` 或应用侧 UUID。

`BatchResponse`：

```json
{
  "results": [ { "index": 0, "rows_affected": 1, "duration_ms": 2 } ],
  "durability": "committed_local",
  "duration_ms": 8,
  "request_id": "..."
}
```

- `transactional: true`：任一失败整体回滚，附 `"error": { "failed_index": 2, "code": "...", "message": "..." }`。
- `transactional: false`：逐条独立执行，失败项带 `error_code` / `error_message`，后续语句继续。

### 3.3 文档数据（数据库管理页）

| Method | Path | 权限 | 成功状态 | 说明 |
|---|---|---|---|---|
| GET | `:p/databases/:databaseID/data/collections` | DatabaseRead | 200 | `{"collections":["users"]}`，最多 200 条 |
| POST | `:p/databases/:databaseID/data/collections` | DatabaseWrite | **201 无 body** | `{"name":"users"}` |
| GET | `:p/databases/:databaseID/data/collections/:collection` | DatabaseRead | 200 | `{"rows":[{ "id":"u1", ...文档字段 }]}`，最多 1000 行，`created_at DESC` |
| POST | `:p/databases/:databaseID/data/collections/:collection/documents` | DatabaseWrite | 201 | body 为任意 JSON 对象；可带 `"id"`，缺省生成 UUID；回显文档 |
| PUT | `:p/databases/:databaseID/data/collections/:collection/documents/:id` | DatabaseWrite | 200 | body 为 JSON 对象（`id` 被剔除）；回显；不存在 404 `not_found` |
| DELETE | `:p/databases/:databaseID/data/collections/:collection/documents/:id` | DatabaseWrite | **204 无 body** | 表不存在也返回 204（幂等） |

兼容旧路径（**无** `databaseID`，隐式取项目第一个库）：`:p/data/collections...`。新 UI 只走带 `databaseID` 的路径。`DataHandler.acquire` 在路径含 `:databaseID` 时必须 `GetDatabase` 该库，**禁止**静默回落到第一个库。

其他约束：
- 集合名规则 `^[A-Za-z][A-Za-z0-9_]{0,62}$`，不合法 → 400 `invalid_request`（中文提示「集合名称仅支持字母、数字和下划线，且必须以字母开头」）。
- 文档存于 DuckLake 表 `(id VARCHAR, data VARCHAR, created_at VARCHAR)`，`data` 为 JSON 文本；返回时 `data` 已展开为对象字段并合入 `id`。
- 集合不存在时 `GET` 返回空 `rows`（不 404）；`POST documents` 会自动建表（`CREATE TABLE IF NOT EXISTS`）。
- 只读实例上所有写操作 → `503 writer_unavailable`。

### 3.4 S3 对象存储（S3Manager 页）

| Method | Path | 权限 | 成功状态 | 说明 |
|---|---|---|---|---|
| GET | `:p/s3/objects?prefix=images/&refresh=1` | DatabaseRead | 200 | 默认读 `sys_s3_objects`；`refresh=1`/`true` 对账平面 A 后返回 |
| POST | `:p/s3/objects` | DatabaseWrite | 200 | multipart：`key` + `file` 字段 → 对象元数据 |
| DELETE | `:p/s3/objects?key=images/a.png` | DatabaseWrite | 200 | → `{"ok":true}` |
| GET | `:p/s3/presign?key=images/a.png` | DatabaseRead | 200 | → `{"url":"..."}`，TTL 固定 15 分钟 |

- 上传/删除成功后维护 `sys_s3_objects` 索引；列表默认读表。
- **项目隔离**：服务端自动在物理 key 前拼 `{projectID}/`；返回给前端的 key 已剥掉项目段。
- **字段命名例外**：本组用 camelCase `lastModified`（RFC3339 UTC）；服务端时间为零值时该字段省略。
- key 校验（`objectstore.ValidateFileKey`）：非空、≤1024 字节、无 NUL、不以 `/` 开头、不含 `\`、按 `/` 分段后不含 `.` 或 `..` → 违反则 400 `invalid_file_key`。`prefix` 为空串时跳过校验。
- `key`/`file` 缺失走 echo `HTTPError` → 400 `invalid_request`，message 为 `key is required` / `file is required`。
- 上传体积受 `max_request_bytes` 限制，超限 → 413 `request_too_large`；**前端应在选择文件后本地预校验并提示**。
- 无上传进度接口；进度需前端用 axios `onUploadProgress` 自行采集。

### 3.5 LLM Gateway（LlmManager 页）

| Method | Path | 权限 | 说明 |
|---|---|---|---|
| GET | `:p/llm/providers` | DatabaseRead | `{"providers":["openai"]}` |
| POST | `:p/llm/chat` | DatabaseRead | 非流式对话 |
| POST | `:p/llm/stream` | DatabaseRead | SSE 流式对话 |

> 路由声明的权限是 `DatabaseRead`（**不是 `LLMInvoke`**）；`LLMInvoke` 目前只存在于种子 key 权限集，未参与路由校验。调用前服务端额外执行 `CheckQuota(projectID, "llm")`。

请求体（chat / stream 相同）：

```json
{
  "model": "gpt-4o-mini",
  "messages": [
    { "role": "system", "content": "你是助手" },
    { "role": "user", "content": "你好" }
  ],
  "max_tokens": 1024,
  "temperature": 0.7
}
```

`model` / `max_tokens` / `temperature` 可选，缺省由项目默认供应商配置决定；`messages` 非空必填。

> ⚠️ **参数校验错误码与其他接口不一致**：`messages` 为空数组或 body JSON 非法时，handler 返回未映射的普通 error → 实际响应是 **500 `internal_error`**（不是 400）。前端应在提交前本地校验 `messages.length > 0`。

`chat` 响应：

```json
{
  "content": "你好！有什么可以帮你？",
  "usage": { "prompt_tokens": 12, "completion_tokens": 8, "total_tokens": 20 },
  "model": "gpt-4o-mini",
  "provider": "openai",
  "finish_reason": "stop"
}
```

`stream` 响应：`Content-Type: text/event-stream`、`Cache-Control: no-cache`，SSE 帧 `data: {...}\n\n`：

```
data: {"type":"chunk","content":"你"}
data: {"type":"chunk","content":"好"}
data: {"type":"end"}
```

- 服务端 chunk 结构固定为 `LLMStreamChunk`：`{ "type": string, "content"?: string, "finish_reason"?: string }`（`omitempty`）。结束帧恒为 `{"type":"end"}`。
- **流已开始后无法传递错误**：reader 报错时服务端只补发 `{"type":"end"}` 并关闭，前端无法区分「正常结束」与「中途失败」。需区分则后端要补 `{"type":"error"}` 帧（见 §6.4）。
- 首帧之前的失败（配额 / 供应商初始化）在 `WriteHeader(200)` 之前发生，仍是标准 JSON 错误响应。
- 前端 `http-api.ts llmStream` 额外兼容 `delta` / OpenAI `choices[0].delta.content` 形态，属防御性代码，可保留。
- 前端用原生 `fetch` + ReadableStream 消费（axios 不支持流式），`AbortController` 支持「停止生成」；错误分支的 `resp.json()` 需 `.catch()` 兜底。

### 3.8 日志查询（Logs 页，HTTP 轮询，无 WebSocket）

| Method | Path | 权限 | 说明 |
|---|---|---|---|
| GET | `:p/logs?level=&q=&from=&to=&limit=` | DatabaseRead | 查 `sys_log_events`；`from`/`to` 为 RFC3339 |
| GET | `:p/logs/retention` | DatabaseRead | `{"scope","keep_days","updated_at"}` |
| PUT | `:p/logs/retention` | ProjectAdmin | `{"keep_days":14}` |

一期不做 WebSocket。HTTP access 摘要异步写入系统库。

### 3.9 设置与 LLM 会话

| Method | Path | 权限 | 说明 |
|---|---|---|---|
| GET/PUT | `:p/llm/settings` | DatabaseRead / ProjectAdmin | 项目默认 provider/model/temperature/max_tokens |
| GET/POST | `:p/llm/sessions` | DatabaseRead / DatabaseWrite | 会话列表 / 创建 |
| GET/DELETE | `:p/llm/sessions/:id` | DatabaseRead / DatabaseWrite | 详情 / 归档 |
| GET/POST | `:p/llm/sessions/:id/messages` | DatabaseRead / DatabaseWrite | 历史 / 追加消息 |
| GET/PUT | `:p/settings` | DatabaseRead / ProjectAdmin | 项目级 `sys_settings_project` |
| GET/PUT | `/v1/settings` | ProjectAdmin | 实例级 `sys_settings_global` |

`POST :p/llm/chat` 成功后服务端写入 `sys_llm_messages`（可带可选 `session_id`；缺省自动建会话）。

`GET/PUT :p/llm/settings` 载荷：

```json
{
  "default_provider": "openai",
  "default_model": "gpt-4o-mini",
  "temperature": 0.7,
  "max_tokens": 1024
}
```

主题仍走前端 localStorage，不需要后端接口。

### 3.10 Dashboard 指标

| Method | Path | 权限 | 响应 |
|---|---|---|---|
| GET | `:p/metrics/summary` | DatabaseRead | `{ "total_requests", "error_rate", "avg_latency_ms", "active_databases" }` |
| GET | `:p/metrics/trend?days=7` | DatabaseRead | `{ "points": [ { "date", "requests", "errors" } ] }` |

数据来自 `sys_metric_samples`（HTTP 中间件采样）。现有 `/metrics` Prometheus 文本仍保留给 Grafana。

### 3.11 尚未实现的前端缺口

| 前端调用 | 影响页面 | 处置 |
|---|---|---|
| `GET /faas/functions` 等 | FaaSManager | 后端无 FaaS 模块（未立项） |
| `GET :p/llm/provider-catalog` / `provider-configs` | Settings 厂商凭证 | 本期未做；LLM settings/sessions 已落地 |
| `GET /ws/logs` | 旧 Logs 页 | 改用 `GET :p/logs` HTTP 轮询 |

> Dashboard 请改调 `GET :p/metrics/summary|trend`（已实现）。旧 `/metrics/summary` 根路径仍不存在。

### 3.12 Cloud Agent（AgentManager 页，替代 LLM 对话）

前缀 `:p` = `/v1/projects/:projectID`。实现：`internal/api/agent_handler.go` + `internal/cloudagent`（eino ChatModelAgent）。模型与厂商密钥仍走 `sys_llm_settings` / LLM Gateway，**不**写入 agent 表或 prompt。

权限与 LLM 对齐：读 `DatabaseRead`，写 agent/thread `DatabaseWrite`，**run 与 cancel 为 `DatabaseRead`**（另 `CheckQuota(projectID, "llm")`）。`/llm/*` 保持可用（deprecated）；UI `/llm` → `/agents`。

| Method | Path | 权限 | 成功状态 | 说明 |
|---|---|---|---|---|
| GET | `:p/agents/modules` | DatabaseRead | 200 | 内置模块目录（含 `team_supported: false` stub） |
| GET | `:p/agents` | DatabaseRead | 200 | `{ "agents":[CloudAgent] }`；首次列出时幂等种子 Database / S3 / Logs |
| POST | `:p/agents` | DatabaseWrite | 201 | 创建；`name` 必填；未知 `module` → 400 |
| GET | `:p/agents/:agentID` | DatabaseRead | 200 | 详情；不存在 404 |
| PATCH | `:p/agents/:agentID` | DatabaseWrite | 200 | 部分更新；空 `name` 保留原值 |
| DELETE | `:p/agents/:agentID` | DatabaseWrite | **204 无 body** | 软删（`archived_at`） |
| GET | `:p/agent-threads` | DatabaseRead | 200 | `{ "threads":[...] }`，最多 50 |
| POST | `:p/agent-threads` | DatabaseWrite | 201 | `{ "title" }` |
| GET | `:p/agent-threads/:threadID` | DatabaseRead | 200 | 详情 |
| DELETE | `:p/agent-threads/:threadID` | DatabaseWrite | **204 无 body** | 软删 |
| GET | `:p/agent-threads/:threadID/messages` | DatabaseRead | 200 | `{ "messages":[...] }`，最多 200 |
| POST | `:p/agent-threads/:threadID/runs` | DatabaseRead | 200 | `{ content, mentions:[{agent_id}], stream? }`；quota `llm` |
| POST | `:p/agent-runs/:runID/cancel` | DatabaseRead | 200 | 取消进行中的 run |

`GET :p/agents/modules`：

```json
{
  "modules": [
    {
      "id": "database",
      "name": "Database",
      "description": "Readonly database inspection and SQL",
      "default_tools": ["list_databases", "list_collections", "readonly_sql"],
      "team_supported": false
    }
  ]
}
```

模块 id：`database` / `s3` / `logs` / `general`。`team_supported` 恒 false（Host Multi-Agent 为 Phase 4）。

`CloudAgent`：

```json
{
  "id": "…",
  "name": "Database",
  "module": "database",
  "description": "…",
  "system_prompt": "",
  "tool_ids": ["list_databases", "list_collections", "readonly_sql"],
  "model_override": "",
  "team_enabled": false,
  "created_at": "2026-09-17T00:00:00Z",
  "updated_at": "2026-09-17T00:00:00Z"
}
```

- `POST`：`name` 为空 → 400；`module` 缺省 `general`；未知 module → 400 `unknown module`；`tool_ids` 缺省该模块默认只读工具，未知 id 被丢弃。
- `DELETE` agent/thread 为软删；列表不返回已归档行。
- `mentions` 为空时 run 使用项目第一个未归档 agent；`mentions[0].agent_id` 不存在 → 400。
- `content` 为空 → 400。
- Phase 1 只读工具：`list_databases` / `list_collections` / `readonly_sql`（sqlguard.ReadOnly）/ `list_objects` / `head_object` / `search_logs` / `log_level_stats`。**无写工具。**

非流式 run（`stream: false` 或缺省）：

```json
{
  "run": { "id": "…", "thread_id": "…", "agent_id": "…", "status": "completed" },
  "message": { "id": "…", "role": "assistant", "content": "…", "tool_calls": [], "run_id": "…" },
  "agent_id": "…"
}
```

流式（`stream: true`）：`Content-Type: text/event-stream`、`Cache-Control: no-cache`，SSE 帧 `data: {...}\n\n`：

```
data: {"type":"run","run_id":"…"}
data: {"type":"token","content":"你","run_id":"…"}
data: {"type":"tool_call","name":"list_databases","arguments":"{}","run_id":"…"}
data: {"type":"tool_result","name":"list_databases","content":"[…]","run_id":"…"}
data: {"type":"end"}
```

| `type` | 字段 | 说明 |
|---|---|---|
| `run` | `run_id` | 首帧，便于前端 cancel |
| `token` | `content` | 模型增量（前端亦兼容 `chunk`） |
| `tool_call` | `name`, `arguments` | 只读工具调用 |
| `tool_result` | `name`, `content` | 工具返回 JSON 文本 |
| `error` | `message` | 中途失败（与 §6.4 LLM stream 不同，本接口有 error 帧） |
| `end` | — | 结束 |

运行时不可用时流式仍 200：先 `error` 再 `end`。`cancel` 取消 run context；已结束的 run 原样返回。

Prompt 组装（禁止密钥）：platform base → module template → `agent.system_prompt` → 项目非机密 env → 截断只读 snapshot → history → user。S3/provider key 永不进入 prompt。

---

## 4. 调用样例索引（见 proto.http）

| 分组 | 覆盖 |
|---|---|
| 基础设施 | `/health/live`、`/health/ready`、`/metrics`（注释态） |
| Projects | `GET /v1/projects`、`POST /v1/projects` |
| Databases | 创建 / 列表 / 详情 / open / close / 备份(501) / 删除(注释态) |
| SQL | query(元数据) / 建表 / 插入 / 带参查询 / batch 事务 / batch 非事务 |
| Documents | 集合列表 / 建集合 / 文档列表 / 插入 / 更新 / 删除(注释态) |
| S3 | 列表（含 refresh）/ multipart 上传 / 预签名 / 删除(注释态) |
| LLM | providers / chat / stream / settings / sessions |
| Cloud Agent | modules / agents CRUD / threads / messages / runs（SSE） / cancel |
| Quota & Audit | 配额 / 审计（读 `sys_operations`） |
| Metrics / Logs / Settings | summary、trend、logs、retention、project/global settings |
| 错误演示 | 无认证 / 错误 key / 不存在的 project / 只读意图写语句 / SPA fallback |

破坏性或依赖前序步骤的请求以 `#` 注释保留，按需打开。

---

## 5. 前端接入方式（对照 `ui/src/services`）

```
services/types.ts 的 Api 接口域        对应章节    状态
  db.*        → §3.3    ✔ 已接，路径含 databaseID
  s3.*        → §3.4    ✔ 已接；列表默认同索引表，可加 refresh=1
  llm.*       → §3.5    ✔ 已接（deprecated UI）；chat 成功后服务端落库
  agents.* / agentThreads.* → §3.12  ✔ Cloud Agent；UI `/agents`，`/llm` 重定向
  llmSettings.*/sessions → §3.9  ✔ 后端已落地（模型供给仍走此配置）
  databases.* → §3.1    ✔ 列表过滤 kind=user
  sql.*       → §3.2    ✔
  quota.*     → §3.6    ✔
  audit.*     → §3.6    ✔ 读 sys_operations
  metrics.*   → §3.10   ✔ GET :p/metrics/summary|trend
  faas.*      → §3.11   后端无此模块，仅 Mock 可用
  logs.*      → §3.8    ✔ HTTP GET :p/logs（无 WS）
  projects.*  → §3.0    ✔ list + create
```

`GET /v1/projects` 返回当前 Key 可见项目（DevMode 种子 UUID `00000000-0000-0000-0000-000000000002`）。`POST /v1/projects` 创建项目。

拦截器约定（`http.ts`）：
1. 请求头自动注入 `Authorization: Bearer <key>`（key 来自 localStorage，默认 DevMode 种子 key）。
2. 响应 `content-type: text/html` → 转译为「接口不存在或返回了 HTML 页面」（SPA fallback 防御）。
3. 错误统一提取 `error.message` → `Promise.reject(Error(msg))`；需容忍 501 响应缺少 `request_id`。
4. 手写 `fetch`（LLM stream）不经 axios 拦截器，须自行做 HTML fallback 与 JSON 解析防御。

---

## 6. 待补接口建议（后端 TODO）

### 6.1 Dashboard / Logs / Settings（已落地）

见 §3.8–§3.10。前端需从 Mock / WS / Prometheus 文本切到这些 JSON API。

### 6.2 文档 API 显式指定数据库（已落地）

路径 `:p/databases/:databaseID/data/collections/...` 已挂载；`acquire` 在带 `databaseID` 时 `GetDatabase` 该库。旧 `:p/data/*` 仍取第一个库以保持兼容。

### 6.3 错误码一致性修正（P1，前端错误提示质量）

以下场景当前返回 500 `internal_error`，建议映射为 4xx：
- `llm/chat`、`llm/stream` 的 `messages` 为空或 body 非法（`llm_handler.go` 用 `errors.New` 而非 `NewAPIError`）
- `databases` 列表 `limit` 参数越界（`parseListParams` 返回 `fmt.Errorf`）
- 文档 API 在项目无库时的 `no database configured for project`（建议 409 或 404 + 专用 code）
- 各 handler 的 `project context missing`（理论不可达，但同样未映射）

### 6.4 SSE 错误帧（P2，流式失败可感知）

`llm/stream` 中途失败时仅补发 `{"type":"end"}`，前端无法区分正常结束与失败。建议增加 `{"type":"error","message":"..."}` 帧。

### 6.5 厂商凭证 catalog（P2）

`GET :p/llm/provider-catalog` 与 `GET/PUT/DELETE :p/llm/provider-configs` 仍未实现；密钥须走 `credential_ref`，禁止回显明文。

### 6.6 不建议本轮前端接入的能力

- **FaaS**：后端无对应模块，属未立项功能。
- **实时日志 WebSocket**：改用 HTTP 轮询 `GET :p/logs`。

