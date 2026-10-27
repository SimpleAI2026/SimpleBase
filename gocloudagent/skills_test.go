package gocloudagent

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
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
	if !AllowArgv(SkillsForModule(ModuleDatabase), []string{"sql", "exec"}) || AllowArgv(SkillsForModule(ModuleDatabase), []string{"function", "delete"}) {
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
		if ModuleTemplate(module) == generalModulePrompt {
			t.Fatalf("module %s uses general prompt", module)
		}
		if len(DefaultToolsForModule(module)) != 0 {
			t.Fatalf("module %s advertises unimplemented built-in tools", module)
		}
	}
	if KnownModule("users") || KnownSkill("users") {
		t.Fatal("users is not a delegable module or skill")
	}
}

func TestDatabaseSkillUsesSupportedDataModel(t *testing.T) {
	body := readSkill(skillDocPath("database"))
	if !strings.Contains(body, "--data-model sql") || strings.Contains(body, "--data-model relational") {
		t.Fatalf("database create 示例必须使用 sql: %s", body)
	}
}

// 顶层 SKILL.md 带 frontmatter（skill-creator 规范），注入 prompt 时必须剥离。
func TestSkillIndexFrontmatterStripped(t *testing.T) {
	raw, err := skillFS.ReadFile(skillIndexPath)
	if err != nil || !strings.HasPrefix(string(raw), "---\nname: simplebase\n") {
		t.Fatalf("index frontmatter missing: err=%v", err)
	}
	body := SkillPrompt(nil)
	if strings.HasPrefix(body, "---") || strings.Contains(body, "description:") || !strings.HasPrefix(body, "# SimpleBase") {
		t.Fatalf("frontmatter leaked into prompt: %.80q", body)
	}
	if got := stripFrontmatter("no frontmatter"); got != "no frontmatter" {
		t.Fatal(got)
	}
}

// planv6.0 v2 §4.3：每份 SKILL.md 的体积下限与上限。
// 下限防止「没有写 flag 与示例」的薄文本回归；上限与 maxSkillChars 对齐，
// 超限会被截断导致尾部示例丢失。
func TestSkillBodiesHaveReasonableSize(t *testing.T) {
	const minBytes, maxBytes = 600, maxSkillChars
	if got := len(readSkill(skillIndexPath)); got < 300 || got > maxBytes {
		t.Fatalf("index size=%d", got)
	}
	for id := range skillPrefixes {
		body := readSkill(skillDocPath(id))
		if n := len(body); n < minBytes || n > maxBytes {
			t.Fatalf("skill %s size=%d want %d..%d", id, n, minBytes, maxBytes)
		}
	}
}

func TestEmbeddedAllowsMatchPrefixes(t *testing.T) {
	if SkillPrompt(nil) == "" {
		t.Fatal("embedded skill index is empty")
	}
	allows, err := EmbeddedSkillAllows()
	if err != nil {
		t.Fatal(err)
	}
	if len(allows) != len(skillPrefixes)+1 {
		t.Fatalf("embedded skills=%d want %d", len(allows), len(skillPrefixes)+1)
	}
	if _, ok := allows["index"]; !ok {
		t.Fatal("embedded skill index missing")
	}
	if len(allows["index"]) != 0 {
		t.Fatalf("index allow %v", allows["index"])
	}
	for id, want := range skillPrefixes {
		got := append([]string(nil), allows[id]...)
		exp := append([]string(nil), want...)
		sort.Strings(got)
		sort.Strings(exp)
		if !reflect.DeepEqual(got, exp) {
			t.Fatalf("skill %s allow %v want %v", id, got, exp)
		}
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
