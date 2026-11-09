package gocloudagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cli"
)

// TestPromptTemplateExpansionDoesNotGrantSkillPermission 锁定 §13.7 的硬约束：
// `/name` 只做文本展开，绝不授予写权限；授权只看 RunRequest.Skills。
//
// 断言放在父包（而不是 cli 子包）：需要同时引用 gocloudagent.AllowArgv 与
// cli.PromptTemplate，而 cli 不能反向 import gocloudagent（父包门面会成环）。
func TestPromptTemplateExpansionDoesNotGrantSkillPermission(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, ".gocloudagent")
	if err := os.MkdirAll(filepath.Join(home, "prompts"), 0o700); err != nil {
		t.Fatal(err)
	}
	// 模板正文里明写一条写命令——展开它不应带来任何授权。
	tpl := "请执行 database delete --id x\n"
	if err := os.WriteFile(filepath.Join(home, "prompts", "drop.md"), []byte(tpl), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + dir, "GOCLOUDAGENT_HOME=" + home}

	templates, err := cli.LoadPromptTemplates(env)
	if err != nil {
		t.Fatal(err)
	}
	expanded, ok := cli.ExpandSlashCommand(templates, "/drop")
	if !ok || !strings.Contains(expanded, "database delete") {
		t.Fatalf("template should expand with its command text, got %q ok=%v", expanded, ok)
	}

	// 关键断言：没有 skill 时该写命令仍被拒。
	if AllowArgv(nil, []string{"database", "delete", "--id", "x"}) {
		t.Fatal("expanding a template must never make a write command allowed")
	}
	// 反向对照：带上 skill 才放行（证明拒因是 skills 而非命令本身非法）。
	if !AllowArgv([]string{"database"}, []string{"database", "delete", "--id", "x"}) {
		t.Fatal("the same command should be allowed once the skill is loaded")
	}
}

// TestCLIFacadeIsWired 确认父包门面确实转发到子包（入口依赖的稳定性）。
func TestCLIFacadeIsWired(t *testing.T) {
	dir := t.TempDir()
	out, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errf, err := os.Create(filepath.Join(dir, "err"))
	if err != nil {
		t.Fatal(err)
	}
	defer errf.Close()

	env := []string{"HOME=" + dir, "GOCLOUDAGENT_HOME=" + filepath.Join(dir, ".gocloudagent")}
	SetCLIVersion("test-version")
	// `version` 子命令不触网、不需要配置，用来验证门面通了。
	if code := RunCLI(env, []string{"version"}, out, errf); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	b, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "test-version") {
		t.Fatalf("version output = %q, want the injected version", string(b))
	}
}
