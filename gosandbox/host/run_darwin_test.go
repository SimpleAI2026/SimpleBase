//go:build darwin

package host

import (
	"context"
	"os"
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

func TestDarwinGitSubpathReadOnly(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	if err := os.MkdirAll(filepath.Join(ws, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), p, sh("touch "+filepath.Join(ws, ".git", "hooks", "pre-commit")))
	if err != nil || res.ExitCode == 0 {
		t.Fatalf(".git must be read-only: res=%+v err=%v", res, err)
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
