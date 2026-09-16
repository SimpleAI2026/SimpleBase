# SimpleBase 前后端 HTTP 交互协议（proto-http）

> 版本：v2.0（2026-09-15）
> 基础地址：开发 `http://localhost:8080`（vite 代理 `/v1`、`/health`、`/ws` 到 8080）；生产同源（`internal/web` embed SPA，同端口）
> 认证：所有 `/v1/*` 业务路由要求 `Authorization: Bearer <API_KEY>`
> DevMode 种子凭据：`Authorization: Bearer sb_live_dev_key_12345`，项目 `proj-01`，数据库 `default`
> 可执行样例：同目录 `proto.http`（VS Code REST Client / JetBrains HTTP Client 直接运行）
> 依据源码：`internal/api/router.go`、`error.go`、`database_handler.go`、`sql_handler.go`、`sql_types.go`、`data_handler.go`、`s3_handler.go`、`llm_handler.go`、`quota_handler.go`、`audit_handler.go`

---

## 1. 通用约定

### 1.1 请求

| 项 | 约定 |
|---|---|
| 编码 | JSON `Content-Type: application/json`；上传为 `multipart/form-data` |
| 路径参数 | `projectID`（如 `proj-01`）、`databaseID`（catalog 生成的 ID）、`collection`（`^[A-Za-z][A-Za-z0-9_]{0,62}$`） |
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
| 409 | `database_already_exists` | 创建重名库 |
| 409 | `invalid_state` | 状态机非法迁移 |
| 409 | `database_deleting` / `database_not_ready` | 库正在删除 / 未就绪 |
| 413 | `request_too_large` | 请求体超 BodyLimit |
| 422 | `row_limit_exceeded` | 查询行数超限 |
| 429 | `query_concurrency_exceeded` | 并发查询槽位耗尽 |
| 500 | `unsupported_value_type` | 列值类型无法序列化 |
| 500 | `internal_error` | **未映射错误兜底**（多处参数校验落在此，见各章标注） |
| 501 | `not_implemented` | 备份/恢复接口 |
| 503 | `writer_unavailable` | 只读实例收到写请求 |
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
- DevMode 种子（`internal/app/app.go:277-298`）：tenant `00000000-0000-0000-0000-000000000001` / project `proj-01`（"商城后台"）/ database `default` / key `sb_live_dev_key_12345`，权限集 `DatabaseRead + DatabaseWrite + DatabaseAdmin + LLMInvoke + ProjectAdmin`。

---

## 3. 接口清单

下表 `:p` = `/v1/projects/:projectID`。权限列指路由声明的 `auth.Require` 权限。

### 3.1 数据库管理（Databases 页）

| Method | Path | 权限 | 成功状态 | 说明 |
|---|---|---|---|---|
| POST | `:p/databases` | DatabaseAdmin | 201 | 创建库 `{ "name": "..." }`；body 含未知字段直接 400 |
| GET | `:p/databases?limit=&cursor=` | DatabaseRead | 200 | 分页列表，`limit` 默认 50、范围 1–200 |
| GET | `:p/databases/:databaseID` | DatabaseRead | 200 | 详情（唯一可能带 `snapshot` 的接口） |
| POST | `:p/databases/:databaseID/open` | DatabaseAdmin | 200 | 预热（取租约后立即释放），返回 DatabaseResponse |
| POST | `:p/databases/:databaseID/close` | DatabaseAdmin | **204 无 body** | 关闭本地连接（不删数据） |
| POST | `:p/databases/:databaseID/backups` | DatabaseAdmin | **501** | 未实现 |
| POST | `:p/databases/:databaseID/restore` | DatabaseAdmin | **501** | 未实现 |
| DELETE | `:p/databases/:databaseID` | DatabaseAdmin | **202** | 软删除并**同步**清理平面 B（DuckLake S3 前缀）；成功后 catalog 为 `deleted`，响应 `status` 为 `deleted` |

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

