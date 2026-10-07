---
title: HTTP API
order: 2
---

# 云沙盒 HTTP API

请求均使用 `Authorization: Bearer <API_KEY>`，基础路径 `/v1/projects/:projectID/sandboxes`。路由需要有效项目权限；`database:read` 可以查看资源和文件，`database:write` 可以创建、执行和修改。实例必须 `writable=true`；系统项目拒绝写操作。

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/capabilities` | read | 可用性、镜像列表与限制；关闭时仍返回 `available:false` |
| GET | `/` | read | 项目资源列表（创建时间倒序），可选 `status` / `source` / `limit`（1–100，默认 50）/ `cursor`；还有下一页时响应带 `next_cursor` |
| POST | `/` | write | 创建资源；请求头 `Idempotency-Key` 可选，值仅进程内存保存 |
| POST | `/run` | write | 一次性沙盒：写入文件、执行、删除；`keep:true` 可保留 |
| GET | `/:id?refresh=1` | read | 资源详情，`refresh=1` 刷新云端状态 |
| PATCH | `/:id` | write | 更改 `name`、`idle_timeout_s` |
| DELETE | `/:id` | write | 停止、删除 VM 并软删记录 |
| POST | `/:id/start` | write | 立即启动或重建过期工作区 |
| POST | `/:id/stop` | write | 停止 VM，保留元数据 |
| POST | `/:id/exec` | write | 同步执行命令 |
| GET | `/:id/files?path=/workspace` | read | 列出一层目录 |
| GET | `/:id/files/content?path=...` | read | 读文本文件 `{content, encoding, truncated}`；`Accept: application/octet-stream` 返回原始字节 |
| PUT | `/:id/files/content?path=...` | write | 写文件（JSON `{content, encoding}` 或原始字节） |
| DELETE | `/:id/files/content?path=...` | write | 删除文件 |

## 创建和执行

```json
{"name":"etl-check","image":"python:3.12-slim","cpus":1,"memory_mib":256,"network":"none","start":false}
```

创建返回 201 和 `id/status:"pending"`。执行请求：

```json
{"command":"python -V","timeout_s":30,"cwd":"/workspace"}
```

也可使用 `{"cmd":"python","args":["-V"]}` 不通过 shell 执行；两种方式二选一。`env` 最多 32 项，拒绝 `SIMPLEBASE_`、`MSB_`、`AWS_` 前缀。响应：

```json
{"exit_code":0,"stdout":"Python 3.12\n","stderr":"","timed_out":false,"stdout_truncated":false,"stderr_truncated":false,"duration_ms":420,"status":"running"}
```

`exit_code != 0` 或 `timed_out=true` 仍为 HTTP 200；传输故障才是 5xx。输出不写数据库，stdout / stderr 各有 `max_output_bytes` 截断限制。单个文件最大 `max_file_bytes`（默认 1 MiB），路径必须是 `/workspace` 内绝对路径。

## 列表分页

```http
GET /v1/projects/p1/sandboxes?limit=20
→ {"sandboxes":[...], "next_cursor":"<上一页最后一行 id>"}
GET /v1/projects/p1/sandboxes?limit=20&cursor=<next_cursor>
→ {"sandboxes":[...]}            # 无 next_cursor 表示最后一页
```

`cursor` 需与相同的 `status` / `source` 过滤一起使用；未知或属于其它项目的游标返回 400 `sandbox_invalid_spec`。JS SDK 用 `sandboxes.listPage({ cursor })`，Go SDK 用 `ListSandboxesPage(ctx, SandboxListOptions{Cursor: ...})`。

## 错误码

按项目隔离：在本项目路径下访问其它项目的沙盒 id 一律返回 404 `sandbox_not_found`（不暴露存在性）；用其它项目的 API Key 访问本项目路径返回 403 `cross_project_denied`。同一沙盒的操作串行，等待超过 5 秒返回 409 `sandbox_busy`。常见错误码：`sandbox_unavailable` (503)、`sandbox_not_found` (404)、`sandbox_name_conflict` (409)、`sandbox_limit_exceeded` (429)、`sandbox_invalid_spec` / `sandbox_invalid_path` (400)、`sandbox_file_too_large` (413)、`sandbox_busy` (409)、`sandbox_gone` (410)、`sandbox_backend_error` (502)。响应沿用 `{ "error": { "code": "...", "message": "...", "request_id": "..." } }`，不透传 SDK 内部路径或密钥。

`run` 请求示例参阅 [e2e](/docs/sandbox/e2e)。
