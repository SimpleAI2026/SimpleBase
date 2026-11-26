# Design

## 位置与依赖

```text
gocloudagent/mcp/server.go      go-sdk Server 封装（唯一 import SDK 的模块）
internal/agentbridge/mcp_builtin.go  宿主侧注册表、工具实现与进程内传输
internal/api/mcp_dispatch.go    HTTP 入口：/mcp 与 /mcp/builtin/:name
```

根模块不直接 import go-sdk（`go.mod` 中为 indirect）；宿主只使用自有类型。

## 鉴权与范围

- 复用现有 Bearer 解析：API Key → `auth.Principal`；委托令牌 → 降权后的 Principal。
- 每个工具在执行前校验项目一致性与权限位（读/写），权限判定只走 `HasPermission`。
- 端点不提供 OAuth；未授权返回 401，越权返回 403。

## 工具集

- 只读：`list_databases`、`list_collections`、`query_sql`（只读守卫）、`get_document`、`list_documents`、`kv_get`、`kv_scan`、`list_objects`、`list_functions`、`list_cron_jobs`。
- 写入：`execute_sql`、`create_document`、`update_document`、`delete_document`、`kv_set`、`kv_del`、`invoke_function`、`run_cron_job`。
- 工具名加 `builtin__` 前缀，避免与外部 Server 冲突；描述写清权限与破坏性。

## 进程内传输

`gocloudagent/mcp` 用 go-sdk 的 `NewInMemoryTransports` 建立 server/client 会话；`agentbridge` 在装配时把内置 Server 注册为 `Runtime` 可用工具源，不经过 `guardDialContext`。外部 Server 仍走 HTTP 连接器。

## 注册表与启停

`sys_settings_project` 存 `mcp.builtin.<name>.enabled`；默认内置项目 Server 关闭，用户显式开启。`/mcp/builtin/:name` 为未来内置 Server 预留，未知名称返回 404。

## 回滚

关闭项目设置即可下线工具；HTTP 路由可独立回退，不影响外部 MCP 功能。
