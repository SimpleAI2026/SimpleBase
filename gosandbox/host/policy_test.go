package host

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func basePolicy() Policy {
	return Policy{Enabled: true, Mode: ModeWorkspaceWrite, Workspace: "/work/ws"}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Policy)
		wantErr string
	}{
		{"ok", func(p *Policy) {}, ""},
		{"unknown mode", func(p *Policy) { p.Mode = "yolo" }, "unknown mode"},
		{"relative workspace", func(p *Policy) { p.Workspace = "ws" }, "workspace must be an absolute"},
		{"empty workspace", func(p *Policy) { p.Workspace = "" }, "workspace must be an absolute"},
		{"relative writable", func(p *Policy) { p.WritableRoots = []string{"x"} }, "writable root"},
		{"relative deny", func(p *Policy) { p.DenyRead = []string{"x"} }, "deny_read"},
		{"negative mem", func(p *Policy) { p.MemoryMaxBytes = -1 }, "negative"},
		{"bad cpu", func(p *Policy) { p.CPUMax = "lots" }, "cpu_max"},
		{"cpu three fields", func(p *Policy) { p.CPUMax = "1 2 3" }, "cpu_max"},
		{"cpu ok", func(p *Policy) { p.CPUMax = "100000 100000" }, ""},
		{"cpu max ok", func(p *Policy) { p.CPUMax = "max" }, ""},
		{"relative bwrap", func(p *Policy) { p.BwrapPath = "bwrap" }, "bwrap_path"},
		// 未启用时不校验路径：默认配置（enabled=false，workspace 为空）必须合法。
		{"disabled skips path checks", func(p *Policy) { p.Enabled = false; p.Workspace = "" }, ""},
		{"danger skips path checks", func(p *Policy) { p.Mode = ModeDangerFullAccess; p.Workspace = "" }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := basePolicy()
			tc.mutate(&p)
			err := Validate(p)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestIsolatesAndModes(t *testing.T) {
	if (Policy{}).Isolates() {
		t.Fatal("zero policy must not isolate")
	}
	if (Policy{Enabled: true, Mode: ModeDangerFullAccess}).Isolates() {
		t.Fatal("danger-full-access must not isolate")
	}
	if !(Policy{Enabled: true}).Isolates() {
		t.Fatal("enabled with default mode must isolate")
	}
	if (Policy{Enabled: true}).EffectiveMode() != ModeWorkspaceWrite {
		t.Fatal("default mode must be workspace-write")
	}
}

func TestDeniesRead(t *testing.T) {
	p := basePolicy()
	p.DenyRead = []string{"/home/u/.ssh", "/home/u/token"}
	for path, want := range map[string]bool{
		"/home/u/.ssh":              true,
		"/home/u/.ssh/id_rsa":       true,
		"/home/u/.sshx":             false, // 前缀相同但不是子路径
		"/home/u/token":             true,
		"/home/u/.ssh/../other":     false,
		"/home/u/other":             false,
		"/work/ws/file":             false,
		"/home/u/.ssh/sub/../k/key": true,
	} {
		if got := DeniesRead(p, path); got != want {
			t.Errorf("DeniesRead(%q) = %v, want %v", path, got, want)
		}
	}
	// 隔离未开启时不拒绝：与 shell 的实际行为保持一致。
	off := p
	off.Enabled = false
	if DeniesRead(off, "/home/u/.ssh/id_rsa") {
		t.Fatal("disabled policy must not deny reads")
	}
}

func fakeExists(present ...string) func(string) bool {
	return func(s string) bool { return slices.Contains(present, s) }
}

func idx(args []string, seq ...string) int {
	for i := 0; i+len(seq) <= len(args); i++ {
		if slices.Equal(args[i:i+len(seq)], seq) {
			return i
		}
	}
	return -1
}

func mustArgs(t *testing.T, in BwrapInput) []string {
	t.Helper()
	if in.Helper == "" {
		in.Helper = "/opt/gosandbox-inner"
	}
	if in.Argv == nil {
		in.Argv = []string{"/bin/sh", "-c", "echo hi"}
	}
	args, err := BwrapArgs(in)
	if err != nil {
		t.Fatal(err)
	}
	return args
}

// TestBwrapArgsOrder 固定挂载顺序：只读根 → /tmp → 可写绑定 → 只读路径（含强制 deny 表）→ deny_read → 命名空间 → cap-drop → 命令。
// bwrap 后挂的更具体路径覆盖先挂的，顺序错了就会让 .gocloudagent 与 .git/hooks 变回可写。
func TestBwrapArgsOrder(t *testing.T) {
	p := basePolicy()
	p.WritableRoots = []string{"/data/out"}
	p.DenyRead = []string{"/home/u/.ssh", "/home/u/token"}
	args := mustArgs(t, BwrapInput{
		Policy: p, Dir: "/work/ws/sub", Home: "/home/u",
		Exists: fakeExists("/work/ws/.gocloudagent", "/work/ws/.git/hooks", "/home/u/.ssh", "/home/u/token"),
		IsDir:  func(s string) bool { return s == "/home/u/.ssh" },
	})

	want := [][]string{
		{"--new-session"},
		{"--die-with-parent"},
		{"--ro-bind", "/", "/"},
		{"--tmpfs", "/tmp"},
		{"--bind", "/work/ws", "/work/ws"},
		{"--bind", "/data/out", "/data/out"},
		{"--ro-bind", "/work/ws/.gocloudagent", "/work/ws/.gocloudagent"},
		{"--ro-bind", "/work/ws/.git/hooks", "/work/ws/.git/hooks"},
		{"--tmpfs", "/home/u/.ssh"},
		{"--ro-bind", "/dev/null", "/home/u/token"},
		{"--dev", "/dev"},
		{"--proc", "/proc"},
		{"--unshare-user"}, {"--unshare-pid"}, {"--unshare-ipc"}, {"--unshare-net"},
		{"--cap-drop", "ALL"},
		{"--file", "3", InnerPolicyPath},
		{"--chdir", "/work/ws/sub"},
		{"--"},
	}
	last := -1
	for _, seq := range want {
		i := idx(args, seq...)
		if i < 0 {
			t.Fatalf("missing %v in %v", seq, args)
		}
		if i <= last {
			t.Fatalf("%v out of order (at %d, previous at %d) in %v", seq, i, last, args)
		}
		last = i
	}
	tail := args[len(args)-6:]
	if !slices.Equal(tail, []string{"--policy", InnerPolicyPath, "--", "/bin/sh", "-c", "echo hi"}) {
		t.Fatalf("helper invocation = %v", tail)
	}
	if args[idx(args, "--")+1] != "/opt/gosandbox-inner" {
		t.Fatalf("helper must directly follow --: %v", args)
	}
	// 不使用 --unshare-all，也不能出现 --as-pid-1（保留 bwrap 的 reaper）。
	for _, bad := range []string{"--unshare-all", "--as-pid-1"} {
		if slices.Contains(args, bad) {
			t.Fatalf("unexpected %s in %v", bad, args)
		}
	}
}

func TestBwrapArgsReadOnlyDoesNotBindWorkspace(t *testing.T) {
	p := basePolicy()
	p.Mode = ModeReadOnly
	args := mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists("/work/ws/.gocloudagent", "/work/ws/.git/hooks"), Home: "/home/u"})
	if idx(args, "--bind", "/work/ws", "/work/ws") >= 0 {
		t.Fatalf("read-only must not bind the workspace writable: %v", args)
	}
	for _, path := range []string{"/work/ws/.gocloudagent", "/work/ws/.git/hooks"} {
		if idx(args, "--ro-bind", path, path) >= 0 {
			t.Fatalf("read-only has no writable root to re-protect (%s): %v", path, args)
		}
	}
	if idx(args, "--tmpfs", "/tmp") < 0 {
		t.Fatalf("read-only needs a tmpfs /tmp: %v", args)
	}
}

