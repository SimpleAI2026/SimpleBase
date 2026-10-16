package interp

import (
	"testing"
)

// TestExecutorSetProfileAndCleanup 验证 README 声明的预留接口契约：
// SetProfile/GetProfile 为 no-op（恒 nil），SetWaitEventLoop 只存字段不参与执行，
// Runtime.Cleanup 为空实现。断言落在返回值与字段状态上，而非只验证"不 panic"。
func TestExecutorSetProfileAndCleanup(t *testing.T) {
	e := NewExecutor(nil)
	e.SetProfile(true)
	if got := e.GetProfile(); got != nil {
		t.Fatalf("GetProfile must stay nil (reserved), got %v", got)
	}
	// SetProfile 是 no-op，不影响执行路径。
	result, err := e.Execute("answer", `package main
func answer() int { return 1 }`)
	if err != nil || result != 1 {
		t.Fatalf("SetProfile must not affect execution: %v %v", result, err)
	}
	e.SetWaitEventLoop(true)
	if !e.waitEventLoop {
		t.Fatal("SetWaitEventLoop should persist the field (even though Execute ignores it)")
	}
	r := NewRuntime()
	r.Cleanup() // 预留空实现：不得 panic、不得破坏后续使用
	r.SetUserData("k", "v")
	if v, ok := r.GetUserData("k"); !ok || v != "v" {
		t.Fatalf("Cleanup must not corrupt runtime state: %v %v", v, ok)
	}
	id := newSequenceID()
	if len(id) < 5 || id[:4] != "seq-" {
		t.Fatalf("newSequenceID format: %q", id)
	}
}

func TestValidateHTTPFuncsInitAndMethod(t *testing.T) {
	src := `package main
type S struct{}
func (s S) M(x int) int { return x }
func init() {}
func Hello(m map[string]interface{}) string { return "ok" }
`
	infos, err := ValidateHTTPFuncs(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Name != "Hello" {
		t.Fatalf("%+v", infos)
	}
}
