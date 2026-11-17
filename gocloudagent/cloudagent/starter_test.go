package cloudagent

import (
	"context"
	"strings"
	"testing"

	agentskills "github.com/linkxzhou/SimpleBase/gocloudagent/skills"
)

// 搭建会话阶段矩阵（home-starter-prompts-plan §3.3）：各阶段放行的命令前缀。
func TestStarterStageMatrix(t *testing.T) {
	allowed := func(stage string, argv0 string) (read, write bool) {
		skills, readonly, ok := StarterStage(stage)
		if !ok {
			t.Fatalf("stage %q missing", stage)
		}
		argv := []string{argv0, "x"}
		return agentskills.AllowArgv(skills, argv), agentskills.AllowWriteArgv(skills, readonly, argv)
	}
	cases := []struct {
		stage string
		argv0 string
		read  bool
		write bool
	}{
		{StarterStagePlan, "sql", false, false},
		{StarterStageData, "sql", true, true},
		{StarterStageData, "function", false, false},
		{StarterStageFunc, "function", true, true},
		{StarterStageFunc, "sql", true, false}, // database 只读
		{StarterStageFunc, "cron", false, false},
		{StarterStageCron, "cron", true, true},
		{StarterStageCron, "function", true, false}, // gofunction 只读
		{StarterStageCron, "sql", false, false},
		{StarterStageSite, "object", true, true},
		{StarterStageSite, "site", true, true},
		{StarterStageSite, "sql", false, false},
		{StarterStageVerify, "sql", true, false},
		{StarterStageVerify, "function", true, false},
		{StarterStageVerify, "site", true, false},
		{StarterStageVerify, "cron", false, false},
		{StarterStageVerify, "object", false, false},
		{StarterStageVerify, "kv", false, false},
	}
	for _, tc := range cases {
		r, w := allowed(tc.stage, tc.argv0)
		if r != tc.read || w != tc.write {
			t.Errorf("%s/%s: read=%v write=%v want read=%v write=%v", tc.stage, tc.argv0, r, w, tc.read, tc.write)
		}
	}
	if _, _, ok := StarterStage("nope"); ok {
		t.Fatal("unknown stage must not resolve")
	}
	if got := StarterStages(); len(got) != 6 || got[0] != StarterStagePlan || got[5] != StarterStageVerify {
		t.Fatalf("stages=%v", got)
	}
}

func TestStarterTemplateWhitelist(t *testing.T) {
	for _, id := range []string{"dashboard", "portfolio", "notice"} {
		if !IsStarterTemplate(id) {
			t.Errorf("%s must be allowed in B1", id)
		}
	}
	// 需要匿名写入的模板依赖 B3，评审前不接受。
	for _, id := range []string{"todo", "booking", "survey", "", " ", "DASHBOARD", "../x"} {
		if IsStarterTemplate(id) {
			t.Errorf("%q must not be allowed", id)
		}
	}
}

// 搭建阶段内只读技能的写命令在确认之前被拒；本阶段的写命令仍可执行。
func TestExecSimplebaseStageReadonlySkill(t *testing.T) {
	sb := newSkillRecorder()
	r := &Runtime{CLI: sb, ToolProtocol: "text", APIBaseURL: "http://x", Issuer: stubIssuer{}, Policy: testPolicy{}}
	r.rememberToken("r1", "tok")
	skills, readonly, _ := StarterStage(StarterStageFunc)
	req := RunRequest{ProjectID: "p", RunID: "r1", Agent: Agent{ID: "a", Module: ModuleGeneral, Writable: true}, Skills: skills, ReadOnlySkills: readonly}
	ctx := withRunContext(context.Background(), RunContext{Skills: skills, ReadOnlySkills: readonly, Writable: true})

	out, err := r.execSimplebase(ctx, req, []string{"sql", "exec", "--database", "d", "--statement", "INSERT INTO t VALUES (1)"}, "")
	if err != nil || !strings.Contains(out, "write_not_allowed") {
		t.Fatalf("stage must block database writes: %s err=%v", out, err)
	}
	if out, _ = r.execSimplebase(ctx, req, []string{"sql", "query", "--database", "d", "--statement", "SELECT 1"}, ""); strings.Contains(out, "write_not_allowed") || strings.Contains(out, "skill_not_loaded") {
		t.Fatalf("read in readonly skill must be allowed: %s", out)
	}
	if out, _ = r.execSimplebase(ctx, req, []string{"cron", "create", "--name", "x"}, ""); !strings.Contains(out, "skill_not_loaded") {
		t.Fatalf("other stage command must not be loaded: %s", out)
	}
}

// plan 阶段没有技能：任何写命令都不放行。
func TestExecSimplebasePlanStageLoadsNoSkill(t *testing.T) {
	r := &Runtime{CLI: newSkillRecorder(), ToolProtocol: "text", APIBaseURL: "http://x", Issuer: stubIssuer{}, Policy: testPolicy{}}
	skills, readonly, _ := StarterStage(StarterStagePlan)
	req := RunRequest{ProjectID: "p", RunID: "r", Agent: Agent{ID: "a", Module: ModuleGeneral, Writable: true}, Skills: skills, ReadOnlySkills: readonly}
	ctx := withRunContext(context.Background(), RunContext{Skills: skills, Writable: true})
	out, err := r.execSimplebase(ctx, req, []string{"sql", "exec", "--database", "d", "--statement", "CREATE TABLE t(a int)"}, "")
	if err != nil || !strings.Contains(out, "skill_not_loaded") {
		t.Fatalf("plan stage must not run commands: %s err=%v", out, err)
	}
}

// 站点命令在 headless 下返回 confirmation_unavailable（对外公开必须有人确认）。
func TestSiteCommandsNeedConfirmation(t *testing.T) {
	policy := destructiveSitePolicy{}
	r := &Runtime{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}, Policy: policy}
	skills, readonly, _ := StarterStage(StarterStageSite)
	req := RunRequest{ProjectID: "p", RunID: "r", Headless: true, Agent: Agent{ID: "a", Module: ModuleGeneral, Writable: true}, Skills: skills, ReadOnlySkills: readonly}
	ctx := withRunContext(context.Background(), RunContext{Skills: skills, Headless: true, Writable: true})
	for _, argv := range [][]string{{"site", "enable"}, {"site", "publish", "--prefix", "web/"}} {
		out, err := r.execSimplebase(ctx, req, argv, "")
		if err != nil || !strings.Contains(out, "confirmation_unavailable") {
			t.Fatalf("%v: %s err=%v", argv, out, err)
		}
	}
}

func TestStarterPromptMentionsStageAndRules(t *testing.T) {
	p := starterPrompt(StarterStageSite)
	for _, want := range []string{"Current stage: site.", "web/index.html", "never trigger a cron job", "sb:delivery"} {
		if !strings.Contains(p, want) {
			t.Errorf("starter prompt missing %q", want)
		}
	}
	// 搭建提示词不得出现任何看起来像密钥的内容。
	if strings.Contains(strings.ToLower(p), "sb_live_") && !strings.Contains(p, "no key or token") {
		t.Error("starter prompt must forbid keys, not embed them")
	}
}

// destructiveSitePolicy 模拟宿主策略：site enable / publish 为非只读且破坏性。
type destructiveSitePolicy struct{ testPolicy }

func (destructiveSitePolicy) Classify(argv []string, _ string) (CommandDecision, error) {
	if len(argv) >= 2 && argv[0] == "site" {
		return CommandDecision{ReadOnly: argv[1] == "status", Destructive: argv[1] != "status"}, nil
	}
	return CommandDecision{}, nil
}
