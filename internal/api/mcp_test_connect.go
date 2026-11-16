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
