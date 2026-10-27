package lease

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestGateSerializesSameKey 验证同 key 并发只有一个 fn 执行（perf §1.5）。
func TestGateSerializesSameKey(t *testing.T) {
	g := NewGate()
	var execCount int32
	release := make(chan struct{})
	entered := make(chan struct{})

	var wg sync.WaitGroup
	results := make([]error, 10)
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[0] = g.Do("db-1", func() error {
			atomic.AddInt32(&execCount, 1)
			close(entered)
			<-release // 模拟 Acquire 的观察等待
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not enter fn")
	}

	// 第一个调用已经占住 key 之后再启动其余调用。若在它们入队前就放行，
	// 后来的调用会各自成为新的首次执行，execCount 会大于 1。
	for i := 1; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = g.Do("db-1", func() error {
				atomic.AddInt32(&execCount, 1)
				<-release
				return nil
			})
		}(i)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := atomic.LoadInt32(&execCount); got != 1 {
			t.Fatalf("execCount = %d, want 1（同 key 只允许一次执行）", got)
		}
		if !g.g.InFlight("db-1") {
			t.Fatal("leader left inflight before release")
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&execCount); got != 1 {
		t.Fatalf("execCount = %d, want 1（同 key 只允许一次执行）", got)
	}
	for i, err := range results {
		if err != nil {
			t.Fatalf("results[%d] = %v, want nil", i, err)
		}
	}
}

// TestGatePropagatesError 验证失败会传播给所有等待者。
func TestGatePropagatesError(t *testing.T) {
	g := NewGate()
	sentinel := errors.New("held by another")
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = g.Do("db-2", func() error { return sentinel })
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, sentinel) {
			t.Fatalf("errs[%d] = %v, want sentinel", i, err)
		}
	}
}

// TestGateDifferentKeysParallel 不同 key 不互相阻塞。
func TestGateDifferentKeysParallel(t *testing.T) {
	g := NewGate()
	block := make(chan struct{})
	started := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for _, key := range []string{"db-a", "db-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			started <- struct{}{}
			_ = g.Do(key, func() error {
				<-block
				return nil
			})
		}(key)
	}
	<-started
	<-started
	close(block)
	wg.Wait() // 两个 key 都能并行完成
}

// TestGateNilReceiver nil Gate 退化为直接执行。
func TestGateNilReceiver(t *testing.T) {
	var g *Gate
	calls := 0
	err := g.Do("x", func() error { calls++; return nil })
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d, want nil/1", err, calls)
	}
	g.Clear("x") // 不 panic
}

// TestGateTryDoBusy 验证 TryDo 同 key 在途时立即返回 ErrAcquiring（perf §1.5）。
func TestGateTryDoBusy(t *testing.T) {
	g := NewGate()
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = g.TryDo("db-3", func() error {
			<-release
			return nil
		})
	}()
	// 等首个调用进入在途状态
	time.Sleep(50 * time.Millisecond)

	err := g.TryDo("db-3", func() error { return nil })
	if !errors.Is(err, ErrAcquiring) {
		t.Fatalf("TryDo busy = %v, want ErrAcquiring", err)
	}
	close(release)
	<-done

	// 首个完成后：TryDo 可再次进入。
	if err := g.TryDo("db-3", func() error { return nil }); err != nil {
		t.Fatalf("TryDo after idle = %v, want nil", err)
	}
}

// TestGateTryDoDistinctKeys 不同 key 不互相影响。
func TestGateTryDoDistinctKeys(t *testing.T) {
	g := NewGate()
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = g.TryDo("db-a", func() error { <-release; return nil })
	}()
	time.Sleep(50 * time.Millisecond)
	if err := g.TryDo("db-b", func() error { return nil }); err != nil {
		t.Fatalf("TryDo distinct key = %v, want nil", err)
	}
	close(release)
	<-done
}
