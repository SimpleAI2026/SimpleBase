// 文件说明：strutil_test.go — internal/utils/strutil 的单元测试（TestNormalize、TestTruncate 等）
//
// 用法
//
//	go test ./internal/utils/strutil -run 'TestNormalize|TestTruncate|TestTruncateRunes|TestTruncateEllipsis'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api, internal/app, internal/auth 等 12 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/utils/strutil/strutil_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] strutil.go
//
// 主要符号
//
//	TestNormalize(t *testing.T)
//	TestTruncate(t *testing.T)
//	TestTruncateRunes(t *testing.T)
//	TestTruncateEllipsis(t *testing.T)
//	TestSplitCSV(t *testing.T)

package strutil

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"":               "",
		"  Foo\t":        "foo",
		"\u00a0中文\u00a0": "中文",
		"already":        "already",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"abc", 10, "abc"},
		{"abc", 3, "abc"},
		{"abcdef", 3, "abc"},
		{"abc", 0, ""},
		{"abc", -1, ""},
		{"", 5, ""},
	}
	for _, c := range cases {
		if got := Truncate(c.in, c.max); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"abc", 10, "abc"},
		{"abc", 3, "abc"},
		{"零一二", 2, "零一"},
		{"abc", 0, ""},
		{"abc", -1, ""},
		{"", 5, ""},
	}
	for _, c := range cases {
		if got := TruncateRunes(c.in, c.max); got != c.want {
			t.Errorf("TruncateRunes(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
	shortInvalid := string([]byte{0xff})
	if got := TruncateRunes(shortInvalid, 1); got != shortInvalid {
		t.Fatalf("short invalid UTF-8 changed: %q", got)
	}
}

func TestTruncateEllipsis(t *testing.T) {
	if got := TruncateEllipsis("abc", 10); got != "abc" {
		t.Errorf("short string changed: %q", got)
	}
	if got := TruncateEllipsis("abcdefghij", 3); got != "abc…" {
		t.Errorf("got %q, want %q", got, "abc…")
	}
	if got := TruncateEllipsis("abc", 0); got != "…" {
		t.Errorf("max=0 got %q, want …", got)
	}
}

func TestSplitCSV(t *testing.T) {
	got := SplitCSV(" database:read , ,database:admin ")
	want := []string{"database:read", "database:admin"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	// 空输入返回非 nil 空切片，便于 JSON 序列化为 []。
	for _, in := range []string{"", "   ", ",,,"} {
		got := SplitCSV(in)
		if got == nil {
			t.Fatalf("SplitCSV(%q) returned nil", in)
		}
		if len(got) != 0 {
			t.Fatalf("SplitCSV(%q) = %v, want empty", in, got)
		}
	}
	if got := SplitCSV("a"); strings.Join(got, "|") != "a" {
		t.Fatalf("single value got %v", got)
	}
}
