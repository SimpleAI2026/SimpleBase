package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemplate(t *testing.T, env []string, file, content string) {
	t.Helper()
	dir := PromptsDir(env)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPromptTemplates(t *testing.T) {
	env := testEnv(t)
	writeTemplate(t, env, "review.md", "---\nname: review\ndescription: Review a diff\nargument-hint: <scope>\n---\nReview $1 in $2\n")
	writeTemplate(t, env, "plain.md", "No frontmatter here\n")
	writeTemplate(t, env, "ignored.txt", "not markdown")

	tpls, err := LoadPromptTemplates(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(tpls) != 2 {
		t.Fatalf("templates = %v, want review and plain", PromptTemplateNames(tpls))
	}
	if tpls["review"].Description != "Review a diff" || tpls["review"].ArgumentHint != "<scope>" {
		t.Fatalf("review = %+v", tpls["review"])
	}
	if !strings.Contains(tpls["review"].Content, "Review $1 in $2") {
		t.Fatalf("frontmatter not stripped: %q", tpls["review"].Content)
	}
	if tpls["plain"].Content != "No frontmatter here" {
		t.Fatalf("plain = %q", tpls["plain"].Content)
	}
}

// 目录不存在不是错误（首次使用）。
func TestLoadPromptTemplatesMissingDir(t *testing.T) {
	tpls, err := LoadPromptTemplates(testEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(tpls) != 0 {
		t.Fatalf("templates = %v", tpls)
	}
}

func TestExpandPromptTemplate(t *testing.T) {
	cases := []struct {
		name    string
		content string
		args    []string
		want    string
	}{
		{"positional", "Review $1 on $2", []string{"diff", "main"}, "Review diff on main"},
		{"default used when missing", "Scope: ${1:-whole repo}", nil, "Scope: whole repo"},
		{"default skipped when present", "Scope: ${1:-whole repo}", []string{"this file"}, "Scope: this file"},
		{"default on empty arg", "Scope: ${1:-whole repo}", []string{"   "}, "Scope: whole repo"},
		{"all args", "Explain $@", []string{"a", "b", "c"}, "Explain a b c"},
		{"out of range is empty", "Review $5", []string{"a"}, "Review "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExpandPromptTemplate(PromptTemplate{Content: tc.content}, tc.args)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// `/` 只做文本展开；未知名字原样返回（不能误吞用户输入）。
func TestExpandSlashCommand(t *testing.T) {
	tpls := map[string]PromptTemplate{
		"review": {Name: "review", Content: "Review $1"},
	}
	if got, ok := ExpandSlashCommand(tpls, "/review src/main.go"); !ok || got != "Review src/main.go" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	if got, ok := ExpandSlashCommand(tpls, "/unknown x"); ok || got != "/unknown x" {
		t.Fatalf("unknown template must be passed through: %q ok=%v", got, ok)
	}
	if got, ok := ExpandSlashCommand(tpls, "plain text"); ok || got != "plain text" {
		t.Fatalf("non-slash input must be unchanged: %q ok=%v", got, ok)
	}
}

// 模板展开绝不能授予写权限：这是 §13.7 的硬约束。
func TestPromptTemplateDoesNotGrantSkillPermissions(t *testing.T) {
	env := testEnv(t)
	// 模板文本里写着一条写命令，但展开结果不带任何 skill。
	writeTemplate(t, env, "drop.md", "请执行 database delete --id x\n")
	tpls, err := LoadPromptTemplates(env)
	if err != nil {
		t.Fatal(err)
	}
	expanded, ok := ExpandSlashCommand(tpls, "/drop")
	if !ok {
		t.Fatal("template should expand")
	}
	if !strings.Contains(expanded, "database delete") {
		t.Fatalf("expanded = %q", expanded)
	}
	// 「展开模板不授予权限」的断言放在父包（需要 gocloudagent.AllowArgv）：
	// cli 不能 import gocloudagent，否则父包的门面会造成 import 环。
}
