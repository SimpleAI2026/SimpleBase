//go:build linux

package box

// driver_test.go：驱动层的镜像解析与配置校验（不需要 bwrap）。
//
// 起沙盒需要真实 bwrap + helper，那部分由 box/session_test.go 覆盖；
// 这里只测能在任何 Linux 上跑的逻辑：镜像解析、错误分类、路径约束。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// fakeImages 是一个可控的 ImageSource。
type fakeImages struct {
	lower Lower
	err   error
	// asked 记录被问过的引用。
	asked []string
}

// Resolve 实现 ImageSource。
func (f *fakeImages) Resolve(_ context.Context, ref string) (Lower, error) {
	f.asked = append(f.asked, ref)
	if f.err != nil {
		return Lower{}, f.err
	}
	return f.lower, nil
}

// image=host 或空：不需要镜像来源。
func TestResolveLowerHost(t *testing.T) {
	d := &Driver{opts: Options{Workdir: "/workspace"}}
	for _, img := range []string{"", "host"} {
		lower, err := d.resolveLower(context.Background(), vm.Target{Spec: vm.Spec{Image: img}})
		if err != nil {
			t.Fatalf("image %q: %v", img, err)
		}
		if lower.Path != "" {
			t.Fatalf("image %q must use the host root, got %q", img, lower.Path)
		}
	}
}

// 没有镜像来源时，非 host 镜像明确报错，而不是悄悄退到宿主根。
func TestResolveLowerWithoutSource(t *testing.T) {
	d := &Driver{opts: Options{Workdir: "/workspace"}}
	_, err := d.resolveLower(context.Background(), vm.Target{Spec: vm.Spec{Image: "python:3.12-slim"}})
	if !errors.Is(err, vm.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

// 有来源时按引用解析。
func TestResolveLowerWithSource(t *testing.T) {
	src := &fakeImages{lower: Lower{Path: "/lower/abc", ChainID: "abc"}}
	d := &Driver{opts: Options{Workdir: "/workspace", Images: src}}
	lower, err := d.resolveLower(context.Background(), vm.Target{Spec: vm.Spec{Image: "alpine:3.19"}})
	if err != nil {
		t.Fatal(err)
	}
	if lower.Path != "/lower/abc" || lower.ChainID != "abc" {
		t.Fatalf("lower = %+v", lower)
	}
	if len(src.asked) != 1 || src.asked[0] != "alpine:3.19" {
		t.Fatalf("asked = %v", src.asked)
	}
}

// 来源返回空路径：报错而不是挂一个空根。
func TestResolveLowerEmptyPath(t *testing.T) {
	src := &fakeImages{lower: Lower{}}
	d := &Driver{opts: Options{Workdir: "/workspace", Images: src}}
	if _, err := d.resolveLower(context.Background(), vm.Target{Spec: vm.Spec{Image: "alpine"}}); err == nil {
		t.Fatal("empty lower must be rejected")
	}
}

// 来源报错：包装成 ErrBackend。
func TestResolveLowerError(t *testing.T) {
	src := &fakeImages{err: errors.New("not pulled")}
	d := &Driver{opts: Options{Workdir: "/workspace", Images: src}}
	_, err := d.resolveLower(context.Background(), vm.Target{Spec: vm.Spec{Image: "alpine"}})
	if !errors.Is(err, vm.ErrBackend) {
		t.Fatalf("err = %v, want ErrBackend", err)
	}
}

// resolveCwd 只接受工作目录之内的路径。
func TestResolveCwd(t *testing.T) {
	d := &Driver{opts: Options{Workdir: "/workspace"}}
	ok := []struct{ in, want string }{
		{"", "/workspace"},
		{"/workspace", "/workspace"},
		{"/workspace/sub", "/workspace/sub"},
		{"/workspace/a/../b", "/workspace/b"},
	}
	for _, tc := range ok {
		got, err := d.resolveCwd(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("resolveCwd(%q) = %q, %v", tc.in, got, err)
		}
	}
	bad := []string{"/etc", "/", "/workspacex", "../etc"}
	for _, in := range bad {
		if _, err := d.resolveCwd(in); err == nil {
			t.Errorf("resolveCwd(%q) must fail", in)
		}
	}
}

// Status：无目录 = absent；有 ws = running；有 stopped = stopped。
func TestStatus(t *testing.T) {
	root := t.TempDir()
	d := &Driver{opts: Options{Root: root, Workdir: "/workspace"}}
	ctx := context.Background()

	if got, err := d.Status(ctx, "sb1"); err != nil || got != vm.VMAbsent {
		t.Fatalf("absent = %q, %v", got, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sb1", "ws"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.Status(ctx, "sb1"); got != vm.VMRunning {
		t.Fatalf("running = %q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "sb1", "stopped"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.Status(ctx, "sb1"); got != vm.VMStopped {
		t.Fatalf("stopped = %q", got)
	}
}

// resolveArgv：Shell 与 Cmd 二选一。
func TestResolveArgv(t *testing.T) {
	if argv, ok := resolveArgv("echo hi", "", nil); !ok || len(argv) != 3 || argv[0] != "/bin/sh" {
		t.Fatalf("shell = %v %v", argv, ok)
	}
	if argv, ok := resolveArgv("", "/bin/true", nil); !ok || argv[0] != "/bin/true" {
		t.Fatalf("cmd = %v %v", argv, ok)
	}
	if _, ok := resolveArgv("", "", nil); ok {
		t.Fatal("empty command must fail")
	}
}

// cappedBuffer 截断但不让写入失败。
func TestCappedBuffer(t *testing.T) {
	c := &cappedBuffer{max: 4}
	n, err := c.Write([]byte("abcdefgh"))
	if err != nil || n != 8 {
		t.Fatalf("write = %d, %v", n, err)
	}
	if string(c.buf) != "abcd" || !c.truncated {
		t.Fatalf("buf = %q truncated = %v", c.buf, c.truncated)
	}
}
