package sandbox

import (
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// 路径规则的表驱动用例随实现迁到 gosandbox/vm/path_test.go。

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
			got, truncated := strutil.TruncateValidUTF8([]byte(tc.input), tc.max)
			if got != tc.want || truncated != tc.truncated {
				t.Fatalf("truncateUTF8(%q, %d) = %q, %t; want %q, %t", tc.input, tc.max, got, truncated, tc.want, tc.truncated)
			}
		})
	}
}
