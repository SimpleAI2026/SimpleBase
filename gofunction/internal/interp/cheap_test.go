package interp

import (
	"testing"
)

// TestExecutorCleanupContract 验证 Runtime.Cleanup 空实现契约：
// 不得 panic、不得破坏后续使用；执行路径不受预留接口删除影响。
func TestExecutorCleanupContract(t *testing.T) {
	e := NewExecutor(nil)
	result, err := e.Execute("answer", `package main
func answer() int { return 1 }`)
	if err != nil || result != 1 {
		t.Fatalf("Execute: %v %v", result, err)
	}
	r := NewRuntime()
	r.Cleanup() // 预留空实现：不得 panic、不得破坏后续使用
	r.Initialize("")
	if r.SequenceID == "" {
		t.Fatal("Cleanup must not corrupt runtime state")
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
