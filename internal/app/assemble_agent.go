// 文件说明：云 Agent 装配阶段：LLM 服务、实例 provider、cloudagent Runtime（原 app.go 拆出）
package app

import (
	"os"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/usage"
	"go.uber.org/zap"
)

// assembleLLMService 装配 LLM Gateway 服务。resolver 链优先级：
// 项目动态凭据（sys_llm_settings）→ catalog CredentialRef → 实例 YAML
// 供应商回退。仅读取已装配的 a.catalog / a.systemStore / a.usageSvc /
// a.logger，不重新创建任何依赖。
func (a *App) assembleLLMService(cfg config.Config) llmgateway.Service {
	// 设置页把模型连接（API Key / Base URL）写进 sys_llm_settings。每次请求现读该行，
	// 保存后无需重启。catalog CredentialRef 与 YAML 实例供应商仅作回退。
	var resolver llmgateway.ProviderResolver = llmgateway.NewCatalogResolver(a.catalog, nil)
	resolver = llmgateway.NewFirstUsableResolver(
		llmgateway.NewProjectCredResolver(llmgateway.NewSystemCredSource(a.systemStore)),
		resolver,
	)
	if inst := instanceLLMProviders(cfg.LLM.Providers); len(inst) > 0 {
		resolver = llmgateway.NewFallbackResolver(resolver, inst)
	}
	// trace recorder 从 ctx 解 request_id 后缓冲进 sys_llm_traces（planv6.5）。
	return llmgateway.NewService(resolver, usage.NewLLMRecorder(a.usageSvc), newLLMTraceRecorder(a.systemStore), a.logger)
}

// cloudAgentRuntime 装配云 Agent 运行时；沙盒按配置注入（cloud-agent-sandbox-plan §3.1）。
// confirms 是工具审批的进程内等待表（app 先构造），这里直接写进运行时与宿主 Provider——
// router 不再在构造期回写共享运行时（internal-ddd-hexagonal-implementation-plan P3）。
func (a *App) cloudAgentRuntime(confirms *agentrun.ConfirmHub) *cloudagent.Runtime {
	if a.systemStore == nil {
		return nil
	}
	cliPath := strings.TrimSpace(a.cfg.Agent.CLIPath)
	if cliPath == "" {
		if exe, err := os.Executable(); err == nil {
			cliPath = exe
		}
	}
	if a.cfg.Agent.SkillsCLI && a.logger != nil {
		if failure := cloudagent.ValidateExecutable(cliPath); failure != "" {
			a.logger.Warn("cloud agent CLI unavailable", zap.String("reason", failure))
		} else {
			a.logger.Info("cloud agent CLI ready", zap.String("path", cliPath))
		}
	}
	reg := auth.NewDelegationRegistry()
	a.delegations = reg
	provider := &agentbridge.RuntimeProvider{
		DB:             agentbridge.NewCloudAgentDB(a.catalog, dbbridge.RegistryFrom(a.registry)),
		Obj:            agentbridge.NewCloudAgentObj(a.fileStore, a.systemStore),
		Logs:           agentbridge.NewLogAccess(a.systemStore),
		Settings:       agentbridge.NewSettingsAccess(a.systemStore),
		SkillsCLI:      a.cfg.Agent.SkillsCLI,
		CLIPath:        cliPath,
		CLI:            agentbridge.SubprocessCLI{Path: cliPath},
		APIBaseURL:     loopbackBase(a.cfg.HTTP.Address),
		ConfirmTimeout: a.cfg.Agent.EffectiveConfirmTimeout(),
		DelegationTTL:  a.cfg.Agent.EffectiveDelegationTTL(),
		Issuer:         agentbridge.NewDelegationIssuer(a.cfg.Auth.APIKeyHashSecret, reg),
		Policy:         agentbridge.NewCommandPolicy(),
		ToolProtocol:   a.cfg.LLM.EffectiveAgentToolProtocol(),
		Logger:         a.logger,
		TraceLog:       a.cfg.Agent.TraceLog,
		Confirm:        confirms,
	}
	if a.sandboxMgr != nil && a.sandboxMgr.Available() {
		provider.Sandbox = &sandboxAdapter{m: a.sandboxMgr}
	}
	rt := &cloudagent.Runtime{
		LLM: agentbridge.NewCloudAgentLLM(a.llmSvc), Provider: provider,
		MaxIterations: a.cfg.LLM.EffectiveAgentMaxIterations(), RunTimeout: a.cfg.LLM.EffectiveAgentRunTimeout(),
		ToolProtocol: a.cfg.LLM.EffectiveAgentToolProtocol(), ConfirmTimeout: a.cfg.Agent.EffectiveConfirmTimeout(),
		Logger: a.logger, TraceLog: a.cfg.Agent.TraceLog, Confirm: confirms,
	}
	// MCP（planv6.2 §6-4）：默认关闭；关闭时不装配连接器，
	// Agent 保存 mcp: 引用会被 api 层拒绝，run 也不会装配 MCP 工具。
	// 内置 MCP（planv6.7 §3.6）开启时也要装配：云助手经进程内会话使用它。
	// 此时若外部 MCP 关闭，连接器只服务内置 Server。
	if a.cfg.Agent.MCP.Enabled || a.cfg.Agent.MCP.Builtin {
		rt.MCP = agentbridge.NewMCPConnector(a.systemStore, agentbridge.MCPOptions{
			AllowHTTP:       a.cfg.Agent.MCP.AllowHTTP,
			DisableExternal: !a.cfg.Agent.MCP.Enabled,
		})
		rt.MCPPolicy = agentbridge.NewMCPPolicy(a.systemStore)
	}
	return rt
}

// assembleAgentRun 构造 Agent 协作应用服务：固定助手会话的 run、协作群轮次、群定时、审批与事件环。
// 仓储经窄端口注入（*systemdb.Store 隐式满足）；配额 / 审计端口仅在对应服务存在时注入。
func (a *App) assembleAgentRun(runtime *cloudagent.Runtime, confirms *agentrun.ConfirmHub) *agentrun.Service {
	ports := agentrun.FromStore(a.systemStore)
	if a.usageSvc != nil {
		ports.Quota = a.usageSvc
	}
	if a.auditSvc != nil {
		ports.Audit = a.auditSvc
	}
	return agentrun.NewService(agentrun.Config{
		SkillsCLI: a.cfg.Agent.SkillsCLI,
		Sandbox:   a.sandboxMgr != nil && a.sandboxMgr.Available(),
	}, ports, runtime, confirms)
}

func instanceLLMProviders(in map[string]config.ProviderConfig) map[string]llmgateway.InstanceProvider {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]llmgateway.InstanceProvider, len(in))
	for name, p := range in {
		out[name] = llmgateway.InstanceProvider{
			APIKey:        p.APIKey,
			BaseURL:       p.BaseURL,
			DefaultModel:  p.DefaultModel,
			AllowedModels: p.AllowedModels,
			Timeout:       p.Timeout,
		}
	}
	return out
}
