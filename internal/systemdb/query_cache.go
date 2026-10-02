// query_cache.go 进程内短 TTL 缓存 + per-key 单飞（perf §1 P1-D）。
//
// 仪表盘类读接口（metrics/summary、metrics/trend）刷新频繁但数据本身是近似值，
// 用 10s 级 TTL 合并并发与连发请求，避免每次都打系统库唯一连接 / 远端对象存储。
package systemdb

import (
	"sync"
	"time"
)

type queryCacheItem[T any] struct {
	value     T
	expiresAt time.Time
}

type queryCall[T any] struct {
	done  chan struct{}
	value T
	err   error
}

// queryCache 按 key 缓存 fn 结果；同 key 并发调用只执行一次（单飞）。
// fn 出错不缓存。零值不可用，需 NewQueryCache 构造。
type queryCache[T any] struct {
	mu       sync.Mutex
	ttl      time.Duration
	items    map[string]queryCacheItem[T]
	inflight map[string]*queryCall[T]
}

func newQueryCache[T any](ttl time.Duration) *queryCache[T] {
	if ttl <= 0 {
		ttl = 10 * time.Second
	}
	return &queryCache[T]{
		ttl:      ttl,
		items:    map[string]queryCacheItem[T]{},
		inflight: map[string]*queryCall[T]{},
	}
}

// Do 返回 key 的缓存值；未命中/过期时执行 fn（同 key 并发只执行一次）。
func (c *queryCache[T]) Do(key string, fn func() (T, error)) (T, error) {
	now := time.Now()
	c.mu.Lock()
	if it, ok := c.items[key]; ok && now.Before(it.expiresAt) {
		c.mu.Unlock()
		return it.value, nil
	}
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.value, call.err
	}
	call := &queryCall[T]{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	call.value, call.err = fn()
	close(call.done)

	c.mu.Lock()
	delete(c.inflight, key)
	if call.err == nil {
		c.items[key] = queryCacheItem[T]{value: call.value, expiresAt: time.Now().Add(c.ttl)}
	}
	c.mu.Unlock()
	return call.value, call.err
}
