<!-- status: completed -->
<!-- progress: 2026-07-27 已完成 UI 真实 API 对接、数据管理文档 CRUD、移动端适配与本地开发启动规范。 -->

# Plan 11：UI 与后端接口联调实现总结

## 目标

将管理端 UI 从以 Mock 为主的演示形态切换为可调用真实后端 API 的开发形态，补齐数据管理的集合与 JSON 文档操作，并确保前端构建产物可由后端内嵌提供。

## 已实现功能

### 1. API 基础路径、认证与错误处理

- 后端业务路由统一使用 `/v1/projects/:projectID/...`，不使用 `/api` 前缀。
- UI 的 `VITE_API_BASE_URL` 默认为空，浏览器直接请求相对路径；Vite 开发服务器将 `/v1` 代理至 `http://localhost:8080`。
- `ui/src/services/http.ts` 自动添加 `Authorization: Bearer <apiKey>` 请求头：优先读取 `localStorage.sb_api_key`，开发默认值为 `sb_live_dev_key_12345`。
- 响应拦截器拒绝 `text/html`，避免未匹配 API 被 SPA fallback 返回 HTML 后流入页面数据模型。
- 后端业务错误采用 `{ "error": { "code", "message", "request_id" } }`；UI 已适配该错误消息结构。

### 2. DevMode 种子与数据库初始化

DevMode 使用内存 SQLite，每次启动自动创建：

- 租户：`00000000-0000-0000-0000-000000000001`
- 项目：`proj-01`
- API Key：`sb_live_dev_key_12345`
- 默认逻辑数据库：`default`

该初始化仅用于本地开发。进程重启后内存中的集合和文档会被清空。

### 3. 数据管理：集合与 JSON 文档

后端新增 `internal/api/data_handler.go`，通过既有 Catalog、Registry 和 SQL 租约访问项目默认数据库。集合名必须匹配：`^[A-Za-z][A-Za-z0-9_]{0,62}$`。

| Method | Path | 权限 | 功能 |
| --- | --- | --- | --- |
| GET | `/v1/projects/:projectID/data/collections` | `database:read` | 列出用户集合 |
| POST | `/v1/projects/:projectID/data/collections` | `database:write` | 创建集合，body: `{ "name": "users" }` |
| GET | `/v1/projects/:projectID/data/collections/:collection` | `database:read` | 列出集合中的 JSON 文档 |
| POST | `/v1/projects/:projectID/data/collections/:collection/documents` | `database:write` | 新增 JSON 文档；未提供 `id` 时生成 UUID |
| PUT | `/v1/projects/:projectID/data/collections/:collection/documents/:id` | `database:write` | 按 ID 覆盖更新指定文档；不存在返回 404 |
| DELETE | `/v1/projects/:projectID/data/collections/:collection/documents/:id` | `database:write` | 删除指定文档 |

数据以 SQLite 表的 `id`、JSON `data` 和 `created_at` 三列存储；返回给 UI 时 JSON 字段会展开，并附带 `id`。

### 4. 数据管理 UI

`ui/src/pages/DataManager.vue` 已对接真实接口，并提供：

- 选择集合、刷新集合文档；
- 新建集合；
- 新增 JSON 文档与实时 JSON 格式校验；
- 表格展示文档 JSON；
- 编辑指定文档：点击“编辑”后载入完整 JSON，保存时调用 `PUT`；
- 删除文档二次确认。

`ui/.env.development` 已固定为 `VITE_USE_MOCK=false`。`mock.js` 仍保留与真实 `Api` 接口一致的实现，仅供显式切换时使用。

### 5. UI 布局与功能清理

- 移动端（≤768px）将侧边栏改为抽屉导航，Header 提供菜单按钮；工具栏、表格、聊天输入区等已适配窄屏。
- 移除了独立“项目管理”页面、前端路由、菜单项、Mock CRUD 和类型定义。
- 保留 `projectID` 作为后端资源隔离边界；S3、LLM、数据管理等业务 API 仍须使用它，不能移除。

### 6. S3 与 LLM 已对接接口

| 功能 | 路径 |
| --- | --- |
| S3 对象列表/上传/删除/预签名 | `/v1/projects/:projectID/s3/*` |
| LLM provider 列表、非流式对话、SSE 流式对话 | `/v1/projects/:projectID/llm/*` |

上传请求体限制由配置 `limits.max_request_bytes` 控制，当前开发配置为 `10485760`（10 MiB）。

## 前端产物与服务启动

后端使用 `go:embed internal/web/dist` 提供 SPA 静态资源。更新 UI 后必须执行：

```bash
cd ui && npm run build
cd .. && cp -R ui/dist/. internal/web/dist/
go build -o simplebased ./cmd/simplebased
```

默认服务是前台、非 daemon 运行：

```bash
./simplebased
```

按 `Ctrl+C` 发送 `SIGINT`，`RunWithSignal` 会调用优雅关闭流程。不要在日常启动命令末尾添加 `&`，否则终端无法使用 `Ctrl+C` 管理该进程。

## 验证记录

- `go test ./internal/api ./internal/app` 通过。
- `npm run build` 通过。
- 已实测：创建集合 → 新增文档 → 按 ID 更新文档 → 查询文档，更新后的字段可正确返回。

## 后续边界

- 当前数据管理选择项目下的第一个逻辑数据库；多数据库选择与切换不在本阶段范围内。
- DevMode 数据为内存态，不提供持久化保障。
- Dashboard、FaaS、日志等页面尚未全部具备对应真实后端业务 API；缺失接口不得依赖 SPA fallback 作为数据响应。
