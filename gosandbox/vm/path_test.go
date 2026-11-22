package vm

import (
	"errors"
	"testing"
)

func TestCleanPathRootWorkdir(t *testing.T) {
	for _, tc := range []struct {
		name      string
		workdir   string
		path      string
		allowRoot bool
		want      string
		invalid   bool
	}{
		{name: "child", workdir: "/", path: "/etc/../tmp/a", allowRoot: true, want: "/tmp/a"},
		{name: "root", workdir: "/", path: "/", allowRoot: true, want: "/"},
		{name: "root forbidden", workdir: "/", path: "/", invalid: true},
		{name: "trailing slash", workdir: "/workspace/", path: "/workspace/a", want: "/workspace/a"},
		{name: "relative workdir", workdir: "workspace", path: "/workspace/a", invalid: true},
		{name: "empty workdir", workdir: "", path: "/workspace/a", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CleanPath(tc.workdir, tc.path, tc.allowRoot)
			if tc.invalid {
				if !errors.Is(err, ErrInvalidPath) {
					t.Fatalf("err = %v, want ErrInvalidPath", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("CleanPath(%q, %q) = %q, %v; want %q", tc.workdir, tc.path, got, err, tc.want)
			}
		})
	}
}

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
			got, err := CleanPath("/workspace", tc.input, tc.allowRoot)
			if tc.invalid {
				if !errors.Is(err, ErrInvalidPath) {
					t.Fatalf("CleanPath(%q) error = %v", tc.input, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("CleanPath(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
			}
		})
	}
}
