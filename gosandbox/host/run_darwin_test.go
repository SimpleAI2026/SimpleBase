//go:build darwin

package host

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func darwinPolicy(t *testing.T, mode Mode) (Policy, string) {
	t.Helper()
	if _, err := os.Stat(SeatbeltExecutable); err != nil {
		t.Skip("sandbox-exec not available")
	}
	// 工作区放在 HOME 下，而不是 t.TempDir()：macOS 的临时目录（/private/var/folders）
	// 本来就对沙盒可写，放在里面会让 read-only 的断言失去意义。
	base, err := os.MkdirTemp(os.Getenv("HOME"), ".gosandbox-test-")
	if err != nil {
		t.Skipf("cannot create a workspace under HOME: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	ws, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	return Policy{Enabled: true, Mode: mode, Workspace: ws, Timeout: 20 * time.Second}, ws
}

func sh(script string) Command {
	return Command{Argv: []string{"/bin/sh", "-c", script}, Env: []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}}
}

func TestDarwinWorkspaceWrite(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	f := filepath.Join(ws, "f.txt")
	res, err := Run(context.Background(), p, sh("echo ok > "+f+" && cat "+f))
	if err != nil || res.ExitCode != 0 || !strings.Contains(string(res.Output), "ok") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// 工作区外的文件写不进去，宿主上也不会出现。
func TestDarwinEscapeOutsideWorkspace(t *testing.T) {
	p, _ := darwinPolicy(t, ModeWorkspaceWrite)
	outside := filepath.Join(os.Getenv("HOME"), ".gosandbox-escape-probe")
	defer os.Remove(outside)
	res, err := Run(context.Background(), p, sh("touch "+outside))
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("write outside workspace must fail: %s", res.Output)
	}
	if _, serr := os.Stat(outside); serr == nil {
		t.Fatal("file appeared on the host")
	}
}

func TestDarwinReadOnlyBlocksWorkspaceWrite(t *testing.T) {
	p, ws := darwinPolicy(t, ModeReadOnly)
	res, err := Run(context.Background(), p, sh("touch "+filepath.Join(ws, "f")))
	if err != nil || res.ExitCode == 0 {
		t.Fatalf("read-only must block writes: res=%+v err=%v", res, err)
	}
}

// .git 内会被宿主侧执行的部分只读：hooks、config。
func TestDarwinGitHooksAndConfigReadOnly(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	if err := os.MkdirAll(filepath.Join(ws, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".git", "config"), []byte("[core]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{
		"new hook":        "touch " + filepath.Join(ws, ".git", "hooks", "pre-commit"),
		"config rewrite":  "echo '[alias]' >> " + filepath.Join(ws, ".git", "config"),
		"config replaced": "rm -f " + filepath.Join(ws, ".git", "config"),
	} {
		res, err := Run(context.Background(), p, sh(script))
		if err != nil || res.ExitCode == 0 {
			t.Errorf("%s must fail: res=%+v err=%v out=%s", name, res, err, res.Output)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(ws, ".git", "config")); string(b) != "[core]\n" {
		t.Fatalf(".git/config changed on the host: %q", b)
	}
}

// D1：.git 的其余部分可写，所以沙盒内 git init / add / commit 能完整工作。
func TestDarwinGitInitAndCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	script := "cd " + ws + " && git init -q . && echo hi > a.txt && git add a.txt && " +
		"git -c user.name=t -c user.email=t@t commit -q -m first && git log --oneline"
	res, err := Run(context.Background(), p, Command{Argv: []string{"/bin/sh", "-c", script},
		Env: []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + ws, "GIT_CONFIG_NOSYSTEM=1"}})
	if err != nil || res.ExitCode != 0 || !strings.Contains(string(res.Output), "first") {
		t.Fatalf("git init+commit must work in the sandbox: res=%+v err=%v out=%s", res, err, res.Output)
	}
}

// D7：.git 不存在时不保护（git init 才能成功）；init 完成后的下一条命令起 hooks 即受保护。
func TestDarwinGitHooksProtectedAfterInit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	env := []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + ws, "GIT_CONFIG_NOSYSTEM=1"}
	res, err := Run(context.Background(), p, Command{Argv: []string{"/bin/sh", "-c", "cd " + ws + " && git init -q ."}, Env: env})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("git init must succeed when .git does not exist yet: res=%+v err=%v out=%s", res, err, res.Output)
	}
	hook := filepath.Join(ws, ".git", "hooks", "pre-commit")
	res, err = Run(context.Background(), p, Command{Argv: []string{"/bin/sh", "-c", "echo evil > " + hook}, Env: env})
	if err != nil || res.ExitCode == 0 {
		t.Fatalf("writing a hook after init must fail: res=%+v err=%v out=%s", res, err, res.Output)
	}
	if _, serr := os.Stat(hook); serr == nil {
		t.Fatal("hook appeared on the host")
	}
}

