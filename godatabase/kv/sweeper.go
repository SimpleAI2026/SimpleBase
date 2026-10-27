// 文件说明：sweeper.go — godatabase/kv 中 Sweeper、NewSweeper 等符号的实现
//
// 用法
//
//	NewSweeper(reg *registry.Registry, interval time.Duration, logger ...
//	(s *Sweeper) Start(ctx context.Context)  // Start 启动后台循环（幂等
//	(s *Sweeper) Close() error  // Close 停止后台循环并等待退出
//	同包调用：Close() → hash.go, key.go 等
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/kv/sweeper.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/registry
//	[同包] core.go, executor.go, hash.go, key.go, list.go 等 10 个
//	[关键引用] Close() ← hash.go, key.go, list.go
//
// 主要符号
//
//	type Sweeper struct {…}
//	NewSweeper(reg *registry.Registry, interval time.Duration, logger ...
//	(s *Sweeper) Start(ctx context.Context)
//	(s *Sweeper) Close() error
//	(s *Sweeper) SweepOnce(ctx context.Context) map[string]int64
//	... 另有 2 个顶层符号

package kv

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
)

// Sweeper 是实例级 TTL 后台清扫任务：周期遍历 registry 中已打开的可写库，
// 批量删除过期 key（merge-on-read 下控制 delete file 数量）。
// 绝不主动打开未打开的库（避免抖动）；生命周期随 app.Close 回收。
type Sweeper struct {
	reg      *registry.Registry
	interval time.Duration
	logger   godatabase.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewSweeper 构造清扫器。interval <= 0 时取默认 60s。
func NewSweeper(reg *registry.Registry, interval time.Duration, logger godatabase.Logger) *Sweeper {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Sweeper{reg: reg, interval: interval, logger: logger}
}

// Start 启动后台循环（幂等；重复调用不生效）。
func (s *Sweeper) Start(ctx context.Context) {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.mu.Unlock()
	go s.loop(ctx, s.done)
}

// Close 停止后台循环并等待退出。幂等。
func (s *Sweeper) Close() error {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

// SweepOnce 立即执行一轮清扫（同步；测试与手动触发用）。返回各项目删除数。
func (s *Sweeper) SweepOnce(ctx context.Context) map[string]int64 {
	out := make(map[string]int64)
	for _, id := range s.reg.OpenedProjects() {
		n, err := s.sweepProject(ctx, id)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("kv sweeper: sweep project failed",
					zap.String("project_id", id), zap.String("err", err.Error()))
			}
			continue
		}
		if n > 0 {
			out[id] = n
		}
	}
	return out
}

func (s *Sweeper) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.SweepOnce(ctx)
		}
	}
}

// sweepProject 清扫单个已打开项目文件内的 KV 保留表。系统库直接跳过：
// KV 与 SQL、集合共用项目文件，Sweeper 绝不主动打开未打开的项目。
func (s *Sweeper) sweepProject(ctx context.Context, projectID string) (int64, error) {
	db, ok := s.reg.Snapshot(projectID)
	if !ok || db.IsSystem() {
		return 0, nil
	}
	lease, err := s.reg.Acquire(ctx, db, godatabase.ReadWrite)
	if err != nil {
		return 0, err
	}
	defer lease.Release()

	var n int64
	store := New(lease.Handle.Conn(), WithOnWrite(func(ctx context.Context) {
		if n > 0 {
			lease.Handle.NotifyWrite(ctx)
		}
	}))
	has, err := store.HasSchema(ctx)
	if err != nil || !has {
		return 0, err
	}
	err = store.Update(ctx, func(tx *Tx) error {
		var derr error
		n, derr = tx.DeleteExpired(ctx)
		return derr
	})
	if err != nil {
		return 0, err
	}
	if n > 0 && s.logger != nil {
		s.logger.Info("kv sweeper: expired keys deleted",
			zap.String("project_id", projectID), zap.Int64("count", n))
	}
	return n, nil
}
