// 文件说明：manager_exec.go — 沙盒内执行命令与文件读写（Exec / ReadFile / WriteFile / ListDir）
//
// 用法
//
//	Exec、ReadFile、WriteFile（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] internal/api, internal/agentbridge, internal/app
//	         ▼
//	[本文件] internal/sandbox/manager_exec.go
//	         ▼
//	[下游] gosandbox/vm
//	[同包] errors.go, paths.go, proc.go
//
// 主要符号
//
//	func (m *Manager) validExec(in ExecInput) (vm.ExecSpec, error)
//	func (m *Manager) execSandbox(ctx context.Context, r Sandbox, spec vm.ExecSpec, threadID string) (ExecOutput,
//	func (m *Manager) Exec(ctx context.Context, projectID, id string, in ExecInput) (ExecOutput, error)
//	func (m *Manager) ReadFile(ctx context.Context, projectID, id, p string) (FileContent, error)
//	func (m *Manager) WriteFile(ctx context.Context, projectID, id, p string, data []byte) error
//	func (m *Manager) writeFile(ctx context.Context, projectID, id, p string, data []byte, check func(Sandbox) err
//	func (m *Manager) RemoveFile(ctx context.Context, projectID, id, p string) error
//	func (m *Manager) ListDir(ctx context.Context, projectID, id, p string) ([]vm.FileEntry, error)

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"go.uber.org/zap"
)

func (m *Manager) validExec(in ExecInput) (vm.ExecSpec, error) {
	if (strings.TrimSpace(in.Command) == "") == (strings.TrimSpace(in.Cmd) == "") {
		return vm.ExecSpec{}, ErrInvalidSpec
	}
	if len(in.Command) > 4096 || len(in.Cmd) > 1024 || len(in.Args) > 128 {
		return vm.ExecSpec{}, ErrInvalidSpec
	}
	for _, arg := range in.Args {
		if len(arg) > 4096 || strings.ContainsRune(arg, 0) {
			return vm.ExecSpec{}, ErrInvalidSpec
		}
	}
	if err := validateEnv(in.Env); err != nil {
		return vm.ExecSpec{}, err
	}
	cwd := in.Cwd
	if cwd == "" {
		cwd = m.workdir()
	}
	var err error
	cwd, err = vm.CleanPath(m.workdir(), cwd, true)
	if err != nil {
		return vm.ExecSpec{}, err
	}
	timeout := time.Duration(in.TimeoutS) * time.Second
	if timeout == 0 {
		timeout = m.cfg.ExecTimeout
	}
	max := m.cfg.ExecTimeoutMax
	if max <= 0 {
		max = 300 * time.Second
	}
	if timeout < time.Second || timeout > max {
		return vm.ExecSpec{}, fmt.Errorf("%w: timeout_s", ErrInvalidSpec)
	}
	return vm.ExecSpec{Shell: in.Command, Cmd: in.Cmd, Args: in.Args, Cwd: cwd, Env: in.Env, Timeout: timeout}, nil
}

func (m *Manager) execSandbox(ctx context.Context, r Sandbox, spec vm.ExecSpec, threadID string) (ExecOutput, error) {
	var out ExecOutput
	start := m.now()
	result, err := m.driver.Exec(ctx, m.target(r), spec)
	if err != nil {
		fields := []zap.Field{zap.String("sandbox_id", r.ID), zap.String("vm_name", r.CloudName), zap.Error(err)}
		if threadID != "" {
			fields = append(fields, zap.String("thread_id", threadID))
		}
		m.logger.Warn("sandbox driver exec failed", fields...)
		return out, fmt.Errorf("%w: exec", vm.ErrBackend)
	}
	fields := []zap.Field{zap.String("sandbox_id", r.ID), zap.Int("exit_code", result.ExitCode),
		zap.Bool("timed_out", result.TimedOut), zap.Duration("duration", m.now().Sub(start)),
		zap.Int("stdout_bytes", len(result.Stdout)), zap.Int("stderr_bytes", len(result.Stderr))}
	m.logger.Info("sandbox exec done", fields...)
	if err := m.markActive(ctx, &r, false); err != nil {
		return out, err
	}
	out.Stdout, out.StdoutTruncated = vm.TruncateUTF8(result.Stdout, m.cfg.MaxOutputBytes)
	out.Stderr, out.StderrTruncated = vm.TruncateUTF8(result.Stderr, m.cfg.MaxOutputBytes)
	out.ExitCode = result.ExitCode
	out.TimedOut = result.TimedOut
	out.DurationMs = m.now().Sub(start).Milliseconds()
	out.Status = r.Status
	return out, nil
}

