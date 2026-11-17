// 文件说明：snapshots.go — 沙盒文件系统快照与 fork 的业务层
//
// 这一层只做三件事：找沙盒、把 vm 类型转成业务类型、把错误归一。
// 「快照只有文件系统」这个限制在这里转成对调用方可见的字段
// （Processes=false），而不是靠文档提醒。
//
// 用法
//
//	m.Snapshot(ctx, projectID, id, "before-upgrade")
//	m.Fork(ctx, projectID, snapshotID, sandbox.CreateInput{Name: "sb2"})
//
// 调用架构
//
//	[上游] internal/api（fork / snapshots 端点）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/snapshots.go
//	         │
//	         ▼
//	[下游] gosandbox/vm（Snapshotter）
//	[同包] manager.go, logs.go
//
// 主要符号
//
//	type Snapshot struct {…}
//	(m *Manager) CanSnapshot() bool
//	(m *Manager) Snapshot(ctx, projectID, id, label string) (Snapshot, error)
//	(m *Manager) Snapshots(ctx) ([]Snapshot, error)
//	(m *Manager) RemoveSnapshot(ctx, id string) error
//	(m *Manager) Fork(ctx, projectID, snapshotID string, in CreateInput) (Sandbox, error)

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Snapshot 是一个文件系统快照的对外视图。
type Snapshot struct {
	ID      string    `json:"snapshot_id"`
	Name    string    `json:"sandbox_name"`
	Label   string    `json:"label,omitempty"`
	Parent  string    `json:"parent,omitempty"`
	Size    int64     `json:"size_bytes"`
	Digest  string    `json:"digest"`
	Created time.Time `json:"created_at"`
	// Processes 恒为 false：共享内核的容器做不到内存快照。
	// 显式给出这个字段，调用方不必读文档才知道 fork 出来的沙盒没有进程。
	Processes bool `json:"processes"`
}

// CanSnapshot 报告驱动是否支持快照与 fork。
func (m *Manager) CanSnapshot() bool {
	if !m.Available() {
		return false
	}
	_, ok := m.driver.(vm.Snapshotter)
	return ok
}

// Snapshot 给一个沙盒拍文件系统快照。
func (m *Manager) Snapshot(ctx context.Context, projectID, id, label string) (Snapshot, error) {
	if !m.Available() {
		return Snapshot{}, vm.ErrUnavailable
	}
	d, ok := m.driver.(vm.Snapshotter)
	if !ok {
		return Snapshot{}, fmt.Errorf("%w: snapshots", vm.ErrNotSupported)
	}
	r, err := m.Get(ctx, projectID, id, false)
	if err != nil {
		return Snapshot{}, err
	}
	info, err := d.Snapshot(ctx, m.target(r), label)
	if err != nil {
		return Snapshot{}, snapshotErr("snapshot", err)
	}
	return toSnapshot(info), nil
}

// Snapshots 列出全部快照。
//
// 不按项目过滤：快照是所有租户共用的磁盘状态，但要能对应回沙盒名，
// 调用方据此判断归属。加项目过滤需要把项目写进快照索引（M4）。
func (m *Manager) Snapshots(ctx context.Context) ([]Snapshot, error) {
	if !m.Available() {
		return nil, vm.ErrUnavailable
	}
	d, ok := m.driver.(vm.Snapshotter)
	if !ok {
		return nil, fmt.Errorf("%w: snapshots", vm.ErrNotSupported)
	}
	list, err := d.Snapshots(ctx)
	if err != nil {
		return nil, snapshotErr("list snapshots", err)
	}
	out := make([]Snapshot, 0, len(list))
	for _, it := range list {
		out = append(out, toSnapshot(it))
	}
	return out, nil
}

// RemoveSnapshot 删一个快照。
func (m *Manager) RemoveSnapshot(ctx context.Context, snapshotID string) error {
	if !m.Available() {
		return vm.ErrUnavailable
	}
	d, ok := m.driver.(vm.Snapshotter)
	if !ok {
		return fmt.Errorf("%w: snapshots", vm.ErrNotSupported)
	}
	if err := d.RemoveSnapshot(ctx, snapshotID); err != nil {
		return snapshotErr("remove snapshot", err)
	}
	return nil
}

// Fork 从快照建一个新沙盒（DB 行 + 磁盘内容）。
//
// 顺序很重要：先建 DB 行，再恢复磁盘。反过来的话磁盘上会留下一份
// 没有主人的沙盒目录，清理时无法判断该不该删。恢复失败则回滚 DB 行。
func (m *Manager) Fork(ctx context.Context, projectID, snapshotID, actor string, in CreateInput) (Sandbox, error) {
	if !m.Available() {
		return Sandbox{}, vm.ErrUnavailable
	}
	d, ok := m.driver.(vm.Snapshotter)
	if !ok {
		return Sandbox{}, fmt.Errorf("%w: fork", vm.ErrNotSupported)
	}
	// 不允许外部指定「从快照恢复」之外的行为：fork 出来的沙盒一定不启动进程。
	in.Start = false
	row, err := m.Create(ctx, projectID, in, actor)
	if err != nil {
		return Sandbox{}, err
	}
	target := m.target(row)
	if _, err := d.Fork(ctx, snapshotID, target); err != nil {
		// 回滚 DB 行：磁盘没恢复成功，这一行不该继续存在。
		if derr := m.Delete(context.WithoutCancel(ctx), projectID, row.ID); derr != nil {
			m.logger.Warn("fork rollback failed; sandbox row left behind",
				zap.String("sandbox_id", row.ID), zap.Error(derr))
		}
		return Sandbox{}, snapshotErr("fork", err)
	}
	return row, nil
}

// toSnapshot 把 vm 类型转成对外视图。
func toSnapshot(it vm.SnapshotInfo) Snapshot {
	return Snapshot{
		ID: it.ID, Name: it.Name, Label: it.Label, Parent: it.Parent,
		Size: it.Size, Digest: it.Digest, Created: it.Created,
		Processes: false,
	}
}

// snapshotErr 归一快照层的错误。
func snapshotErr(what string, err error) error {
	switch {
	case errors.Is(err, vm.ErrNotSupported):
		return fmt.Errorf("%w: %s", vm.ErrNotSupported, what)
	case errors.Is(err, vm.ErrNotFound):
		return fmt.Errorf("%w: %s", ErrNotFound, what)
	case errors.Is(err, vm.ErrInvalid):
		// 状态不合法（例如目标沙盒还在跑）：是调用方的错。
		return fmt.Errorf("%w: %s: %v", ErrInvalidSpec, what, err)
	case errors.Is(err, vm.ErrUnavailable):
		return vm.ErrUnavailable
	}
	return fmt.Errorf("%w: %s", vm.ErrBackend, what)
}
