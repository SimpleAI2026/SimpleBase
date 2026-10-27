package gocloudagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// failingPolicy 的 Classify 总是报错，模拟策略不可用。
type failingPolicy struct{}

func (failingPolicy) Classify([]string, string) (CommandDecision, error) {
	return CommandDecision{}, errors.New("policy down")
}
func (failingPolicy) RedactArgv(a []string) []string { return a }

// 策略缺失时 CLI 工具必须拒绝：不确认、不发 tool_start、不调用 CLIRunner。
// 即使助手可写、命令看似只读，也不能放行（fail-closed）。
func TestExecSimplebase_NilPolicyFailsClosed(t *testing.T) {
	sb := newSkillRecorder()
	r := &Runtime{CLI: sb, ToolProtocol: "text", APIBaseURL: "http://x", Issuer: stubIssuer{}}
	r.rememberToken("r1", "tok")
	for _, writable := range []bool{true, false} {
		req := makeAgentRequest(writable)
		var events []Event
		ctx := withRunContext(context.Background(), RunContext{
			Skills: []string{"database"}, Writable: writable,
			Emit: func(e Event) { events = append(events, e) },
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
	r := &Runtime{CLI: sb, ToolProtocol: "text", APIBaseURL: "http://x", Issuer: stubIssuer{}, Policy: failingPolicy{}}
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
	src := Principal{
		UserID: "u1",
		Permissions: map[Permission]struct{}{
			DatabaseRead: {}, DatabaseWrite: {}, DatabaseAdmin: {},
		},
	}
	got := stripWritePermissions(src)
	if got.HasPermission(DatabaseWrite) || got.HasPermission(DatabaseAdmin) {
		t.Fatalf("write perms survived: %v", got.Permissions)
	}
	if !src.HasPermission(DatabaseWrite) || !src.HasPermission(DatabaseAdmin) {
		t.Fatalf("source principal was mutated: %v", src.Permissions)
	}
}

// plainLLM 直接给出最终回答，不产生工具调用。
type plainLLM struct{}

func (plainLLM) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{Content: "done"}, nil
}
func (plainLLM) Stream(ctx context.Context, p string, r ChatRequest) (TokenStream, error) {
	return &onceStream{content: "done"}, nil
}

// 不可写助手的委托令牌签发入口：Runtime 必须把降权后的身份交给 Issuer，
// 且调用方传入的原始身份不被改动。
type capturingIssuer struct{ got Principal }

func (c *capturingIssuer) Issue(p Principal, _, _ string, _ time.Duration) (string, error) {
	c.got = p
	return "tok", nil
}
func (c *capturingIssuer) RevokeRun(string) {}

func TestStartRun_ReadonlyAgentIssuesStrippedPrincipal(t *testing.T) {
	for _, tc := range []struct {
		name         string
		writable     bool
		wantWrite    bool
		wantReadKept bool
	}{
		{"readonly agent", false, false, true},
		{"writable agent", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			iss := &capturingIssuer{}
			rt := &Runtime{LLM: plainLLM{}, SkillsCLI: true, Issuer: iss, Policy: testPolicy{}, ToolProtocol: "text", APIBaseURL: "http://x"}
			caller := Principal{
				UserID: "u1", TenantID: "t1", Role: "admin",
				Permissions: map[Permission]struct{}{DatabaseRead: {}, DatabaseWrite: {}, DatabaseAdmin: {}},
			}
			_, err := rt.StartRun(context.Background(), RunRequest{
				ProjectID: "p", ThreadID: "t", RunID: "r", UserText: "hi",
				Principal: caller, Agent: Agent{ID: "a", Name: "Bot", Module: ModuleGeneral, Writable: tc.writable},
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if iss.got.HasPermission(DatabaseWrite) != tc.wantWrite || iss.got.HasPermission(DatabaseAdmin) != tc.wantWrite {
				t.Fatalf("issued perms=%v want write=%v", iss.got.Permissions, tc.wantWrite)
			}
			if iss.got.HasPermission(DatabaseRead) != tc.wantReadKept {
				t.Fatalf("read permission lost: %v", iss.got.Permissions)
			}
			// 身份字段必须原样保留，宿主签发依赖它们。
			if iss.got.UserID != "u1" || iss.got.TenantID != "t1" || iss.got.Role != "admin" {
				t.Fatalf("identity fields lost: %+v", iss.got)
			}
			// 调用方原对象不能被降权污染。
			if !caller.HasPermission(DatabaseWrite) || !caller.HasPermission(DatabaseAdmin) {
				t.Fatalf("caller principal mutated: %v", caller.Permissions)
			}
		})
	}
}
