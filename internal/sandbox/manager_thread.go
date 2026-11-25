// 文件说明：manager_thread.go — 按会话（thread）绑定的沙盒：EnsureForThread / ExecForThread / ReleaseThread 等
//
// 用法
//
//	EnsureForThread、ExecForThread、ReadFileForThread（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] internal/api, internal/agentbridge, internal/app
//	         ▼
//	[本文件] internal/sandbox/manager_thread.go
//	         ▼
//	[下游] gosandbox/vm
//	[同包] errors.go, paths.go, proc.go
//
// 主要符号
//
//	func (m *Manager) EnsureForThread(ctx context.Context, projectID, threadID string) (Sandbox, error)
//	func (m *Manager) ExecForThread(ctx context.Context, projectID, threadID string, in ExecInput) (ExecOutput, er
//	func (m *Manager) ReadFileForThread(ctx context.Context, projectID, threadID, p string) (FileContent, error)
//	func (m *Manager) WriteFileForThread(ctx context.Context, projectID, threadID, p string, data []byte) error
//	func (m *Manager) ReleaseThread(ctx context.Context, projectID, threadID string) error

package sandbox

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"go.uber.org/zap"
)

// EnsureForThread 为 agent thread 取一个沙盒（按 thread 幂等）。
func (m *Manager) EnsureForThread(ctx context.Context, projectID, threadID string) (Sandbox, error) {
	if !m.Available() {
		return Sandbox{}, vm.ErrUnavailable
	}
	var r Sandbox
	err := m.withProjectLock(ctx, projectID, func() error {
		var err error
		r, err = m.store.GetSandboxByThread(ctx, projectID, threadID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		count, err := m.store.CountSandboxes(ctx, projectID)
		if err != nil {
			return err
		}
		max := m.cfg.MaxPerProject
		if max <= 0 {
			max = 5
		}
		if count >= max {
			return ErrLimitExceeded
		}
		id := uuid.NewString()
		r, err = m.store.CreateSandbox(ctx, Sandbox{ID: id, ProjectID: projectID,
			Name: "agent-" + strings.ReplaceAll(threadID, "-", ""), CloudName: CloudName(id),
			Source: "agent", ThreadID: threadID, Image: m.cfg.Image, CPUs: m.cfg.CPUs, MemoryMiB: m.cfg.MemoryMiB,
			Network: m.cfg.Network, IdleTimeoutS: int64(m.cfg.IdleTimeout.Seconds()),
			MaxDurationS: int64(m.cfg.MaxDuration.Seconds()), Status: "pending", CreatedAt: m.now().UTC()})
		return err
	})
	return r, err
}

func (m *Manager) ExecForThread(ctx context.Context, projectID, threadID string, in ExecInput) (ExecOutput, error) {
	r, err := m.EnsureForThread(ctx, projectID, threadID)
	if err != nil {
		return ExecOutput{}, err
	}
	spec, err := m.validExec(in)
	if err != nil {
		return ExecOutput{}, err
	}
	var out ExecOutput
	err = m.withLock(ctx, r.ID, func() error {
		r, err = m.Get(ctx, projectID, r.ID, false)
		if err != nil {
			return err
		}
		out, err = m.execSandbox(ctx, r, spec, threadID)
		return err
	})
	return out, err
}

func (m *Manager) ReadFileForThread(ctx context.Context, projectID, threadID, p string) (FileContent, error) {
	r, err := m.EnsureForThread(ctx, projectID, threadID)
	if err != nil {
		return FileContent{}, err
	}
	return m.ReadFile(ctx, projectID, r.ID, p)
}

func (m *Manager) WriteFileForThread(ctx context.Context, projectID, threadID, p string, data []byte) error {
	r, err := m.EnsureForThread(ctx, projectID, threadID)
	if err != nil {
		return err
	}
	return m.writeFile(ctx, projectID, r.ID, p, data, nil, zap.String("thread_id", threadID))
}

func (m *Manager) ReleaseThread(ctx context.Context, projectID, threadID string) error {
	if !m.Available() {
		return nil
	}
	r, err := m.store.GetSandboxByThread(ctx, projectID, threadID)
	if errors.Is(err, sql.ErrNoRows) {
		// 没有元数据行 = 这个 thread 没有沙盒，无需清理。
		return nil
	}
	if err != nil {
		return err
	}
	return m.Delete(ctx, projectID, r.ID)
}