func TestBwrapArgsSkipsMissingPaths(t *testing.T) {
	p := basePolicy()
	p.DenyRead = []string{"/home/u/missing"}
	args := mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists(), Home: "/home/u"})
	joined := strings.Join(args, " ")
	if strings.Contains(joined, ".git") || strings.Contains(joined, ".gocloudagent") || strings.Contains(joined, "/home/u/missing") {
		t.Fatalf("missing paths must not be mounted (bwrap would fail): %s", joined)
	}
}

func TestBwrapArgsReadOnlySubpathsOverride(t *testing.T) {
	p := basePolicy()
	exists := fakeExists("/work/ws/.git", "/work/ws/.git/hooks", "/work/ws/.gocloudagent")

	// 默认：.gocloudagent 只读；整个 .git 可写，但 .git/hooks 由强制表盖住。
	args := mustArgs(t, BwrapInput{Policy: p, Exists: exists, Home: "/home/u"})
	if idx(args, "--ro-bind", "/work/ws/.gocloudagent", "/work/ws/.gocloudagent") < 0 {
		t.Fatalf("default must cover .gocloudagent: %v", args)
	}
	if idx(args, "--ro-bind", "/work/ws/.git", "/work/ws/.git") >= 0 {
		t.Fatalf("default must leave .git writable so git commit works: %v", args)
	}
	if idx(args, "--ro-bind", "/work/ws/.git/hooks", "/work/ws/.git/hooks") < 0 {
		t.Fatalf("force-deny must cover .git/hooks: %v", args)
	}

	// 显式空切片关闭 ReadOnlySubpaths 的默认，但不关强制表。
	p.ReadOnlySubpaths = []string{}
	args = mustArgs(t, BwrapInput{Policy: p, Exists: exists, Home: "/home/u"})
	if idx(args, "--ro-bind", "/work/ws/.gocloudagent", "/work/ws/.gocloudagent") >= 0 {
		t.Fatalf("explicit empty read_only_subpaths must disable the default: %v", args)
	}
	if idx(args, "--ro-bind", "/work/ws/.git/hooks", "/work/ws/.git/hooks") < 0 {
		t.Fatalf("explicit empty read_only_subpaths must not disable the force-deny table: %v", args)
	}

	// 旧行为可显式取得：整个 .git 只读。
	p.ReadOnlySubpaths = []string{".git", ".gocloudagent"}
	args = mustArgs(t, BwrapInput{Policy: p, Exists: exists, Home: "/home/u"})
	if idx(args, "--ro-bind", "/work/ws/.git", "/work/ws/.git") < 0 {
		t.Fatalf("explicit legacy config must cover the whole .git: %v", args)
	}

	p.ReadOnlySubpaths = []string{"/abs/protected", "rel"}
	args = mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists("/abs/protected", "/work/ws/rel"), Home: "/home/u"})
	if idx(args, "--ro-bind", "/abs/protected", "/abs/protected") < 0 ||
		idx(args, "--ro-bind", "/work/ws/rel", "/work/ws/rel") < 0 {
		t.Fatalf("absolute and relative subpaths must both resolve: %v", args)
	}
}

