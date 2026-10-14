package cloudagent

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/sbcli"
)

func TestResolveAtTokenAndSkills(t *testing.T) {
	id, isAgent := ResolveAtToken("数据库", []string{"数据库"})
	if id != "database" || isAgent {
		t.Fatalf("alias wins over agent name: %s %v", id, isAgent)
	}
	id, isAgent = ResolveAtToken("Database", []string{"Database"})
	if id != "" || !isAgent {
		t.Fatalf("@Database is an agent: %s %v", id, isAgent)
	}
	id, _ = ResolveAtToken("@云函数", nil)
	if id != "gofunction" {
		t.Fatal(id)
	}
	db, err := NormalizeSkills(nil, ModuleDatabase)
	if err != nil || !reflect.DeepEqual(db, []string{"database"}) {
		t.Fatalf("module fallback %v %v", db, err)
	}
	if !AllowArgv(db, []string{"sql", "exec"}) || AllowArgv(db, []string{"function", "delete"}) {
		t.Fatal("database prefixes")
	}
	both, err := NormalizeSkills([]string{"database", "gofunction", "database"}, ModuleGeneral)
	if err != nil || !AllowArgv(both, []string{"function", "delete"}) || !AllowArgv(both, []string{"sql", "query"}) {
		t.Fatalf("union %v %v", both, err)
	}
	none, err := NormalizeSkills(nil, ModuleGeneral)
	if err != nil || len(none) != 0 || AllowArgv(none, []string{"database", "list"}) {
		t.Fatalf("general %v", none)
	}
	if _, err := NormalizeSkills([]string{"nope"}, ModuleGeneral); err == nil || !strings.Contains(err.Error(), "unknown_skill") {
		t.Fatal(err)
	}
}

func TestStatisticalModulesUseExistingSkills(t *testing.T) {
	for _, module := range []string{"kv", "gofunction", "cron"} {
		if !KnownModule(module) {
			t.Fatalf("module %s not listed", module)
		}
		skills, err := NormalizeSkills(nil, module)
		if err != nil || !reflect.DeepEqual(skills, []string{module}) {
			t.Fatalf("module %s skills=%v err=%v", module, skills, err)
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

// planv6.0 v2 §4.2：`<!-- readonly: … -->` 标注必须与 sbcli.CommandReadOnly
// 判定一致，避免文档与宿主判定漂移。
func TestSkillReadOnlyAnnotationsMatchSbcli(t *testing.T) {
	annotations, err := EmbeddedSkillReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := annotations["index"]; ok {
		t.Fatal("索引不应有 readonly 标注")
	}
	// kv 全是写命令（exec 语义），不应出现标注。
	if _, ok := annotations["kv"]; ok {
		t.Fatal("kv 不应对 readonly 做任何声明")
	}
	for id, cmds := range annotations {
		allows := skillPrefixes[id]
		for _, cmd := range cmds {
			parts := strings.SplitN(cmd, " ", 2)
			if len(parts) != 2 {
				t.Fatalf("skill %s 的 readonly 项格式错误: %q", id, cmd)
			}
			// 标注的命令必须落在该 skill 的 allow 前缀内。
			allowed := false
			for _, p := range allows {
				if p == parts[0] {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Fatalf("skill %s 标注了不属于自己的命令 %q", id, cmd)
			}
			if !sbcli.CommandReadOnlyWithArgv(parts) {
				t.Fatalf("skill %s 标注 %q 为只读，但 sbcli 判定为写", id, cmd)
			}
		}
	}
	// 反向：每个 skill 里实际的只读命令都应被标出（防止漏标导致模型误判）。
	for id := range skillPrefixes {
		body := readSkill(skillDocPath(id))
		declared := map[string]bool{}
		for _, c := range annotations[id] {
			declared[c] = true
		}
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "simplebase ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			if !sbcli.CommandReadOnly(fields[1], fields[2]) {
				continue
			}
			if !declared[fields[1]+" "+fields[2]] {
				t.Fatalf("skill %s 的只读命令 %q 未在 readonly 标注中登记", id, fields[1]+" "+fields[2])
			}
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

// TestSkillCommandsDispatchable：skill 正文里出现的每个
// `simplebase <resource> <action>` 都必须能被 sbcli 分发（planv6.0 §3.6）。
// 防止 skill 文档示例与 CLI 实现漂移。
func TestSkillCommandsDispatchable(t *testing.T) {
	allows, err := EmbeddedSkillAllows()
	if err != nil {
		t.Fatal(err)
	}
	for id := range skillPrefixes {
		body := readSkill(skillDocPath(id))
		if body == "" {
			t.Fatalf("skill %s 正文缺失", id)
		}
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "simplebase ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			resource, action := fields[1], fields[2]
			if !sbcli.DispatchKnown(resource, action) {
				t.Fatalf("skill %s 的示例无法分发: %q", id, line)
			}
		}
	}
	_ = allows
}

func TestDestructiveArgv(t *testing.T) {
	if !destructiveArgv([]string{"database", "delete", "--id", "shop"}, "") {
		t.Fatal("delete")
	}
	if destructiveArgv([]string{"sql", "exec", "--database", "d", "--statement", "INSERT INTO t VALUES (1)"}, "") {
		t.Fatal("insert")
	}
	if !destructiveArgv([]string{"sql", "exec", "--database", "d", "--statement", "DELETE FROM t"}, "") {
		t.Fatal("sql delete")
	}
	if destructiveArgv([]string{"kv", "exec", "--command", "SET", "--arg", "a", "--arg", "1"}, "") {
		t.Fatal("set")
	}
	if !destructiveArgv([]string{"kv", "exec", "--command", "DEL", "--arg", "a"}, "") {
		t.Fatal("del")
	}
	if !destructiveArgv([]string{"sql", "exec", "--database", "d", "--statement", "CALL something()"}, "") {
		t.Fatal("unsure")
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
	rt := &Runtime{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}}
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
	rt := &Runtime{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}}
	ctx := withRunContext(context.Background(), RunContext{Headless: true, Writable: false})
	out, err := rt.execSimplebase(ctx, RunRequest{ProjectID: "p", RunID: "r", Skills: []string{"database"}, Headless: true}, []string{"database", "delete", "--id", "shop"}, "")
	if err != nil || !strings.Contains(out, "write_not_allowed") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

type stubIssuer struct{}

func (stubIssuer) Issue(auth.Principal, string, string, time.Duration) (string, error) {
	return "tok", nil
}
func (stubIssuer) RevokeRun(string) {}
