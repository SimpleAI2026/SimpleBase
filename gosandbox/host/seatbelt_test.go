package host

import (
	"errors"
	"strings"
	"testing"
)

func seatbeltPolicy() Policy {
	return Policy{Enabled: true, Mode: ModeWorkspaceWrite, Workspace: "/work/ws",
		WritableRoots: []string{"/work/extra"}, DenyRead: []string{"/home/u/.ssh"}}
}

// SBPL 以 (deny default) 开头，且没有任何 network 放行规则：网络恒为拒绝。
func TestSeatbeltProfileDeniesNetworkByDefault(t *testing.T) {
	for _, mode := range []Mode{ModeReadOnly, ModeWorkspaceWrite} {
		p := seatbeltPolicy()
		p.Mode = mode
		profile, _, err := SeatbeltProfile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(profile, "(version 1)\n(deny default)") {
			t.Fatalf("%s: profile must start with deny default:\n%s", mode, profile)
		}
		if strings.Contains(profile, "network") {
			t.Fatalf("%s: profile must not mention network:\n%s", mode, profile)
		}
	}
}

// 写权限只来自 -D 参数；路径文本不进入策略正文。
func TestSeatbeltProfileUsesParamsNotPaths(t *testing.T) {
	profile, params, err := SeatbeltProfile(seatbeltPolicy())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/work/ws", "/work/extra", "/home/u/.ssh"} {
		if strings.Contains(profile, path) {
			t.Fatalf("path %q must not be inlined in the profile:\n%s", path, profile)
		}
	}
	joined := strings.Join(params, "\n")
	for _, want := range []string{"W0=/work/ws", "W1=/work/extra", "D0=/home/u/.ssh"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("params missing %q: %v", want, params)
		}
	}
}

// 带引号和括号的路径不会改变策略语义：它只出现在 -D 参数里。
func TestSeatbeltProfileHostilePath(t *testing.T) {
	p := seatbeltPolicy()
	p.Workspace = `/work/a") (allow network*) ("b`
	profile, params, err := SeatbeltProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(profile, "allow network") {
		t.Fatalf("hostile path leaked into profile:\n%s", profile)
	}
	if !strings.Contains(strings.Join(params, "\n"), `allow network*`) {
		t.Fatalf("hostile path must travel as a parameter: %v", params)
	}
}

// read-only 不放行工作区写入；workspace-write 默认盖住 .gocloudagent 与强制 deny 表（仅已存在的路径）。
func TestSeatbeltProfileModes(t *testing.T) {
	exists := fakeExists("/work/ws/.gocloudagent", "/work/ws/.git/hooks")

	ro := seatbeltPolicy()
	ro.Mode = ModeReadOnly
	_, roParams, err := seatbeltProfile(ro, "/home/u", exists)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(roParams, "\n"), "=/work/ws") {
		t.Fatalf("read-only must not make the workspace writable: %v", roParams)
	}
	_, rwParams, err := seatbeltProfile(seatbeltPolicy(), "/home/u", exists)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(rwParams, "\n")
	// 用名称而不是序号断言：强制表会让 R 序号随存在的路径而变。
	for _, want := range []string{"=/work/ws/.gocloudagent", "=/work/ws/.git/hooks"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing read-only path %q: %v", want, rwParams)
		}
	}
	// 整个 .git 不在默认只读里，git commit 才能工作。
	for _, p := range rwParams {
		if strings.HasSuffix(p, "=/work/ws/.git") {
			t.Fatalf("whole .git must stay writable by default: %v", rwParams)
		}
	}
}

// D7：不存在的路径不生成 deny 规则，否则沙盒内 git init 会直接失败（macOS 真机实测）。
func TestSeatbeltProfileSkipsMissingPaths(t *testing.T) {
	profile, params, err := seatbeltProfile(seatbeltPolicy(), "/home/u", fakeExists())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range params {
		if strings.HasPrefix(p, "R") {
			t.Fatalf("missing paths must not produce deny rules: %v", params)
		}
	}
	if strings.Contains(profile, `(param "R`) {
		t.Fatalf("profile must not reference R params:\n%s", profile)
	}
}

func TestSeatbeltProfileForceDenyDisabled(t *testing.T) {
	p := seatbeltPolicy()
	p.DisableForceDeny = true
	_, params, err := seatbeltProfile(p, "/home/u", fakeExists("/work/ws/.git/hooks", "/work/ws/.gocloudagent"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(params, "\n")
	if strings.Contains(joined, ".git/hooks") {
		t.Fatalf("DisableForceDeny must drop the table: %v", params)
	}
	if !strings.Contains(joined, "=/work/ws/.gocloudagent") {
		t.Fatalf("ReadOnlySubpaths still applies: %v", params)
	}
}

// deny 规则必须排在 allow 之后，才能覆盖前面的放行。
func TestSeatbeltProfileDenyAfterAllow(t *testing.T) {
	profile, _, err := seatbeltProfile(seatbeltPolicy(), "/home/u",
		fakeExists("/work/ws/.gocloudagent", "/work/ws/.git/hooks"))
	if err != nil {
		t.Fatal(err)
	}
	lastAllowWrite := strings.LastIndex(profile, "(allow file-write*")
	firstDeny := strings.Index(profile, `(deny file-write* (subpath (param "R0")))`)
	denyRead := strings.Index(profile, `(deny file-read* (subpath (param "D0")))`)
	if firstDeny < 0 || denyRead < 0 || firstDeny < lastAllowWrite || denyRead < lastAllowWrite {
		t.Fatalf("deny rules must follow allow rules:\n%s", profile)
	}
}

func TestSeatbeltArgsShape(t *testing.T) {
	args, err := SeatbeltArgs(seatbeltPolicy(), []string{"/bin/echo", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != "-p" || !strings.HasPrefix(args[1], "(version 1)") {
		t.Fatalf("args = %v", args[:2])
	}
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
		}
	}
	if sep < 0 || strings.Join(args[sep+1:], " ") != "/bin/echo hi" {
		t.Fatalf("argv must follow --: %v", args)
	}
	if _, err := SeatbeltArgs(seatbeltPolicy(), nil); err == nil {
		t.Fatal("empty argv must be rejected")
	}
}

func TestSeatbeltProfileNotIsolated(t *testing.T) {
	for name, p := range map[string]Policy{
		"disabled": {Workspace: "/work/ws"},
		"danger":   {Enabled: true, Mode: ModeDangerFullAccess, Workspace: "/work/ws"},
	} {
		if _, _, err := SeatbeltProfile(p); !errors.Is(err, ErrNotIsolated) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	if _, _, err := SeatbeltProfile(Policy{Enabled: true, Mode: ModeWorkspaceWrite, Workspace: "rel"}); err == nil {
		t.Fatal("invalid policy must be rejected")
	}
}