// 工作区根下的 .gitmodules、.mcp.json、.vscode 同样受强制表保护（存在时）。
func TestDarwinForceDenyWorkspaceFiles(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	for _, name := range []string{".gitmodules", ".mcp.json"} {
		if err := os.WriteFile(filepath.Join(ws, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(ws, ".vscode"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{".gitmodules", ".mcp.json", ".vscode/tasks.json"} {
		res, err := Run(context.Background(), p, sh("echo x > "+filepath.Join(ws, target)))
		if err != nil || res.ExitCode == 0 {
			t.Errorf("%s must be read-only: res=%+v err=%v", target, res, err)
		}
	}
	// 普通文件不受影响。
	res, err := Run(context.Background(), p, sh("echo ok > "+filepath.Join(ws, "plain.txt")))
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("plain file must stay writable: res=%+v err=%v", res, err)
	}
}

// DisableForceDeny 回到旧行为，并在 Result.Warnings 里如实说明。
func TestDarwinDisableForceDeny(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	if err := os.MkdirAll(filepath.Join(ws, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	p.DisableForceDeny = true
	res, err := Run(context.Background(), p, sh("touch "+filepath.Join(ws, ".git", "hooks", "pre-commit")))
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("hooks must be writable when the table is disabled: res=%+v err=%v out=%s", res, err, res.Output)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("Warnings = %v", res.Warnings)
	}
}

// 显式配置旧值时，整个 .git 仍然只读（想要旧行为的操作者的出口）。
func TestDarwinLegacyWholeGitReadOnly(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	if err := os.MkdirAll(filepath.Join(ws, ".git", "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	p.ReadOnlySubpaths = []string{".git", ".gocloudagent"}
	res, err := Run(context.Background(), p, sh("touch "+filepath.Join(ws, ".git", "refs", "x")))
	if err != nil || res.ExitCode == 0 {
		t.Fatalf("legacy config must keep the whole .git read-only: res=%+v err=%v", res, err)
	}
}

func TestDarwinNoNetwork(t *testing.T) {
	p, _ := darwinPolicy(t, ModeWorkspaceWrite)
	res, err := Run(context.Background(), p, sh("/usr/bin/curl -sS --max-time 5 http://example.com >/dev/null"))
	if err != nil || res.ExitCode == 0 {
		t.Fatalf("network must be denied: res=%+v err=%v out=%s", res, err, res.Output)
	}
}

func TestDarwinDenyRead(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	secretDir := filepath.Join(ws, "secrets")
	if err := os.Mkdir(secretDir, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(secretDir, "token")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	p.DenyRead = []string{secretDir}
	res, err := Run(context.Background(), p, sh("cat "+secret))
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 || strings.Contains(string(res.Output), "TOPSECRET") {
		t.Fatalf("deny_read must hide the file: res=%+v out=%s", res, res.Output)
	}
}

func TestDarwinDenyReadGlobRecursive(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	subdir := filepath.Join(ws, "x", "y")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	sec1 := filepath.Join(ws, "k.pem")
	sec2 := filepath.Join(subdir, "deep.pem")
	if err := os.WriteFile(sec1, []byte("PRIVATE_KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sec2, []byte("NESTED_KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	p.DenyRead = []string{ws + "/**/*.pem"}
	script := "cat " + sec1 + " " + sec2 + "; echo FRESH > " + subdir + "/new.pem; cat " + subdir + "/new.pem"
	res, err := Run(context.Background(), p, sh(script))
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Output)
	if strings.Contains(out, "PRIVATE_KEY") || strings.Contains(out, "NESTED_KEY") {
		t.Fatalf("glob **/*.pem must deny both existing files: %s", out)
	}
	if strings.Contains(out, "FRESH") {
		t.Fatalf("macOS regex must cover newly created files (future semantics): %s", out)
	}
}

// 哨兵在 rm -rf 之后仍在（工作区外）。
func TestDarwinEscapeSentinel(t *testing.T) {
	p, _ := darwinPolicy(t, ModeWorkspaceWrite)
	dir, err := os.MkdirTemp(os.Getenv("HOME"), ".gosandbox-sentinel-")
	if err != nil {
		t.Skip("cannot create sentinel under HOME")
	}
	defer os.RemoveAll(dir)
	sentinel := filepath.Join(dir, "sentinel")
	if err := os.WriteFile(sentinel, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), p, sh("rm -rf "+dir)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel must survive: %v", err)
	}
}

func TestDarwinTimeoutKillsTree(t *testing.T) {
	p, _ := darwinPolicy(t, ModeWorkspaceWrite)
	p.Timeout = time.Second
	start := time.Now()
	res, err := Run(context.Background(), p, sh("sleep 30 & sleep 30 & wait"))
	if err != nil || !res.TimedOut {
		t.Fatalf("want timeout: res=%+v err=%v", res, err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("timeout took too long: %s", time.Since(start))
	}
}

// 限额在 macOS 上恒为未生效，并如实报告。
func TestDarwinLimitsUnenforced(t *testing.T) {
	p, _ := darwinPolicy(t, ModeWorkspaceWrite)
	p.MemoryMaxBytes = 1 << 28
	res, err := Run(context.Background(), p, sh("true"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Limits.Configured || res.Limits.Enforced {
		t.Fatalf("limits = %+v", res.Limits)
	}
}
