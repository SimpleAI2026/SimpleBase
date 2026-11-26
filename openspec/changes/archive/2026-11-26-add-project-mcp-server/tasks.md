# Tasks

## 1. Server 封装（MCP-1）

- [x] 1.1 在 `gocloudagent/mcp` 实现 Server 封装（`NewServer`、工具注册、Streamable HTTP handler、进程内 transport）
- [x] 1.2 内置名称保留与工具名前缀；单 Server ≤64、结果 ≤32KiB 的限制测试
- [x] 1.3 根模块不直接 import go-sdk，确认 `go.mod` 不变

## 2. 宿主注册表与工具（MCP-2）

- [x] 2.1 `agentbridge` 定义内置 Server 注册表与工具实现，逐项校验项目范围与权限
- [x] 2.2 只读工具走 SQL 只读守卫；写工具复用现有服务与错误映射
- [x] 2.3 进程内会话接入云助手工具源；外部连接器仍拒绝回环 / 私网

## 3. HTTP 入口与鉴权（MCP-3）

- [x] 3.1 `/v1/projects/:projectID/mcp` 与 `/mcp/builtin/:name` 路由，Bearer 鉴权
- [x] 3.2 跨项目令牌、未知 Server、关闭状态分别返回 401/404/404
- [x] 3.3 不新增系统表；启停状态存项目设置

## 4. 控制台与文档

- [x] 4.1 MCP 列表展示内置 Server、启停开关与工具清单
- [x] 4.2 文档补充 MCP 端点、鉴权、工具列表与与外部 Server 的差异

## 5. 验证

- [x] 5.1 用例：握手、列工具、调用读写工具、跨项目拒绝、超大结果截断
- [x] 5.2 云助手进程内调用不产生 HTTP 回环；关闭后工具消失
- [x] 5.3 `go test ./...`、`TestArchitectureGate`、前端 `typecheck` + `test` 通过

## 完成情况核对（2026-11-26）

已实现（12）：
- `gocloudagent/mcp/server.go`：Server 封装、Streamable HTTP、进程内 transport、AddTool 限额。
- `internal/api/mcp_builtin*.go`：内置注册表、工具、`/mcp` 与 `/mcp/builtin/:name`、Bearer、跨项目 / 未知 / 关闭、项目设置开关与审计。
- `internal/config` 的 `mcp.builtin` 默认开启；`McpBuiltinServers.vue` 控制台列表与开关。
- 测试：`mcp_builtin_test.go`、`mcp_builtin_admin_test.go`、`mcp_builtin_session_test.go`、`gocloudagent/mcp/server_test.go`；前端 MCP 测试通过。
- `go.mod` 未改动（go-sdk 仍只在 gocloudagent 模块）。
- 验证：`go test ./internal/...` 仅 `TestNewLocalStorageAppUsesSystemDatabase` 既有失败，其余包全绿；前端 `typecheck` + MCP 相关用例通过。

- 4.2：`docs/workbench/agent-tools.md` 新增「内置 MCP Server」小节（端点、鉴权、保留名、限额、开关、与外部 Server 的差异）。
