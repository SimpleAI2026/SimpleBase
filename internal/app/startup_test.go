// 文件说明：startup_test.go — internal/app 的单元测试（TestStartupClockNilSafe、TestStartupClockWritesSteps 等）
//
// 用法
//
//	go test ./internal/app -run 'TestStartupClockNilSafe|TestStartupClockWritesSteps|TestSetSimpleBaseEnvOnlyNames|TestDefaultLogLevelIsDebug'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/startup_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_shutdown_test.go, app.go, app_alerts.go, app_lease.go, app_test.go 等 41 个
//
// 主要符号
//
//	TestStartupClockNilSafe(t *testing.T)
//	TestStartupClockWritesSteps(t *testing.T)
//	TestSetSimpleBaseEnvOnlyNames(t *testing.T)
//	TestDefaultLogLevelIsDebug(t *testing.T)

package app

import (
	"bytes"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// startupClock/step 必须在 nil 路径下不 panic：大量测试直接构造 &App{}。
func TestStartupClockNilSafe(t *testing.T) {
	var c *startupClock
	c.step("noop")

	var a *App
	a.step("noop")
	if (&App{}).elapsedMs() != 0 {
		t.Fatal("nil clock should report 0 elapsed")
	}
}

func TestStartupClockWritesSteps(t *testing.T) {
	var buf bytes.Buffer
	logger := observability.NewLogger("debug", "json", &buf)
	c := newStartupClock(logger)
	c.step("first")
	c.step("second", observability.String("k", "v"))

	out := buf.String()
	for _, want := range []string{"startup step: first", "startup step: second", "elapsed_ms", "total_ms", `"k":"v"`} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

// setSimpleBaseEnv 只输出变量名，绝不输出值（密钥不得落日志）。
func TestSetSimpleBaseEnvOnlyNames(t *testing.T) {
	t.Setenv("SIMPLEBASE_LOG_LEVEL", "debug")
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "super-secret-value")
	t.Setenv("PATH", "/usr/bin")

	names := setSimpleBaseEnv()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
		if n == "PATH" {
			t.Fatal("non-SIMPLEBASE_ variables must be filtered out")
		}
	}
	if !found["SIMPLEBASE_LOG_LEVEL"] || !found["SIMPLEBASE_AUTH_APIKEY_SECRET"] {
		t.Fatalf("expected SIMPLEBASE_ names, got %v", names)
	}
	for _, n := range names {
		if n == "super-secret-value" {
			t.Fatal("secret value leaked into env names")
		}
	}
}

// 默认日志级别为 debug：部署与容器下默认就能看到排查所需的上下文。
func TestDefaultLogLevelIsDebug(t *testing.T) {
	if config.DefaultLogLevel != config.LogLevelDebug {
		t.Fatalf("default log level must be debug, got %q", config.DefaultLogLevel)
	}
}
