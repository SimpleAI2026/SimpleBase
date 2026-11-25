// 文件说明：executil_test.go — gosandbox/internal/executil 的单元测试（TestCappedBuffer、TestHasEnv 等）
//
// 用法
//
//	go test ./gosandbox/internal/executil -run 'TestCappedBuffer|TestHasEnv'
//
// 调用架构
//
//	[上游] gosandbox/host, gosandbox/vm/box, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/internal/executil/executil_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] executil.go
//
// 主要符号
//
//	TestCappedBuffer(t *testing.T)
//	TestHasEnv(t *testing.T)

package executil

import "testing"

func TestCappedBuffer(t *testing.T) {
	for _, tc := range []struct {
		name      string
		max       int
		writes    []string
		want      string
		truncated bool
	}{
		{name: "unlimited", writes: []string{"abc", "def"}, want: "abcdef"},
		{name: "exact", max: 4, writes: []string{"abcd"}, want: "abcd"},
		{name: "single overflow", max: 4, writes: []string{"abcdefgh"}, want: "abcd", truncated: true},
		{name: "multiple writes", max: 4, writes: []string{"ab", "cd", "ef"}, want: "abcd", truncated: true},
		{name: "empty write", max: 4, writes: []string{"", "abcde"}, want: "abcd", truncated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := NewCappedBuffer(tc.max)
			for _, write := range tc.writes {
				n, err := b.Write([]byte(write))
				if err != nil || n != len(write) {
					t.Fatalf("Write(%q) = %d, %v", write, n, err)
				}
			}
			if string(b.Bytes()) != tc.want || b.Truncated() != tc.truncated {
				t.Fatalf("Bytes = %q, Truncated = %v; want %q, %v", b.Bytes(), b.Truncated(), tc.want, tc.truncated)
			}
		})
	}
}

func TestHasEnv(t *testing.T) {
	env := []string{"PATH=/bin", "TERM=xterm", "EMPTY="}
	for _, key := range []string{"PATH", "TERM", "EMPTY"} {
		if !HasEnv(env, key) {
			t.Errorf("missing %q", key)
		}
	}
	for _, key := range []string{"", "PATHS", "TER", "MISSING"} {
		if HasEnv(env, key) {
			t.Errorf("unexpected match for %q", key)
		}
	}
}
