// 文件说明：bridge_test.go — internal/agentbridge 的单元测试（TestCommandPolicyReadOnlyMatchesSbcli、TestDestructiveArgv 等）
//
// 用法
//
//	go test ./internal/agentbridge -run 'TestCommandPolicyReadOnlyMatchesSbcli|TestDestructiveArgv|TestSQLDestructiveQuotedLiteral|TestRedactArgv'
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/bridge_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, cli.go 等 33 个
//
// 主要符号
//
//	TestCommandPolicyReadOnlyMatchesSbcli(t *testing.T)
//	TestDestructiveArgv(t *testing.T)
//	TestSQLDestructiveQuotedLiteral(t *testing.T)
//	TestRedactArgv(t *testing.T)
//	TestClassifyErrorMatchesHostSentinels(t *testing.T)
//	TestAgentMappingIsAlwaysWritableAndKeepsTools(t *testing.T)
//	TestMessagesMapping(t *testing.T)
//	TestStoreAccessWrappers(t *testing.T)
//	... 另有 1 个顶层符号

package agentbridge_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/sbcli"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	_ "github.com/uglyer/go-sqlite3"
)

// 这些用例验证「宿主规则」一侧：只读表、SQL 切分、错误哨兵、store 适配。
// gocloudagent 模块内不复制这些规则，因此对账只能在这里做。

func TestCommandPolicyReadOnlyMatchesSbcli(t *testing.T) {
	p := agentbridge.NewCommandPolicy()
	for _, argv := range [][]string{
		{"sql", "query", "--database", "d"},
		{"sql", "exec", "--database", "d", "--statement", "INSERT INTO t VALUES (1)"},
		{"database", "delete", "--id", "d"},
		{"log", "retention", "get"},
		{"log", "retention", "set", "--days", "30"},
	} {
		got, err := p.Classify(argv, "")
		if err != nil {
			t.Fatalf("%v: %v", argv, err)
		}
		if got.ReadOnly != sbcli.CommandReadOnlyWithArgv(argv) {
			t.Fatalf("readonly drifted for %v: %v", argv, got.ReadOnly)
		}
	}
	// 空 argv 是装配/参数错误，必须报错而不是放行。
	if _, err := p.Classify(nil, ""); err == nil {
		t.Fatal("empty argv must error")
	}
}

func TestDestructiveArgv(t *testing.T) {
	cases := []struct {
		argv []string
		in   string
		want bool
	}{
		{[]string{"database", "delete", "--id", "shop"}, "", true},
		{[]string{"sql", "exec", "--database", "d", "--statement", "INSERT INTO t VALUES (1)"}, "", false},
		{[]string{"sql", "exec", "--database", "d", "--statement", "DELETE FROM t"}, "", true},
		{[]string{"kv", "exec", "--command", "SET", "--arg", "a", "--arg", "1"}, "", false},
		{[]string{"kv", "exec", "--command", "DEL", "--arg", "a"}, "", true},
		{[]string{"sql", "exec", "--database", "d", "--statement", "CALL something()"}, "", true},
		// 缺 --statement 时无法判定，保守视为破坏性。
		{[]string{"sql", "exec", "--database", "d"}, "", true},
		// stdin 场景：--statement - 时读 stdin。
		{[]string{"sql", "exec", "--statement", "-"}, "DELETE FROM t", true},
		{[]string{"sql", "exec", "--statement", "-"}, "SELECT 1", false},
	}
	for _, tc := range cases {
		if got := agentbridge.DestructiveArgv(tc.argv, tc.in); got != tc.want {
			t.Fatalf("argv=%v stdin=%q got=%v want=%v", tc.argv, tc.in, got, tc.want)
		}
	}
}

