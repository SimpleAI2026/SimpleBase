//go:build linux

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

// 这些用例调用真 bwrap：没有 bwrap、用户命名空间被禁用或 helper 构建失败时自动 Skip。

func buildHelper(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), HelperName)
	cmd := exec.Command("go", "build", "-o", out, "github.com/linkxzhou/SimpleBase/gosandbox/cmd/gosandbox-inner")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build helper: %v\n%s", err, b)
	}
	return out
}

func linuxPolicy(t *testing.T, mode Mode) (Policy, string) {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	ResetProbeCache()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := Policy{Enabled: true, Mode: mode, Workspace: ws, Timeout: 30 * time.Second, HelperPath: buildHelper(t)}
	if _, err := Probe(context.Background(), p); err != nil {
		t.Skipf("isolation not available on this host: %v", err)
	}
	return p, ws
}

func shell(script string) Command {
	return Command{Argv: []string{"/bin/sh", "-c", script}, Env: []string{"PATH=/usr/bin:/bin"}}
}

func TestLinuxWorkspaceWrite(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	f := filepath.Join(ws, "f.txt")
	res, err := Run(context.Background(), p, shell("echo ok > "+f+" && cat "+f))
	if err != nil || res.ExitCode != 0 || !strings.Contains(string(res.Output), "ok") {
		t.Fatalf("res=%+v err=%v out=%s", res, err, res.Output)
	}
}

func TestLinuxEscapeEtc(t *testing.T) {
	p, _ := linuxPolicy(t, ModeWorkspaceWrite)
	res, err := Run(context.Background(), p, shell("touch /etc/sb-iso-escape"))
	if err != nil || res.ExitCode == 0 {
		t.Fatalf("write to /etc must fail: res=%+v err=%v", res, err)
	}
	if _, serr := os.Stat("/etc/sb-iso-escape"); serr == nil {
		os.Remove("/etc/sb-iso-escape")
		t.Fatal("file appeared on the host")
	}
}

// 工作区外的哨兵在 rm -rf 之后仍在（评测 L3 的回归测试）。
func TestLinuxEscapeSentinel(t *testing.T) {
	p, _ := linuxPolicy(t, ModeWorkspaceWrite)
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "sentinel")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), p, shell("rm -rf "+dir+" /")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(sentinel)
	if err != nil || string(b) != "keep" {
		t.Fatalf("sentinel must survive: %v %q", err, b)
	}
}

func TestLinuxReadOnlyBlocksWorkspaceWrite(t *testing.T) {
	p, ws := linuxPolicy(t, ModeReadOnly)
	res, err := Run(context.Background(), p, shell("touch "+filepath.Join(ws, "f")))
	if err != nil || res.ExitCode == 0 {
		t.Fatalf("read-only must block writes: res=%+v err=%v", res, err)
	}
}