// Exec 同步执行；非零退出与超时为成功 HTTP 结果（§7.2）。
func (m *Manager) Exec(ctx context.Context, projectID, id string, in ExecInput) (ExecOutput, error) {
	spec, err := m.validExec(in)
	if err != nil {
		return ExecOutput{}, err
	}
	var out ExecOutput
	err = m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Source == "agent" {
			return ErrBusy
		}
		if r.Status == "expired" {
			return ErrGone
		}
		out, err = m.execSandbox(ctx, r, spec, "")
		return err
	})
	return out, err
}

func (m *Manager) ReadFile(ctx context.Context, projectID, id, p string) (FileContent, error) {
	var result FileContent
	clean, err := vm.CleanPath(m.workdir(), p, false)
	if err != nil {
		return result, err
	}
	err = m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Status == "expired" {
			return ErrGone
		}
		data, err := m.driver.ReadFile(ctx, m.target(r), clean)
		if err != nil {
			if errors.Is(err, vm.ErrFileNotFound) {
				return err
			}
			m.logger.Warn("sandbox driver read failed", zap.String("sandbox_id", id), zap.String("path", clean), zap.Error(err))
			return fmt.Errorf("%w: read", vm.ErrBackend)
		}
		if err = m.markActive(ctx, &r, false); err != nil {
			return err
		}
		max := m.cfg.MaxFileBytes
		if max > 0 && len(data) > max {
			result.Truncated = true
			data = data[:max]
		}
		result.Content = data
		return nil
	})
	return result, err
}

func (m *Manager) WriteFile(ctx context.Context, projectID, id, p string, data []byte) error {
	return m.writeFile(ctx, projectID, id, p, data, func(r Sandbox) error {
		if r.Source == "agent" {
			return ErrBusy
		}
		if r.Status == "expired" {
			return ErrGone
		}
		return nil
	})
}

// writeFile 是 WriteFile / WriteFileForThread 共用的写入主体：校验路径与大小后在沙盒锁内
// 取行、执行 check（可为 nil）、调 driver 写入并刷新活跃时间。extra 只用于补充失败日志字段。
func (m *Manager) writeFile(ctx context.Context, projectID, id, p string, data []byte, check func(Sandbox) error, extra ...zap.Field) error {
	clean, err := vm.CleanPath(m.workdir(), p, false)
	if err != nil {
		return err
	}
	if m.cfg.MaxFileBytes > 0 && len(data) > m.cfg.MaxFileBytes {
		return ErrFileTooLarge
	}
	return m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if check != nil {
			if err = check(r); err != nil {
				return err
			}
		}
		if err = m.driver.WriteFile(ctx, m.target(r), clean, data); err != nil {
			fields := append([]zap.Field{zap.String("sandbox_id", id), zap.String("path", clean)}, extra...)
			m.logger.Warn("sandbox driver write failed", append(fields, zap.Error(err))...)
			return fmt.Errorf("%w: write", vm.ErrBackend)
		}
		return m.markActive(ctx, &r, false)
	})
}

func (m *Manager) RemoveFile(ctx context.Context, projectID, id, p string) error {
	clean, err := vm.CleanPath(m.workdir(), p, false)
	if err != nil {
		return err
	}
	return m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Source == "agent" {
			return ErrBusy
		}
		if r.Status == "expired" {
			return ErrGone
		}
		if err = m.driver.RemoveFile(ctx, m.target(r), clean); err != nil {
			m.logger.Warn("sandbox driver remove file failed", zap.String("sandbox_id", id), zap.String("path", clean), zap.Error(err))
			return fmt.Errorf("%w: remove file", vm.ErrBackend)
		}
		return m.markActive(ctx, &r, false)
	})
}

func (m *Manager) ListDir(ctx context.Context, projectID, id, p string) ([]vm.FileEntry, error) {
	if p == "" {
		p = m.workdir()
	}
	clean, err := vm.CleanPath(m.workdir(), p, true)
	if err != nil {
		return nil, err
	}
	var entries []vm.FileEntry
	err = m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Status == "expired" {
			return ErrGone
		}
		entries, err = m.driver.ListDir(ctx, m.target(r), clean)
		if err != nil {
			m.logger.Warn("sandbox driver list failed", zap.String("sandbox_id", id), zap.String("path", clean), zap.Error(err))
			return fmt.Errorf("%w: list", vm.ErrBackend)
		}
		if len(entries) > 500 {
			entries = entries[:500]
		}
		return m.markActive(ctx, &r, false)
	})
	return entries, err
}
