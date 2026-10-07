package cloudagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/voocel/litellm/providers"
)

func TestAgentClassifyErrorDoesNotExposeUpstreamDetails(t *testing.T) {
	secret := "redacted-test-secret"
	for _, tc := range []struct {
		err  error
		code string
	}{
		{ErrThreadBusy, "agent_thread_busy"},
		{llmgateway.ErrNoProviders, "llm_not_configured"},
		{llmgateway.ErrModelNotAllowed, "llm_model_not_allowed"},
		{context.DeadlineExceeded, "llm_timeout"},
		{&providers.LiteLLMError{StatusCode: 401, Message: secret}, "llm_auth_failed"},
		{&providers.LiteLLMError{StatusCode: 429, Message: secret}, "llm_rate_limited"},
		{&providers.LiteLLMError{StatusCode: 502, Message: secret}, "llm_upstream_error"},
		{fmt.Errorf("provider failed: %s", secret), "llm_upstream_error"},
	} {
		got := ClassifyError(fmt.Errorf("wrapped: %w", tc.err))
		if got == nil || got.Code != tc.code || strings.Contains(got.Message, secret) {
			t.Fatalf("input=%T code=%+v", tc.err, got)
		}
	}
	if ClassifyError(nil) != nil {
		t.Fatal("nil error should not classify")
	}
	if !errors.Is(ErrThreadBusy, ErrThreadBusy) {
		t.Fatal("sentinel mismatch")
	}
}
