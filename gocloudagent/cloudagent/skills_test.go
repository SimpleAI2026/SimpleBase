package cloudagent

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	agentskills "github.com/linkxzhou/SimpleBase/gocloudagent/skills"
)

func TestSkillsForModule(t *testing.T) {
	if !reflect.DeepEqual(SkillsForModule(ModuleDatabase), []string{"database"}) {
		t.Fatalf("database module: %v", SkillsForModule(ModuleDatabase))
	}
	// general 不自动加载子技能：`@` 不再提供按次选择，跨模块写入要先建对应专用助手。
	if got := SkillsForModule(ModuleGeneral); got != nil {
		t.Fatalf("general must not load sub-skills: %v", got)
	}
	if got := SkillsForModule("nope"); got != nil {
		t.Fatalf("unknown module must not load skills: %v", got)
	}
	if got := SkillsForModule(""); got != nil {
		t.Fatalf("empty module must not load skills: %v", got)
	}
	if !reflect.DeepEqual(SkillsForModule(" Database "), []string{"database"}) {
		t.Fatalf("module id is trimmed and lowercased: %v", SkillsForModule(" Database "))
	}
	if !agentskills.AllowArgv(SkillsForModule(ModuleDatabase), []string{"sql", "exec"}) || agentskills.AllowArgv(SkillsForModule(ModuleDatabase), []string{"function", "delete"}) {
		t.Fatal("database prefixes")
	}
}

func TestStatisticalModulesUseExistingSkills(t *testing.T) {
	for _, module := range []string{"kv", "gofunction", "cron"} {
		if !KnownModule(module) {
			t.Fatalf("module %s not listed", module)
		}
		skills := SkillsForModule(module)
		if !reflect.DeepEqual(skills, []string{module}) {
			t.Fatalf("module %s skills=%v", module, skills)
		}
		if moduleTemplate(module) == generalModulePrompt {
			t.Fatalf("module %s uses general prompt", module)
		}
		if len(DefaultToolsForModule(module)) != 0 {
			t.Fatalf("module %s advertises unimplemented built-in tools", module)
		}
	}
	if KnownModule("users") || agentskills.Known("users") {
		t.Fatal("users is not a delegable module or skill")
	}
}

func TestTokenInArgvRejected(t *testing.T) {
	rt := &Runtime{}
	out, err := rt.execSimplebase(context.Background(), RunRequest{ProjectID: "p", RunID: "r"}, []string{"database", "list", "SIMPLEBASE_TOKEN"}, "")
	if err != nil || !strings.Contains(out, "token_in_argv") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

func TestHeadlessDestructiveUnavailable(t *testing.T) {
	rt := &Runtime{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}, Policy: testPolicy{}}
	// 助手写门禁先于 headless 判定，因此这里需要可写助手才能走到确认分支。
	ctx := withRunContext(context.Background(), RunContext{Headless: true, Writable: true})
	out, err := rt.execSimplebase(ctx, RunRequest{ProjectID: "p", RunID: "r", Skills: []string{"database"}, Headless: true}, []string{"database", "delete", "--id", "shop"}, "")
	if err != nil || !strings.Contains(out, "confirmation_unavailable") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

// planv6.0 v2 §3.8：只读助手的写命令在 headless 与交互两种模式下都被拒，
// 且都在确认之前拦截。
func TestReadonlyAgentBeatsHeadlessCheck(t *testing.T) {
	rt := &Runtime{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}, Policy: testPolicy{}}
	ctx := withRunContext(context.Background(), RunContext{Headless: true, Writable: false})
	out, err := rt.execSimplebase(ctx, RunRequest{ProjectID: "p", RunID: "r", Skills: []string{"database"}, Headless: true}, []string{"database", "delete", "--id", "shop"}, "")
	if err != nil || !strings.Contains(out, "write_not_allowed") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

type stubIssuer struct{}

func (stubIssuer) Issue(Principal, string, string, time.Duration) (string, error) {
	return "tok", nil
}
func (stubIssuer) RevokeRun(string) {}
