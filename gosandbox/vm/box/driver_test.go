//go:build linux

// 文件说明：driver_test.go — gosandbox/vm/box 的单元测试（TestResolveLowerHost、TestResolveLowerWithoutSource 等）
//
// 用法
//
//	go test ./gosandbox/vm/box -run 'TestResolveLowerHost|TestResolveLowerWithoutSource|TestResolveLowerWithSource|TestResolveLowerEmptyPath'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/driver_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] capabilities.go, contracts_linux_test.go, diskuse_linux.go, driver_linux.go, driver_other.go 等 15 个
//
// 主要符号
//
//	type fakeImages struct{…}
//	(f *fakeImages) Resolve(_ context.Context, ref string) (Lower, error)
//	TestResolveLowerHost(t *testing.T)
//	TestResolveLowerWithoutSource(t *testing.T)
//	TestResolveLowerWithSource(t *testing.T)
//	TestResolveLowerEmptyPath(t *testing.T)
//	TestResolveLowerError(t *testing.T)
//	TestResolveCwd(t *testing.T)
//	... 另有 3 个顶层符号

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
	ok := []struct{ in, want string }{
		{"", "/workspace"},
		{"/workspace", "/workspace"},
		{"/workspace/sub", "/workspace/sub"},
		{"/workspace/a/../b", "/workspace/b"},
	}
	for _, tc := range ok {
		got, err := resolveCwd("/workspace", tc.in)
		if err != nil || got != tc.want {
			t.Errorf("resolveCwd(%q) = %q, %v", tc.in, got, err)
		}
	}
	bad := []string{"/etc", "/", "/workspacex", "../etc"}
	for _, in := range bad {
		if _, err := resolveCwd("/workspace", in); err == nil {
			t.Errorf("resolveCwd(%q) must fail", in)
		}
	}
}

func TestTargetWorkdirOverridesDefault(t *testing.T) {
	d := &Driver{opts: Options{Workdir: "/default"}}
	target := vm.Target{Spec: vm.Spec{Workdir: "/custom"}}
	if got := d.workdir(target); got != "/custom" {
		t.Fatalf("workdir = %q", got)
	}
	env := d.commandEnv(d.workdir(target), nil)
	found := false
	for _, entry := range env {
		if entry == "HOME=/custom" {
			found = true
		}
	}
	if !found {
		t.Fatalf("target workdir missing from environment: %v", env)
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
