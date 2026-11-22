package agentbridge

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	agentskills "github.com/linkxzhou/SimpleBase/internal/agentbridge/agentskills"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

func TestSkillsForModule(t *testing.T) {
	if !reflect.DeepEqual(SkillsForModule(ModuleDatabase), []string{"database"}) {
		t.Fatalf("database module: %v", SkillsForModule(ModuleDatabase))
	}
	// general 加载全部已注册技能，路由选通用助手后仍可执行跨模块任务。
	if got := SkillsForModule(ModuleGeneral); !reflect.DeepEqual(got, agentskills.IDs()) {
		t.Fatalf("general skills: %v", got)
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

func TestGeneralSkillsAllowEveryModuleCommand(t *testing.T) {
	skills := SkillsForModule(ModuleGeneral)
	for _, argv := range [][]string{
		{"database", "list"},
		{"function", "list"},
		{"cron", "list"},
		{"object", "list"},
		{"site", "list"},
		{"sandbox", "list"},
		{"kv", "list"},
		{"log", "list"},
		{"sql", "exec"},
	} {
		if !agentskills.AllowArgv(skills, argv) {
			t.Errorf("general must allow %v", argv)
		}
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
	rt := &RuntimeProvider{}
	out, err := rt.execSimplebase(context.Background(), RunRequest{RunID: "r", Context: RunContext{ProjectID: "p"}}, []string{"database", "list", "SIMPLEBASE_TOKEN"}, "")
	if err != nil || !strings.Contains(out, "token_in_argv") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

func TestHeadlessDestructiveUnavailable(t *testing.T) {
	rt := &RuntimeProvider{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}, Policy: testPolicy{}}
	// 助手写门禁先于 headless 判定，因此这里需要可写助手才能走到确认分支。
	ctx := withRunContext(context.Background(), RunContext{ProjectID: "p1", Headless: true, Writable: true, Skills: []string{"database"}})
	req := makeAgentRequest(true)
	req.Headless = true
	out, err := rt.execSimplebase(ctx, req, []string{"database", "delete", "--id", "shop"}, "")
	if err != nil || !strings.Contains(out, "confirmation_unavailable") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

// planv6.0 v2 §3.8：只读助手的写命令在 headless 与交互两种模式下都被拒，
// 且都在确认之前拦截。
func TestReadonlyAgentBeatsHeadlessCheck(t *testing.T) {
	rt := &RuntimeProvider{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}, Policy: testPolicy{}}
	ctx := withRunContext(context.Background(), RunContext{ProjectID: "p1", Headless: true, Writable: false, Skills: []string{"database"}})
	req := makeAgentRequest(false)
	req.Headless = true
	out, err := rt.execSimplebase(ctx, req, []string{"database", "delete", "--id", "shop"}, "")
	if err != nil || !strings.Contains(out, "write_not_allowed") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

type stubIssuer struct{}

func (stubIssuer) Issue(auth.Principal, string, string, time.Duration) (string, error) {
	return "tok", nil
}
func (stubIssuer) RevokeRun(string) {}