### 3.3 文档数据（DataManager 页）

| Method | Path | 权限 | 成功状态 | 说明 |
|---|---|---|---|---|
| GET | `:p/data/collections` | DatabaseRead | 200 | `{"collections":["users"]}`，最多 200 条 |
| POST | `:p/data/collections` | DatabaseWrite | **201 无 body** | `{"name":"users"}` |
| GET | `:p/data/collections/:collection` | DatabaseRead | 200 | `{"rows":[{ "id":"u1", ...文档字段 }]}`，最多 1000 行，`created_at DESC` |
| POST | `:p/data/collections/:collection/documents` | DatabaseWrite | 201 | body 为任意 JSON 对象；可带 `"id"`，缺省生成 UUID；回显文档 |
| PUT | `:p/data/collections/:collection/documents/:id` | DatabaseWrite | 200 | body 为 JSON 对象（`id` 被剔除）；回显；不存在 404 `not_found` |
| DELETE | `:p/data/collections/:collection/documents/:id` | DatabaseWrite | **204 无 body** | 表不存在也返回 204（幂等） |

**重要：本组接口不带 `databaseID`。** 服务端 `DataHandler.acquire` 自动取该 project 下 `ListDatabases(limit=1)` 的**第一个**数据库。因此：
- 项目内有多个库时，文档 API 只操作第一个库，前端不能假设它等于用户在 Databases 页选中的库；
- 项目下无任何库时返回 `500 internal_error`（服务端错误 `no database configured for project` 未映射），前端需给出「请先创建数据库」引导。

其他约束：
- 集合名规则 `^[A-Za-z][A-Za-z0-9_]{0,62}$`，不合法 → 400 `invalid_request`（中文提示「集合名称仅支持字母、数字和下划线，且必须以字母开头」）。
- 文档存于 DuckLake 表 `(id VARCHAR, data VARCHAR, created_at VARCHAR)`，`data` 为 JSON 文本；返回时 `data` 已展开为对象字段并合入 `id`。
- 集合不存在时 `GET` 返回空 `rows`（不 404）；`POST documents` 会自动建表（`CREATE TABLE IF NOT EXISTS`）。
- 只读实例上所有写操作 → `503 writer_unavailable`。

### 3.4 S3 对象存储（S3Manager 页）

| Method | Path | 权限 | 成功状态 | 说明 |
|---|---|---|---|---|
| GET | `:p/s3/objects?prefix=images/` | DatabaseRead | 200 | `[{ "key","size","lastModified" }]`，最多 1000 条 |
| POST | `:p/s3/objects` | DatabaseWrite | 200 | multipart：`key` + `file` 字段 → 对象元数据 |
| DELETE | `:p/s3/objects?key=images/a.png` | DatabaseWrite | 200 | → `{"ok":true}` |
| GET | `:p/s3/presign?key=images/a.png` | DatabaseRead | 200 | → `{"url":"..."}`，TTL 固定 15 分钟 |

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

### 3.9 设置页 / 厂商凭证 / 项目 LLM 默认值（**后端未实现**，前端契约草案）

> 来源：`ui-settings-chat-plan.md`。本轮**只定契约，不实现 Go handler**。
> 现网可用的仍是 §3.5：`providers` / `chat` / `stream`。设置页一期可纯本地存储；下列路由供二期对齐 catalog `LLMProviderConfig`。

#### 3.9.1 项目 LLM 默认设置

| Method | Path | 权限（建议） | 说明 |
|---|---|---|---|
| GET | `:p/llm/settings` | DatabaseRead | 返回项目默认模型/供应商/采样参数 |
| PUT | `:p/llm/settings` | ProjectAdmin | 全量更新（或 PATCH 语义，实现时二选一写死） |

`GET/PUT` 响应与 PUT 请求体：

```json
{
  "default_provider": "openai",
  "default_model": "gpt-4o-mini",
  "temperature": 0.7,
  "max_tokens": 1024
}
```

