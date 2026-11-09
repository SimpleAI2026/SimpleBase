package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProbeNotIsolated(t *testing.T) {
	for name, p := range map[string]Policy{
		"disabled": {},
		"danger":   {Enabled: true, Mode: ModeDangerFullAccess},
	} {
		if _, err := Probe(context.Background(), p); !errors.Is(err, ErrNotIsolated) {
			t.Errorf("%s: err = %v, want ErrNotIsolated", name, err)
		}
	}
}

func TestRunNotIsolatedDoesNotExec(t *testing.T) {
	// 假 PATH 里放一个会留下痕迹的 bwrap：danger-full-access 与 enabled=false 都不得执行它。
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	script := "#!/bin/sh\ntouch " + marker + "\n"
	if err := os.WriteFile(filepath.Join(dir, "bwrap"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for name, p := range map[string]Policy{
		"disabled": {Workspace: "/x"},
		"danger":   {Enabled: true, Mode: ModeDangerFullAccess, Workspace: "/x"},
	} {
		_, err := Run(context.Background(), p, Command{Argv: []string{"/bin/true"}})
		if !errors.Is(err, ErrNotIsolated) {
			t.Errorf("%s: err = %v, want ErrNotIsolated", name, err)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("bwrap was executed although isolation was not requested")
	}
}

func TestResolveBwrapRejectsRelative(t *testing.T) {
	if _, err := resolveBwrap("bwrap"); err == nil || !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatalf("relative configured path must be rejected, got %v", err)
	}
}

// cwd 里放一个可执行的假 bwrap：不得被选中。
func TestResolveBwrapIgnoresCwdBinary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bwrap"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("PATH", ".:"+t.TempDir()) // PATH 里含 "."，exec.LookPath 会返回 ErrDot
	got, err := resolveBwrap("")
	if err == nil {
		t.Fatalf("picked %q from the current directory", got)
	}
	if !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveBwrapNotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := resolveBwrap(""); !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

// bwrap 是 setuid 时拒绝（CVE-2020-5291）；只用 mode 位判断，不需要真的 setuid 二进制。
func TestSetuidBwrapRejected(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	ResetProbeCache()
	t.Cleanup(ResetProbeCache)
	dir := t.TempDir()
	bw := filepath.Join(dir, "bwrap")
	if err := os.WriteFile(bw, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bw, 0o755|os.ModeSetuid); err != nil {
		t.Skipf("cannot set setuid bit here: %v", err)
	}
	_, err := Probe(context.Background(), Policy{Enabled: true, Workspace: "/x", BwrapPath: bw})
	if !errors.Is(err, ErrIsolationUnavailable) || !strings.Contains(err.Error(), "setuid") {
		t.Fatalf("err = %v, want setuid rejection", err)
	}
}

// probeLinux 的各道门槛：用可替换点模拟。
func TestProbeLinuxGates(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	dir := t.TempDir()
	bw := filepath.Join(dir, "bwrap")
	if err := os.WriteFile(bw, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pol := Policy{Enabled: true, Workspace: "/x", BwrapPath: bw}

	oldRun, oldABI, oldSec, oldCg := runProbe, landlockABIFn, seccompFn, cgroupDetectFn
	t.Cleanup(func() {
		runProbe, landlockABIFn, seccompFn, cgroupDetectFn = oldRun, oldABI, oldSec, oldCg
		ResetProbeCache()
	})
	help := "usage: bwrap\n --disable-userns\n"
	setup := func(helpOut string, smokeErr error, abi int, sec bool) {
		ResetProbeCache()
		runProbe = func(_ context.Context, _ string, args ...string) (string, error) {
			switch args[0] {
			case "--version":
				return "bubblewrap 0.9.0\n", nil
			case "--help":
				return helpOut, nil
			}
			if smokeErr != nil {
				return "bwrap: No permissions to create new namespace\n", smokeErr
			}
			return "", nil
		}
		landlockABIFn = func() int { return abi }
		seccompFn = func() bool { return sec }
		cgroupDetectFn = func() string { return cgroupNone }
	}

	t.Run("old bwrap without --disable-userns", func(t *testing.T) {
		setup("usage: bwrap\n", nil, 6, true)
		if _, err := Probe(context.Background(), pol); err == nil || !strings.Contains(err.Error(), "--disable-userns") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("userns blocked points to launchpad", func(t *testing.T) {
		setup(help, errors.New("exit 1"), 6, true)
		_, err := Probe(context.Background(), pol)
		if err == nil || !strings.Contains(err.Error(), "2046477") {
			t.Fatalf("err = %v", err)
		}
	})
	// TestLandlockABIGate：ABI 2 失败；ABI 3、6 通过（严格 V3，无 BestEffort 开关）。
	for abi, wantErr := range map[int]bool{0: true, 1: true, 2: true, 3: false, 6: false, 10: false} {
		t.Run("landlock ABI gate", func(t *testing.T) {
			setup(help, nil, abi, true)
			_, err := Probe(context.Background(), pol)
			if (err != nil) != wantErr {
				t.Fatalf("abi %d: err = %v, wantErr %v", abi, err, wantErr)
			}
			if err != nil && !errors.Is(err, ErrIsolationUnavailable) {
				t.Fatalf("abi %d: err must wrap ErrIsolationUnavailable: %v", abi, err)
			}
		})
	}
	t.Run("seccomp unsupported", func(t *testing.T) {
		setup(help, nil, 6, false)
		if _, err := Probe(context.Background(), pol); err == nil || !strings.Contains(err.Error(), "seccomp") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("all gates pass and result is cached", func(t *testing.T) {
		setup(help, nil, 6, true)
		caps, err := Probe(context.Background(), pol)
		if err != nil {
			t.Fatal(err)
		}
		if caps.LandlockABI != 6 || !caps.UserNS || !caps.DisableUserNS || caps.Cgroup != cgroupNone {
			t.Fatalf("caps = %+v", caps)
		}
		calls := 0
		runProbe = func(context.Context, string, ...string) (string, error) { calls++; return "", nil }
		if _, err := Probe(context.Background(), pol); err != nil || calls != 0 {
			t.Fatalf("second Probe must use the cache (calls=%d, err=%v)", calls, err)
		}
	})
}
