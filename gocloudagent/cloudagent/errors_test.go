// 文件说明：errors_test.go — gocloudagent/cloudagent 的单元测试（TestAgentClassifyErrorDoesNotExposeUpstreamDetails 等）
//
// 用法
//
//	go test ./gocloudagent/cloudagent -run 'TestAgentClassifyErrorDoesNotExposeUpstreamDetails'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/errors_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//
// 主要符号
//
//	TestAgentClassifyErrorDoesNotExposeUpstreamDetails(t *testing.T)
//	type timeoutErr struct{…}
//	(timeoutErr) Error() string
//	(timeoutErr) Timeout() bool
//	(timeoutErr) Temporary() bool

package cloudagent

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
