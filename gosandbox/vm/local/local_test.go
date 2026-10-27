//go:build unix

package local

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// newTestDriver 构造一个绕过冒烟测试的驱动，run 可替换。
func newTestDriver(t *testing.T, run func(context.Context, host.Policy, host.Command) (host.Result, error)) *Driver {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if run == nil {
		run = func(context.Context, host.Policy, host.Command) (host.Result, error) { return host.Result{}, nil }
	}
	return &Driver{opts: Options{Root: root, MaxFileBytes: 16, MaxOutputBytes: 8}, run: run}
}

func target(name string) vm.Target {
	return vm.Target{Name: name, Spec: vm.Spec{Workdir: "/workspace", CPUs: 1, MemoryMiB: 256, Network: "none"}}
}

func TestNewRejectsRelativeRoot(t *testing.T) {
	if _, err := New(context.Background(), Options{Root: "relative"}); !errors.Is(err, vm.ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if _, err := New(context.Background(), Options{}); !errors.Is(err, vm.ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestLifecycleStatusStopRemove(t *testing.T) {
	d := newTestDriver(t, nil)
	ctx := context.Background()
	tg := target("sbx-a")
	if st, _ := d.Status(ctx, tg.Name); st != vm.VMAbsent {
		t.Fatalf("fresh vm status = %q", st)
	}
	if err := d.Ensure(ctx, tg); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.Status(ctx, tg.Name); st != vm.VMRunning {
		t.Fatalf("after ensure = %q", st)
	}
	if err := d.Stop(ctx, tg.Name); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.Status(ctx, tg.Name); st != vm.VMStopped {
		t.Fatalf("after stop = %q", st)
	}
	if err := d.Ensure(ctx, tg); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.Status(ctx, tg.Name); st != vm.VMRunning {
		t.Fatalf("ensure must clear stopped marker, got %q", st)
	}
	if err := d.Remove(ctx, tg.Name); err != nil {
		t.Fatal(err)
	}
	if err := d.Remove(ctx, tg.Name); err != nil {
		t.Fatalf("remove must be idempotent: %v", err)
	}
	if st, _ := d.Status(ctx, tg.Name); st != vm.VMAbsent {
		t.Fatalf("after remove = %q", st)
	}
	if err := d.Stop(ctx, "sbx-never"); err != nil {
		t.Fatalf("stop of absent vm must succeed: %v", err)
	}
}

func TestInvalidVMNameRejected(t *testing.T) {
	d := newTestDriver(t, nil)
	for _, name := range []string{"", "../x", "a/b", "A", ".."} {
		if err := d.Ensure(context.Background(), target(name)); !errors.Is(err, vm.ErrBackend) {
			t.Fatalf("name %q: want ErrBackend, got %v", name, err)
		}
	}
}

func TestPublicNetworkRejected(t *testing.T) {
	d := newTestDriver(t, nil)
	tg := target("sbx-net")
	tg.Spec.Network = "public"
	if err := d.Ensure(context.Background(), tg); !errors.Is(err, vm.ErrBackend) {
		t.Fatalf("public network must be rejected, got %v", err)
	}
}

func TestFileRoundTripAndPaths(t *testing.T) {
	d := newTestDriver(t, nil)
	ctx := context.Background()
	tg := target("sbx-files")
	if err := d.WriteFile(ctx, tg, "/workspace/a/b.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	got, err := d.ReadFile(ctx, tg, "/workspace/a/b.txt")
	if err != nil || string(got) != "hello" {
		t.Fatalf("read = %q, %v", got, err)
	}
	if _, err := d.ReadFile(ctx, tg, "/workspace/missing"); !errors.Is(err, vm.ErrFileNotFound) {
		t.Fatalf("want ErrFileNotFound, got %v", err)
	}
	entries, err := d.ListDir(ctx, tg, "/workspace")
	if err != nil || len(entries) != 1 || entries[0].Name != "a" || entries[0].Kind != "directory" ||
		entries[0].Path != "/workspace/a" {
		t.Fatalf("list = %+v, %v", entries, err)
	}
	if err := d.RemoveFile(ctx, tg, "/workspace/a/b.txt"); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveFile(ctx, tg, "/workspace/a/b.txt"); err != nil {
		t.Fatalf("remove of missing file must succeed: %v", err)
	}
	for _, bad := range []string{"/etc/passwd", "/workspace/../etc/passwd", "/workspacex/a", "relative"} {
		if err := d.WriteFile(ctx, tg, bad, []byte("x")); !errors.Is(err, vm.ErrBackend) {
			t.Fatalf("write %q: want ErrBackend, got %v", bad, err)
		}
		if _, err := d.ReadFile(ctx, tg, bad); !errors.Is(err, vm.ErrBackend) {
			t.Fatalf("read %q: want ErrBackend, got %v", bad, err)
		}
	}
	if err := d.WriteFile(ctx, tg, "/workspace", []byte("x")); !errors.Is(err, vm.ErrBackend) {
		t.Fatalf("writing the workspace itself must fail, got %v", err)
	}
}

func TestReadFileLimit(t *testing.T) {
	d := newTestDriver(t, nil)
	ctx := context.Background()
	tg := target("sbx-limit")
	if err := d.WriteFile(ctx, tg, "/workspace/big", []byte(strings.Repeat("x", 100))); err != nil {
		t.Fatal(err)
	}
	got, err := d.ReadFile(ctx, tg, "/workspace/big")
	if err != nil || len(got) != 17 { // MaxFileBytes+1，上层据此判断截断
		t.Fatalf("len = %d, err = %v", len(got), err)
	}
}

// 沙盒内命令可以在工作区里放符号链接；父进程的读写不得跟出去。
func TestSymlinkEscapeBlocked(t *testing.T) {
	d := newTestDriver(t, nil)
	ctx := context.Background()
	tg := target("sbx-link")
	if err := d.Ensure(ctx, tg); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("top-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws, _ := d.wsDir(tg.Name)
	if err := os.Symlink(secret, filepath.Join(ws, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(secret), filepath.Join(ws, "dirlink")); err != nil {
		t.Fatal(err)
	}
	if b, err := d.ReadFile(ctx, tg, "/workspace/link"); err == nil {
		t.Fatalf("read through symlink must fail, got %q", b)
	}
	if err := d.WriteFile(ctx, tg, "/workspace/dirlink/pwned", []byte("x")); err == nil {
		t.Fatal("write through symlinked dir must fail")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(secret), "pwned")); err == nil {
		t.Fatal("file escaped the workspace")
	}
	if _, err := d.ListDir(ctx, tg, "/workspace/dirlink"); err == nil {
		t.Fatal("list through symlinked dir must fail")
	}
}

func TestExecBuildsPolicyAndCommand(t *testing.T) {
	var gotPolicy host.Policy
	var gotCmd host.Command
	d := newTestDriver(t, func(_ context.Context, p host.Policy, c host.Command) (host.Result, error) {
		gotPolicy, gotCmd = p, c
		return host.Result{Output: []byte("out"), ExitCode: 3}, nil
	})
	ctx := context.Background()
	tg := target("sbx-exec")
	res, err := d.Exec(ctx, tg, vm.ExecSpec{Shell: "echo hi", Env: map[string]string{"FOO": "bar"}, Timeout: 7 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Stdout) != "out" || res.ExitCode != 3 || res.TimedOut {
		t.Fatalf("result = %+v", res)
	}
	if !gotPolicy.Isolates() || gotPolicy.Mode != host.ModeWorkspaceWrite || gotPolicy.Timeout != 7*time.Second {
		t.Fatalf("policy = %+v", gotPolicy)
	}
	if gotPolicy.MemoryMaxBytes != 256<<20 || gotPolicy.CPUMax != "100000 100000" || gotPolicy.MaxOutputBytes != 9 {
		t.Fatalf("limits = %+v", gotPolicy)
	}
	ws, _ := d.wsDir(tg.Name)
	if gotPolicy.Workspace != ws || gotCmd.Dir != ws {
		t.Fatalf("workspace/dir = %q / %q, want %q", gotPolicy.Workspace, gotCmd.Dir, ws)
	}
	if strings.Join(gotCmd.Argv, "|") != "/bin/sh|-c|echo hi" {
		t.Fatalf("argv = %v", gotCmd.Argv)
	}
	env := strings.Join(gotCmd.Env, "\n")
	if !strings.Contains(env, "FOO=bar") || !strings.Contains(env, "PATH=") {
		t.Fatalf("env = %q", env)
	}
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "must-not-leak")
	if _, err := d.Exec(ctx, tg, vm.ExecSpec{Shell: "true"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(gotCmd.Env, "\n"), "must-not-leak") {
		t.Fatal("server environment leaked into the sandbox")
	}
}

func TestExecCwdValidation(t *testing.T) {
	d := newTestDriver(t, nil)
	ctx := context.Background()
	tg := target("sbx-cwd")
	if err := d.WriteFile(ctx, tg, "/workspace/sub/f", []byte("x")); err != nil {
		t.Fatal(err)
	}
	ws, _ := d.wsDir(tg.Name)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(ws, "esc")); err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{"/etc", "/workspace/missing", "/workspace/esc", "/workspace/sub/f"} {
		res, err := d.Exec(ctx, tg, vm.ExecSpec{Shell: "true", Cwd: cwd})
		if err != nil || res.ExitCode == 0 || len(res.Stdout)+len(res.Stderr) == 0 {
			t.Fatalf("cwd %q must fail as a command result, got %+v, %v", cwd, res, err)
		}
	}
	if res, err := d.Exec(ctx, tg, vm.ExecSpec{Shell: "true", Cwd: "/workspace/sub"}); err != nil || res.ExitCode != 0 {
		t.Fatalf("valid cwd failed: %+v, %v", res, err)
	}
}

func TestExecIsolationUnavailableIsBackendError(t *testing.T) {
	d := newTestDriver(t, func(context.Context, host.Policy, host.Command) (host.Result, error) {
		return host.Result{}, host.ErrIsolationUnavailable
	})
	_, err := d.Exec(context.Background(), target("sbx-fc"), vm.ExecSpec{Shell: "true"})
	if !errors.Is(err, vm.ErrBackend) {
		t.Fatalf("fail-closed must surface as ErrBackend, got %v", err)
	}
}

func TestExecTimeoutIsResultNotError(t *testing.T) {
	d := newTestDriver(t, func(context.Context, host.Policy, host.Command) (host.Result, error) {
		return host.Result{ExitCode: -1, TimedOut: true}, nil
	})
	res, err := d.Exec(context.Background(), target("sbx-to"), vm.ExecSpec{Shell: "sleep 9", Timeout: time.Second})
	if err != nil || !res.TimedOut || res.ExitCode != -1 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestExecMissingCommand(t *testing.T) {
	d := newTestDriver(t, nil)
	res, err := d.Exec(context.Background(), target("sbx-nf"), vm.ExecSpec{Cmd: "definitely-not-a-command-xyz"})
	if err != nil || res.ExitCode != 127 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestDenyAroundHidesSiblingsButKeepsWorkspace(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "sandbox", "vmA", "ws")
	for _, p := range []string{keep, filepath.Join(root, "sandbox", "vmB", "ws"), filepath.Join(root, "db"), filepath.Join(root, "sandbox", "vmA", "stopped.d")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := denyAround(root, keep)
	has := func(suffix string) bool {
		for _, g := range got {
			if strings.HasSuffix(g, suffix) {
				return true
			}
		}
		return false
	}
	if !has("/db") || !has("/sandbox/vmB") || !has("/sandbox/vmA/stopped.d") {
		t.Fatalf("deny = %v", got)
	}
	for _, g := range got {
		if g == keep || strings.HasPrefix(keep, g+string(filepath.Separator)) {
			t.Fatalf("workspace must stay reachable, deny contains %q", g)
		}
	}
	if d := denyAround(root, filepath.Join(t.TempDir(), "elsewhere")); len(d) != 1 || d[0] != filepath.Clean(root) {
		t.Fatalf("unrelated workspace must deny the whole dir, got %v", d)
	}
}

func TestPolicyDenyReadCoversProtectedPaths(t *testing.T) {
	d := newTestDriver(t, nil)
	data := t.TempDir()
	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	d.opts.ProtectDirs = []string{data}
	d.opts.ProtectFiles = []string{cfgFile}
	ws, _ := d.wsDir("sbx-p")
	_ = os.MkdirAll(ws, 0o755)
	deny := d.denyRead(ws)
	realData, _ := filepath.EvalSymlinks(data)
	realCfg, _ := filepath.EvalSymlinks(cfgFile)
	var haveData, haveCfg bool
	for _, p := range deny {
		haveData = haveData || p == realData
		haveCfg = haveCfg || p == realCfg
	}
	if !haveData || !haveCfg {
		t.Fatalf("deny = %v, want %q and %q", deny, realData, realCfg)
	}
}

// ---- 真隔离（macOS Seatbelt / Linux bwrap）；环境不具备时跳过 ----

func realDriver(t *testing.T) *Driver {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no host isolation on " + runtime.GOOS)
	}
	d, err := New(context.Background(), Options{Root: t.TempDir(), MaxFileBytes: 1 << 20, MaxOutputBytes: 1 << 16})
	if err != nil {
		t.Skipf("host isolation unavailable here: %v", err)
	}
	return d
}

func TestRealIsolationRunsAndConfines(t *testing.T) {
	d := realDriver(t)
	ctx := context.Background()
	tg := target("sbx-real")
	res, err := d.Exec(ctx, tg, vm.ExecSpec{Shell: "echo hello && pwd && echo data > out.txt", Timeout: 20 * time.Second})
	if err != nil || res.ExitCode != 0 || !strings.Contains(string(res.Stdout), "hello") {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if b, err := d.ReadFile(ctx, tg, "/workspace/out.txt"); err != nil || strings.TrimSpace(string(b)) != "data" {
		t.Fatalf("file written in sandbox not visible: %q, %v", b, err)
	}
	// 不能放在 TMPDIR 下：Seatbelt/bwrap 都为编译器放开了临时目录的写入。
	home, herr := os.UserHomeDir()
	if herr != nil {
		t.Skip("no home directory")
	}
	sentinelDir, err := os.MkdirTemp(home, ".gosandbox-sentinel-")
	if err != nil {
		t.Skipf("cannot create a sentinel under HOME: %v", err)
	}
	defer os.RemoveAll(sentinelDir)
	sentinel := filepath.Join(sentinelDir, "sentinel")
	res, err = d.Exec(ctx, tg, vm.ExecSpec{Shell: "echo pwned > '" + sentinel + "'", Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 {
		t.Fatal("write outside the workspace must fail")
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("sandbox wrote outside its workspace")
	}
}

func TestRealIsolationHidesProtectedData(t *testing.T) {
	d := realDriver(t)
	data := t.TempDir()
	secret := filepath.Join(data, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP-SECRET-VALUE"), 0o600); err != nil {
		t.Fatal(err)
	}
	d.opts.ProtectDirs = []string{data}
	res, err := d.Exec(context.Background(), target("sbx-hide"), vm.ExecSpec{Shell: "cat '" + secret + "'", Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.Stdout), "TOP-SECRET-VALUE") {
		t.Fatalf("protected data readable from the sandbox: %q", res.Stdout)
	}
}

// 云沙盒继承 host 的新默认：整个 .git 可写所以能 git commit，但 hooks 在 init 之后的命令里只读（planv6.3 D1/D7）。
func TestRealIsolationGitCommitButHooksProtected(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	d := realDriver(t)
	ctx := context.Background()
	tg := target("sbx-git")
	res, err := d.Exec(ctx, tg, vm.ExecSpec{Timeout: 30 * time.Second, Shell: "git init -q . && echo hi > a.txt && git add a.txt && " +
		"git -c user.name=t -c user.email=t@t commit -q -m first && git log --oneline"})
	if err != nil || res.ExitCode != 0 || !strings.Contains(string(res.Stdout), "first") {
		t.Fatalf("git init+commit must work: res=%+v err=%v out=%s", res, err, res.Stdout)
	}
	res, err = d.Exec(ctx, tg, vm.ExecSpec{Shell: "echo evil > .git/hooks/pre-commit", Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("writing .git/hooks after init must fail: %s", res.Stdout)
	}
}

func TestRealIsolationTimeoutAndNetwork(t *testing.T) {
	d := realDriver(t)
	res, err := d.Exec(context.Background(), target("sbx-to2"), vm.ExecSpec{Shell: "sleep 30", Timeout: time.Second})
	if err != nil || !res.TimedOut {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	res, err = d.Exec(context.Background(), target("sbx-net2"), vm.ExecSpec{
		Shell: "exec 3<>/dev/tcp/1.1.1.1/80", Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 {
		t.Fatal("network must be unreachable")
	}
}

func TestRealIsolationOutputCapped(t *testing.T) {
	d := realDriver(t)
	d.opts.MaxOutputBytes = 100
	res, err := d.Exec(context.Background(), target("sbx-cap"), vm.ExecSpec{
		Shell: "yes x | head -c 100000", Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Stdout) != 101 {
		t.Fatalf("output len = %d, want 101 (cap+1)", len(res.Stdout))
	}
}
