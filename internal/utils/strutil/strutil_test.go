package strutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

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

func TestTruncateValidUTF8(t *testing.T) {
	// 截断点落在多字节字符中间时回退到字符边界。
	s := []byte("零一二")
	got, truncated := TruncateValidUTF8(s, 4)
	if !truncated {
		t.Fatal("expected truncated=true")
	}
	if got != "零" {
		t.Fatalf("got %q, want %q", got, "零")
	}
	if n, _ := TruncateValidUTF8(s, 100); n != "零一二" {
		t.Fatalf("no-truncation case got %q", n)
	}
	// 非法字节被替换为 U+FFFD，仍然是合法 UTF-8。
	bad, _ := TruncateValidUTF8([]byte{0xff, 0xfe}, 0)
	if !utf8.ValidString(bad) {
		t.Fatalf("invalid UTF-8 survived: %q", bad)
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
