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
)

// PromptParts is the ordered assembly input. Secrets must never be placed in these fields.
type PromptParts struct {
	ModuleTemplate string
	AgentPrompt    string
	ProjectEnv     string
	Snapshot       string
}

// AssembleInstruction builds the system instruction (platform → module → agent → env → snapshot).
func AssembleInstruction(p PromptParts) string {
	parts := []string{
		platformBasePrompt,
		strings.TrimSpace(p.ModuleTemplate),
		truncate(strings.TrimSpace(p.AgentPrompt), maxPromptSection),
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
	return out
}

func truncate(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "\n…[truncated]"
}
