// 文件说明：process_test.go — gocloudagent/cloudagent 的单元测试（TestProcessHelper、TestProcessRunnerCapturesStdoutStderrSeparately 等）
//
// 用法
//
//	go test ./gocloudagent/cloudagent -run 'TestProcessHelper|TestProcessRunnerCapturesStdoutStderrSeparately|TestProcessRunnerNonZeroExitIsNotAnError|TestProcessRunnerReplacesEnvironment'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/process_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//
// 主要符号
//
//	TestProcessHelper(t *testing.T)
//	helperRunner(t *testing.T) (ProcessRunner, []string)
//	helperEnv(mode string, extra ...string) []string
//	TestProcessRunnerCapturesStdoutStderrSeparately(t *testing.T)
//	TestProcessRunnerNonZeroExitIsNotAnError(t *testing.T)
//	TestProcessRunnerReplacesEnvironment(t *testing.T)
//	TestProcessRunnerStdin(t *testing.T)
//	TestProcessRunnerUsesDisposableWorkingDir(t *testing.T)
//	... 另有 9 个顶层符号

package cloudagent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

// helper 子进程：测试二进制以 PROCESS_HELPER=<mode> 重新执行自己。
func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("PROCESS_HELPER")
	if mode == "" {
		t.Skip("helper only")
	}
	switch mode {
	case "echo":
		os.Stdout.WriteString("out:" + strings.Join(os.Args[3:], ",") + "\n")
		os.Stderr.WriteString("err\n")
	case "env":
		os.Stdout.WriteString(os.Getenv("PROCESS_SENTINEL") + "|" + os.Getenv("PARENT_ONLY"))
	case "stdin":
		var b bytes.Buffer
		b.ReadFrom(os.Stdin)
		os.Stdout.WriteString("got:" + b.String())
	case "big":
		// 远超上限的输出：任何一路都必须被排空，否则子进程会阻塞。
		chunk := bytes.Repeat([]byte("x"), 1<<10)
		for i := 0; i < 512; i++ {
			os.Stdout.Write(chunk)
			os.Stderr.Write(chunk)
		}
	case "cwd":
		wd, _ := os.Getwd()
		os.Stdout.WriteString(wd)
	case "exit3":
		os.Stderr.WriteString("boom")
		os.Exit(3)
	}
	os.Exit(0)
}

func helperRunner(t *testing.T) (ProcessRunner, []string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return ProcessRunner{Path: exe, TempDirPrefix: "cloudagent-test-"}, []string{"-test.run=TestProcessHelper", "--"}
}

func helperEnv(mode string, extra ...string) []string {
	return append([]string{"PROCESS_HELPER=" + mode}, extra...)
}

func TestProcessRunnerCapturesStdoutStderrSeparately(t *testing.T) {
	r, base := helperRunner(t)
	out, errOut, code, err := r.Run(context.Background(), append(base, "a", "b"), helperEnv("echo"), "")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(out, "out:a,b") || strings.Contains(out, "err") || errOut != "err\n" {
		t.Fatalf("stdout=%q stderr=%q", out, errOut)
	}
}

func TestProcessRunnerNonZeroExitIsNotAnError(t *testing.T) {
	r, base := helperRunner(t)
	_, errOut, code, err := r.Run(context.Background(), base, helperEnv("exit3"), "")
	if err != nil || code != 3 || errOut != "boom" {
		t.Fatalf("code=%d err=%v stderr=%q", code, err, errOut)
	}
}