func TestSQLDestructiveQuotedLiteral(t *testing.T) {
	if agentbridge.SQLDestructive(`INSERT INTO t VALUES ('DELETE FROM x'), (2)`) {
		t.Fatal("字符串字面量里的 DELETE 被误判为破坏性")
	}
	if !agentbridge.SQLDestructive(`INSERT INTO t VALUES (1); DELETE FROM t`) {
		t.Fatal("多语句中的 DELETE 未识别")
	}
	if agentbridge.SQLDestructive("-- DROP TABLE x\nSELECT 1") {
		t.Fatal("注释里的 DROP 被误判")
	}
	if !agentbridge.SQLDestructive("CALL something()") {
		t.Fatal("未知语句应保守视为破坏性")
	}
}

func TestRedactArgv(t *testing.T) {
	// --statement 保留首关键字 + 前 200 字符。
	long := strings.Repeat("x", 400)
	out := agentbridge.RedactArgv([]string{"sql", "exec", "--statement", "DELETE FROM t WHERE s='" + long + "'"})
	stmt := out[3]
	if !strings.HasPrefix(stmt, "DELETE FROM t WHERE s=") {
		t.Fatalf("statement 首段被破坏: %q", stmt)
	}
	if len(stmt) > 220 {
		t.Fatalf("statement 未截断: %d", len(stmt))
	}

	// kv exec 的 --arg 除第一个外全部 ***。
	out = agentbridge.RedactArgv([]string{"kv", "exec", "--command", "SET", "--arg", "key1", "--arg", "secret-value"})
	if out[5] != "key1" {
		t.Fatalf("第一个 --arg 应保留 key: %q", out[5])
	}
	if out[7] != "***" {
		t.Fatalf("其余 --arg 应脱敏: %q", out[7])
	}

	// 敏感 flag 值替换 ***。
	out = agentbridge.RedactArgv([]string{"x", "y", "--api_key", "sk-123", "--password=hunter2"})
	if !strings.Contains(strings.Join(out, " "), "***") {
		t.Fatalf("敏感值未脱敏: %v", out)
	}
	for _, s := range out {
		if strings.Contains(s, "sk-123") || strings.Contains(s, "hunter2") {
			t.Fatalf("敏感值泄漏: %v", out)
		}
	}
	// 原 argv 不能被就地修改：脱敏结果必须是副本。
	in := []string{"kv", "exec", "--command", "SET", "--arg", "k", "--arg", "secret"}
	out = agentbridge.RedactArgv(in)
	if in[7] != "secret" {
		t.Fatalf("RedactArgv mutated input: %v", in)
	}
	if out[7] != "***" {
		t.Fatalf("copy not redacted: %v", out)
	}
}

func TestClassifyErrorMatchesHostSentinels(t *testing.T) {
	secret := "redacted-test-secret"
	for _, tc := range []struct {
		err  error
		code string
	}{
		{llmgateway.ErrNoProviders, "llm_not_configured"},
		{llmgateway.ErrModelNotConfigured, "llm_not_configured"},
		{fmt.Errorf("llmgateway: chat: %w", llmgateway.ErrModelNotConfigured), "llm_not_configured"},
		{llmgateway.ErrModelNotAllowed, "llm_model_not_allowed"},
		{cloudagent.ErrThreadBusy, "agent_thread_busy"},
		{context.DeadlineExceeded, "llm_timeout"},
		{fmt.Errorf("provider failed: %s", secret), "llm_upstream_error"},
	} {
		got := agentbridge.ClassifyError(fmt.Errorf("wrapped: %w", tc.err))
		if got == nil || got.Code != tc.code || strings.Contains(got.Message, secret) {
			t.Fatalf("input=%T got=%+v", tc.err, got)
		}
	}
	if agentbridge.ClassifyError(nil) != nil {
		t.Fatal("nil error should not classify")
	}
}

