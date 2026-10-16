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

func TestLinuxGitReadonly(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	if err := os.MkdirAll(filepath.Join(ws, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), p, shell("mkdir "+filepath.Join(ws, ".git", "hooks")))
	if err != nil || res.ExitCode == 0 {
		t.Fatalf(".git must be read-only: res=%+v err=%v", res, err)
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
