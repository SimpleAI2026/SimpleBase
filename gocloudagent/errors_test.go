package gocloudagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/voocel/litellm/providers"
)

// 模块内的错误分类只覆盖模块自有语义：llmgateway 哨兵由宿主
// internal/agentbridge.ClassifyError 先行匹配（那边有对应测试）。
func TestAgentClassifyErrorDoesNotExposeUpstreamDetails(t *testing.T) {
	secret := "redacted-test-secret"
	for _, tc := range []struct {
		err  error
		code string
	}{
		{ErrThreadBusy, "agent_thread_busy"},
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
	got := ClassifyError(fmt.Errorf("dial: %w", timeoutErr{}))
	if got == nil || got.Code != "llm_timeout" {
		t.Fatalf("timeout classification=%+v", got)
	}
}

// timeoutErr 实现 net.Error，验证超时识别不依赖具体实现类型。
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }
