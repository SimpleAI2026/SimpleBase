// 文件说明：symlink_test.go — gocloudagent/tools 的单元测试（TestResolvePathBlocksSymlinkEscape、TestResolvePathAllowsInternalSymlink 等）
//
// 用法
//
//	go test ./gocloudagent/tools -run 'TestResolvePathBlocksSymlinkEscape|TestResolvePathAllowsInternalSymlink|TestResolvePathAllowsWorkspaceRootReachedThroughSymlink|TestFileToolsRefuseSymlinkEscape'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/tools/symlink_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] base.go, fsutil.go, http.go, isolation_test.go, mcp.go 等 15 个
//
// 主要符号
//
//	symlinkWorkspace(t *testing.T) (ws *Workspace, root, outside string)
//	TestResolvePathBlocksSymlinkEscape(t *testing.T)
//	TestResolvePathAllowsInternalSymlink(t *testing.T)
//	TestResolvePathAllowsWorkspaceRootReachedThroughSymlink(t *testing.T)
//	TestFileToolsRefuseSymlinkEscape(t *testing.T)
//	TestGrepAndGlobDoNotFollowEscapingSymlinks(t *testing.T)

package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// symlinkWorkspace 在 workspace 内建立指向 workspace 外的符号链接：
//
//	root/link-file -> outside/secret.txt
//	root/link-dir  -> outside/
//	root/inside.txt（普通文件）
func symlinkWorkspace(t *testing.T) (ws *Workspace, root, outside string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	root = t.TempDir()
	outside = t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("TOP-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link-file")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link-dir")); err != nil {
		t.Fatal(err)
	}
	return &Workspace{Root: root, Yolo: true}, root, outside
}

// BUG-13：词法上在 workspace 内、真实路径却在外面的符号链接必须被拒绝。
func TestResolvePathBlocksSymlinkEscape(t *testing.T) {
	ws, _, _ := symlinkWorkspace(t)
	for _, p := range []string{"link-file", "link-dir", "link-dir/secret.txt", "link-dir/new.txt", "link-dir/sub/new.txt"} {
		if got, err := ws.resolvePath(p); err == nil {
			t.Errorf("resolvePath(%q) must be blocked, got %q", p, got)
		}
	}
	// 指向 workspace 内部的符号链接、普通文件、尚不存在的写目标都允许。
	for _, p := range []string{"inside.txt", "new.txt", "newdir/new.txt"} {
		if _, err := ws.resolvePath(p); err != nil {
			t.Errorf("resolvePath(%q) should be allowed: %v", p, err)
		}
	}
}

func TestResolvePathAllowsInternalSymlink(t *testing.T) {
	ws, root, _ := symlinkWorkspace(t)
	if err := os.Symlink(filepath.Join(root, "inside.txt"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.resolvePath("alias"); err != nil {
		t.Fatalf("internal symlink must be allowed: %v", err)
	}
}

// workspace 根本身经由符号链接给出（macOS 的 /var → /private/var 就是这种情形）时，
// 内部文件不能被误判为越界。
func TestResolvePathAllowsWorkspaceRootReachedThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkRoot := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(real, linkRoot); err != nil {
		t.Fatal(err)
	}
	ws := &Workspace{Root: linkRoot}
	for _, p := range []string{"a.txt", "new.txt", filepath.Join(linkRoot, "a.txt")} {
		if _, err := ws.resolvePath(p); err != nil {
			t.Errorf("resolvePath(%q) via symlinked root should be allowed: %v", p, err)
		}
	}
}

func TestFileToolsRefuseSymlinkEscape(t *testing.T) {
	ws, root, outside := symlinkWorkspace(t)
	tools, err := ws.LocalTools()
	if err != nil {
		t.Fatal(err)
	}
	read := invoke(t, findTool(t, tools, "read"), `{"path":"link-file"}`)
	if strings.Contains(read, "TOP-SECRET") || !strings.Contains(read, "path_not_allowed") {
		t.Fatalf("read_file followed the symlink: %s", read)
	}
	write := invoke(t, findTool(t, tools, "write"), `{"path":"link-dir/pwned.txt","content":"x"}`)
	if !strings.Contains(write, "path_not_allowed") {
		t.Fatalf("write_file followed the symlinked directory: %s", write)
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned.txt")); err == nil {
		t.Fatal("write_file created a file outside the workspace")
	}
	edit := invoke(t, findTool(t, tools, "edit"), `{"path":"link-file","old_text":"TOP-SECRET","new_text":"x"}`)
	if !strings.Contains(edit, "path_not_allowed") {
		t.Fatalf("edit_file followed the symlink: %s", edit)
	}
	if b, _ := os.ReadFile(filepath.Join(outside, "secret.txt")); string(b) != "TOP-SECRET" {
		t.Fatalf("outside file was modified: %q", b)
	}
	// 正常文件仍可读写。
	if ok := invoke(t, findTool(t, tools, "read"), `{"path":"inside.txt"}`); !strings.Contains(ok, "hello") {
		t.Fatalf("inside read failed: %s", ok)
	}
	_ = root
}

// glob / grep 不得列出或读取符号链接指向 workspace 外的文件内容。
func TestGrepAndGlobDoNotFollowEscapingSymlinks(t *testing.T) {
	ws, root, _ := symlinkWorkspace(t)
	hits, _, err := grepWalkSkip(root, regexp.MustCompile("TOP-SECRET"), "", 50, ws.walkSkipper())
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("grep read through an escaping symlink: %v", hits)
	}
	files, err := globWalkSkip(root, "**", ws.walkSkipper())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasPrefix(f, "link-") {
			t.Fatalf("glob listed an escaping symlink: %v", files)
		}
	}
}
