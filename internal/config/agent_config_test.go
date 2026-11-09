package config

import (
	"strings"
	"testing"
	"time"
)

func TestAgentConfigEnvironmentOverrides(t *testing.T) {
	t.Setenv("SIMPLEBASE_LLM_AGENT_TOOL_PROTOCOL", "native")
	t.Setenv("SIMPLEBASE_LLM_AGENT_MAX_ITERATIONS", "12")
	t.Setenv("SIMPLEBASE_LLM_AGENT_RUN_TIMEOUT", "4m")
	c := defaults()
	applyEnvLLM(&c)
	if c.LLM.EffectiveAgentToolProtocol() != "native" || c.LLM.EffectiveAgentMaxIterations() != 12 || c.LLM.EffectiveAgentRunTimeout() != 4*time.Minute {
		t.Fatalf("agent config=%+v", c.LLM)
	}
}

func TestAgentConfigInvalidRanges(t *testing.T) {
	for _, tc := range []struct {
		name string
		llm  LLMConfig
		want string
	}{
		{"protocol", LLMConfig{AgentToolProtocol: "unknown"}, "agent_tool_protocol"},
		{"iterations-negative", LLMConfig{AgentMaxIterations: -1}, "agent_max_iterations"},
		{"iterations-large", LLMConfig{AgentMaxIterations: 21}, "agent_max_iterations"},
		{"timeout-short", LLMConfig{AgentRunTimeout: 5 * time.Second}, "agent_run_timeout"},
		{"timeout-long", LLMConfig{AgentRunTimeout: 16 * time.Minute}, "agent_run_timeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateLLMAgent(tc.llm); len(err) == 0 || !strings.Contains(err[0].Error(), tc.want) {
				t.Fatalf("errors=%v", err)
			}
		})
	}
}
