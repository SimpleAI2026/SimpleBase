package gofunction

import (
	"testing"
)

func TestExecutorSetProfileAndCleanup(t *testing.T) {
	e := NewExecutor(nil)
	e.SetProfile(true)
	if e.GetProfile() != nil {
		t.Fatal("profile reserved")
	}
	e.SetWaitEventLoop(true)
	r := NewRuntime()
	r.Cleanup()
	_ = newSequenceID()
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