- 字段均可选；`null`/省略表示清除覆盖、回退服务端全局默认。
- 未知 `default_provider` → 400 `invalid_request`。

#### 3.9.2 预置厂商与凭证状态

| Method | Path | 权限（建议） | 说明 |
|---|---|---|---|
| GET | `:p/llm/provider-catalog` | DatabaseRead | 返回可配置厂商模板（静态 catalog，可含服务端裁剪） |
| GET | `:p/llm/provider-configs` | DatabaseRead | 项目已保存的厂商配置**状态**（不含 secret 明文） |
| PUT | `:p/llm/provider-configs/:providerId` | ProjectAdmin | 创建/更新某厂商配置与凭证 |
| DELETE | `:p/llm/provider-configs/:providerId` | ProjectAdmin | 删除配置并作废凭证引用 |

`GET :p/llm/provider-catalog` 示例：

```json
{
  "providers": [
    {
      "id": "openai",
      "name": "OpenAI",
      "protocol": "openai_chat",
      "fields": [
        { "key": "api_key", "label": "API Key", "secret": true, "required": true },
        { "key": "base_url", "label": "Base URL", "secret": false, "required": false },
        { "key": "organization", "label": "Organization", "secret": false, "required": false }
      ],
      "suggested_models": ["gpt-4o-mini", "gpt-4o"]
    }
  ]
}
```

`GET :p/llm/provider-configs` 示例（**禁止**返回 key 明文）：

```json
{
  "configs": [
    {
      "provider_id": "openai",
      "enabled": true,
      "default_model": "gpt-4o-mini",
      "credential_configured": true,
      "credential_hint": "sk-...abc",
      "updated_at": "2026-09-16T06:00:00Z"
    }
  ]
}
```

`PUT :p/llm/provider-configs/:providerId` 请求体：

```json
{
  "enabled": true,
  "default_model": "gpt-4o-mini",
  "credentials": {
    "api_key": "sk-...",
    "base_url": "https://api.openai.com/v1"
  }
}
```

- `credentials` 中 `secret: true` 字段只写不读；更新时可只传要轮换的字段。
- 服务端应存 `CredentialRef`（或等价密文），与 `internal/catalog` 的 `LLMProviderConfig` 对齐。
- 响应同 `provider-configs` 单条结构；**永不回显** `api_key` 全文。

#### 3.9.3 与 §3.5 对话接口的演进（可选，后端 TODO）

现有 `POST :p/llm/chat|stream` 请求体可增加可选字段（向后兼容）：

```json
{
  "provider": "openai",
  "model": "gpt-4o-mini",
  "messages": [{ "role": "user", "content": "hi" }]
}
```

- 缺省 `provider`：行为与今日一致（项目默认供应商）。
- 指定未配置凭证的 provider → 400/409（实现时定码），前端设置页引导去配置。

#### 3.9.4 主题设置

主题（`light` | `dark` | `system`）**不需要后端接口**；前端 `localStorage` + `stores/settings` 即可。勿为此新增 HTTP 路由。

### 3.6 配额与审计

| Method | Path | 权限 | 响应 |
|---|---|---|---|
| GET | `:p/quota` | DatabaseRead | `{"llm_allowed":true,"database_allowed":true}` |
| GET | `:p/audit?limit=50` | DatabaseRead | `{"operations":[],"limit":50}` |

> `GET :p/audit` 是**占位实现**：handler 完全忽略 `AuditService`（源码里 `_ = h.svc`），`operations` 恒为空数组，仅回显 `limit`。前端接入后不会有数据，审计页在后端补齐前不应作为独立菜单项。

### 3.7 Prometheus 指标与健康检查（无认证）

| Method | Path | 响应 |
|---|---|---|
| GET | `/health/live` | 200 存活（不做 I/O） |
| GET | `/health/ready` | 200 就绪（检查 catalog/S3）；依赖未就绪 503 |
| GET | `/metrics` | Prometheus **文本格式**（路径由 `observability.metrics_path` 配置），不适合前端直接消费，见 §6.1 |

### 3.8 前端已调用但后端未实现的路由（当前 UI 的 404 来源）

已用 `rg` 在 `internal/`、`cmd/` 全部 `.go` 源码中确认：以下路径**完全不存在**（仅出现在 `internal/web/dist` 的前端构建产物里）。它们命中 SPA fallback 返回 `index.html`，前端拦截器转为「接口不存在或返回了 HTML 页面」。

| 前端调用 | 影响页面 | 处置 |
|---|---|---|
| `GET /metrics/summary`、`GET /metrics/trend` | Dashboard 全部内容 | 后端补 JSON 汇总接口（§6.1）；落地前 Dashboard 改用 `:p/quota` + `:p/databases` 填充 |
| `GET /faas/functions`、`POST /faas/deploy`、`POST /faas/invoke/:name` | FaaSManager 整页 | **后端无 FaaS 模块**（功能未立项，非路由遗漏）。菜单隐藏，代码与 mock 保留 |
| `GET /ws/logs`（WebSocket） | Logs 整页 | 后端无任何 WebSocket 实现。改为 Mock 演示 + 显式提示，或后端另立 plan |

> `vite.config.ts` 代理只配了 `/v1`、`/health`、`/ws`，**`/metrics` 与 `/faas` 前缀未代理**，开发模式下这两组请求打到 vite devServer(5173) 自身并返回 index.html——与生产行为一致（都是 HTML），但排障时易误判。后端补 `/metrics/summary` 时若挂在 `/v1/projects/:p/...` 下则无需改代理。

---

## 4. 调用样例索引（见 proto.http）

| 分组 | 覆盖 |
|---|---|
| 基础设施 | `/health/live`、`/health/ready`、`/metrics`（注释态） |
| Databases | 创建 / 列表 / 详情 / open / close / 备份(501) / 删除(注释态) |
| SQL | query(元数据) / 建表 / 插入 / 带参查询 / batch 事务 / batch 非事务 |
| Documents | 集合列表 / 建集合 / 文档列表 / 插入 / 更新 / 删除(注释态) |
| S3 | 列表 / multipart 上传 / 预签名 / 删除(注释态) |
| LLM | providers / chat / stream |
| Quota & Audit | 配额 / 审计 |
| 错误演示 | 无认证 / 错误 key / 不存在的 project / 只读意图写语句 / SPA fallback |

破坏性或依赖前序步骤的请求以 `#` 注释保留，按需打开。

---

## 5. 前端接入方式（对照 `ui/src/services`）

```
services/types.ts 的 Api 接口域        对应章节    状态
  db.*        → §3.3    ✔ 已接，需去 proj-01 硬编码（8 处）
  s3.*        → §3.4    ✔ 已接，需补上传进度与体积预校验
  llm.*       → §3.5    ✔ 已接，需补 messages 本地校验
  llmSettings.*/providerConfigs.* → §3.9  前端契约已定，后端未实现（见 ui-settings-chat-plan.md）
  databases.* → §3.1    待新增（Phase 2）
  sql.*       → §3.2    待新增（Phase 2）
  quota.*     → §3.6    待新增（Phase 2）
  audit.*     → §3.6    待新增（低优先，后端占位）
  metrics.*   → §6.1    待后端实现，当前调用必然失败
  faas.*      → §3.8    后端无此模块，仅 Mock 可用
  logs.*      → §3.8    后端无 WS，仅 Mock 可用
```

**没有「项目列表」接口**：后端只有 `/v1/projects/:projectID/*` 子路由，不存在 `GET /v1/projects`。前端无法枚举项目，项目切换只能由用户手工输入 + 本地保存项目 ID。

