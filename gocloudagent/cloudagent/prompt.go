package cloudagent

import (
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

const (
	maxHistoryChars = 12000
	maxUserChars    = 8000
)

// AssembleInstruction joins non-empty prompt sections in the supplied order.
func AssembleInstruction(sections ...string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		if section = strings.TrimSpace(section); section != "" {
			parts = append(parts, section)
		}
	}
	return agentcore.RedactSecrets(strings.Join(parts, "\n\n"))
}

func truncate(s string, max int) string { return agentcore.Truncate(s, max) }
