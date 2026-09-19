package gofunction

// 本文件承载运行时/执行器混合场景。

import (
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"testing"
	"time"
)

func TestRunWithContextRecover(t *testing.T) {
	p := &Program{}
	_, _, err := p.RunWithContext("t", "x")
	if err == nil {
		t.Fatal("expected recover error")
	}
}

func TestExecutorTimeoutAndInterruptProgram(t *testing.T) {
	ex := NewExecutor(nil)
	ex.SetExecutionTimeout(50)
	got, err := ex.Execute("test", `package main
func test() int { return 1 }`)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("got %#v", got)
	}
	ex.SetExecutionTimeout(0)
	ex.StartTimeoutTimer("timeout")
	time.Sleep(5 * time.Millisecond)
	ex.StopTimeoutTimer()

	p, err := BuildProgram("t", "main", `package main
func test() int { return 1 }`)
	if err != nil {
		t.Fatal(err)
	}
	ex.mu.Lock()
	ex.vm.Program = p
	ex.mu.Unlock()
	if err := ex.Interrupt("stop"); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeNilUserData(t *testing.T) {
	r := NewRuntime()
	r.userData = nil
	if _, ok := r.GetUserData("x"); ok {
		t.Fatal("expected missing")
	}
	r.SetUserData("x", 1)
}

// TestExecutorTimeoutInterruptsExecution 端到端验证超时真正中断执行（P2-4）：
// 死循环脚本在超时后被取消，Execute 返回错误而非永久阻塞。
func TestExecutorTimeoutInterruptsExecution(t *testing.T) {
	ex := NewExecutor(nil)
	ex.SetExecutionTimeout(100) // 100ms
	defer ex.Close()

	start := time.Now()
	_, err := ex.Execute("test", `package main
func test() int {
	for {}
	return 0
}`)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error for infinite loop")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("execution not interrupted, elapsed %v", elapsed)
	}
	if !ex.IsInterrupted() {
		t.Fatal("expected interrupted flag")
	}
}

// TestExecutorInterruptCancelsRuningExecution 验证手动 Interrupt
// 能取消正在运行的脚本执行（经 cancelFunc 真正生效）。
func TestExecutorInterruptCancelsRunning(t *testing.T) {
	ex := NewExecutor(nil)
	ex.SetExecutionTimeout(-1) // 关闭自动超时
	defer ex.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := ex.Execute("test", `package main
func test() int {
	x := 0
	for {
		x++
	}
	return x
}`)
		errCh <- err
	}()

	time.Sleep(50 * time.Millisecond) // 等执行进入死循环
	if err := ex.Interrupt("manual stop"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error after interrupt")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execution not interrupted within 5s")
	}
}

func TestExecutorNilVMSequence(t *testing.T) {
	ex := NewExecutor(nil)
	ex.vm = nil
	if ex.GetSequenceID() != "" {
		t.Fatal("empty id")
	}
	ex.SetSequenceID("z") // vm nil, no-op
	ex.vm = nil
	ex.Close()
}
