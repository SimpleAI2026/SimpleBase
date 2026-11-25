// 文件说明：manager_lifecycle.go — 沙盒生命周期：Start / Stop / Pause / Resume / Delete 与指标能力探测
//
// 用法
//
//	Start、Stop、Pause（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] internal/api, internal/agentbridge, internal/app
//	         ▼
//	[本文件] internal/sandbox/manager_lifecycle.go
//	         ▼
//	[下游] gosandbox/vm
//	[同包] errors.go, paths.go, proc.go
//
// 主要符号
//
//	func (m *Manager) markActive(ctx context.Context, r *Sandbox, reset bool) error
//	func (m *Manager) Start(ctx context.Context, projectID, id string) (r Sandbox, err error)
//	func (m *Manager) Stop(ctx context.Context, projectID, id string) (r Sandbox, err error)
//	func (m *Manager) Pause(ctx context.Context, projectID, id string) (r Sandbox, err error)
//	func (m *Manager) Resume(ctx context.Context, projectID, id string) (r Sandbox, err error)
//	func (m *Manager) Metrics(ctx context.Context, projectID, id string) (vm.Metrics, error)
//	func (m *Manager) CanPause() bool
//	func (m *Manager) CanMetrics() bool
//	... 另有 1 个顶层符号

package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"go.uber.org/zap"
)

func (m *Manager) markActive(ctx context.Context, r *Sandbox, reset bool) error {
	now := m.now().UTC()
	changed := false
	if reset || r.StartedAt == nil || r.Status == "expired" {
		r.StartedAt = &now
		expires := now.Add(time.Duration(r.MaxDurationS) * time.Second)
		r.ExpiresAt = &expires
		changed = true
	}
	if r.Status != "running" {
		r.Status = "running"
		changed = true
	}
	if r.LastActiveAt == nil || now.Sub(*r.LastActiveAt) >= 30*time.Second {
		r.LastActiveAt = &now
		changed = true
	}
	if changed {
		return m.store.UpdateSandbox(ctx, *r)
	}
	return nil
}

func (m *Manager) Start(ctx context.Context, projectID, id string) (r Sandbox, err error) {
	begin := m.now()
	defer func() { m.logOp("start", projectID, id, begin, err, zap.String("status", r.Status)) }()
	err = m.withLock(ctx, id, func() error {
		var err error
		r, err = m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if err = m.driver.Ensure(ctx, m.target(r)); err != nil {
			m.logger.Warn("sandbox driver ensure failed", zap.String("sandbox_id", id),
				zap.String("vm_name", r.CloudName), zap.String("image", r.Image), zap.Error(err))
			return fmt.Errorf("%w: start", vm.ErrBackend)
		}
		return m.markActive(ctx, &r, r.Status == "expired")
	})
	return r, err
}

func (m *Manager) Stop(ctx context.Context, projectID, id string) (r Sandbox, err error) {
	begin := m.now()
	defer func() { m.logOp("stop", projectID, id, begin, err) }()
	err = m.withLock(ctx, id, func() error {
		var err error
		r, err = m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		// 暂停没有内存快照：进程随之丢失，先关掉会话与进程，避免留下指向已停止工作区的句柄。
		m.closeSandboxProcs(id)
		// 冻结中的沙盒要先解冻，否则 Stop 只是写标记，进程仍冻在 cgroup 里。
		if r.Status == "paused" {
			if pz, ok := m.driver.(vm.Pauser); ok {
				_ = pz.Resume(ctx, m.target(r))
			}
		}
		if r.Status != "pending" {
			if err = m.driver.Stop(ctx, r.CloudName); err != nil {
				m.logger.Warn("sandbox driver stop failed", zap.String("sandbox_id", id),
					zap.String("vm_name", r.CloudName), zap.Error(err))
				return fmt.Errorf("%w: stop", vm.ErrBackend)
			}
		}
		r.Status = "stopped"
		return m.store.UpdateSandbox(ctx, r)
	})
	return r, err
}

// Pause 冻结沙盒：进程与内存保留，恢复是毫秒级。
//
// 驱动实现了 vm.Pauser 时用 cgroup.freeze；否则退回 Stop（进程丢失），
// 并在返回值里如实反映状态，调用方据 Capabilities.Pause 决定是否暴露该能力。
func (m *Manager) Pause(ctx context.Context, projectID, id string) (r Sandbox, err error) {
	begin := m.now()
	defer func() { m.logOp("pause", projectID, id, begin, err, zap.String("status", r.Status)) }()
	err = m.withLock(ctx, id, func() error {
		var err error
		r, err = m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Status == "paused" {
			return ErrAlreadyPaused
		}
		p, ok := m.driver.(vm.Pauser)
		freezing := ok && m.CanPause()
		if !freezing {
			// 没有冻结能力：退回 Stop，语义与旧行为一致。
			m.closeSandboxProcs(id)
			if r.Status != "pending" {
				if err = m.driver.Stop(ctx, r.CloudName); err != nil {
					return fmt.Errorf("%w: pause", vm.ErrBackend)
				}
			}
			r.Status = "stopped"
			return m.store.UpdateSandbox(ctx, r)
		}
		if err = p.Pause(ctx, r.CloudName); err != nil {
			m.logger.Warn("sandbox driver pause failed", zap.String("sandbox_id", id),
				zap.String("vm_name", r.CloudName), zap.Error(err))
			return fmt.Errorf("%w: pause", vm.ErrBackend)
		}
		r.Status = "paused"
		return m.store.UpdateSandbox(ctx, r)
	})
	return r, err
}

// Resume 解冻沙盒。已停止（或从未启动）时等价于 Start。
func (m *Manager) Resume(ctx context.Context, projectID, id string) (r Sandbox, err error) {
	begin := m.now()
	defer func() { m.logOp("resume", projectID, id, begin, err, zap.String("status", r.Status)) }()
	err = m.withLock(ctx, id, func() error {
		var err error
		r, err = m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Status != "paused" {
			// 不是冻结状态：按 Start 处理（幂等）。
			if err = m.driver.Ensure(ctx, m.target(r)); err != nil {
				return fmt.Errorf("%w: resume", vm.ErrBackend)
			}
			return m.markActive(ctx, &r, r.Status == "expired")
		}
		if p, ok := m.driver.(vm.Pauser); ok {
			if err = p.Resume(ctx, m.target(r)); err != nil {
				m.logger.Warn("sandbox driver resume failed", zap.String("sandbox_id", id),
					zap.String("vm_name", r.CloudName), zap.Error(err))
				return fmt.Errorf("%w: resume", vm.ErrBackend)
			}
		}
		return m.markActive(ctx, &r, false)
	})
	return r, err
}

// Metrics 读沙盒的资源用量。驱动不支持时返回 vm.ErrNotSupported。
func (m *Manager) Metrics(ctx context.Context, projectID, id string) (vm.Metrics, error) {
	if !m.Available() {
		return vm.Metrics{}, vm.ErrUnavailable
	}
	r, err := m.Get(ctx, projectID, id, false)
	if err != nil {
		return vm.Metrics{}, err
	}
	meter, ok := m.driver.(vm.Meter)
	if !ok {
		return vm.Metrics{}, fmt.Errorf("%w: metrics", vm.ErrNotSupported)
	}
	return meter.Metrics(ctx, r.CloudName)
}

// CanPause 报告驱动是否支持真冻结（而不是退回 Stop）。
func (m *Manager) CanPause() bool {
	if !m.Available() {
		return false
	}
	if _, ok := m.driver.(vm.Pauser); !ok {
		return false
	}
	if f, ok := m.driver.(vm.Freezer); ok {
		return f.CanFreeze()
	}
	return true
}

// CanMetrics 报告驱动是否能提供资源用量。
func (m *Manager) CanMetrics() bool {
	if !m.Available() {
		return false
	}
	_, ok := m.driver.(vm.Meter)
	return ok
}

func (m *Manager) Delete(ctx context.Context, projectID, id string) (err error) {
	begin := m.now()
	defer func() { m.logOp("delete", projectID, id, begin, err) }()
	return m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		m.closeSandboxProcs(id)
		// 即使尚为 pending，也可能是驱动首次启动后、落库前失败；Remove 必须幂等。
		if err := m.driver.Remove(ctx, r.CloudName); err != nil {
			m.logger.Error("sandbox removal failed, kept for reaper retry", zap.String("sandbox_id", id),
				zap.String("vm_name", r.CloudName), zap.Error(err))
			// 不隐藏失败：保留元数据供 reaper 重试，避免 VM 泄漏且仍然计费。
			r.LastError = removalFailedMarker
			if updateErr := m.store.UpdateSandbox(ctx, r); updateErr != nil {
				return updateErr
			}
			return fmt.Errorf("%w: remove", vm.ErrBackend)
		}
		return m.store.SoftDeleteSandbox(ctx, projectID, id, "")
	})
}
