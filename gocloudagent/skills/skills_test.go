package skills

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

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
	body := Prompt(nil)
	if strings.HasPrefix(body, "---") || strings.Contains(body, "description:") || !strings.HasPrefix(body, "# SimpleBase") {
		t.Fatalf("frontmatter leaked into prompt: %.80q", body)
	}
	if got := stripFrontmatter("no frontmatter"); got != "no frontmatter" {
		t.Fatal(got)
	}
}

// planv6.0 v2 §4.3：每份 SKILL.md 的体积下限与上限。
// 下限防止「没有写 flag 与示例」的薄文本回归；上限与 MaxChars 对齐，
// 超限会被截断导致尾部示例丢失。
func TestSkillBodiesHaveReasonableSize(t *testing.T) {
	const minBytes, maxBytes = 600, MaxChars
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
	if Prompt(nil) == "" {
		t.Fatal("embedded skill index is empty")
	}
	allows, err := EmbeddedAllows()
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
