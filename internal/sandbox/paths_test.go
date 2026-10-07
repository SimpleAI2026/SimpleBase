package sandbox

import (
	"errors"
	"testing"
)

func TestCleanPath(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		allowRoot         bool
		invalid           bool
	}{
		{name: "file", input: "/workspace/a.txt", want: "/workspace/a.txt"},
		{name: "clean", input: "/workspace//sub/../a.txt", want: "/workspace/a.txt"},
		{name: "root allowed", input: "/workspace", want: "/workspace", allowRoot: true},
		{name: "empty", invalid: true},
		{name: "relative", input: "a.txt", invalid: true},
		{name: "outside", input: "/etc/passwd", invalid: true},
		{name: "traversal", input: "/workspace/../etc/passwd", invalid: true},
		{name: "prefix", input: "/workspacex/a.txt", invalid: true},
		{name: "root forbidden", input: "/workspace", invalid: true},
		{name: "nul", input: "/workspace/a\x00b", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanPath("/workspace", tc.input, tc.allowRoot)
			if tc.invalid {
				if !errors.Is(err, ErrInvalidPath) {
					t.Fatalf("cleanPath(%q) error = %v", tc.input, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("cleanPath(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
			}
		})
	}
}

func TestTruncateUTF8(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		max               int
		truncated         bool
	}{
		{name: "short", input: "abc", max: 5, want: "abc"},
		{name: "no limit", input: "abc", max: 0, want: "abc"},
		{name: "ascii", input: "abcdef", max: 3, want: "abc", truncated: true},
		{name: "multibyte", input: "你好a", max: 4, want: "你", truncated: true},
		{name: "invalid utf8", input: "\xff", max: 3, want: "�"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, truncated := truncateUTF8([]byte(tc.input), tc.max)
			if got != tc.want || truncated != tc.truncated {
				t.Fatalf("truncateUTF8(%q, %d) = %q, %t; want %q, %t", tc.input, tc.max, got, truncated, tc.want, tc.truncated)
			}
		})
	}
}