// planv6.7 v2 §3.4：写能力不再按助手配置，映射出的 Agent 恒为可写。
func TestAgentMappingIsAlwaysWritableAndKeepsTools(t *testing.T) {
	src := systemdb.CloudAgent{
		ID: "a1", ProjectID: "p1", Name: "Bot", Module: "database", Description: "d",
		SystemPrompt: "sp", ToolIDs: []string{"list_databases"}, ModelOverride: "m1",
	}
	got := agentbridge.Agent(src)
	if got.ID != "a1" || got.ModelOverride != "m1" || len(got.ToolIDs) != 1 {
		t.Fatalf("agent mapping lossy: %+v", got)
	}
	if !got.Writable {
		t.Fatal("every agent must be writable; destructive tools still need confirmation")
	}
	// 工具列表必须是副本：避免模块侧改动影响宿主实体。
	got.ToolIDs[0] = "mutated"
	if src.ToolIDs[0] != "list_databases" {
		t.Fatal("tool ids shared with source")
	}
}

func TestMessagesMapping(t *testing.T) {
	if agentbridge.Messages(nil) != nil {
		t.Fatal("nil messages")
	}
	got := agentbridge.Messages([]systemdb.AgentMessage{
		{Role: "user", Content: "u"},
		{Role: "assistant", Content: "a", ToolCallsJSON: "[]"},
	})
	if len(got) != 2 || got[1].ToolCallsJSON != "[]" {
		t.Fatalf("messages=%+v", got)
	}
}

func TestStoreAccessWrappers(t *testing.T) {
	if agentbridge.NewLogAccess(nil) != nil || agentbridge.NewSettingsAccess(nil) != nil {
		t.Fatal("nil store")
	}
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := systemdb.EnsureSystemSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(db)
	store.RecordLog(systemdb.LogEvent{ProjectID: "p1", Level: "info", Logger: "t", Message: "hello coverage"})
	if err := store.FlushLogs(ctx); err != nil {
		t.Fatal(err)
	}
	logs := agentbridge.NewLogAccess(store)
	events, err := logs.SearchLogs(ctx, "p1", "info", "coverage", 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("search n=%d err=%v", len(events), err)
	}
	if events[0].Logger != "t" || !strings.Contains(events[0].Message, "hello") {
		t.Fatalf("event mapping=%+v", events[0])
	}
	stats, err := logs.LevelStats(ctx, "p1")
	if err != nil || len(stats) == 0 {
		t.Fatalf("stats=%v err=%v", stats, err)
	}
	settings := agentbridge.NewSettingsAccess(store)
	st, err := settings.LLMSettings(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if st.ProjectID != "p1" {
		t.Fatalf("default settings %+v", st)
	}
	if err := store.PutLLMSettings(ctx, systemdb.LLMSettings{ProjectID: "p1", DefaultModel: "m1", Temperature: 0.1, MaxTokens: 16}); err != nil {
		t.Fatal(err)
	}
	st, err = settings.LLMSettings(ctx, "p1")
	if err != nil || st.DefaultModel != "m1" {
		t.Fatalf("settings %+v err=%v", st, err)
	}
}

// 委托签发必须只包含传入（已降权）的权限，而不是某个缓存的原始主体。
func TestDelegationIssuerUsesGivenPermissions(t *testing.T) {
	reg := auth.NewDelegationRegistry()
	issuer := agentbridge.NewDelegationIssuer("test-secret", reg)
	p := auth.Principal{
		UserID: "u1", Username: "alice", Role: auth.RoleAdmin, TenantID: "t1",
		ProjectIDs:  map[string]struct{}{"p1": {}},
		Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}},
	}
	tok, err := issuer.Issue(p, "p1", "run-1", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := auth.VerifyJWT("test-secret", tok, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, perm := range claims.Perms {
		got[perm] = true
	}
	if !got[string(auth.DatabaseRead)] {
		t.Fatalf("missing read permission: %v", claims.Perms)
	}
	if got[string(auth.DatabaseWrite)] || got[string(auth.DatabaseAdmin)] {
		t.Fatalf("delegation escalated: %v", claims.Perms)
	}
	if claims.ProjectID != "p1" || claims.RunID != "run-1" || claims.TenantID != "t1" {
		t.Fatalf("claims scope wrong: %+v", claims)
	}
}