// .git 内会被宿主侧执行的部分只读：hooks、config。
func TestLinuxGitHooksAndConfigReadOnly(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	if err := os.MkdirAll(filepath.Join(ws, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(ws, ".git", "config")
	if err := os.WriteFile(cfg, []byte("[core]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{
		"new hook":       "touch " + filepath.Join(ws, ".git", "hooks", "pre-commit"),
		"config rewrite": "echo '[alias]' >> " + cfg,
	} {
		res, err := Run(context.Background(), p, shell(script))
		if err != nil || res.ExitCode == 0 {
			t.Errorf("%s must fail: res=%+v err=%v out=%s", name, res, err, res.Output)
		}
	}
	if b, _ := os.ReadFile(cfg); string(b) != "[core]\n" {
		t.Fatalf(".git/config changed on the host: %q", b)
	}
}

// D1：.git 的其余部分可写，所以沙盒内 git init / add / commit 能完整工作。
func TestLinuxGitInitAndCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	script := "cd " + ws + " && git init -q . && echo hi > a.txt && git add a.txt && " +
		"git -c user.name=t -c user.email=t@t commit -q -m first && git log --oneline"
	res, err := Run(context.Background(), p, Command{Argv: []string{"/bin/sh", "-c", script},
		Env: []string{"PATH=/usr/bin:/bin", "HOME=" + ws, "GIT_CONFIG_NOSYSTEM=1"}})
	if err != nil || res.ExitCode != 0 || !strings.Contains(string(res.Output), "first") {
		t.Fatalf("git init+commit must work in the sandbox: res=%+v err=%v out=%s", res, err, res.Output)
	}
}

// D7：.git 不存在时不保护（git init 才能成功）；init 完成后的下一条命令起 hooks 即受保护。
func TestLinuxGitHooksProtectedAfterInit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	env := []string{"PATH=/usr/bin:/bin", "HOME=" + ws, "GIT_CONFIG_NOSYSTEM=1"}
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
func TestLinuxForceDenyWorkspaceFiles(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	for _, name := range []string{".gitmodules", ".mcp.json"} {
		if err := os.WriteFile(filepath.Join(ws, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(ws, ".vscode"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{".gitmodules", ".mcp.json", ".vscode/tasks.json"} {
		res, err := Run(context.Background(), p, shell("echo x > "+filepath.Join(ws, target)))
		if err != nil || res.ExitCode == 0 {
			t.Errorf("%s must be read-only: res=%+v err=%v", target, res, err)
		}
	}
	res, err := Run(context.Background(), p, shell("echo ok > "+filepath.Join(ws, "plain.txt")))
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("plain file must stay writable: res=%+v err=%v", res, err)
	}
}

// DisableForceDeny 回到旧行为，并在 Result.Warnings 里如实说明。
func TestLinuxDisableForceDeny(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	if err := os.MkdirAll(filepath.Join(ws, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	p.DisableForceDeny = true
	res, err := Run(context.Background(), p, shell("touch "+filepath.Join(ws, ".git", "hooks", "pre-commit")))
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("hooks must be writable when the table is disabled: res=%+v err=%v out=%s", res, err, res.Output)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("Warnings = %v", res.Warnings)
	}
}

// 显式配置旧值时，整个 .git 仍然只读（想要旧行为的操作者的出口）。
func TestLinuxLegacyWholeGitReadOnly(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	if err := os.MkdirAll(filepath.Join(ws, ".git", "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	p.ReadOnlySubpaths = []string{".git", ".gocloudagent"}
	res, err := Run(context.Background(), p, shell("touch "+filepath.Join(ws, ".git", "refs", "x")))
	if err != nil || res.ExitCode == 0 {
		t.Fatalf("legacy config must keep the whole .git read-only: res=%+v err=%v", res, err)
	}
}

// deny_read 的文件在沙盒里是空的，宿主内容不变。
func TestLinuxDenyReadToken(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	token := filepath.Join(ws, "token")
	if err := os.WriteFile(token, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	p.DenyRead = []string{token}
	res, err := Run(context.Background(), p, shell("cat "+token))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.Output), "TOPSECRET") {
		t.Fatalf("token leaked: %s", res.Output)
	}
	if b, _ := os.ReadFile(token); string(b) != "TOPSECRET" {
		t.Fatalf("host file changed: %q", b)
	}
}

func TestLinuxDenyReadGlobEnv(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	subdir := filepath.Join(ws, "a", "b")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	env1 := filepath.Join(ws, ".env")
	env2 := filepath.Join(subdir, ".env")
	if err := os.WriteFile(env1, []byte("ROOT_SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env2, []byte("NESTED_SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	p.DenyRead = []string{ws + "/**/.env"}
	script := "cat " + env1 + " " + env2 + "; echo NEW > " + subdir + "/fresh.env; cat " + subdir + "/fresh.env"
	res, err := Run(context.Background(), p, shell(script))
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Output)
	if strings.Contains(out, "ROOT_SECRET") || strings.Contains(out, "NESTED_SECRET") {
		t.Fatalf("glob **/.env must hide existing .env files: %s", out)
	}
	if !strings.Contains(out, "NEW") {
		t.Fatalf("snapshot semantics: newly created matching file must not be masked: %s", out)
	}
}

func TestLinuxNoNetwork(t *testing.T) {
	p, _ := linuxPolicy(t, ModeWorkspaceWrite)
	res, err := Run(context.Background(), p, shell("cat /proc/net/dev | grep -v -E '^(Inter|\\s*face|\\s*lo:)' | grep -q . && exit 1; exit 0"))
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("host network interfaces visible: res=%+v err=%v out=%s", res, err, res.Output)
	}
}

// 默认不能新建 AF_UNIX socket（挡 docker.sock、ssh-agent）：用 python 或 perl 探测，都没有则跳过。
func TestLinuxNoUnixSocket(t *testing.T) {
	p, _ := linuxPolicy(t, ModeWorkspaceWrite)
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	res, err := Run(context.Background(), p, shell(py+" -c 'import socket; socket.socket(socket.AF_UNIX)'"))
	if err != nil || res.ExitCode == 0 {
		t.Fatalf("AF_UNIX socket must be denied: res=%+v err=%v out=%s", res, err, res.Output)
	}
}

func TestLinuxTimeoutKillsTree(t *testing.T) {
	p, _ := linuxPolicy(t, ModeWorkspaceWrite)
	p.Timeout = time.Second
	res, err := Run(context.Background(), p, shell("sleep 31 & sleep 32 & wait"))
	if err != nil || !res.TimedOut {
		t.Fatalf("want timeout: res=%+v err=%v", res, err)
	}
	time.Sleep(300 * time.Millisecond)
	if out, _ := exec.Command("pgrep", "-f", "sleep 3[12]").Output(); strings.TrimSpace(string(out)) != "" {
		t.Fatalf("leftover processes: %s", out)
	}
}

// 配了限额但无法生效：如实报告 Enforced=false，命令仍在隔离下运行。
func TestLinuxLimitsUnenforcedReported(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	p.MemoryMaxBytes = 1 << 28
	res, err := Run(context.Background(), p, shell("touch "+filepath.Join(ws, "x")))
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !res.Limits.Configured {
		t.Fatal("Configured must be true")
	}
	if !res.Limits.Enforced && res.Limits.Backend != cgroupNone {
		t.Fatalf("unenforced limits must report backend none: %+v", res.Limits)
	}
}

// helper 装载失败：父进程得到 ErrIsolationUnavailable，而不是命令的退出码。
func TestLinuxHelperSetupFailure(t *testing.T) {
	p, _ := linuxPolicy(t, ModeWorkspaceWrite)
	fake := filepath.Join(t.TempDir(), HelperName)
	script := "#!/bin/sh\necho 'gosandbox-inner: setup failed: simulated' >&2\nexit 71\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p.HelperPath = fake
	_, err := Run(context.Background(), p, shell("echo should-not-run"))
	if err == nil || !strings.Contains(err.Error(), "simulated") {
		t.Fatalf("err = %v", err)
	}
}

// 用户命令自己退出 71，不是装载失败。
func TestLinuxExit71IsNotSetupFailure(t *testing.T) {
	p, _ := linuxPolicy(t, ModeWorkspaceWrite)
	res, err := Run(context.Background(), p, shell("exit 71"))
	if err != nil || res.ExitCode != 71 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
