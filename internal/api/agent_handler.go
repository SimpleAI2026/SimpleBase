// 文件说明：agent_handler.go — cloudAgentHandler 结构体、构造、能力探测与跨资源辅助
//
// 用法
//
//	newCloudAgentHandler(deps)                 // routes_agent.go 装配；只做依赖注入，不再创建任何长期状态
//	h.caps()                                   // 实例能力快照（LLM / MCP / CLI 技能 / 沙盒），供 HTTP 层做 503 判定
//	h.canWritePrincipal(p)                     // SEC-04 的破坏性操作判定
//	h.resolveAgentTag(ctx, projectID, id)      // 转发到 agentrun.ResolveAgentTag（预置群默认值只有一份实现）
//
// 调用架构
//
//	[上游] internal/app（经 Dependencies.AgentRun 注入应用服务）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/agentrun, internal/agentbridge, internal/auth, internal/systemdb
//	[同包] agent_run_handler.go, agent_approval_handler.go, collab_handler.go 等
//
// 主要符号
//
//	type cloudAgentHandler struct {…}
//	newCloudAgentHandler(deps Dependencies) *cloudAgentHandler
//	(h *cloudAgentHandler) caps() agentrun.Capabilities
//	(h *cloudAgentHandler) skillsCLI() bool
//	(h *cloudAgentHandler) sandboxAvailable() bool
//	(h *cloudAgentHandler) resolveAgentTag(ctx context.Context, projectID, id string) (systemdb.AgentTag, error)
//	(h *cloudAgentHandler) canWritePrincipal(p auth.Principal) bool
//
// 边界（internal-ddd-hexagonal-implementation-plan §3.3 / §3.6）：run 与群轮次的编排、长期状态
// （事件环、确认表、群引擎）与生命周期全部在 internal/agentrun；这里只剩协议、读端口与授权判定。

package api

import (
	"context"

	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type cloudAgentHandler struct {
	store    *systemdb.Store
	svc      *agentrun.Service
	usage    UsageService
	audit    AuditService
	writable bool
	// llm 提供模型列表（BUG-07）；可为 nil（LLM 未启用）。
	llm LLMService
}

// newCloudAgentHandler 构造云 Agent handler：只注入依赖。事件环、确认表、群引擎与启动收敛
// 由 internal/agentrun.Service 持有（app 构造），router 不再回写运行时或延迟绑定 cron 执行器。
func newCloudAgentHandler(deps Dependencies) *cloudAgentHandler {
	return &cloudAgentHandler{
		store:    deps.System,
		svc:      deps.AgentRun,
		usage:    deps.Usage,
		audit:    deps.Audit,
		writable: deps.Config.Instance.Writable,
		llm:      deps.LLM,
	}
}

// caps 返回实例能力快照；未注入应用服务时按「什么都没配置」处理。
func (h *cloudAgentHandler) caps() agentrun.Capabilities {
	if h.svc == nil {
		return agentrun.Capabilities{}
	}
	return h.svc.Capabilities()
}

func (h *cloudAgentHandler) skillsCLI() bool        { return h.caps().SkillsCLI }
func (h *cloudAgentHandler) sandboxAvailable() bool { return h.caps().Sandbox }

// resolveAgentTag 按 ID 取标签（预置群恒存在）；规则归 agentrun，入口与群引擎共用同一份实现。
func (h *cloudAgentHandler) resolveAgentTag(ctx context.Context, projectID, id string) (systemdb.AgentTag, error) {
	return h.svc.ResolveAgentTag(ctx, projectID, id)
}

// canWritePrincipal 判断主体是否可执行破坏性 run 操作（SEC-04）：
// 登录态按角色 CanWrite（admin 只读拒绝），API Key 按 DatabaseWrite 位。
func (h *cloudAgentHandler) canWritePrincipal(p auth.Principal) bool {
	if p.UserID != "" {
		return p.Role.CanWrite()
	}
	return p.HasPermission(auth.DatabaseWrite)
}
