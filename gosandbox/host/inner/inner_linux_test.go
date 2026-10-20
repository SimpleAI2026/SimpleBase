//go:build linux

package inner

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/landlock-lsm/go-landlock/landlock"

	"github.com/linkxzhou/SimpleBase/gosandbox/host/internal/spec"
)

func stub(t *testing.T) (restricted *int, loaded *int) {
	t.Helper()
	oldABI, oldRestrict, oldLoad := abiFn, restrictPathsFn, loadSeccompFn
	t.Cleanup(func() { abiFn, restrictPathsFn, loadSeccompFn = oldABI, oldRestrict, oldLoad })
	restricted, loaded = new(int), new(int)
	restrictPathsFn = func(landlock.Config, ...landlock.Rule) error { *restricted++; return nil }
	loadSeccompFn = func(seccomp.Filter) error { *loaded++; return nil }
	return
}

// TestLandlockGate：ABI < 3 在装任何规则之前就失败；ABI >= 3 才进入 V3。
func TestApplyABIGate(t *testing.T) {
	for abi, wantErr := range map[int]bool{0: true, 1: true, 2: true, 3: false, 6: false, 10: false} {
		restricted, loaded := stub(t)
		abiFn = func() int { return abi }
		err := Apply(spec.Spec{Mode: "workspace-write", Workspace: "/w"})
		if (err != nil) != wantErr {
			t.Fatalf("abi %d: err = %v, wantErr %v", abi, err, wantErr)
		}
		if wantErr && (*restricted != 0 || *loaded != 0) {
			t.Fatalf("abi %d: nothing may be installed on failure", abi)
		}
	}
}

// Landlock 先于 seccomp：顺序反了 seccomp 会拒绝 Landlock 需要的 syscall。
func TestApplyOrder(t *testing.T) {
	var order []string
	stub(t)
	abiFn = func() int { return 6 }
	restrictPathsFn = func(landlock.Config, ...landlock.Rule) error { order = append(order, "landlock"); return nil }
	loadSeccompFn = func(seccomp.Filter) error { order = append(order, "seccomp"); return nil }
	if err := Apply(spec.Spec{Mode: "read-only"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "landlock,seccomp" {
		t.Fatalf("order = %v", order)
	}
}

// 任一步失败都不继续，也不降级。
func TestApplyStopsOnFailure(t *testing.T) {
	stub(t)
	abiFn = func() int { return 6 }
	restrictPathsFn = func(landlock.Config, ...landlock.Rule) error { return errors.New("boom") }
	called := false
	loadSeccompFn = func(seccomp.Filter) error { called = true; return nil }
	if err := Apply(spec.Spec{Mode: "workspace-write"}); err == nil || !strings.Contains(err.Error(), "landlock") {
		t.Fatalf("err = %v", err)
	}
	if called {
		t.Fatal("seccomp must not load after landlock failed")
	}
}

func TestApplyRejectsUnknownMode(t *testing.T) {
	stub(t)
	abiFn = func() int { return 6 }
	for _, m := range []string{"", "danger-full-access", "bogus"} {
		if err := Apply(spec.Spec{Mode: m}); err == nil {
			t.Fatalf("mode %q must be rejected", m)
		}
	}
}

// read-only 不给工作区写权限；workspace-write 给。
func TestLandlockRulesByMode(t *testing.T) {
	ro := LandlockRules(spec.Spec{Mode: "read-only", Workspace: "/w", ScratchDirs: []string{"/tmp"}})
	rw := LandlockRules(spec.Spec{Mode: "workspace-write", Workspace: "/w", ScratchDirs: []string{"/tmp"}})
	if len(rw) <= len(ro) && strings.Join(ruleStrings(ro), "|") == strings.Join(ruleStrings(rw), "|") {
		t.Fatalf("workspace-write must differ from read-only:\nro=%v\nrw=%v", ruleStrings(ro), ruleStrings(rw))
	}
	for _, r := range ruleStrings(ro) {
		if strings.Contains(r, "/w") && strings.Contains(strings.ToLower(r), "write") {
			t.Fatalf("read-only must not grant workspace write: %s", r)
		}
	}
}

func ruleStrings(rules []landlock.Rule) []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = fmt.Sprint(r)
	}
	return out
}

func TestParseArgs(t *testing.T) {
	p, ctty, argv, err := parseArgs([]string{"--policy", "/p.json", "--", "/bin/sh", "-c", "echo"})
	if err != nil || p != "/p.json" || ctty || strings.Join(argv, " ") != "/bin/sh -c echo" {
		t.Fatalf("%q %v %v %v", p, ctty, argv, err)
	}
	for _, bad := range [][]string{nil, {"--policy"}, {"--policy", "p"}, {"--", "x"}, {"--bogus"}, {"--policy", "p", "--"}} {
		if _, _, _, err := parseArgs(bad); err == nil {
			t.Fatalf("%v must be rejected", bad)
		}
	}
}

func TestParseArgsCtty(t *testing.T) {
	_, ctty, argv, err := parseArgs([]string{"--policy", "/p.json", "--ctty", "--", "/bin/sh"})
	if err != nil || !ctty || len(argv) != 1 {
		t.Fatalf("%v %v %v", ctty, argv, err)
	}
}

// 装载失败：固定前缀 + 退出码 71，供父进程区分。
func TestMainSetupFailureSignal(t *testing.T) {
	stub(t)
	abiFn = func() int { return 0 }
	code := Main([]string{"--policy", "/nonexistent", "--", "/bin/true"}, nil)
	if code != spec.ExitSetupFailed {
		t.Fatalf("code = %d", code)
	}
}

func TestSeccompFilterAssembles(t *testing.T) {
	f := SeccompFilter()
	if _, err := f.Policy.Assemble(); err != nil {
		t.Fatalf("filter must assemble on this arch: %v", err)
	}
	if !f.NoNewPrivs {
		t.Fatal("no_new_privs must be set")
	}
}
