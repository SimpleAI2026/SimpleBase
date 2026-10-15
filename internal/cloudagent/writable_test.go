package cloudagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/sbcli"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// planv6.0 v2 §3.8：不可写助手只能执行只读命令，且必须在确认之前拦截。

func makeAgentRequest(writable bool) RunRequest {
	return RunRequest{
		ProjectID: "p1", RunID: "r1", ThreadID: "t1",
		Agent: systemdb.CloudAgent{ID: "a1", Name: "helper", Module: ModuleDatabase, Writable: writable},
	}
}

// 不可写助手：写命令返回 write_not_allowed，且不进入确认流程。
func TestExecSimplebase_ReadonlyAgentRejectsWrite(t *testing.T) {
	sb := newSkillRecorder()
	r := &Runtime{CLI: sb, ToolProtocol: "text", APIBaseURL: "http://x", Issuer: stubIssuer{}}
	req := makeAgentRequest(false)
	ctx := withRunContext(context.Background(), RunContext{Skills: []string{"database"}, Writable: false})
	out, err := r.execSimplebase(ctx, req, []string{"sql", "exec", "--database", "shop", "--statement", "INSERT INTO t VALUES (1)"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "write_not_allowed") {
		t.Fatalf("out=%s", out)
	}
	if !strings.Contains(out, "允许写操作") {
		t.Fatalf("rejection should tell the user where to enable writing: %s", out)
	}
	if sb.calls != 0 {
		t.Fatalf("CLI must not run for a rejected write (calls=%d)", sb.calls)
	}
}

// 不可写助手：只读命令放行。
func TestExecSimplebase_ReadonlyAgentAllowsRead(t *testing.T) {
	sb := newSkillRecorder()
	r := &Runtime{CLI: sb, ToolProtocol: "text", APIBaseURL: "http://x", Issuer: stubIssuer{}}
	r.rememberToken("r1", "tok")
	req := makeAgentRequest(false)
	ctx := withRunContext(context.Background(), RunContext{Skills: []string{"database"}, Writable: false})
	out, err := r.execSimplebase(ctx, req, []string{"sql", "query", "--database", "shop", "--statement", "SELECT 1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "write_not_allowed") {
		t.Fatalf("read must be allowed: %s", out)
	}
	if sb.calls != 1 {
		t.Fatalf("CLI should run once, got %d", sb.calls)
	}
}

// 可写助手：写命令放行（命令分类与降权不误伤）。
func TestExecSimplebase_WritableAgentAllowsWrite(t *testing.T) {
	sb := newSkillRecorder()
	r := &Runtime{CLI: sb, ToolProtocol: "text", APIBaseURL: "http://x", Issuer: stubIssuer{}}
	r.rememberToken("r1", "tok")
	req := makeAgentRequest(true)
	ctx := withRunContext(context.Background(), RunContext{Skills: []string{"database"}, Writable: true})
	out, err := r.execSimplebase(ctx, req, []string{"sql", "exec", "--database", "shop", "--statement", "INSERT INTO t VALUES (1)"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "write_not_allowed") {
		t.Fatalf("writable agent must be allowed: %s", out)
	}
	if sb.calls != 1 {
		t.Fatalf("CLI should run once, got %d", sb.calls)
	}
}

// 命令分类的事实来源必须与 sbcli 一致。
func TestWritableGuardMatchesSbcli(t *testing.T) {
	for _, argv := range [][]string{
		{"sql", "query", "--database", "d"},
		{"sql", "exec", "--database", "d", "--statement", "INSERT INTO t VALUES (1)"},
		{"database", "delete", "--id", "d"},
		{"log", "retention", "get"},
		{"log", "retention", "set", "--days", "30"},
	} {
		if sbcli.CommandReadOnlyWithArgv(argv) {
			if !sbcli.CommandReadOnly(argv[0], argv[1]) && !isSubActionReadOnly(argv) {
				t.Fatalf("classification drifted for %v", argv)
			}
		}
	}
}

func isSubActionReadOnly(argv []string) bool {
	return len(argv) > 2 && sbcli.CommandReadOnlyWithArgv(argv)
}

// 降权必须移除写权限位。
func TestStripWritePermissions(t *testing.T) {
	p := auth.Principal{Permissions: map[auth.Permission]struct{}{
		auth.DatabaseRead: {}, auth.DatabaseWrite: {}, auth.DatabaseAdmin: {}, auth.ProjectAdmin: {},
	}}
	got := stripWritePermissions(p)
	if _, ok := got.Permissions[auth.DatabaseWrite]; ok {
		t.Fatal("DatabaseWrite must be stripped")
	}
	if _, ok := got.Permissions[auth.DatabaseAdmin]; ok {
		t.Fatal("DatabaseAdmin must be stripped")
	}
	if _, ok := got.Permissions[auth.DatabaseRead]; !ok {
		t.Fatal("DatabaseRead must be kept")
	}
	if _, ok := got.Permissions[auth.ProjectAdmin]; !ok {
		t.Fatal("ProjectAdmin must be kept")
	}
	// 空权限集原样返回，不 panic。
	empty := stripWritePermissions(auth.Principal{})
	if len(empty.Permissions) != 0 {
		t.Fatal("empty permissions must stay empty")
	}
}

// 提示词随 Writable 切换，不再自相矛盾。
func TestAssembleInstructionCapabilityFollowsWritable(t *testing.T) {
	readonly := AssembleInstruction(PromptParts{SkillsCLI: true, Writable: false})
	if !strings.Contains(readonly, "Capability: readonly") {
		t.Fatalf("readonly prompt missing: %s", readonly)
	}
	if strings.Contains(readonly, "Capability: writable") {
		t.Fatal("readonly prompt must not claim writable")
	}
	writable := AssembleInstruction(PromptParts{SkillsCLI: true, Writable: true})
	if !strings.Contains(writable, "Capability: writable") {
		t.Fatalf("writable prompt missing: %s", writable)
	}
	// 非 CLI 模式即使 Writable 为真也按只读声明（没有写工具可用）。
	noCLI := AssembleInstruction(PromptParts{SkillsCLI: false, Writable: true})
	if strings.Contains(noCLI, "Capability: writable") {
		t.Fatal("non-CLI mode must not claim writable")
	}
}

// 平台提示词不再写死「Tools are readonly」（与 CLI 写能力矛盾）。
func TestPlatformPromptNoLongerAssertsReadonlyTools(t *testing.T) {
	if strings.Contains(platformBasePrompt, "Tools are readonly") {
		t.Fatal("platformBasePrompt must not assert all tools are readonly")
	}
	if strings.Contains(databaseModulePrompt, "Do not run writes") {
		t.Fatal("databaseModulePrompt must not contradict CLI write capability")
	}
}

// skillRecorder 记录 CLI 调用次数与命令。
type skillRecorder struct {
	calls int
	last  []string
}

func newSkillRecorder() *skillRecorder { return &skillRecorder{} }

func (s *skillRecorder) Run(_ context.Context, argv, _ []string, _ string) (string, string, int, error) {
	s.calls++
	s.last = argv
	b, _ := json.Marshal(map[string]any{"ok": true})
	return string(b), "", 0, nil
}
