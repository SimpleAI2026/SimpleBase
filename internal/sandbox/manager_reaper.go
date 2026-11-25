// 文件说明：manager_reaper.go — 沙盒空闲冻结、过期回收与后台 reaper 生命周期
//
// 用法
//
//	ReapOnce、StartReaper、Close（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] internal/api, internal/agentbridge, internal/app
//	         ▼
//	[本文件] internal/sandbox/manager_reaper.go
//	         ▼
//	[下游] gosandbox/vm
//	[同包] errors.go, paths.go, proc.go
//
// 主要符号
//
//	func (m *Manager) freezeIdle(ctx context.Context, now time.Time)
//	func (m *Manager) ReapOnce(ctx context.Context) error
//	func (m *Manager) StartReaper(ctx context.Context)
//	func (m *Manager) Close() error

package sandbox

import (
	"context"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"go.uber.org/zap"
)

// freezeIdle 把空闲超过 FreezeAfter 的运行中沙盒冻结（cgroup.freeze）。
//
// 冻结保留内存与进程，恢复是毫秒级；这是密度手段，不是回收手段：
// 真正的回收仍由 ListExpiredSandboxes + 软删负责。驱动不支持 Pauser 时直接返回。
func (m *Manager) freezeIdle(ctx context.Context, now time.Time) {
	if m.cfg.FreezeAfter <= 0 {
		return
	}
	if _, ok := m.driver.(vm.Pauser); !ok {
		return
	}
	idle, err := m.store.ListIdleRunningSandboxes(ctx, now.Add(-m.cfg.FreezeAfter), 100)
	if err != nil {
		m.logger.Warn("reaper listing idle sandboxes failed", zap.Error(err))
		return
	}
	for _, r := range idle {
		if ctx.Err() != nil {
			return
		}
		// 有活跃进程（终端 / 后台命令）的沙盒不算空闲：进程自己会刷 activity。
		if len(m.procs.list(r.ID)) > 0 {
			continue
		}
		if _, err := m.Pause(ctx, r.ProjectID, r.ID); err != nil {
			m.logger.Warn("sandbox idle freeze failed", zap.String("sandbox_id", r.ID), zap.Error(err))
			continue
		}
		m.logger.Info("sandbox frozen while idle", zap.String("sandbox_id", r.ID),
			zap.Duration("freeze_after", m.cfg.FreezeAfter))
	}
}

// ReapOnce 每轮只处理至多 100 行，先清理遗留 VM，再清过期状态和软删行。
func (m *Manager) ReapOnce(ctx context.Context) error {
	if !m.Available() {
		return nil
	}
	now := m.now()
	m.reapProcs(ctx)
	failed, err := m.store.ListFailedSandboxRemovals(ctx, 100)
	if err != nil {
		return err
	}
	for _, r := range failed {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.logger.Info("reaper retrying failed removal", zap.String("sandbox_id", r.ID))
		_ = m.Delete(ctx, r.ProjectID, r.ID)
	}
	stale, err := m.store.ListStaleRunSandboxes(ctx, now.Add(-m.cfg.ExecTimeoutMax-10*time.Minute), 100)
	if err != nil {
		return err
	}
	for _, r := range stale {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.logger.Info("reaper deleting stale run sandbox", zap.String("sandbox_id", r.ID))
		_ = m.Delete(ctx, r.ProjectID, r.ID)
	}
	m.freezeIdle(ctx, now)
	expired, err := m.store.ListExpiredSandboxes(ctx, now, 100)
	if err != nil {
		return err
	}
	for _, r := range expired {
		m.logger.Info("reaper marking sandbox expired", zap.String("sandbox_id", r.ID))
		m.closeSandboxProcs(r.ID)
		r.Status = "expired"
		if err = m.store.UpdateSandbox(ctx, r); err != nil {
			return err
		}
	}
	purged, err := m.store.PurgeDeletedSandboxes(ctx, now.Add(-7*24*time.Hour))
	if len(failed)+len(stale)+len(expired) > 0 || purged > 0 {
		m.logger.Info("reaper round done", zap.Int("retry_removal", len(failed)), zap.Int("stale_run", len(stale)),
			zap.Int("expired", len(expired)), zap.Int64("purged", purged))
	}
	return err
}

// StartReaper 启动单一低频清理协程；ctx 取消时退出。
func (m *Manager) StartReaper(ctx context.Context) {
	if !m.Available() || m.cfg.ReapInterval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(m.cfg.ReapInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.ReapOnce(ctx); err != nil && ctx.Err() == nil {
					m.logger.Warn("sandbox reaper round failed", zap.Error(err))
				}
			}
		}
	}()
	m.logger.Info("sandbox reaper started", zap.Duration("interval", m.cfg.ReapInterval))
}

// Close 关闭全部会话与进程。
func (m *Manager) Close() error {
	if m == nil || m.procs == nil {
		return nil
	}
	for _, h := range m.procs.all() {
		_ = h.Close()
	}
	return nil
}
