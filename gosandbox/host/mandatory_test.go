package host

import (
	"slices"
	"strings"
	"testing"
)

func mandatoryPolicy() Policy {
	return Policy{Enabled: true, Mode: ModeWorkspaceWrite, Workspace: "/work/ws"}
}

func TestForceDenyWritePathsOnlyExisting(t *testing.T) {
	p := mandatoryPolicy()
	// 只有 .git/hooks 与 .vscode 存在。
	exists := fakeExists("/work/ws/.git/hooks", "/work/ws/.vscode")
	got := ForceDenyWritePaths(p, "", exists)
	want := []string{"/work/ws/.git/hooks", "/work/ws/.vscode"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// D7：工作区还没有 .git 时不产生任何规则，否则沙盒内 git init 会直接失败。
func TestForceDenyWritePathsNothingWhenGitMissing(t *testing.T) {
	got := ForceDenyWritePaths(mandatoryPolicy(), "", fakeExists())
	if len(got) != 0 {
		t.Fatalf("missing paths must not be protected: %v", got)
	}
}

func TestForceDenyWritePathsCoversWholeTable(t *testing.T) {
	p := mandatoryPolicy()
	var present []string
	for _, s := range ForceDenyWriteSubpaths {
		present = append(present, "/work/ws/"+s)
	}
	got := ForceDenyWritePaths(p, "", fakeExists(present...))
	if len(got) != len(ForceDenyWriteSubpaths) {
		t.Fatalf("got %d paths, want %d: %v", len(got), len(ForceDenyWriteSubpaths), got)
	}
	// 表里的每一项都必须是 .git 内部的具体路径，或是工作区根下的独立文件/目录；
	// 绝不能把整个 .git 放进去，否则 git commit 又会失败。
	if slices.Contains(got, "/work/ws/.git") {
		t.Fatal(".git as a whole must not be in the force-deny table")
	}
	if !slices.IsSorted(got) {
		t.Fatalf("result must be sorted for stable argv: %v", got)
	}
}

func TestForceDenyWritePathsExtraWritableRoot(t *testing.T) {
	p := mandatoryPolicy()
	p.WritableRoots = []string{"/data/repo"}
	got := ForceDenyWritePaths(p, "", fakeExists("/data/repo/.git/hooks"))
	if !slices.Equal(got, []string{"/data/repo/.git/hooks"}) {
		t.Fatalf("got %v", got)
	}
}

func TestForceDenyWritePathsHomeOnlyWhenCovered(t *testing.T) {
	exists := fakeExists("/home/u/.bashrc", "/home/u/.gitconfig")
	p := mandatoryPolicy()

	// 可写根没覆盖 HOME：根本来就只读，不生成条目。
	if got := ForceDenyWritePaths(p, "/home/u", exists); len(got) != 0 {
		t.Fatalf("HOME not covered, want none: %v", got)
	}
	// 工作区就是 HOME，或其祖先：覆盖。
	for _, ws := range []string{"/home/u", "/home"} {
		p.Workspace = ws
		got := ForceDenyWritePaths(p, "/home/u", exists)
		want := []string{"/home/u/.bashrc", "/home/u/.gitconfig"}
		if !slices.Equal(got, want) {
			t.Fatalf("workspace %s: got %v, want %v", ws, got, want)
		}
	}
	// 额外可写根覆盖 HOME 也算。
	p.Workspace = "/work/ws"
	p.WritableRoots = []string{"/home/u"}
	if got := ForceDenyWritePaths(p, "/home/u", exists); len(got) != 2 {
		t.Fatalf("writable root covering HOME: %v", got)
	}
	// 前缀相同但不是祖先，不算覆盖。
	p.WritableRoots = []string{"/home/u2"}
	if got := ForceDenyWritePaths(p, "/home/u", exists); len(got) != 0 {
		t.Fatalf("/home/u2 does not cover /home/u: %v", got)
	}
}

func TestForceDenyWritePathsDisabledAndModes(t *testing.T) {
	exists := fakeExists("/work/ws/.git/hooks")

	off := mandatoryPolicy()
	off.DisableForceDeny = true
	if got := ForceDenyWritePaths(off, "", exists); got != nil {
		t.Fatalf("DisableForceDeny must yield none: %v", got)
	}
	// read-only 档没有可写区，不需要再盖。
	ro := mandatoryPolicy()
	ro.Mode = ModeReadOnly
	if got := ForceDenyWritePaths(ro, "", exists); got != nil {
		t.Fatalf("read-only must yield none: %v", got)
	}
	notIsolated := mandatoryPolicy()
	notIsolated.Enabled = false
	if got := ForceDenyWritePaths(notIsolated, "", exists); got != nil {
		t.Fatalf("not isolated must yield none: %v", got)
	}
}

func TestReadOnlyPathsMergesAndDedups(t *testing.T) {
	p := mandatoryPolicy()
	// 用户把 .git/hooks 也写进了 ReadOnlySubpaths：合并后只出现一次。
	p.ReadOnlySubpaths = []string{".gocloudagent", ".git/hooks", "missing"}
	exists := fakeExists("/work/ws/.gocloudagent", "/work/ws/.git/hooks", "/work/ws/.vscode")
	got := readOnlyPaths(p, "", exists)
	want := []string{"/work/ws/.gocloudagent", "/work/ws/.git/hooks", "/work/ws/.vscode"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// 显式空切片只关闭 ReadOnlySubpaths，不关闭强制表。
func TestExplicitEmptyReadOnlySubpathsKeepsForceDeny(t *testing.T) {
	p := mandatoryPolicy()
	p.ReadOnlySubpaths = []string{}
	exists := fakeExists("/work/ws/.gocloudagent", "/work/ws/.git/hooks")
	got := readOnlyPaths(p, "", exists)
	if !slices.Equal(got, []string{"/work/ws/.git/hooks"}) {
		t.Fatalf("got %v", got)
	}
}

// D1：默认值不再包含整个 .git；想要旧行为必须显式配置。
func TestDefaultReadOnlySubpathsExcludesWholeGit(t *testing.T) {
	if slices.Contains(DefaultReadOnlySubpaths, ".git") {
		t.Fatalf("default must not make the whole .git read-only: %v", DefaultReadOnlySubpaths)
	}
	if !slices.Contains(DefaultReadOnlySubpaths, ".gocloudagent") {
		t.Fatalf("default must keep .gocloudagent: %v", DefaultReadOnlySubpaths)
	}
	// 旧行为仍可显式取得。
	p := mandatoryPolicy()
	p.ReadOnlySubpaths = []string{".git", ".gocloudagent"}
	got := readOnlyPaths(p, "", fakeExists("/work/ws/.git", "/work/ws/.git/hooks", "/work/ws/.gocloudagent"))
	if !slices.Contains(got, "/work/ws/.git") {
		t.Fatalf("explicit legacy config must still cover .git: %v", got)
	}
}

func TestWarnings(t *testing.T) {
	p := mandatoryPolicy()
	if w := Warnings(p); len(w) != 0 {
		t.Fatalf("default must have no warnings: %v", w)
	}
	p.DisableForceDeny = true
	w := Warnings(p)
	if len(w) != 1 || !strings.Contains(w[0], "force-deny") {
		t.Fatalf("warnings = %v", w)
	}
	// 未隔离时没有意义，不上报。
	p.Enabled = false
	if w := Warnings(p); len(w) != 0 {
		t.Fatalf("not isolated must not warn: %v", w)
	}
}

func TestBwrapArgsIncludeForceDeny(t *testing.T) {
	p := mandatoryPolicy()
	exists := fakeExists("/work/ws/.git", "/work/ws/.git/hooks", "/work/ws/.git/config", "/work/ws/.vscode")
	args := mustArgs(t, BwrapInput{Policy: p, Exists: exists, Home: "/home/u"})
	for _, path := range []string{"/work/ws/.git/hooks", "/work/ws/.git/config", "/work/ws/.vscode"} {
		if idx(args, "--ro-bind", path, path) < 0 {
			t.Errorf("missing --ro-bind for %s in %v", path, args)
		}
	}
	// 整个 .git 不能被盖成只读，否则 git commit 会失败。
	if idx(args, "--ro-bind", "/work/ws/.git", "/work/ws/.git") >= 0 {
		t.Errorf("whole .git must stay writable by default: %v", args)
	}
	// 强制 deny 的挂载必须在可写 bind 之后，否则会被盖回可写。
	bind := idx(args, "--bind", "/work/ws", "/work/ws")
	ro := idx(args, "--ro-bind", "/work/ws/.git/hooks", "/work/ws/.git/hooks")
	if bind < 0 || ro < bind {
		t.Errorf("force-deny must follow the writable bind (bind=%d ro=%d): %v", bind, ro, args)
	}
}

// D7：工作区没有 .git 时，bwrap 参数里不能出现任何 .git 路径（否则 bwrap 会因目标不存在而失败，
// 即使不失败也会让 git init 无法创建 .git）。
func TestBwrapArgsNoGitWhenMissing(t *testing.T) {
	args := mustArgs(t, BwrapInput{Policy: mandatoryPolicy(), Exists: fakeExists(), Home: "/home/u"})
	if strings.Contains(strings.Join(args, " "), ".git") {
		t.Fatalf("no .git path may be mounted when it does not exist: %v", args)
	}
}

func TestBwrapArgsDisableForceDeny(t *testing.T) {
	p := mandatoryPolicy()
	p.DisableForceDeny = true
	args := mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists("/work/ws/.git/hooks"), Home: "/home/u"})
	if idx(args, "--ro-bind", "/work/ws/.git/hooks", "/work/ws/.git/hooks") >= 0 {
		t.Fatalf("DisableForceDeny must remove the table: %v", args)
	}
}

func TestBwrapArgsHomeFilesWhenWorkspaceIsHome(t *testing.T) {
	p := mandatoryPolicy()
	p.Workspace = "/home/u"
	args := mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists("/home/u/.bashrc"), Home: "/home/u"})
	if idx(args, "--ro-bind", "/home/u/.bashrc", "/home/u/.bashrc") < 0 {
		t.Fatalf("workspace == HOME must protect .bashrc: %v", args)
	}
	p.Workspace = "/work/ws"
	args = mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists("/home/u/.bashrc"), Home: "/home/u"})
	if idx(args, "--ro-bind", "/home/u/.bashrc", "/home/u/.bashrc") >= 0 {
		t.Fatalf("HOME not covered: no entry expected: %v", args)
	}
}
