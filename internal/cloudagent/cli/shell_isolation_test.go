package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
)

func boolPtr(b bool) *bool { return &b }

// 缺省配置：隔离关闭，行为与旧版一致。
func TestShellIsolationDefaultsDisabled(t *testing.T) {
	p, err := ShellIsolation{}.Policy(t.TempDir(), "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	if p.Isolates() || p.Enabled {
		t.Fatalf("default must be disabled: %+v", p)
	}
}

// 命令行只覆盖传了的字段，其余沿用文件配置。
func TestOverlayShellIsolationKeepsUnsetFields(t *testing.T) {
	base := ShellIsolation{Enabled: boolPtr(true), Mode: "read-only", PidsMax: 64, DenyRead: []string{"~/.ssh"}}
	got := overlayShellIsolation(base, ShellIsolation{Mode: "workspace-write", CPUMax: "100000 100000"})
	want := ShellIsolation{Enabled: boolPtr(true), Mode: "workspace-write", PidsMax: 64,
		DenyRead: []string{"~/.ssh"}, CPUMax: "100000 100000"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	// 显式写 enabled=false 要能覆盖文件里的 true。
	off := overlayShellIsolation(base, ShellIsolation{Enabled: boolPtr(false)})
	if off.Enabled == nil || *off.Enabled {
		t.Fatalf("explicit false must win: %+v", off.Enabled)
	}
}

func TestShellIsolationFromFlag(t *testing.T) {
	for _, mode := range []string{"read-only", "workspace-write", "danger-full-access"} {
		got, err := ShellIsolationFromFlag(mode)
		if err != nil || got.Enabled == nil || !*got.Enabled || got.Mode != mode {
			t.Fatalf("%s: %+v err=%v", mode, got, err)
		}
	}
	if _, err := ShellIsolationFromFlag("yolo"); err == nil {
		t.Fatal("unknown mode must be rejected")
	}
}

func TestParseFlagsShellIsolation(t *testing.T) {
	got := parseFlags(Options{}, []string{"-p", "x", "--shell-isolation", "read-only"})
	if got.FlagErr != nil || got.Prompt != "x" {
		t.Fatalf("got %+v", got)
	}
	si := got.Config.ShellIsolation
	if si.Enabled == nil || !*si.Enabled || si.Mode != "read-only" {
		t.Fatalf("flag not applied: %+v", si)
	}
	// 模式值不能漏进 prompt。
	if strings.Contains(got.Prompt, "read-only") {
		t.Fatalf("flag value leaked into prompt: %q", got.Prompt)
	}
	bad := parseFlags(Options{}, []string{"--shell-isolation", "nope"})
	if bad.FlagErr == nil {
		t.Fatal("invalid mode must set FlagErr")
	}
}

// 非法配置直接报错，不静默当成关闭。
func TestShellIsolationPolicyRejectsInvalid(t *testing.T) {
	ws := t.TempDir()
	for name, si := range map[string]ShellIsolation{
		"mode":     {Enabled: boolPtr(true), Mode: "bogus"},
		"timeout":  {Enabled: boolPtr(true), Timeout: "soon"},
		"negative": {Enabled: boolPtr(true), Timeout: "-5s"},
		"relative": {Enabled: boolPtr(true), WritableRoots: []string{"rel/dir"}},
		"missing":  {Enabled: boolPtr(true), WritableRoots: []string{filepath.Join(ws, "nope")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := si.Policy(ws, "/home/u"); err == nil {
				t.Fatalf("%s must be rejected", name)
			}
		})
	}
}

// "~" 展开，工作区符号链接被解析成真实路径。
func TestShellIsolationPolicyExpandsHomeAndSymlinks(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "ws-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	extra := filepath.Join(home, "cache")
	if err := os.Mkdir(extra, 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := ShellIsolation{Enabled: boolPtr(true), Mode: "workspace-write",
		WritableRoots: []string{"~/cache"}, DenyRead: []string{"~/.ssh"}, Timeout: "30s"}.Policy(link, home)
	if err != nil {
		t.Fatal(err)
	}
	wantWS, _ := filepath.EvalSymlinks(real)
	if p.Workspace != wantWS {
		t.Fatalf("workspace = %q want %q", p.Workspace, wantWS)
	}
	if len(p.WritableRoots) != 1 || p.WritableRoots[0] != extra {
		t.Fatalf("writable roots = %v", p.WritableRoots)
	}
	if len(p.DenyRead) != 1 || p.DenyRead[0] != filepath.Join(home, ".ssh") {
		t.Fatalf("deny_read = %v", p.DenyRead)
	}
	if p.Timeout.Seconds() != 30 || p.Mode != host.ModeWorkspaceWrite || !p.Isolates() {
		t.Fatalf("policy = %+v", p)
	}
}

// config.json 里的 shell_isolation 段能落盘并读回。
func TestShellIsolationConfigRoundTrip(t *testing.T) {
	env := testEnv(t)
	cfg := Config{Endpoint: "http://127.0.0.1:8080", Token: "t", ProjectID: "p",
		ShellIsolation: ShellIsolation{Enabled: boolPtr(true), Mode: "read-only", PidsMax: 32}}
	if _, err := SaveConfig(env, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(env)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.ShellIsolation, cfg.ShellIsolation) {
		t.Fatalf("got %+v want %+v", got.ShellIsolation, cfg.ShellIsolation)
	}
}
