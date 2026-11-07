// 文件说明：evictor.go — godatabase/registry 中 Evictor、NewEvictor 等符号的实现
//
// 用法
//
//	NewEvictor(registry *Registry, interval time.Duration) *Evictor
//	(e *Evictor) Run(ctx context.Context)  // Run 阻塞运行直到 Stop 被调用
//	(e *Evictor) Stop()  // Stop 请求 Evictor 停止并等待其退出
//
// 调用架构
//
//	[上游] godatabase/kv, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/registry/evictor.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] handle.go, registry.go
//
// 主要符号
//
//	type Evictor struct {…}
//	NewEvictor(registry *Registry, interval time.Duration) *Evictor
//	(e *Evictor) Run(ctx context.Context)
//	(e *Evictor) Stop()

package registry

import (
	"context"
	"sync"
	"time"
)

// Evictor 周期性调用 Registry.CloseIdle，回收空闲数据库连接。
// 建议每分钟运行一次（见 plan4.md「生命周期与缓存」）。
// 注入 CapacityEnsurer 时（R1 第 3 条：缓存字节淘汰接线），同周期调用
// EnsureCapacity，把 cache.Manager 的字节/数量上限纳入统一回收节奏。
type Evictor struct {
	registry *Registry
	interval time.Duration
	capacity CapacityEnsurer
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// CapacityEnsurer 由 cache.Manager 实现：确保缓存有 needBytes 可用空间，
// 超限时按 LRU 淘汰（含 maxDatabases 数量维度）。registry 只依赖此可选
// 接口，不反向依赖 cache 包。
type CapacityEnsurer interface {
	EnsureCapacity(ctx context.Context, needBytes int64) error
}

// NewEvictor 构造一个后台淘汰器。interval <= 0 时默认 1 分钟。
// capacity 可为 nil（不接线缓存淘汰，兼容既有调用方与测试）。
func NewEvictor(registry *Registry, interval time.Duration, capacity CapacityEnsurer) *Evictor {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Evictor{
		registry: registry,
		interval: interval,
		capacity: capacity,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Run 阻塞运行直到 Stop 被调用；调用方应在独立 goroutine 中启动。
func (e *Evictor) Run(ctx context.Context) {
	defer close(e.done)
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stop:
			return
		case now := <-ticker.C:
			_ = e.registry.CloseIdle(ctx, now)
			// R1：缓存字节/数量超限回收。目标 1 字节即触发
			// 「超出上限的都淘汰」；失败只影响磁盘水位，不中断周期。
			if e.capacity != nil {
				_ = e.capacity.EnsureCapacity(ctx, 1)
			}
		}
	}
}

// Stop 请求 Evictor 停止并等待其退出；幂等，可安全多次调用。
func (e *Evictor) Stop() {
	e.stopOnce.Do(func() { close(e.stop) })
	<-e.done
}
