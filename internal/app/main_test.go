// 文件说明：main_test.go — internal/app 的单元测试（TestMain 等）
//
// 用法
//
//	go test ./internal/app -run 'TestMain'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/main_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_shutdown_test.go, app.go, app_alerts.go, app_lease.go, app_test.go 等 41 个
//
// 主要符号
//
//	TestMain(m *testing.M)

package app

import (
	"flag"
	"os"
	"testing"
)

// TestMain 为整包测试显式开启 fake 沙盒后端，并解析测试标志（arch_gate_test.go 的 -update）。
// fake 后端只在 SIMPLEBASE_SANDBOX_ALLOW_FAKE=1 时允许（替代旧开发模式）。
func TestMain(m *testing.M) {
	_ = os.Setenv("SIMPLEBASE_SANDBOX_ALLOW_FAKE", "1")
	flag.Parse()
	os.Exit(m.Run())
}