// 显式环境整体替换，不继承父进程环境。
func TestProcessRunnerReplacesEnvironment(t *testing.T) {
	t.Setenv("PARENT_ONLY", "leaked")
	r, base := helperRunner(t)
	out, _, _, err := r.Run(context.Background(), base, helperEnv("env", "PROCESS_SENTINEL=ok"), "")
	if err != nil || !strings.HasSuffix(strings.TrimSpace(out), "ok|") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestProcessRunnerStdin(t *testing.T) {
	r, base := helperRunner(t)
	out, _, _, err := r.Run(context.Background(), base, helperEnv("stdin"), "hello")
	if err != nil || !strings.Contains(out, "got:hello") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

// 工作目录是空的临时目录，并在运行后删除。
func TestProcessRunnerUsesDisposableWorkingDir(t *testing.T) {
	r, base := helperRunner(t)
	out, _, _, err := r.Run(context.Background(), base, helperEnv("cwd"), "")
	if err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSpace(out)
	if !strings.Contains(filepath.Base(dir), "cloudagent-test-") {
		t.Fatalf("cwd=%q", dir)
	}
	if _, statErr := os.Stat(dir); statErr == nil {
		t.Fatalf("temp dir %q must be removed after the run", dir)
	}
}

// BUG-01：输出远超上限时命令仍然成功，且两路输出都被截断到上限。
func TestProcessRunnerOverLimitOutputStillSucceeds(t *testing.T) {
	r, base := helperRunner(t)
	r.OutputLimit = 4096
	out, errOut, code, err := r.Run(context.Background(), base, helperEnv("big"), "")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if len(out) != 4096 || len(errOut) != 4096 {
		t.Fatalf("stdout=%d stderr=%d, want 4096 each", len(out), len(errOut))
	}
}

func TestProcessRunnerRequiresPath(t *testing.T) {
	if _, _, _, err := (ProcessRunner{}).Run(context.Background(), nil, nil, ""); err == nil {
		t.Fatal("empty path must fail")
	}
}

func TestProcessRunnerStartFailureKeepsError(t *testing.T) {
	_, _, _, err := ProcessRunner{Path: filepath.Join(t.TempDir(), "missing")}.Run(context.Background(), nil, []string{}, "")
	if err == nil {
		t.Fatal("start failure must return an error")
	}
	if got := ProcessStartFailure(err); got != "not_found" {
		t.Fatalf("category=%q", got)
	}
}

// limitWriter 必须报告完整输入长度（io.Writer 契约），同时只保留上限内的字节。
func TestLimitWriterReportsFullLength(t *testing.T) {
	var buf bytes.Buffer
	w := &limitWriter{w: &buf, n: 5}
	for _, chunk := range []string{"abc", "defgh", "ijk"} {
		n, err := w.Write([]byte(chunk))
		if n != len(chunk) || err != nil {
			t.Fatalf("Write(%q)=%d,%v", chunk, n, err)
		}
	}
	if buf.String() != "abcde" {
		t.Fatalf("kept=%q", buf.String())
	}
}

func TestLimitProcessOutputKeepsValidUTF8(t *testing.T) {
	const limit = 65536
	if got := LimitProcessOutput("短", limit); got != "短" {
		t.Fatalf("short string changed: %q", got)
	}
	exact := strings.Repeat("a", limit)
	if got := LimitProcessOutput(exact, limit); got != exact {
		t.Fatalf("exact-limit ASCII changed: len=%d", len(got))
	}
	if got := LimitProcessOutput(exact+"bcd", limit); len(got) != limit {
		t.Fatalf("long ASCII len=%d", len(got))
	}
	// 「中」3 字节，65536 % 3 == 1：上限处切断半个字符。
	cut := LimitProcessOutput(strings.Repeat("中", limit), limit)
	if !utf8.ValidString(cut) || len(cut) != limit-1 {
		t.Fatalf("len=%d valid=%v", len(cut), utf8.ValidString(cut))
	}
	// 多字节恰在边界结束时不多删。
	pad := strings.Repeat("a", limit%3) + strings.Repeat("中", limit/3)
	if got := LimitProcessOutput(pad+"中", limit); got != pad {
		t.Fatalf("boundary-aligned input over-trimmed: len=%d want %d", len(got), len(pad))
	}
	// 原本无效的字节不被规范化。
	bad := strings.Repeat("\xff", 10)
	if got := LimitProcessOutput(bad, limit); got != bad {
		t.Fatalf("invalid bytes altered: %q", got)
	}
}

func TestValidateExecutableCategories(t *testing.T) {
	if got := ValidateExecutable("  "); got != "not_configured" {
		t.Fatalf("blank=%q", got)
	}
	dir := t.TempDir()
	if got := ValidateExecutable(filepath.Join(dir, "missing")); got != "not_found" {
		t.Fatalf("missing=%q", got)
	}
	if got := ValidateExecutable(dir); got != "not_executable" {
		t.Fatalf("dir=%q", got)
	}
	if runtime.GOOS == "windows" {
		return
	}
	file := filepath.Join(dir, "tool")
	if err := os.WriteFile(file, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ValidateExecutable(file); got != "not_executable" {
		t.Fatalf("no exec bit=%q", got)
	}
	if err := os.Chmod(file, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ValidateExecutable(file); got != "" {
		t.Fatalf("executable=%q", got)
	}
}

func TestProcessStartFailureCategories(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&os.PathError{Err: os.ErrNotExist}, "not_found"},
		{&os.PathError{Err: os.ErrPermission}, "not_executable"},
		{&os.PathError{Err: errString("bad executable format")}, "invalid_binary"},
		{errString("something else"), "start_failed"},
	}
	for _, tc := range cases {
		if got := ProcessStartFailure(tc.err); got != tc.want {
			t.Errorf("%v => %q want %q", tc.err, got, tc.want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }
