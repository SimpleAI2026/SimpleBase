package registry

import (
	"context"
	"time"
)

// Evictor 周期性调用 Registry.CloseIdle，回收空闲数据库连接。
// 建议每分钟运行一次（见 plan4.md「生命周期与缓存」）。
type Evictor struct {
	registry *Registry
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
}

// NewEvictor 构造一个后台淘汰器。interval <= 0 时默认 1 分钟。
func NewEvictor(registry *Registry, interval time.Duration) *Evictor {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Evictor{
		registry: registry,
		interval: interval,
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
		}
	}
}

// Stop 请求 Evictor 停止并等待其退出。
func (e *Evictor) Stop() {
	close(e.stop)
	<-e.done
}
