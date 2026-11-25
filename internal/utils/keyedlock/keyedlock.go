// 文件说明：keyedlock.go — 按键串行的互斥锁，解锁即回收键
//
// 用法
//
//	var locks keyedlock.Map            // 零值可用，不可拷贝
//	unlock, err := locks.Lock(ctx, k)  // 同一 k 串行；等待可被 ctx 取消
//	defer unlock()
//	locks.Len()                        // 当前持有或等待中的键数（测试用）
//
// 与 map[string]*sync.Mutex 的区别：解锁时删除键，键集合不会随历史键数量增长；
// 等待方可被 ctx 取消。不同键互不等待。
//
// 调用架构
//
//	[上游] internal/systemdb（项目级播种 / 标签写入串行）、internal/api（计划推进串行）
//	         │
//	         ▼
//	[本文件] internal/utils/keyedlock/keyedlock.go
//	         │
//	         ▼
//	[下游] 仅标准库
//
// 主要符号
//
//	type Map struct{…}
//	(m *Map) Lock(ctx context.Context, key string) (func(), error)
//	(m *Map) Len() int

package keyedlock

import (
	"context"
	"sync"
)

// Map 是按字符串键串行的锁集合；零值可用。
type Map struct {
	mu sync.Mutex
	m  map[string]chan struct{}
}

// Lock 获取 key 对应的锁。同一 key 已被持有时等待其释放，ctx 取消时返回 ctx.Err()。
// 返回的 unlock 必须调用恰好一次；调用后键被删除并唤醒所有等待者重新竞争。
func (k *Map) Lock(ctx context.Context, key string) (func(), error) {
	for {
		k.mu.Lock()
		if k.m == nil {
			k.m = make(map[string]chan struct{})
		}
		if done := k.m[key]; done != nil {
			k.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if err := ctx.Err(); err != nil {
			k.mu.Unlock()
			return nil, err
		}
		done := make(chan struct{})
		k.m[key] = done
		k.mu.Unlock()
		return func() {
			k.mu.Lock()
			delete(k.m, key)
			close(done)
			k.mu.Unlock()
		}, nil
	}
}

// Len 返回当前被持有的键数。
func (k *Map) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.m)
}
