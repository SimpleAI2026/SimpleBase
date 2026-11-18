// 文件说明：资源收尾辅助：metrics gatherer、closer 包装与反序关闭（原 app.go 拆出）
package app

import (
	"context"
	"io"

	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	"github.com/linkxzhou/SimpleBase/godatabase/lease"
	"github.com/prometheus/client_golang/prometheus"
)

// metricsGatherer 把 Registerer 适配为 Gatherer；仅 *prometheus.Registry
// 两者兼备，其余实现回退 DefaultGatherer（保持旧装配行为）。
func metricsGatherer(reg prometheus.Registerer) prometheus.Gatherer {
	if g, ok := reg.(prometheus.Gatherer); ok {
		return g
	}
	return prometheus.DefaultGatherer
}

// Close 反向关闭所有已注册资源。供 New 失败路径和测试 cleanup 调用。
func (a *App) Close(ctx context.Context) error {
	if a.schedulerCancel != nil {
		a.schedulerCancel()
	}
	var firstErr error
	if a.registry != nil {
		if err := a.registry.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	a.closerMu.Lock()
	closers := a.closers
	a.closers = nil
	a.closerMu.Unlock()
	for i := len(closers) - 1; i >= 0; i-- {
		if err := closers[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// registerCloser 让各 plan 注册需要反向关闭的资源。
func (a *App) registerCloser(c io.Closer) {
	a.closerMu.Lock()
	defer a.closerMu.Unlock()
	a.closers = append(a.closers, c)
}

type syncerCloser struct{ cs *ducklake.CatalogSyncer }

func (c syncerCloser) Close() error {
	if c.cs == nil {
		return nil
	}
	return c.cs.Close(context.Background())
}

// leaseCloser 进程关闭时停止全部续约。
type leaseCloser struct{ m *lease.Manager }

func (c leaseCloser) Close() error {
	if c.m == nil {
		return nil
	}
	c.m.StopAll()
	return nil
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }
