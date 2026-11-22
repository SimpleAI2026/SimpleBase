package agentbridge

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	agentskills "github.com/linkxzhou/SimpleBase/internal/agentbridge/agentskills"
)

// RuntimeProvider adapts SimpleBase resources to the generic agent runtime.
type RuntimeProvider struct {
	DB       DatabaseAccess
	Obj      ObjectAccess
	Logs     LogReader
	Settings SettingsReader
	Sandbox  Sandbox

	SkillsCLI      bool
	CLI            CLIRunner
	CLIPath        string
	APIBaseURL     string
	Issuer         DelegationTokens
	Confirm        cloudagent.ConfirmationWaiter
	ConfirmTimeout time.Duration
	DelegationTTL  time.Duration
	Policy         CommandClassifier
	ToolProtocol   string
	Logger         cloudagent.Logger
	TraceLog       bool

	mu     sync.Mutex
	tokens map[string]string
	gate   *callGate
}

func (p *RuntimeProvider) Prepare(ctx context.Context, req RunRequest, emit func(Event)) (cloudagent.RunPreparation, error) {
	rc, err := hostContext(req.Context)
	prep := cloudagent.RunPreparation{ModelScope: rc.ProjectID, Cleanup: func() {}}
	if err != nil {
		return prep, err
	}
	prep.Model = p.resolveModelName(ctx, req, rc)
	prep.Instruction = p.buildInstruction(ctx, req, rc)

	var cleanups []func()
	prep.Cleanup = func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	if p.SkillsCLI {
		if p.Issuer != nil {
			principal := rc.Principal
			if !req.Agent.Writable {
				principal = stripWritePermissions(principal)
			}
			token, err := p.Issuer.Issue(principal, rc.ProjectID, req.RunID, p.delegationTTL())
			if err != nil {
				return prep, err
			}
			p.rememberToken(req.RunID, token)
			cleanups = append(cleanups,
				func() { p.forgetToken(req.RunID) },
				func() { p.Issuer.RevokeRun(req.RunID) },
			)
		}
		cleanups = append(cleanups, func() { p.ensureGate().dropRun(req.RunID) })
		prep.Tools, err = p.buildSkillTools(req)
		prep.OnToolCall = func(callID string) { p.ensureGate().release(req.RunID, callID) }
	} else {
		prep.Tools, err = buildTools(req.Agent.ToolIDs, toolDeps{DB: p.DB, Obj: p.Obj, Logs: p.Logs, Sandbox: p.Sandbox})
	}
	if err != nil {
		return prep, err
	}
	return prep, nil
}

func (p *RuntimeProvider) resolveModelName(ctx context.Context, req RunRequest, rc RunContext) string {
	model := strings.TrimSpace(req.Agent.ModelOverride)
	if model == "" && p.Settings != nil {
		if settings, err := p.Settings.LLMSettings(ctx, rc.ProjectID); err == nil {
			model = strings.TrimSpace(settings.DefaultModel)
		}
	}
	return model
}

func (p *RuntimeProvider) buildInstruction(ctx context.Context, req RunRequest, rc RunContext) string {
	moduleTemplate := moduleTemplate(req.Agent.Module)
	if rc.PlanStage != "" {
		moduleTemplate += "\n\n" + PlanStagePrompt(rc.PlanStage)
	}
	parts := PromptParts{
		ModuleTemplate: moduleTemplate,
		AgentPrompt:    req.Agent.SystemPrompt,
		ProjectEnv: fmt.Sprintf("Project env:\n- project_id: %s\n- agent: %s (%s)\n- module: %s",
			rc.ProjectID, req.Agent.Name, req.Agent.ID, req.Agent.Module),
		Snapshot: p.buildSnapshot(ctx, req, rc),
	}
	if p.SkillsCLI {
		parts.SkillsCLI = true
		parts.Writable = req.Agent.Writable
		parts.SkillText = agentskills.Prompt(rc.Skills)
		if len(rc.Skills) > 0 {
			parts.ProjectEnv += "\n- skills: " + strings.Join(rc.Skills, ",")
		}
		if rc.PlanStage != "" {
			parts.ProjectEnv += "\n- plan_stage: " + string(rc.PlanStage)
			if len(rc.ReadOnlySkills) > 0 {
				parts.ProjectEnv += "\n- readonly_skills: " + strings.Join(rc.ReadOnlySkills, ",")
			}
		}
		if !slices.Contains(rc.Skills, ModuleDatabase) && req.Agent.Module != ModuleDatabase {
			parts.Snapshot = ""
		}
	}
	return AssembleInstruction(parts)
}

func (p *RuntimeProvider) buildSnapshot(ctx context.Context, req RunRequest, rc RunContext) string {
	var b strings.Builder
	b.WriteString("Readonly snapshot (truncated, no secrets):\n")
	switch req.Agent.Module {
	case ModuleDatabase:
		if p.DB == nil {
			b.WriteString("- databases: unavailable\n")
			break
		}
		list, err := p.DB.ListDatabases(ctx, rc.Principal, rc.ProjectID)
		if err != nil {
			b.WriteString("- databases: " + err.Error() + "\n")
			break
		}
		fmt.Fprintf(&b, "- databases (%d):\n", len(list))
		for i, database := range list {
			if i >= 20 {
				b.WriteString("  …\n")
				break
			}
			fmt.Fprintf(&b, "  - %s %s status=%s\n", database.Name, database.ID, database.Status)
		}
	case ModuleS3:
		if p.Obj == nil {
			b.WriteString("- objects: unavailable\n")
			break
		}
		list, err := p.Obj.ListObjects(ctx, rc.ProjectID, "", 20)
		if err != nil {
			b.WriteString("- objects: " + err.Error() + "\n")
			break
		}
		fmt.Fprintf(&b, "- objects (%d shown):\n", len(list))
		for _, object := range list {
			fmt.Fprintf(&b, "  - %s size=%d\n", object.Key, object.Size)
		}
	case ModuleLogs:
		if p.Logs == nil {
			b.WriteString("- logs: unavailable\n")
			break
		}
		if stats, err := p.Logs.LevelStats(ctx, rc.ProjectID); err == nil {
			b.WriteString("- log levels: ")
			for i, stat := range stats {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "%s=%d", stat.Level, stat.Count)
			}
			b.WriteString("\n")
		}
	default:
		b.WriteString("- no module snapshot\n")
	}
	return b.String()
}

func (p *RuntimeProvider) trace() tracer {
	return cloudagent.NewTracer(p.Logger, p.TraceLog)
}

func (p *RuntimeProvider) confirmTimeout() time.Duration {
	if p.ConfirmTimeout > 0 {
		return p.ConfirmTimeout
	}
	return 2 * time.Minute
}

func (p *RuntimeProvider) delegationTTL() time.Duration {
	if p.DelegationTTL > 0 {
		return p.DelegationTTL
	}
	return 15 * time.Minute
}

func (p *RuntimeProvider) rememberToken(runID, token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tokens == nil {
		p.tokens = map[string]string{}
	}
	p.tokens[runID] = token
}

func (p *RuntimeProvider) forgetToken(runID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.tokens, runID)
}

func (p *RuntimeProvider) tokenFor(runID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tokens[runID]
}
