package kv

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// Sweeper 是实例级 TTL 后台清扫任务：周期遍历 registry 中已打开的可写库，
// 批量删除过期 key（merge-on-read 下控制 delete file 数量）。
// 绝不主动打开未打开的库（避免抖动）；生命周期随 app.Close 回收。
type Sweeper struct {
	reg      *registry.Registry
	interval time.Duration
	logger   observability.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewSweeper 构造清扫器。interval <= 0 时取默认 60s。
func NewSweeper(reg *registry.Registry, interval time.Duration, logger observability.Logger) *Sweeper {
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
	go s.loop(ctx)
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

// SweepOnce 立即执行一轮清扫（同步；测试与手动触发用）。返回各库删除数。
func (s *Sweeper) SweepOnce(ctx context.Context) map[string]int64 {
	out := make(map[string]int64)
	for _, id := range s.reg.OpenedIDs() {
		n, err := s.sweepDB(ctx, id)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("kv sweeper: sweep database failed",
					zap.String("database_id", id), zap.String("err", err.Error()))
			}
			continue
		}
		if n > 0 {
			out[id] = n
		}
	}
	return out
}

func (s *Sweeper) loop(ctx context.Context) {
	defer close(s.done)
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

// sweepDB 清扫单个已打开的库。系统库、非 kind=kv 的用户库直接跳过：
// Sweeper 只扫项目 KV catalog（key-value-ducklake-plan §2），不碰用户库。
func (s *Sweeper) sweepDB(ctx context.Context, databaseID string) (int64, error) {
	db, ok := s.reg.Snapshot(databaseID)
	if !ok || catalog.IsSystemDatabase(db) || !catalog.IsKVDatabase(db) {
		return 0, nil
	}
	lease, err := s.reg.Acquire(ctx, db, database.ReadWrite)
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
			zap.String("database_id", databaseID), zap.Int64("count", n))
	}
	return n, nil
}
