// 文件说明：limit_test.go — godatabase/limit 的单元测试（TestNormalize、TestClamp 等）
//
// 用法
//
//	go test ./godatabase/limit -run 'TestNormalize|TestClamp'
//
// 调用架构
//
//	[上游] godatabase/blob, internal/audit, internal/auth, internal/catalog 等 7 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/limit/limit_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] limit.go
//
// 主要符号
//
//	TestNormalize(t *testing.T)
//	TestClamp(t *testing.T)

package limit

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		n, def, max, want int
	}{
		{50, 20, 100, 50},
		{0, 20, 100, 20},
		{-1, 20, 100, 20},
		{500, 20, 100, 20},
		{100, 20, 100, 100},
		{5, 20, 0, 5},
		{0, 20, 0, 20},
	}
	for _, tt := range tests {
		if got := Normalize(tt.n, tt.def, tt.max); got != tt.want {
			t.Errorf("Normalize(%d, %d, %d) = %d, want %d", tt.n, tt.def, tt.max, got, tt.want)
		}
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		n, def, max, want int
	}{
		{50, 50, 100, 50},
		{0, 50, 100, 50},
		{-5, 50, 100, 50},
		{500, 50, 100, 100},
		{100, 50, 100, 100},
		{7, 1000, 0, 7},
	}
	for _, tt := range tests {
		if got := Clamp(tt.n, tt.def, tt.max); got != tt.want {
			t.Errorf("Clamp(%d, %d, %d) = %d, want %d", tt.n, tt.def, tt.max, got, tt.want)
		}
	}
}
