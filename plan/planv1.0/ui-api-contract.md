> **注意（2026-09-16）**：本文为 v0.1 早期契约，现行权威文档为 `plan/planv2.0/proto-http.md`。设置页 / 厂商凭证 / AiChat 见 `plan/planv2.0/ui-settings-chat-plan.md` 与 proto-http §3.9（后端未实现）。

# UI ↔ Backend API 对接契约

> 状态：前端已就绪，等待后端按契约实现
> 适用版本：SimpleBase UI v0.1+
> 基础路径：`/api`（生产可通过 `VITE_API_BASE_URL` 覆盖）

## 一、约定的后端接口（UI 期望）

| Method | Path | 说明 | 响应 |
|---|---|---|---|
| GET  | `/metrics/summary` | 总体指标 | `{ totalRequests, errorRate, avgLatencyMs, activeFunctions }` |
| GET  | `/metrics/trend`   | 近 7 天趋势 | `[{ date:'M/D', requests, errors }, ...]` |
| GET  | `/db/collections` | 集合列表 | `string[]` |
| GET  | `/db/collections/:c` | 文档列表 | `[{ id, ... }, ...]` |
| POST | `/db/collections/:c` | 新增文档 | `{ id, ... }` |
| DELETE | `/db/collections/:c/:id` | 删除文档 | `{ ok: true }` |
| GET  | `/v1/projects/:projectId/s3/objects?prefix=` | 对象列表 | `[{ key, size, lastModified }, ...]` |
| GET  | `/v1/projects/:projectId/s3/presign?key=` | 预签名 | `{ url }` |
| DELETE | `/v1/projects/:projectId/s3/objects?key=` | 删除对象 | `{ ok: true }` |
| POST | `/v1/projects/:projectId/s3/objects` (multipart: key, file) | 上传 | `{ key, size, lastModified }` |
| GET  | `/projects` | 项目列表 | `[{ id, name, desc, createdAt }, ...]` |
| POST | `/projects` | 新建项目 | `{ id, name, desc, createdAt }` |
| PUT  | `/projects/:id` | 更新项目 | `{ id, name, desc, createdAt }` |
| GET  | `/faas/functions` | 函数列表 | `[{ name, version, runtime, updatedAt }, ...]` |
| POST | `/faas/deploy` (multipart: name, file) | 部署函数 | `{ name, version, ... }` |
| POST | `/faas/invoke/:name` | 调用函数 | `{ ... }`（任意结构） |
| GET  | `/v1/projects/:projectId/llm/providers` | 可用供应商列表 | `{ providers: ['openai', ...] }` |
| POST | `/v1/projects/:projectId/llm/chat` | 非流式对话 | `{ content, usage, model, provider, finish_reason }` |
| POST | `/v1/projects/:projectId/llm/stream` | 流式对话（SSE） | `data: {...}` 逐帧推送，结束帧 `{"type":"end"}` |

LLM 请求体（chat/stream 相同）：

```json
{ "model": "gpt-4o", "messages": [{ "role": "user", "content": "..." }], "max_tokens": 1024, "temperature": 0.7 }
```

- `model` / `max_tokens` / `temperature` 均可选；未指定时由项目默认供应商配置决定。
- SSE 帧兼容三种 chunk 结构：`{delta}`、`{content}`、OpenAI 风格 `{choices:[{delta:{content}}]}`。
- 以上 LLM 路径与后端 `internal/api/llm_handler.go` 实现一一对应（非约定待实现）。

S3 对象存储路径说明：
- 路由挂在 `/v1/projects/:projectId/s3/*` 下，与其他业务路由一致，通过 API Key 认证 + project context 中间件。
- **项目隔离**：handler 层自动把 `{projectId}/` 拼到用户 key 前面作为物理前缀，用户可见的 key 不含项目段。
- 后端实现：`internal/objectstore/filestore.go`（`FileStore` 接口 + S3 实现 + DevMode 内存实现），`internal/api/s3_handler.go`（HTTP handler）。
- key 校验：`objectstore.ValidateFileKey` 拒绝空 key、绝对路径、`..` 穿越、NUL 字符、反斜杠、超长 key；校验失败返回 400 `invalid_file_key`。

WS：`ws://<host>/ws/logs`，按行推送明文日志。

## 二、日志格式兼容

UI 会按以下顺序识别级别（任一命中即高亮）：

1. `[ERROR]` / `[WARN]` —— mock 默认格式
2. ` ERROR ` / ` WARN ` —— 全大写单词
3. `level=error` / `level=warn` —— 结构化字段

推荐后端统一输出：`[LEVEL] message`，保持简单。

## 三、错误约定

任意业务错误返回 `{ message: 'xxx' }`，HTTP 状态码可任意。UI 已统一从 `response.data.message` 提取错误信息。

## 四、UI 实现位置

```
ui/src/services/
├── types.ts      # 接口契约类型（与上表一一对应）
├── http-api.ts   # 真实后端实现（按上表拼路径）
├── mock.js       # 离线开发 mock
└── api.ts        # 入口：VITE_USE_MOCK 自动切换
```