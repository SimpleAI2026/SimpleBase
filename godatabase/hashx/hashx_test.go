// 文件说明：hashx_test.go — godatabase/hashx 的单元测试（TestSum、TestShort 等）
//
// 用法
//
//	go test ./godatabase/hashx -run 'TestSum|TestShort'
//
// 调用架构
//
//	[上游] godatabase/blob, godatabase/ducklake, internal/api, internal/auth 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/hashx/hashx_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] hashx.go
//
// 主要符号
//
//	const emptySum
//	TestSum(t *testing.T)
//	TestShort(t *testing.T)

package hashx

import "testing"

const emptySum = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestSum(t *testing.T) {
	if got := Sum(nil); got != emptySum {
		t.Fatalf("Sum(nil) = %s", got)
	}
	if got := SumString(""); got != emptySum {
		t.Fatalf("SumString(empty) = %s", got)
	}
	if Sum([]byte("a")) != SumString("a") {
		t.Fatal("Sum and SumString disagree")
	}
}

func TestShort(t *testing.T) {
	if got := Short(nil, 8); got != emptySum[:16] {
		t.Fatalf("Short(nil, 8) = %s", got)
	}
	if Short([]byte("x"), 0) != "" || Short([]byte("x"), -1) != "" {
		t.Fatal("n<=0 should be empty")
	}
	if got := Short([]byte("x"), 100); got != Sum([]byte("x")) {
		t.Fatalf("n>32 should clamp to full digest, got %s", got)
	}
}
