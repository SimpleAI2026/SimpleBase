// 文件说明：mcp_test_connect.go — internal/api 的内部实现（newMCPTestConnect 等）
//
// 用法
//
//	同包调用：newMCPTestConnect() → routes_agent.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/mcp_test_connect.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] newMCPTestConnect ← routes_agent.go
//
// 主要符号
//
//	newMCPTestConnect(deps Dependencies) func(ctx context.Context, srv systemdb.MCPServer) ([]string, error)

package api

import (
	"context"

	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// newMCPTestConnect 构造连通性测试的闭包。
//
// 只在 agent.mcp.enabled=true 时装配（见 mountAgentRoutes）：关闭时 test 接口
// 返回 503，避免在未启用 SSRF 防护/凭据口径的实例上发起外连。
func newMCPTestConnect(deps Dependencies) func(ctx context.Context, srv systemdb.MCPServer) ([]string, error) {
	opts := agentbridge.MCPOptions{AllowHTTP: deps.Config.Agent.MCP.AllowHTTP}
	return func(ctx context.Context, srv systemdb.MCPServer) ([]string, error) {
		return agentbridge.TestMCPServer(ctx, srv, opts)
	}
}
