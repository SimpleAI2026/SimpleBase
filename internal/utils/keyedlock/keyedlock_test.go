// 文件说明：keyedlock_test.go — internal/utils/keyedlock 的单元测试（TestSameKeySerializes、TestDifferentKeysDoNotBlock 等）
//
// 用法
//
//	go test ./internal/utils/keyedlock -run 'TestSameKeySerializes|TestDifferentKeysDoNotBlock|TestWaitCanBeCanceled'
//
// 调用架构
//
//	[上游] internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/utils/keyedlock/keyedlock_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] keyedlock.go
//
// 主要符号
//
//	TestSameKeySerializes(t *testing.T)
//	TestDifferentKeysDoNotBlock(t *testing.T)
//	TestWaitCanBeCanceled(t *testing.T)

package keyedlock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSameKeySerializes(t *testing.T) {
	var m Map
	var inside, maxInside int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := m.Lock(context.Background(), "k")
			if err != nil {
				t.Error(err)
				return
			}
			n := atomic.AddInt32(&inside, 1)
			for {
				cur := atomic.LoadInt32(&maxInside)
				if n <= cur || atomic.CompareAndSwapInt32(&maxInside, cur, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&inside, -1)
			unlock()
		}()
	}
	wg.Wait()
	if maxInside != 1 {
		t.Fatalf("同一 key 并发进入临界区: %d", maxInside)
	}
	if m.Len() != 0 {
		t.Fatalf("解锁后应回收键: %d", m.Len())
	}
}

func TestDifferentKeysDoNotBlock(t *testing.T) {
	var m Map
	u1, err := m.Lock(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	defer u1()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	u2, err := m.Lock(ctx, "b")
	if err != nil {
		t.Fatalf("不同 key 不应互相等待: %v", err)
	}
	u2()
}

func TestWaitCanBeCanceled(t *testing.T) {
	var m Map
	unlock, err := m.Lock(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Lock(ctx, "k"); err != context.Canceled {
		t.Fatalf("err=%v want context.Canceled", err)
	}
	// 已取消的 ctx 即使键空闲也不应拿到锁。
	if _, err := m.Lock(ctx, "free"); err != context.Canceled {
		t.Fatalf("free key with canceled ctx: %v", err)
	}
	if m.Len() != 1 {
		t.Fatalf("Len=%d want 1", m.Len())
	}
}