拦截器约定（`http.ts`）：
1. 请求头自动注入 `Authorization: Bearer <key>`（key 来自 localStorage，默认 DevMode 种子 key）。
2. 响应 `content-type: text/html` → 转译为「接口不存在或返回了 HTML 页面」（SPA fallback 防御）。
3. 错误统一提取 `error.message` → `Promise.reject(Error(msg))`；需容忍 501 响应缺少 `request_id`。
4. 手写 `fetch`（LLM stream）不经 axios 拦截器，须自行做 HTML fallback 与 JSON 解析防御。

---

## 6. 待补接口建议（后端 TODO）

### 6.1 Dashboard 指标汇总（P0，Dashboard 整页依赖）

现有 `/metrics` 是 Prometheus 文本格式，前端无法直接消费。建议新增：

```
GET /v1/projects/:projectID/metrics/summary        权限 DatabaseRead
→ { "total_requests": 1234, "error_rate": 0.02, "avg_latency_ms": 15, "active_databases": 1 }

GET /v1/projects/:projectID/metrics/trend?days=7   权限 DatabaseRead
→ { "points": [ { "date": "9/15", "requests": 100, "errors": 2 } ] }
```

- 挂在 `/v1/projects/:projectID` 组下可复用现有认证 + project 中间件，且无需改 vite 代理。
- `trend` 建议返回对象包裹（`{points:[...]}`）而非裸数组，与 `collections` / `databases` / `providers` 风格一致；前端 adapter 层解包。
- 实现可读 `observability.Metrics` 的 Prometheus counter，或聚合访问日志。

### 6.2 文档 API 显式指定数据库（P1，多库场景正确性）

当前 `:p/data/*` 隐式取项目第一个库（§3.3），多库项目下语义错误。建议二选一：
- 路径改为 `:p/databases/:databaseID/data/collections/...`；或
- 保留现路径，增加可选 query `?database_id=xxx`，缺省仍取第一个（向后兼容）。

### 6.3 错误码一致性修正（P1，前端错误提示质量）

以下场景当前返回 500 `internal_error`，建议映射为 4xx：
- `llm/chat`、`llm/stream` 的 `messages` 为空或 body 非法（`llm_handler.go` 用 `errors.New` 而非 `NewAPIError`）
- `databases` 列表 `limit` 参数越界（`parseListParams` 返回 `fmt.Errorf`）
- 文档 API 在项目无库时的 `no database configured for project`（建议 409 或 404 + 专用 code）
- 各 handler 的 `project context missing`（理论不可达，但同样未映射）

### 6.4 SSE 错误帧（P2，流式失败可感知）

`llm/stream` 中途失败时仅补发 `{"type":"end"}`，前端无法区分正常结束与失败。建议增加 `{"type":"error","message":"..."}` 帧。

### 6.5 审计查询实装（P2）

`GET :p/audit` 为占位实现，恒返回空数组。需接入 `AuditService` 实际存储查询后前端才有意义。


### 6.7 设置页厂商凭证与项目 LLM 默认值（P1，前端设置页 / AiChat）

见 §3.9 与 `ui-settings-chat-plan.md`。建议实现顺序：

1. `GET/PUT :p/llm/settings`（默认 provider/model/采样参数）
2. `GET :p/llm/provider-catalog`（可先写死与前端预置表一致的 JSON）
3. `GET/PUT/DELETE :p/llm/provider-configs...`（凭证只写不读，落 `CredentialRef`）
4. `chat`/`stream` 接受可选 `provider`，并做配额/审计打点

安全约束：

- 响应与日志禁止打印 API Key；错误信息不得回显密钥片段（`credential_hint` 最多保留前后极少字符）。
- 不在 DevMode 种子或仓库示例中提交真实 key。

### 6.6 不建议本轮前端接入的能力

- **FaaS**：后端无对应模块，属未立项功能。
- **实时日志 WebSocket**：后端无 WS 实现，需独立 plan（涉及日志管道、连接管理、鉴权）。
