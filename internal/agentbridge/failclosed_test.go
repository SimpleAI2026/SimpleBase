// 文件说明：failclosed_test.go — internal/agentbridge 的单元测试（TestExecSimplebase_NilPolicyFailsClosed、TestExecSimplebase_PolicyErrorFailsClosed 等）
//
// 用法
//
//	go test ./internal/agentbridge -run 'TestExecSimplebase_NilPolicyFailsClosed|TestExecSimplebase_PolicyErrorFailsClosed|TestStripWritePermissionsDoesNotMutateSource|TestStartRunReadonlyAgentIssuesStrippedPrincipal'
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/failclosed_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//
// 主要符号
//
//	type failingPolicy struct{…}
//	(failingPolicy) Classify([]string, string) (cloudagent.ToolDecision, error)
//	(failingPolicy) RedactArgv(a []string) []string
//	TestExecSimplebase_NilPolicyFailsClosed(t *testing.T)
//	TestExecSimplebase_PolicyErrorFailsClosed(t *testing.T)
//	TestStripWritePermissionsDoesNotMutateSource(t *testing.T)
//	type plainLLM struct{…}
//	(plainLLM) Chat(context.Context, string, cloudagent.ChatRequest) (cloudagent.ChatResponse, error)
//	... 另有 5 个顶层符号

package agentbridge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// failingPolicy 的 Classify 总是报错，模拟策略不可用。
type failingPolicy struct{}

func (failingPolicy) Classify([]string, string) (cloudagent.ToolDecision, error) {
	return cloudagent.ToolDecision{}, errors.New("policy down")
}
func (failingPolicy) RedactArgv(a []string) []string { return a }

// 策略缺失时 CLI 工具必须拒绝：不确认、不发 tool_start、不调用 CLIRunner。
// 即使助手可写、命令看似只读，也不能放行（fail-closed）。
func TestExecSimplebase_NilPolicyFailsClosed(t *testing.T) {
	sb := newSkillRecorder()
	r := &RuntimeProvider{CLI: sb, ToolProtocol: "text", APIBaseURL: "http://x", Issuer: stubIssuer{}}
	r.rememberToken("r1", "tok")
	for _, writable := range []bool{true, false} {
		req := makeAgentRequest(writable)
		var events []cloudagent.Event
		ctx := withRunContext(context.Background(), RunContext{
			Skills: []string{"database"}, Writable: writable,
			Emit: func(e cloudagent.Event) { events = append(events, e) },
		})
		out, err := r.execSimplebase(ctx, req, []string{"database", "list"}, "")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, commandPolicyUnavailable) {
			t.Fatalf("writable=%v out=%s", writable, out)
		}
		for _, e := range events {
			if e.Type == "confirmation_required" || e.Type == "tool_start" {
				t.Fatalf("policy-less run emitted %s", e.Type)
			}
		}
	}
	if sb.calls != 0 {
		t.Fatalf("CLI must not run without a policy (calls=%d)", sb.calls)
	}
}

// 分类出错同样拒绝，而不是退回「确认后执行」。
func TestExecSimplebase_PolicyErrorFailsClosed(t *testing.T) {
	sb := newSkillRecorder()
	r := &RuntimeProvider{CLI: sb, ToolProtocol: "text", APIBaseURL: "http://x", Issuer: stubIssuer{}, Policy: failingPolicy{}}
	r.rememberToken("r1", "tok")
	req := makeAgentRequest(true)
	ctx := withRunContext(context.Background(), RunContext{Skills: []string{"database"}, Writable: true})
	out, err := r.execSimplebase(ctx, req, []string{"database", "list"}, "")
	if err != nil || !strings.Contains(out, commandPolicyUnavailable) {
		t.Fatalf("out=%s err=%v", out, err)
	}
	if sb.calls != 0 {
		t.Fatalf("CLI must not run when policy errors (calls=%d)", sb.calls)
	}
}

// 降权必须复制权限集合：委托签发会遍历它，就地修改会污染调用方身份。
func TestStripWritePermissionsDoesNotMutateSource(t *testing.T) {
	src := auth.Principal{
		UserID: "u1",
		Permissions: map[auth.Permission]struct{}{
			auth.DatabaseRead: {}, auth.DatabaseWrite: {}, auth.DatabaseAdmin: {},
		},
	}
	got := stripWritePermissions(src)
	if got.HasPermission(auth.DatabaseWrite) || got.HasPermission(auth.DatabaseAdmin) {
		t.Fatalf("write perms survived: %v", got.Permissions)
	}
	if !src.HasPermission(auth.DatabaseWrite) || !src.HasPermission(auth.DatabaseAdmin) {
		t.Fatalf("source principal was mutated: %v", src.Permissions)
	}
}

// plainLLM 直接给出最终回答，不产生工具调用。
type plainLLM struct{}

func (plainLLM) Chat(context.Context, string, cloudagent.ChatRequest) (cloudagent.ChatResponse, error) {
	return cloudagent.ChatResponse{Content: "done"}, nil
}
func (plainLLM) Stream(ctx context.Context, p string, r cloudagent.ChatRequest) (cloudagent.TokenStream, error) {
	return &onceStream{content: "done"}, nil
}

// 不可写助手的委托令牌签发入口：Runtime 必须把降权后的身份交给 Issuer，
// 且调用方传入的原始身份不被改动。
type capturingIssuer struct{ got auth.Principal }

func (c *capturingIssuer) Issue(p auth.Principal, _, _ string, _ time.Duration) (string, error) {
	c.got = p
	return "tok", nil
}
func (c *capturingIssuer) RevokeRun(string) {}

func TestStartRunReadonlyAgentIssuesStrippedPrincipal(t *testing.T) {
	for _, tc := range []struct {
		name     string
		writable bool
	}{
		{"readonly agent", false},
		{"writable agent", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issuer := &capturingIssuer{}
			provider := &RuntimeProvider{SkillsCLI: true, Issuer: issuer, Policy: testPolicy{}, ToolProtocol: "text", APIBaseURL: "http://x"}
			rt := &cloudagent.Runtime{LLM: plainLLM{}, Provider: provider, ToolProtocol: "text"}
			caller := auth.Principal{
				UserID: "u1", TenantID: "t1", Role: auth.RoleAdmin,
				Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}, auth.DatabaseWrite: {}, auth.DatabaseAdmin: {}},
			}
			_, err := rt.StartRun(context.Background(), cloudagent.RunRequest{
				ThreadID: "t", RunID: "r", UserText: "hi",
				Context: RunContext{ProjectID: "p", Principal: caller},
				Agent:   cloudagent.Agent{ID: "a", Name: "Bot", Module: ModuleGeneral, Writable: tc.writable},
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if issuer.got.HasPermission(auth.DatabaseWrite) != tc.writable || issuer.got.HasPermission(auth.DatabaseAdmin) != tc.writable {
				t.Fatalf("issued perms=%v want write=%v", issuer.got.Permissions, tc.writable)
			}
			if !issuer.got.HasPermission(auth.DatabaseRead) {
				t.Fatalf("read permission lost: %v", issuer.got.Permissions)
			}
			if issuer.got.UserID != "u1" || issuer.got.TenantID != "t1" || issuer.got.Role != auth.RoleAdmin {
				t.Fatalf("identity fields lost: %+v", issuer.got)
			}
			if !caller.HasPermission(auth.DatabaseWrite) || !caller.HasPermission(auth.DatabaseAdmin) {
				t.Fatalf("caller principal mutated: %v", caller.Permissions)
			}
		})
	}
}
