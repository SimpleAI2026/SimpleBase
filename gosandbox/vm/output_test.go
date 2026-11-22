package vm

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8(t *testing.T) {
	for _, tc := range []struct {
		name      string
		input     []byte
		max       int
		want      string
		truncated bool
	}{
		{name: "empty", input: nil, max: 1},
		{name: "short", input: []byte("abc"), max: 5, want: "abc"},
		{name: "no limit", input: []byte("abc"), max: 0, want: "abc"},
		{name: "ascii", input: []byte("abcdef"), max: 3, want: "abc", truncated: true},
		{name: "multibyte", input: []byte("你好a"), max: 4, want: "你", truncated: true},
		{name: "invalid utf8", input: []byte{0xff, 0xfe}, max: 0, want: "�"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, truncated := TruncateUTF8(tc.input, tc.max)
			if got != tc.want || truncated != tc.truncated {
				t.Fatalf("TruncateUTF8(%q, %d) = %q, %t; want %q, %t", tc.input, tc.max, got, truncated, tc.want, tc.truncated)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("invalid UTF-8: %q", got)
			}
		})
	}
}
