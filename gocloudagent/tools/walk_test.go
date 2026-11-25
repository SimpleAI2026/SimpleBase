// 文件说明：walk_test.go — gocloudagent/tools 的单元测试（TestGlobWalkPatternSemantics、TestGlobToRegexAlwaysCompiles 等）
//
// 用法
//
//	go test ./gocloudagent/tools -run 'TestGlobWalkPatternSemantics|TestGlobToRegexAlwaysCompiles|TestWalkFilesSkipDirsAndNamedRoot|TestWalkFilesStopAndDenyRead'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/tools/walk_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] base.go, fsutil.go, http.go, isolation_test.go, mcp.go 等 15 个
//
// 主要符号
//
//	writeTree(t *testing.T, root string, files ...string)
//	TestGlobWalkPatternSemantics(t *testing.T)
//	TestGlobToRegexAlwaysCompiles(t *testing.T)
//	TestWalkFilesSkipDirsAndNamedRoot(t *testing.T)
//	TestWalkFilesStopAndDenyRead(t *testing.T)
//	TestWalkFilesCap(t *testing.T)
//	BenchmarkGlobWalk(b *testing.B)
//	BenchmarkGlobWalkWithSymlinkGuard(b *testing.B)

package tools

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, rel := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGlobWalkPatternSemantics(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "a.go", "b.txt", "sub/c.go", "sub/deep/d.go", "x(1).go", "q1.go", "q12.go")
	cases := []struct {
		pattern string
		want    []string
	}{
		{"*.go", []string{"a.go", "q1.go", "q12.go", "x(1).go"}},
		{"**/*.go", []string{"a.go", "q1.go", "q12.go", "sub/c.go", "sub/deep/d.go", "x(1).go"}}, // **/ 匹配零层
		{"sub/**", []string{"sub/c.go", "sub/deep/d.go"}},
		{"q?.go", []string{"q1.go"}},     // ? 恰好一个字符
		{"x(1).go", []string{"x(1).go"}}, // 正则元字符按字面量
		{"nomatch/*.go", nil},            // 无匹配：nil 而不是空切片
	}
	for _, tc := range cases {
		got, err := globWalkSkip(root, tc.pattern, nil)
		if err != nil {
			t.Fatalf("%q: %v", tc.pattern, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q => %v, want %v", tc.pattern, got, tc.want)
		}
	}
}

// 非法模式：视为全不匹配，不报错、不 panic（这里用会被转义成非法正则的括号序列难以构造，
// 所以直接覆盖 globToRegex 对元字符的转义：任何输入都必须产生可编译的正则）。
func TestGlobToRegexAlwaysCompiles(t *testing.T) {
	for _, p := range []string{"[", "(", "a{b", "a\\", "a|b", "^$", "**", "***", "a/**/b", "?*?"} {
		if _, err := globWalkSkip(t.TempDir(), p, nil); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
}

// walkFiles：跳过依赖/产物目录，但根目录自身即使叫 dist 也要遍历。
func TestWalkFilesSkipDirsAndNamedRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "dist")
	writeTree(t, root, "keep.txt", "node_modules/skip.txt", "vendor/skip.txt", "ok/inner.txt")
	var got []string
	if err := walkFiles(root, nil, func(rel string) bool { got = append(got, rel); return true }); err != nil {
		t.Fatal(err)
	}
	want := []string{"keep.txt", "ok/inner.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// walkFiles：visit 返回 false 立即结束；deny_read 目录整体跳过且不计数。
func TestWalkFilesStopAndDenyRead(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "a.txt", "b.txt", "secret/c.txt")
	var seen int
	_ = walkFiles(root, nil, func(string) bool { seen++; return false })
	if seen != 1 {
		t.Fatalf("visit must stop after the first false, saw %d", seen)
	}
	var got []string
	skip := func(p string, _ fs.DirEntry) bool { return filepath.Base(p) == "secret" }
	_ = walkFiles(root, skip, func(rel string) bool { got = append(got, rel); return true })
	if !reflect.DeepEqual(got, []string{"a.txt", "b.txt"}) {
		t.Fatalf("got %v", got)
	}
}

// 文件数上限：恰好 maxWalkFiles 个全部可见，多一个则截断。
func TestWalkFilesCap(t *testing.T) {
	if testing.Short() {
		t.Skip("creates 20k files")
	}
	root := t.TempDir()
	for i := 0; i < maxWalkFiles+1; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f%05d", i)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	_ = walkFiles(root, nil, func(string) bool { n++; return true })
	if n != maxWalkFiles {
		t.Fatalf("visited %d, want cap %d", n, maxWalkFiles)
	}
}

func BenchmarkGlobWalk(b *testing.B) {
	root := b.TempDir()
	for i := 0; i < 2000; i++ {
		p := filepath.Join(root, fmt.Sprintf("d%02d", i%20), fmt.Sprintf("f%04d.go", i))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := globWalkSkip(root, "**/*.go", nil); err != nil {
			b.Fatal(err)
		}
	}
}

// 与 BenchmarkGlobWalk 同一棵树，但使用真实的 walkSkipper（每个条目解析符号链接）。
func BenchmarkGlobWalkWithSymlinkGuard(b *testing.B) {
	root := b.TempDir()
	for i := 0; i < 2000; i++ {
		p := filepath.Join(root, fmt.Sprintf("d%02d", i%20), fmt.Sprintf("f%04d.go", i))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	ws := &Workspace{Root: root}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := globWalkSkip(root, "**/*.go", ws.walkSkipper()); err != nil {
			b.Fatal(err)
		}
	}
}
