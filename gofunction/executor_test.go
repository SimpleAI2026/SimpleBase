package gofunction

import (
	"sync"
	"testing"
	"time"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func TestExecutorPoolGetPutClose(t *testing.T) {
	pool := NewDefaultExecutorPool()
	pool.SetServiceName("gofun")
	if pool.GetServiceName() != "gofun" {
		t.Fatalf("service name: %q", pool.GetServiceName())
	}

	ex, err := pool.GetExecutor()
	if err != nil {
		t.Fatal(err)
	}
	if ex.GetSequenceID() == "" {
		ex.SetSequenceID("seq-test")
	}
	ex.SetExecutionTimeout(-1)
	ex.SetWaitEventLoop(false)
	ex.SetProfile(true)
	if ex.GetProfile() != nil && len(ex.GetProfile()) != 0 {
		t.Fatalf("unexpected profile: %v", ex.GetProfile())
	}
	_ = ex.Compile("n", "script", false)

	result, err := ex.Execute("add", `package main
func add() int { return 40 + 2 }`)
	if err != nil {
		t.Fatal(err)
	}
	if result != 42 {
		t.Fatalf("got %#v", result)
	}
	if ex.GetCostTime() < 0 {
		t.Fatal("cost time should be recorded")
	}

	ex.Close() // put back
	active, idle := pool.GetStats()
	if active < 1 && idle < 1 {
		t.Fatalf("stats active=%d idle=%d", active, idle)
	}

	done := make(chan struct{})
	go func() {
		pool.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pool.Close deadlocked")
	}
	// second close is a no-op
	pool.Close()
}

func TestExecutorPoolExhausted(t *testing.T) {
	pool := NewExecutorPool(ExecutorPoolConfig{MaxIdle: 0, MaxActive: 1, MaxAge: time.Minute})
	ex, err := pool.GetExecutor()
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.GetExecutor()
	if err != ErrPoolExhausted {
		t.Fatalf("expected ErrPoolExhausted, got %v", err)
	}
	ex.Close()
	pool.Close()
}

func TestExecutorPoolClosedGet(t *testing.T) {
	pool := NewDefaultExecutorPool()
	pool.Close()
	_, err := pool.GetExecutor()
	if err != ErrPoolClosed {
		t.Fatalf("expected ErrPoolClosed, got %v", err)
	}
}

func TestExecutorMaxAgeClose(t *testing.T) {
	pool := NewExecutorPool(ExecutorPoolConfig{MaxIdle: 2, MaxActive: 2, MaxAge: time.Nanosecond})
	ex, err := pool.GetExecutor()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	ex.Close() // expired: decrementActive, not put back
	_, idle := pool.GetStats()
	if idle != 0 {
		t.Fatalf("expired executor should not sit idle, idle=%d", idle)
	}
	pool.Close()
}

func TestExecutorInterrupt(t *testing.T) {
	ex := NewExecutor(nil)
	if ex.vm == nil {
		t.Fatal("vm should be initialized")
	}
	if err := ex.Interrupt("stop"); err != nil {
		t.Fatal(err)
	}
	if !ex.IsInterrupted() {
		t.Fatal("expected interrupted")
	}
	if err := ex.ClearInterrupt(); err != nil {
		t.Fatal(err)
	}
	if ex.IsInterrupted() {
		t.Fatal("expected cleared")
	}
	ex.SetExecutionTimeout(5)
	ex.StartTimeoutTimer("timeout")
	ex.StopTimeoutTimer()
	ex.Close()
}

func TestExecutorExecutePanicRecover(t *testing.T) {
	ex := NewExecutor(nil)
	ex.SetProfile(true)
	_, err := ex.Execute("test", `package main
func test() {
	panic("exec")
}`)
	if err == nil {
		t.Fatal("expected error from unrecovered panic")
	}
}

func TestExecutorExecuteError(t *testing.T) {
	ex := NewExecutor(nil)
	_, err := ex.Execute("missing", "package main\nfunc other() {}")
	if err == nil {
		t.Fatal("expected error for missing function")
	}
	ex.vm = nil
	_, err = ex.Execute("x", "not valid go")
	if err == nil {
		t.Fatal("expected build error")
	}
}

func TestExecutorPoolConcurrentClose(t *testing.T) {
	pool := NewDefaultExecutorPool()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ex, err := pool.GetExecutor()
			if err != nil {
				return
			}
			ex.Close()
		}()
	}
	wg.Wait()
	pool.Close()
}
