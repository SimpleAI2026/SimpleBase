// 文件说明：云 Agent 装配阶段：LLM 服务、实例 provider、cloudagent Runtime（原 app.go 拆出）
package app

import (
	"os"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/api"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/usage"
	"go.uber.org/zap"
)

// assembleLLMService 装配 LLM Gateway 服务（原 assembleDeps 尾部
// cfg.LLM.Enabled 连续块，B4 提取；resolver 链构造与优先级保持原样：
// 项目动态凭据（sys_llm_settings）→ catalog CredentialRef → 实例 YAML
// 供应商回退）。仅读取已装配的 a.catalog / a.systemStore / a.usageSvc /
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
	return llmgateway.NewService(resolver, usage.NewLLMRecorder(a.usageSvc), api.NewLLMTraceRecorder(a.systemStore), a.logger)
}

// cloudAgentRuntime 装配云 Agent 运行时；沙盒按配置注入（cloud-agent-sandbox-plan §3.1）。
func (a *App) cloudAgentRuntime() *cloudagent.Runtime {
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
		if failure := cloudagent.ValidateCLIPath(cliPath); failure != "" {
			a.logger.Warn("cloud agent CLI unavailable", zap.String("reason", failure))
		} else {
			a.logger.Info("cloud agent CLI ready", zap.String("path", cliPath))
		}
	}
	reg := auth.NewDelegationRegistry()
	a.delegations = reg
	rt := &cloudagent.Runtime{
		LLM:            api.NewCloudAgentLLM(api.NewLLMService(a.llmSvc)),
		DB:             api.NewCloudAgentDB(a.catalog, dbbridge.RegistryFrom(a.registry)),
		Obj:            api.NewCloudAgentObj(a.fileStore, a.systemStore),
		Logs:           agentbridge.NewLogAccess(a.systemStore),
		Settings:       agentbridge.NewSettingsAccess(a.systemStore),
		MaxIterations:  a.cfg.LLM.EffectiveAgentMaxIterations(),
		RunTimeout:     a.cfg.LLM.EffectiveAgentRunTimeout(),
		ToolProtocol:   a.cfg.LLM.EffectiveAgentToolProtocol(),
		SkillsCLI:      a.cfg.Agent.SkillsCLI,
		CLIPath:        cliPath,
		CLI:            cloudagent.SubprocessCLI{Path: cliPath},
		APIBaseURL:     loopbackBase(a.cfg.HTTP.Address),
		ConfirmTimeout: a.cfg.Agent.EffectiveConfirmTimeout(),
		DelegationTTL:  a.cfg.Agent.EffectiveDelegationTTL(),
		Issuer:         agentbridge.NewDelegationIssuer(a.cfg.Auth.APIKeyHashSecret, reg),
		// 命令只读/破坏性分类的唯一入口：规则在宿主（sbcli + sqlguard）。
		Policy: agentbridge.NewCommandPolicy(),
		// planv6.0 v2 §5：运行日志出口与内容层开关（默认关闭）。
		Logger:   a.logger,
		TraceLog: a.cfg.Agent.TraceLog,
	}
	if a.sandboxMgr != nil && a.sandboxMgr.Available() {
		rt.Sandbox = &sandboxAdapter{m: a.sandboxMgr}
	}
	// MCP（planv6.2 §6-4）：默认关闭；关闭时不装配连接器，
	// Agent 保存 mcp: 引用会被 api 层拒绝，run 也不会装配 MCP 工具。
	if a.cfg.Agent.MCP.Enabled {
		rt.MCP = agentbridge.NewMCPConnector(a.systemStore, agentbridge.MCPOptions{
			AllowHTTP: a.cfg.Agent.MCP.AllowHTTP,
		})
		rt.MCPPolicy = agentbridge.NewMCPPolicy(a.systemStore)
	}
	return rt
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
