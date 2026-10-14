package app

import (
	"os"
	"testing"
)

// TestMain 为整包测试显式开启 fake 沙盒后端。
// fake 后端只在 SIMPLEBASE_SANDBOX_ALLOW_FAKE=1 时允许（替代旧开发模式）。
func TestMain(m *testing.M) {
	_ = os.Setenv("SIMPLEBASE_SANDBOX_ALLOW_FAKE", "1")
	os.Exit(m.Run())
}
