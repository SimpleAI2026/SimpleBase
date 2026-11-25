//go:build linux

// 文件说明：capabilities.go — gosandbox/vm/box 的内部实现（Logs、toVMLogs 等）
//
// 用法
//
//	同包调用：newFileWatch() → watch_linux_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/capabilities.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] contracts_linux_test.go, diskuse_linux.go, driver_linux.go, driver_other.go, driver_test.go 等 15 个
//	[关键引用] newFileWatch ← watch_linux_test.go
//
// 主要符号
//
//	(d *Driver) Logs(ctx context.Context, name string, q vm.LogQuery) ([]vm.LogRecord, error)
//	toVMLogs(recs []gosandboxbox.Record) []vm.LogRecord
//	(d *Driver) Watch(ctx context.Context, t vm.Target, path string, opt vm.WatchOptions) (vm.FileWatch, error)
//	(d *Driver) DialPort(ctx context.Context, name string, port uint16) (io.ReadWriteCloser, error)
//	type watchSource interface{…}
//	type fileWatch struct{…}
//	newFileWatch(w watchSource) *fileWatch
//	(f *fileWatch) forward()
//	... 另有 3 个顶层符号

package box

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	gosandboxbox "github.com/linkxzhou/SimpleBase/gosandbox/box"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Logs 实现 vm.LogSource：读沙盒的历史输出。
//
// 沙盒没在运行（或已 Stop）时读它的日志目录：日志是落盘的，
// 停掉之后仍然应该能看——这正是不落盘做不到的事。
func (d *Driver) Logs(ctx context.Context, name string, q vm.LogQuery) ([]vm.LogRecord, error) {
	opt := gosandboxbox.TailOptions{Lines: q.Lines, Since: q.Since}
	for _, s := range q.Streams {
		switch s {
		case "stdout":
			opt.Streams = append(opt.Streams, gosandboxbox.StreamStdout)
		case "stderr":
			opt.Streams = append(opt.Streams, gosandboxbox.StreamStderr)
		}
	}
	dir := d.logDir(name)
	if dir == "" {
		return nil, fmt.Errorf("%w: this driver keeps no logs", vm.ErrNotSupported)
	}
	// 运行中优先问会话（它能拿到还在缓冲里的最后一段）；否则直接读目录。
	if s := d.session(name); s != nil {
		recs, err := s.Logs(nil, opt)
		if err == nil {
			return toVMLogs(recs), nil
		}
	}
	names, err := logNamesIn(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: logs", vm.ErrBackend)
	}
	var all []gosandboxbox.Record
	for _, n := range names {
		recs, terr := gosandboxbox.Tail(dir, n, opt)
		if terr != nil {
			return nil, fmt.Errorf("%w: logs", vm.ErrBackend)
		}
		all = append(all, recs...)
	}
	return toVMLogs(all), nil
}

// toVMLogs 把 box 的记录转成 vm 的记录。
func toVMLogs(recs []gosandboxbox.Record) []vm.LogRecord {
	out := make([]vm.LogRecord, 0, len(recs))
	for _, r := range recs {
		stream := "stdout"
		if r.Stream == gosandboxbox.StreamStderr {
			stream = "stderr"
		}
		out = append(out, vm.LogRecord{Stream: stream, Time: r.Time, Data: r.Data})
	}
	return out
}

// Watch 实现 vm.Watcher：监听沙盒内的目录。
func (d *Driver) Watch(ctx context.Context, t vm.Target, path string, opt vm.WatchOptions) (vm.FileWatch, error) {
	path, err := vm.CleanPath(d.workdir(t), path, true)
	if err != nil {
		return nil, err
	}
	s, err := d.ensure(ctx, t)
	if err != nil {
		return nil, err
	}
	w, err := s.Watch(ctx, path, gosandboxbox.WatchOptions{Recursive: opt.Recursive})
	if err != nil {
		return nil, fmt.Errorf("%w: watch", vm.ErrBackend)
	}
	return newFileWatch(w), nil
}

// DialPort 实现 vm.PortDialer：连到沙盒内的 127.0.0.1:port。
func (d *Driver) DialPort(ctx context.Context, name string, port uint16) (io.ReadWriteCloser, error) {
	s := d.session(name)
	if s == nil {
		return nil, fmt.Errorf("%w: sandbox %q is not running", vm.ErrBackend, name)
	}
	conn, err := s.DialPort(ctx, port)
	if err != nil {
		return nil, fmt.Errorf("%w: dial port %d", vm.ErrBackend, port)
	}
	return conn, nil
}

type watchSource interface {
	Events() <-chan gosandboxbox.FileEvent
	Done() <-chan struct{}
	Overflowed() bool
	Close() error
}

// fileWatch 持有唯一转发者；停止时可以放弃尚未交付的事件。
type fileWatch struct {
	w    watchSource
	out  chan vm.FileEvent
	done chan struct{}
}

func newFileWatch(w watchSource) *fileWatch {
	f := &fileWatch{w: w, out: make(chan vm.FileEvent, 64), done: make(chan struct{})}
	go f.forward()
	return f
}

func (f *fileWatch) forward() {
	defer close(f.done)
	defer close(f.out)
	for {
		select {
		case <-f.w.Done():
			return
		case ev, ok := <-f.w.Events():
			if !ok {
				return
			}
			select {
			case <-f.w.Done():
				return
			case f.out <- vm.FileEvent{Path: ev.Path, Kind: kindOf(ev.Kind)}:
			}
		}
	}
}

func (f *fileWatch) Events() <-chan vm.FileEvent { return f.out }

// Err 实现 vm.FileWatch：正常关闭返回 nil，丢事件时要求调用方重扫。
func (f *fileWatch) Err() error {
	if f.w.Overflowed() {
		return gosandboxbox.ErrWatchOverflow
	}
	return nil
}

// Close 实现 vm.FileWatch。
func (f *fileWatch) Close() error { return f.w.Close() }

var (
	_ vm.LogSource  = (*Driver)(nil)
	_ vm.Watcher    = (*Driver)(nil)
	_ vm.PortDialer = (*Driver)(nil)
)
