//go:build linux

// 文件说明：exec_linux_test.go — gosandbox/vm/box 的单元测试（TestMain、TestStartDefaultShellAndEmptyExec 等）
//
// 用法
//
//	go test ./gosandbox/vm/box -run 'TestMain|TestStartDefaultShellAndEmptyExec'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/exec_linux_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] capabilities.go, contracts_linux_test.go, diskuse_linux.go, driver_linux.go, driver_other.go 等 15 个
//
// 主要符号
//
//	TestMain(m *testing.M)
//	TestStartDefaultShellAndEmptyExec(t *testing.T)

package box

import (
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/agent"
	backend "github.com/linkxzhou/SimpleBase/gosandbox/box"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

func TestMain(m *testing.M) {
	if os.Getenv("GOSANDBOX_EXEC_TEST_HELPER") != "1" {
		os.Exit(m.Run())
	}
	ctrl := os.NewFile(4, "control")
	if ctrl == nil {
		os.Exit(2)
	}
	if err := agent.Serve(ctrl, ctrl, io.Discard); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestStartDefaultShellAndEmptyExec(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	s, err := backend.Start(ctx, backend.Config{Name: "test", Root: root, Workspace: root, Helper: exe, Env: []string{"GOSANDBOX_EXEC_TEST_HELPER=1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d := &Driver{opts: Options{Root: root, Workdir: root}, sessions: map[string]*backend.Session{"test": s}, locks: map[string]*sync.Mutex{}}
	target := vm.Target{Name: "test"}
	res, err := d.Exec(ctx, target, vm.ExecSpec{Args: []string{"ignored"}})
	if err != nil || res.ExitCode != 127 {
		t.Fatalf("empty exec: %+v, %v", res, err)
	}
	p, err := d.Start(ctx, target, vm.StartSpec{Args: []string{"ignored"}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	drained := make(chan struct{})
	go func() { defer close(drained); _, _ = io.Copy(io.Discard, p.Output()) }()
	if _, err := io.WriteString(p.Stdin(), "case $- in *i*) exit 23;; *) exit 24;; esac\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.Done():
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if got := p.Wait(); got.ExitCode != 23 {
		t.Fatalf("default shell is not interactive: %+v", got)
	}
	select {
	case <-drained:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
