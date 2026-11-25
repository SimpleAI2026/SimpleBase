//go:build linux

// 文件说明：contracts_linux_test.go — gosandbox/vm/box 的单元测试（TestFileKindContract、TestResolveArgvContract 等）
//
// 用法
//
//	go test ./gosandbox/vm/box -run 'TestFileKindContract|TestResolveArgvContract|TestLogDirectoryContract'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/contracts_linux_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] capabilities.go, diskuse_linux.go, driver_linux.go, driver_other.go, driver_test.go 等 15 个
//
// 主要符号
//
//	TestFileKindContract(t *testing.T)
//	TestResolveArgvContract(t *testing.T)
//	TestLogDirectoryContract(t *testing.T)

package box

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

func TestFileKindContract(t *testing.T) {
	for _, tc := range []struct {
		kind proto.FileKind
		want string
	}{
		{proto.FileRegular, "file"}, {proto.FileDir, "directory"}, {proto.FileSymlink, "symlink"}, {proto.FileOther, "other"}, {0, "other"}, {255, "other"},
	} {
		if got := kindOf(tc.kind); got != tc.want {
			t.Fatalf("kind %d: %q", tc.kind, got)
		}
		if got := kindOf(tc.kind); got != tc.want {
			t.Fatalf("watch kind %d: %q", tc.kind, got)
		}
	}
}

func TestResolveArgvContract(t *testing.T) {
	for _, tc := range []struct {
		shell, cmd string
		args, want []string
		ok         bool
	}{
		{"echo hi", "ignored", []string{"ignored"}, []string{"/bin/sh", "-c", "echo hi"}, true},
		{"", "tool", []string{"a", "b"}, []string{"tool", "a", "b"}, true},
		{"", "", []string{"ignored"}, []string{"", "ignored"}, false},
	} {
		got, ok := resolveArgv(tc.shell, tc.cmd, tc.args)
		if ok != tc.ok || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("argv: %v, %v", got, ok)
		}
	}
}

func TestLogDirectoryContract(t *testing.T) {
	d := &Driver{opts: Options{Root: t.TempDir()}}
	dir := d.logDir("test")
	if names, err := logNamesIn(dir); names != nil || err != os.ErrNotExist {
		t.Fatalf("missing: %v, %v", names, err)
	}
	if logs, err := d.Logs(context.Background(), "test", vm.LogQuery{}); logs != nil || err != nil {
		t.Fatalf("missing logs: %v, %v", logs, err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if names, err := logNamesIn(dir); names != nil || err != nil {
		t.Fatalf("empty: %v, %v", names, err)
	}
	if logs, err := d.Logs(context.Background(), "test", vm.LogQuery{}); logs == nil || len(logs) != 0 || err != nil {
		t.Fatalf("empty logs: %v, %v", logs, err)
	}
	file := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := logNamesIn(file); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file: %v", err)
	}
}
