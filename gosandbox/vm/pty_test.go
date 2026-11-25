// 文件说明：pty_test.go — gosandbox/vm 的单元测试（TestClampPTYDim 等）
//
// 用法
//
//	go test ./gosandbox/vm -run 'TestClampPTYDim'
//
// 调用架构
//
//	[上游] gosandbox/vm/box, gosandbox/vm/fake, gosandbox/vm/local, internal/api 等 7 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/pty_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] driver.go, env.go, env_test.go, errors.go, output.go 等 9 个
//
// 主要符号
//
//	TestClampPTYDim(t *testing.T)

package vm

import "testing"

func TestClampPTYDim(t *testing.T) {
	if ClampPTYDim(0, 80) != 80 {
		t.Fatal("zero must fall back to the default")
	}
	if ClampPTYDim(50, 80) != 50 {
		t.Fatal("a value within the limit must be kept")
	}
	if ClampPTYDim(PTYDimLimit, 80) != PTYDimLimit {
		t.Fatal("the limit itself is valid")
	}
	if ClampPTYDim(PTYDimLimit+1, 80) != PTYDimLimit {
		t.Fatal("values above the limit must be clamped")
	}
}
