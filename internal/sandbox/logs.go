// 文件说明：logs.go — 沙盒日志与目录监听在业务层的入口
//
// Manager 只负责「找沙盒 → 转给驱动」，不实现日志格式：格式在
// gosandbox/box（Docker local 驱动同形的二进制记录），这里只做能力探测
// 与错误映射，避免把驱动的类型泄漏到 API 层。
//
// 用法
//
//	m.Logs(ctx, projectID, id, vm.LogQuery{Lines: 100})
//	w, err := m.Watch(ctx, projectID, id, "/workspace", true)
//
// 调用架构
//
//	[上游] internal/api（envd logs / WatchDir）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/logs.go
//	         │
//	         ▼
//	[下游] gosandbox/vm
//	[同包] manager.go, errors.go, snapshots.go
//
// 主要符号
//
//	(m *Manager) Logs(ctx, projectID, id string, q vm.LogQuery) ([]vm.LogRecord, error)
//	(m *Manager) Watch(ctx, projectID, id, path string, recursive bool) (vm.FileWatch, error)
//	(m *Manager) CanLogs() bool
//	(m *Manager) CanWatch() bool

package sandbox

import (
	"context"
	"fmt"
	"io"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// 日志回放的参数与记录类型就是 vm.LogQuery / vm.LogRecord：格式在驱动侧
// （gosandbox/box），业务层不再自己复写一份同形类型。

// Logs 读沙盒的历史输出。
//
// 驱动不支持时返回 vm.ErrNotSupported，由 API 层映射 501——
// 而不是返回空列表（那会让调用方以为「没有输出」）。
func (m *Manager) Logs(ctx context.Context, projectID, id string, q vm.LogQuery) ([]vm.LogRecord, error) {
	if !m.Available() {
		return nil, vm.ErrUnavailable
	}
	r, err := m.Get(ctx, projectID, id, false)
	if err != nil {
		return nil, err
	}
	src, ok := m.driver.(vm.LogSource)
	if !ok {
		return nil, fmt.Errorf("%w: logs", vm.ErrNotSupported)
	}
	recs, err := src.Logs(ctx, r.CloudName, q)
	if err != nil {
		return nil, fmt.Errorf("%w: logs", vm.ErrBackend)
	}
	return recs, nil
}

// Watch 开始监听沙盒内的一个路径。
func (m *Manager) Watch(ctx context.Context, projectID, id, path string, recursive bool) (vm.FileWatch, error) {
	if !m.Available() {
		return nil, vm.ErrUnavailable
	}
	r, err := m.Get(ctx, projectID, id, false)
	if err != nil {
		return nil, err
	}
	src, ok := m.driver.(vm.Watcher)
	if !ok {
		return nil, fmt.Errorf("%w: file watching", vm.ErrNotSupported)
	}
	w, err := src.Watch(ctx, m.target(r), path, vm.WatchOptions{Recursive: recursive})
	if err != nil {
		return nil, fmt.Errorf("%w: watch", vm.ErrBackend)
	}
	return w, nil
}

// CanLogs 报告驱动是否能提供历史日志。
func (m *Manager) CanLogs() bool {
	if !m.Available() {
		return false
	}
	_, ok := m.driver.(vm.LogSource)
	return ok
}

// CanWatch 报告驱动是否能提供目录监听。
func (m *Manager) CanWatch() bool {
	if !m.Available() {
		return false
	}
	_, ok := m.driver.(vm.Watcher)
	return ok
}

// CanPorts 报告驱动是否能提供入站端口中继（E2B getHost）。
func (m *Manager) CanPorts() bool {
	if !m.Available() {
		return false
	}
	_, ok := m.driver.(vm.PortDialer)
	return ok
}

// DialPort 连到沙盒内 127.0.0.1:port（入站反向代理用）。
func (m *Manager) DialPort(ctx context.Context, projectID, id string, port uint16) (io.ReadWriteCloser, error) {
	if !m.Available() {
		return nil, vm.ErrUnavailable
	}
	r, err := m.Get(ctx, projectID, id, false)
	if err != nil {
		return nil, err
	}
	d, ok := m.driver.(vm.PortDialer)
	if !ok {
		return nil, fmt.Errorf("%w: port forwarding", vm.ErrNotSupported)
	}
	conn, err := d.DialPort(ctx, r.CloudName, port)
	if err != nil {
		return nil, fmt.Errorf("%w: connect port %d", vm.ErrBackend, port)
	}
	return conn, nil
}
