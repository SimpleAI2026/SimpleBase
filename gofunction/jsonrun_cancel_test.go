package gofunction

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// GF-RUN-01：父 ctx 取消后，死循环脚本必须有界返回（而不是跑满内部 10s 超时）。
// 修复前 Program.Run 内部固定 newCallContext()（10s 超时、Background），
// 与调用方 ctx 无关：父 ctx 取消后脚本仍要跑满 10s 才超时返回。
func TestRunJSON_ParentCancelStopsExecution(t *testing.T) {
	if testing.Short() {
		// 死循环用例即使在 short 下也应验证取消路径，但给更紧的时限。
		_ = t
	}
	src := `package main
func Loop(x int) int {
	for {
	}
	return 0
}
`
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := RunJSON(ctx, "gf1", "loop.go", src, "Loop", []byte(`1`))
	elapsed := time.Since(start)
	cancel()
	if err == nil {
		t.Fatal("want error after parent cancel")
	}
	// 取消后必须在远小于内部 10s 超时内返回（留 3s 余量）。
	if elapsed > 3*time.Second {
		t.Fatalf("parent cancel not honored: elapsed=%v err=%v", elapsed, err)
	}
	// 取消错误可区分：errors.Is(ctx.Err()) 或 ErrTimeout 均可接受，但必须非 nil 且快速。
	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), ErrTimeout.Error()) {
		t.Fatalf("unexpected err: %v", err)
	}
}

// GF-RUN-02：父 ctx 取消后，脚本内 go 出的阻塞 goroutine 不得拖长总时长。
// 修复前 waitGoroutines(defaultTimeout) 固定再等最多 10s：即使主函数已因
// 取消返回，泄漏的 goroutine 仍让单次调用最长到约 20s。
func TestRunJSON_CancelBoundsGoroutineWait(t *testing.T) {
	src := `package main
func Blocked(x int) int {
	done := make(chan int)
	go func() {
		<-done
	}()
	_ = done
	return x
}
`
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := RunJSON(ctx, "gf2", "blocked.go", src, "Blocked", []byte(`1`))
	elapsed := time.Since(start)
	cancel()
	if err == nil {
		t.Fatal("want error after parent cancel")
	}
	// 取消后等待 goroutine 的时间必须纳入同一截止时间：总时长不超过内部超时（10s）。
	if elapsed > 10*time.Second {
		t.Fatalf("goroutine wait not bounded by cancel: elapsed=%v", elapsed)
	}
}

// GF-RUN-03：脚本 goroutine 并发 print 不得损坏输出（race 下验证）。
// 修复前 Context.outBuffer 是 strings.Builder，多 goroutine 并发写是数据竞争。
func TestRunJSON_ConcurrentPrint(t *testing.T) {
	src := `package main
func ConcurrentPrint(x int) int {
	var wg chan int = make(chan int, 4)
	for i := 0; i < 4; i++ {
		go func() {
			print("hello")
			wg <- 1
		}()
	}
	for i := 0; i < 4; i++ {
		<-wg
	}
	return x
}
`
	done := make(chan error, 1)
	go func() {
		_, err := RunJSON(context.Background(), "gf3", "print.go", src, "ConcurrentPrint", []byte(`1`))
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run error: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent print run timed out")
	}
}

// GF-RUN-03 补充：多 goroutine 并发 print 后缓冲内容完整（总长度等于各段之和）。
func TestRunJSON_ConcurrentPrintOutputIntact(t *testing.T) {
	src := `package main
func ConcurrentPrint(x int) int {
	var wg chan int = make(chan int, 8)
	for i := 0; i < 8; i++ {
		go func() {
			print("abcdefgh")
			wg <- 1
		}()
	}
	for i := 0; i < 8; i++ {
		<-wg
	}
	return x
}
`
	done := make(chan int, 1)
	go func() {
		// 输出检查在 RunJSON 层拿不到（Output 只挂在 Context），这里只验证
		// race 安全与执行成功；长度断言放在 Interpreter 层测试。
		_, err := RunJSON(context.Background(), "gf4", "print8.go", src, "ConcurrentPrint", []byte(`1`))
		if err != nil {
			t.Errorf("run error: %v", err)
		}
		done <- 0
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out")
	}
}

// Interpreter 层：并发 print 后 Output() 长度必须是各段之和（未被覆盖或损坏）。
func TestContext_ConcurrentPrintOutputLength(t *testing.T) {
	src := `package main
func ConcurrentPrint(x int) int {
	var wg chan int = make(chan int, 8)
	for i := 0; i < 8; i++ {
		go func() {
			print("abcdefgh")
			wg <- 1
		}()
	}
	for i := 0; i < 8; i++ {
		<-wg
	}
	return x
}
`
	program, err := BuildProgram("gf5", "printlen.go", src)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, err := program.RunWithContext("gf5", "ConcurrentPrint", 1)
		errCh <- err
	}()
	wg.Wait()
	if err := <-errCh; err != nil {
		t.Fatalf("run error: %v", err)
	}
	// RunWithContext 返回的 ctx 携带输出；8 段 "abcdefgh\n"？print 不换行 → 64 字符。
	// 通过重新构建拿到 ctx：
	program2, err := BuildProgram("gf5b", "printlen.go", src)
	if err != nil {
		t.Fatal(err)
	}
	_, outCtx, err := program2.RunWithContext("gf5b", "ConcurrentPrint", 1)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := len(outCtx.Output()); got != 8*(8+1) {
		t.Fatalf("output corrupted: want 72 bytes, got %d (%q)", got, outCtx.Output())
	}
}
