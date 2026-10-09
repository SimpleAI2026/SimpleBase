package cloudagent

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxPromptSection = 4000
	maxHistoryChars  = 12000
	maxUserChars     = 8000
)

var (
	secretLineRE = regexp.MustCompile(`(?i)((?:aws_)?(?:secret|access)[_-]?key(?:[_-]?id)?|api[_-]?key|password|credential_ref|authorization)\s*[:=]\s*\S+`)
	awsKeyRE     = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	bearerRE     = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/]+=*`)
	apiTokenRE   = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`)
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
}

const platformSkillsPrompt = `You are SimpleBase Cloud Agent, a project-scoped assistant.
Rules:
- Stay in the current project. Never guess another tenant or project.
- Writes go only through the simplebase tool, and only for skills loaded this turn.
- Never invent rows, object keys, or command results.
- Never output secrets: S3 keys, provider API keys, passwords, tokens, DSN, or credential_ref values. Never ask the user to paste a token.
- If a tool errors, explain the error code; do not fabricate a success.
- Answer in the user's language.`

// AssembleInstruction builds the system instruction (platform → module → agent → env → snapshot).
func AssembleInstruction(p PromptParts) string {
	base := platformBasePrompt
	if p.SkillsCLI {
		base = platformSkillsPrompt
	}
	parts := []string{
		base,
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

// RedactSecrets strips credential-like substrings from prompt text.
func RedactSecrets(s string) string {
	if s == "" {
		return s
	}
	out := secretLineRE.ReplaceAllString(s, "$1=[REDACTED]")
	out = awsKeyRE.ReplaceAllString(out, "[REDACTED_AWS_KEY]")
	out = bearerRE.ReplaceAllString(out, "Bearer [REDACTED]")
	out = apiTokenRE.ReplaceAllString(out, "[REDACTED_API_KEY]")
	return out
}

func truncate(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "\n…[truncated]"
}
