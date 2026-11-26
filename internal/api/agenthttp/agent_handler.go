package agenthttp

import (
	"context"

	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

type CloudAgentHandler struct {
	store    *systemdb.Store
	svc      *agentrun.Service
	usage    httpkit.UsageService
	audit    httpkit.AuditService
	writable bool
	// llm 提供模型列表（BUG-07）；可为 nil（LLM 未启用）。
	llm httpkit.LLMService
}

// newCloudAgentHandler 构造云 Agent handler：只注入依赖。事件环、确认表、群引擎与启动收敛
// 由 internal/agentrun.Service 持有（app 构造），router 不再回写运行时或延迟绑定 cron 执行器。
// New 构造云 Agent handler：只注入依赖（窄参数，不接收整包 Dependencies）。
func New(store *systemdb.Store, svc *agentrun.Service, usage httpkit.UsageService, audit httpkit.AuditService, llm httpkit.LLMService, writable bool) *CloudAgentHandler {
	return &CloudAgentHandler{store: store, svc: svc, usage: usage, audit: audit, writable: writable, llm: llm}
}

// caps 返回实例能力快照；未注入应用服务时按「什么都没配置」处理。
func (h *CloudAgentHandler) caps() agentrun.Capabilities {
	if h.svc == nil {
		return agentrun.Capabilities{}
	}
	return h.svc.Capabilities()
}

func (h *CloudAgentHandler) skillsCLI() bool        { return h.caps().SkillsCLI }
func (h *CloudAgentHandler) sandboxAvailable() bool { return h.caps().Sandbox }

// resolveAgentTag 按 ID 取标签（预置群恒存在）；规则归 agentrun，入口与群引擎共用同一份实现。
func (h *CloudAgentHandler) resolveAgentTag(ctx context.Context, projectID, id string) (systemdb.AgentTag, error) {
	return h.svc.ResolveAgentTag(ctx, projectID, id)
}

// canWritePrincipal 判断主体是否可执行破坏性 run 操作（SEC-04）：
// 登录态按角色 CanWrite（admin 只读拒绝），API Key 按 DatabaseWrite 位。
func (h *CloudAgentHandler) canWritePrincipal(p auth.Principal) bool {
	if p.UserID != "" {
		return p.Role.CanWrite()
	}
	return p.HasPermission(auth.DatabaseWrite)
}
