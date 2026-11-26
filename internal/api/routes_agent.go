// 文件说明：routes_agent.go — 云 Agent / 协作 / 计划 / MCP Server 配置路由挂载
//
// 用法
//
//	mountAgentRoutes(p, deps, g, bm)  // 调用方保证 deps.System != nil
//
// 调用架构
//
//	NewRouter（router.go）→ mountV1Routes → mountSystemRoutes → mountAgentRoutes
//	权限闸门统一经 routeGuards（routes_guards.go），禁止在路由块里直接写 auth.Require。
//
// 主要符号
//
//	func mountAgentRoutes(p *echo.Group, deps Dependencies, g routeGuards, bm *builtinMCPHandler)

package api

import (
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/api/agenthttp"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// mountAgentRoutes 挂载云 Agent + 定时执行路由块。
// 全部 require（无 delegable）：委托令牌默认拒绝（planv6.0 D1）。
// POST /agent-threads/:threadID/runs 保持 DatabaseRead + g.writeGate，
// 不得改成 DatabaseWrite（创建 run 属于低门槛操作，SEC-04 只收紧
// cancel/confirm）。调用方保证 deps.System != nil。
func mountAgentRoutes(p *echo.Group, deps Dependencies, g routeGuards, bm *builtinMCPHandler) {
	ah := agenthttp.New(deps.System, deps.AgentRun, deps.Usage, deps.Audit, deps.LLM, deps.Config.Instance.Writable)
	p.GET("/agents/modules", ah.ListModules, g.require(auth.DatabaseRead))
	p.GET("/agents", ah.ListAgents, g.require(auth.DatabaseRead))
	p.GET("/agents/models", ah.ListAgentModels, g.require(auth.DatabaseRead))
	p.POST("/agents", ah.CreateAgent, g.require(auth.DatabaseWrite), g.writeGate)
	p.GET("/agents/:agentID", ah.GetAgent, g.require(auth.DatabaseRead))
	p.PATCH("/agents/:agentID", ah.PatchAgent, g.require(auth.DatabaseWrite), g.writeGate)
	p.DELETE("/agents/:agentID", ah.DeleteAgent, g.require(auth.DatabaseWrite), g.writeGate)
	p.GET("/agent-threads", ah.ListThreads, g.require(auth.DatabaseRead))
	p.POST("/agent-threads", ah.CreateThread, g.require(auth.DatabaseWrite), g.writeGate)
	// planv6.7 AGC-NAV：工作台标签 CRUD。
	p.GET("/agent-tags", ah.ListAgentTags, g.require(auth.DatabaseRead))
	p.POST("/agent-tags", ah.CreateAgentTag, g.require(auth.DatabaseWrite), g.writeGate)
	p.PATCH("/agent-tags/:tagID", ah.PatchAgentTag, g.require(auth.DatabaseWrite), g.writeGate)
	p.DELETE("/agent-tags/:tagID", ah.DeleteAgentTag, g.require(auth.DatabaseWrite), g.writeGate)
	// planv6.7 v2：群成员与告警/统计输入入口。
	p.POST("/agent-tags/:tagID/members", ah.AddGroupMembers, g.require(auth.DatabaseWrite), g.writeGate)
	p.DELETE("/agent-tags/:tagID/members/:agentID", ah.RemoveGroupMember, g.require(auth.DatabaseWrite), g.writeGate)
	p.POST("/agent-tags/:tagID/events", ah.GroupEvent, g.require(auth.DatabaseRead), g.writeGate)
	p.GET("/agent-threads/:threadID", ah.GetThread, g.require(auth.DatabaseRead))
	p.PATCH("/agent-threads/:threadID", ah.PatchThread, g.require(auth.DatabaseWrite), g.writeGate)
	p.DELETE("/agent-threads/:threadID", ah.DeleteThread, g.require(auth.DatabaseWrite), g.writeGate)
	p.GET("/agent-threads/:threadID/messages", ah.ListMessages, g.require(auth.DatabaseRead))
	p.GET("/agent-threads/:threadID/runs", ah.ListThreadRuns, g.require(auth.DatabaseRead))
	p.POST("/agent-threads/:threadID/runs", ah.CreateRun, g.require(auth.DatabaseRead), g.writeGate)
	// planv6.7 v2：协作轮次。发起一轮与创建 run 同权限口径（低门槛 + g.writeGate）；
	// 取消/重试/回复在 handler 内再校验主体（破坏性动作）。
	p.GET("/agent-threads/:threadID/turns", ah.ListThreadTurns, g.require(auth.DatabaseRead))
	p.POST("/agent-threads/:threadID/turns", ah.CreateTurn, g.require(auth.DatabaseRead), g.writeGate)
	p.GET("/agent-turns/:turnID", ah.GetTurn, g.require(auth.DatabaseRead))
	p.GET("/agent-turns/:turnID/events", ah.TurnEvents, g.require(auth.DatabaseRead))
	p.POST("/agent-turns/:turnID/plan-confirmations", ah.ConfirmTurnPlan, g.require(auth.DatabaseWrite), g.writeGate)
	p.POST("/agent-turns/:turnID/actions", ah.TurnAction, g.require(auth.DatabaseWrite), g.writeGate)
	p.POST("/agent-tasks/:taskID/replies", ah.ReplyHumanTask, g.require(auth.DatabaseRead), g.writeGate)
	p.GET("/agent-human-tasks", ah.ListHumanTasks, g.require(auth.DatabaseRead))
	p.GET("/agent-workspace-capabilities", ah.WorkspaceCapabilities, g.require(auth.DatabaseRead))
	// SEC-04：取消/确认是破坏性操作（终止他人 run / 放行工具执行），
	// 权限从 DatabaseRead 收紧为 DatabaseWrite；handler 内再校验主体。
	p.GET("/agent-runs/active", ah.ListActiveRuns, g.require(auth.DatabaseRead))
	// 审批队列：跨 run 的待办列表（侧栏角标 + 审批页）。
	p.GET("/agent-approvals", ah.ListApprovals, g.require(auth.DatabaseRead))

	// MCP Server 配置（planv6.2 §6-2）：凭据只单向写，响应只回 has_credential。
	mh := &mcpServerHandler{
		store: deps.System, limit: deps.Config.Agent.MCP.EffectiveMaxServersPerProject(),
		builtin: bm, audit: deps.Audit,
		publicURL: deps.Config.HTTP.PublicURL, publicOverride: deps.Config.Sites.PublicBaseURL,
	}
	if deps.Config.Agent.MCP.Enabled {
		mh.testConnect = newMCPTestConnect(deps)
	}
	p.GET("/mcp/servers", mh.List, g.require(auth.DatabaseRead))
	p.POST("/mcp/servers", mh.Create, g.require(auth.DatabaseWrite), g.writeGate)
	p.PATCH("/mcp/servers/:serverID", mh.Update, g.require(auth.DatabaseWrite), g.writeGate)
	p.DELETE("/mcp/servers/:serverID", mh.Delete, g.require(auth.DatabaseWrite), g.writeGate)
	p.POST("/mcp/servers/:serverID/test", mh.Test, g.require(auth.DatabaseWrite))
	if bm != nil {
		p.PUT("/mcp/builtin/:name/state", mh.SetState, g.require(auth.DatabaseWrite), g.writeGate)
		p.POST("/mcp/builtin/:name/test", mh.TestBuiltin, g.require(auth.DatabaseWrite))
	}
	p.POST("/agent-runs/:runID/cancel", ah.CancelRun, g.require(auth.DatabaseWrite), g.writeGate)
	p.POST("/agent-runs/:runID/confirmations/:callID", ah.ConfirmRun, g.require(auth.DatabaseWrite), g.writeGate)
	p.GET("/agent-runs/:runID/events", ah.ReplayEvents, g.require(auth.DatabaseRead))
}
