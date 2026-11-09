package interp

import (
	"testing"
	"time"
)

// E 批次缺陷 1：重复 Close 不得重复入池。
// 修复前：Executor.Close 每次都走 putExecutor，同一实例被 PushBack 多次，
// 空闲列表出现重复元素，GetExecutor 两次拿到同一实例。
func TestExecutorDoubleCloseDoesNotDuplicate(t *testing.T) {
	pool := NewExecutorPool(ExecutorPoolConfig{MaxIdle: 4, MaxActive: 4, MaxAge: time.Minute})
	ex, err := pool.GetExecutor()
	if err != nil {
		t.Fatal(err)
	}
	ex.Close()
	ex.Close() // 修复前：第二次 Close 重复入池

	_, idle := pool.GetStats()
	if idle != 1 {
		t.Fatalf("idle should be 1 after double close, got %d", idle)
	}
	first, err := pool.GetExecutor()
	if err != nil {
		t.Fatal(err)
	}
	if first != ex {
		t.Fatalf("expected same executor back, got different instance")
	}
	// 取回后空闲列表应为空；重复入池的缺陷会让这里再拿到同一 ex。
	if second, err := pool.GetExecutor(); err == nil && second == ex {
		t.Fatal("pool must not hand out the same executor twice (duplicate idle entry)")
	} else if err == nil {
		// 新建的执行器：放回并清理。
		second.Close()
	}
	first.Close()
	pool.Close()
}

// E 批次缺陷 2：池关闭后活跃执行器 Close 不得把计数减成负数。
// 修复前：pool.Close 把 activeCount 清零后，仍在外面的执行器 Close 会走
// decrementActive → activeCount = -1。
func TestExecutorPoolCloseThenExecutorCloseNotNegative(t *testing.T) {
	pool := NewExecutorPool(ExecutorPoolConfig{MaxIdle: 2, MaxActive: 2, MaxAge: 0})
	ex, err := pool.GetExecutor()
	if err != nil {
		t.Fatal(err)
	}
	pool.Close() // activeCount 清零
	ex.Close()   // 修复前：activeCount 变 -1

	active, _ := pool.GetStats()
	if active < 0 {
		t.Fatalf("active count must not be negative, got %d", active)
	}
}

// E 批次缺陷 3：reset 必须清除中断标记，复用的执行器不得带旧中断状态。
func TestExecutorResetClearsInterrupt(t *testing.T) {
	pool := NewExecutorPool(ExecutorPoolConfig{MaxIdle: 2, MaxActive: 2, MaxAge: time.Minute})
	ex, err := pool.GetExecutor()
	if err != nil {
		t.Fatal(err)
	}
	_ = ex.Interrupt("old run")
	if !ex.IsInterrupted() {
		t.Fatal("expected interrupted")
	}
	ex.Close() // put back → reset
	if ex.IsInterrupted() {
		t.Fatal("reset must clear interrupt flag")
	}
	pool.Close()
}

// E 批次缺陷 4：旧的超时定时器不得误取消新一次执行。
// 修复前：StartTimeoutTimer 不先停旧 timer；且 interruptTimer 字段无锁访问。
func TestExecutorStaleTimerDoesNotFireOnNewExecution(t *testing.T) {
	ex := NewExecutor(nil)
	ex.SetExecutionTimeout(30) // 30ms
	ex.StartTimeoutTimer("first")
	oldTimer := ex.interruptTimer
	if oldTimer == nil {
		t.Fatal("expected timer started")
	}

	// 第二次启动：必须先停掉旧 timer，避免它稍后触发 Interrupt
	// 取消新执行的 ctx。
	ex.StartTimeoutTimer("second")
	if ex.interruptTimer == oldTimer {
		t.Fatal("StartTimeoutTimer must replace old timer")
	}
	ex.StopTimeoutTimer()

	// 旧 timer 若已被 Stop，等待超过其期限后不应产生中断。
	time.Sleep(60 * time.Millisecond)
	if ex.IsInterrupted() {
		t.Fatal("stopped old timer must not interrupt executor")
	}
	// 字段访问在锁内进行（race 下验证）。
	ex.Close()
}