// 工作区在 /tmp 之下：tmpfs 必须先于 bind，否则工作区会被盖掉。
func TestBwrapArgsWorkspaceUnderTmp(t *testing.T) {
	p := basePolicy()
	p.Workspace = "/tmp/ws"
	args := mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists()})
	tm, bd := idx(args, "--tmpfs", "/tmp"), idx(args, "--bind", "/tmp/ws", "/tmp/ws")
	if tm < 0 || bd < 0 || tm > bd {
		t.Fatalf("tmpfs /tmp (%d) must precede --bind /tmp/ws (%d): %v", tm, bd, args)
	}
}

// /tmp 本身被可写根覆盖：不再叠 tmpfs，否则会把可写的真实 /tmp 换成临时的。
func TestBwrapArgsTmpIsWritableRoot(t *testing.T) {
	p := basePolicy()
	p.Workspace = "/work/ws"
	p.WritableRoots = []string{"/tmp"}
	if args := mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists()}); idx(args, "--tmpfs", "/tmp") >= 0 {
		t.Fatalf("unexpected tmpfs /tmp: %v", args)
	}
}

func TestBwrapArgsDisableUserNS(t *testing.T) {
	p := basePolicy()
	if slices.Contains(mustArgs(t, BwrapInput{Policy: p}), "--disable-userns") {
		t.Fatal("--disable-userns must be opt-in")
	}
	if !slices.Contains(mustArgs(t, BwrapInput{Policy: p, DisableUserNS: true}), "--disable-userns") {
		t.Fatal("--disable-userns missing")
	}
}

func TestBwrapArgsRejects(t *testing.T) {
	if _, err := BwrapArgs(BwrapInput{Policy: Policy{}, Helper: "/h", Argv: []string{"x"}}); !errors.Is(err, ErrNotIsolated) {
		t.Fatalf("disabled policy: %v", err)
	}
	for name, in := range map[string]BwrapInput{
		"relative helper": {Policy: basePolicy(), Helper: "h", Argv: []string{"x"}},
		"no helper":       {Policy: basePolicy(), Argv: []string{"x"}},
		"no argv":         {Policy: basePolicy(), Helper: "/h"},
	} {
		if _, err := BwrapArgs(in); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

// 策略不进 argv：避免出现在 /proc/<pid>/cmdline。
func TestBwrapArgsDoNotLeakPolicyJSON(t *testing.T) {
	p := basePolicy()
	args := strings.Join(mustArgs(t, BwrapInput{Policy: p}), " ")
	if strings.Contains(args, "{") || strings.Contains(args, "writable_roots") {
		t.Fatalf("policy JSON leaked into argv: %s", args)
	}
}
