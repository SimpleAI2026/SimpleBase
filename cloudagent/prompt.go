package cloudagent

import (
	"strings"

	"github.com/linkxzhou/SimpleBase/cloudagent/agentcore"
)

const (
	maxPromptSection = 4000
	maxHistoryChars  = 12000
	maxUserChars     = 8000
)

// PromptParts is the ordered assembly input. Secrets must never be placed in these fields.
type PromptParts struct {
	ModuleTemplate string
	AgentPrompt    string
	ProjectEnv     string
	Snapshot       string
	// SkillsCLI 为真时允许通过 CLI 写用户资源，并附加 SkillText。
	SkillsCLI bool
	SkillText string
	// Writable 决定能力声明是 readonly 还是 writable（planv6.0 v2 §3.5）：
	// 此前模块提示词硬编码「Tools are readonly」，与 CLI 模式的实际写能力矛盾。
	Writable bool
}

const platformSkillsPrompt = `You are SimpleBase Cloud Agent, a project-scoped assistant.
Rules:
- Stay in the current project. Never guess another tenant or project.
- Writes go only through the simplebase tool, and only for skills loaded this turn.
- Never invent rows, object keys, or command results.
- Never output secrets: S3 keys, provider API keys, passwords, tokens, DSN, or credential_ref values. Never ask the user to paste a token.
- If a tool errors, explain the error code; do not fabricate a success.
- Answer in the user's language.`

// capabilityWritable / capabilityReadonly 是能力声明（planv6.0 v2 §3.5）。
// 六处模块常量不再各写一遍「readonly / 不要写」，由这里统一生成。
const (
	capabilityWritable = `Capability: writable. You may run write commands for the skills loaded this turn (e.g. sql exec, document insert, object upload, function create). Destructive commands still require user confirmation. Do not claim a write if the tool did not run.`
	capabilityReadonly = `Capability: readonly. Only run read commands. Write commands (sql exec, document insert/update/delete, object upload/delete, function create/update/delete, cron create/update/delete/trigger, kv exec, schema create-table/add-column, database create/delete, log retention set) are rejected with write_not_allowed, before any confirmation prompt. Do not retry or work around it. Tell the user this agent is read-only and that the "Allow write operations" switch in the agent's edit dialog turns writing on.`
)

// AssembleInstruction builds the system instruction (platform → module → agent → env → snapshot).
func AssembleInstruction(p PromptParts) string {
	base := platformBasePrompt
	if p.SkillsCLI {
		base = platformSkillsPrompt
	}
	capability := capabilityReadonly
	if p.SkillsCLI && p.Writable {
		capability = capabilityWritable
	}
	parts := []string{
		base,
		capability,
		strings.TrimSpace(p.ModuleTemplate),
		truncate(strings.TrimSpace(p.AgentPrompt), maxPromptSection),
		truncateSkill(strings.TrimSpace(p.SkillText)),
		truncate(strings.TrimSpace(p.ProjectEnv), maxPromptSection),
		truncate(strings.TrimSpace(p.Snapshot), maxPromptSection),
	}
	var b strings.Builder
	for _, s := range parts {
		if s == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(s)
	}
	return RedactSecrets(b.String())
}

// RedactSecrets 与 truncate 委托 agentcore（与本地 CLI 共用同一份实现）。
func RedactSecrets(s string) string { return agentcore.RedactSecrets(s) }

func truncate(s string, max int) string { return agentcore.Truncate(s, max) }
