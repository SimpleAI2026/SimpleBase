//go:build linux

// 文件说明：capabilities.go — vm/box 实现的三个可选能力：日志、监听、入站端口
//
// 这三个能力都建立在 M1 的常驻 agent 之上，所以在这里落地：
//   Logs      进程输出在 box 层已经落盘（box/session.go 的 sinks），这里只做查询与类型转换
//   Watch     agent 用 inotify 监听，box 把事件转成 vm.FileEvent
//   DialPort  agent 在沙盒内连 127.0.0.1:port，box 把中继包成 io.ReadWriteCloser
//
// 用法
//
//	d.Logs(ctx, name, vm.LogQuery{Lines: 100})
//	w, err := d.Watch(ctx, t, "/workspace", vm.WatchOptions{Recursive: true})
//	conn, err := d.DialPort(ctx, name, 8080)
//
// 调用架构
//
//	[上游] gosandbox/vm（LogSource / Watcher / PortDialer）
//	         │ 实现这三个接口
//	         ▼
//	[本文件] gosandbox/vm/box/capabilities.go
//	         │
//	         ▼
//	[下游] gosandbox/box, gosandbox/proto
//	[同包] driver_linux.go
//
// 主要符号
//
//	(d *Driver) Logs(ctx, name string, q vm.LogQuery) ([]vm.LogRecord, error)
//	(d *Driver) Watch(ctx, t vm.Target, path string, opt vm.WatchOptions) (vm.FileWatch, error)
//	(d *Driver) DialPort(ctx, name string, port uint16) (io.ReadWriteCloser, error)

package box

import (
	"context"
	"errors"
	"fmt"
	"io"

	gosandboxbox "github.com/linkxzhou/SimpleBase/gosandbox/box"
	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
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
		if errors.Is(err, errNotExist) {
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
	s, err := d.ensure(ctx, t)
	if err != nil {
		return nil, err
	}
	w, err := s.Watch(ctx, path, gosandboxbox.WatchOptions{Recursive: opt.Recursive})
	if err != nil {
		return nil, fmt.Errorf("%w: watch", vm.ErrBackend)
	}
	return &fileWatch{w: w}, nil
}

// DialPort 实现 vm.PortDialer：连到沙盒内的 127.0.0.1:port。
func (d *Driver) DialPort(ctx context.Context, name string, port uint16) (io.ReadWriteCloser, error) {
	s := d.session(name)
	if s == nil {
		return nil, fmt.Errorf("%w: sandbox %q is not running", vm.ErrBackend, name)
	}
	conn, err := s.DialPort(port)
	if err != nil {
		return nil, fmt.Errorf("%w: dial port %d", vm.ErrBackend, port)
	}
	// ctx 取消时关掉连接，避免调用方泄漏。
	if ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-s.Done():
			}
		}()
	}
	return conn, nil
}

// fileWatch 把 box.Watch 适配成 vm.FileWatch。
type fileWatch struct {
	w *gosandboxbox.Watch
}

// Events 实现 vm.FileWatch。
func (f *fileWatch) Events() <-chan vm.FileEvent {
	out := make(chan vm.FileEvent, 64)
	src := f.w.Events()
	go func() {
		defer close(out)
		for ev := range src {
			out <- vm.FileEvent{Path: ev.Path, Kind: fileKindName(ev.Kind)}
		}
	}()
	return out
}

// Err 实现 vm.FileWatch：正常关闭返回 nil。
func (f *fileWatch) Err() error { return nil }

// Close 实现 vm.FileWatch。
func (f *fileWatch) Close() error { return f.w.Close() }

// fileKindName 把协议的 FileKind 转成 vm 的字符串。
func fileKindName(k proto.FileKind) string {
	switch k {
	case proto.FileRegular:
		return "file"
	case proto.FileDir:
		return "directory"
	case proto.FileSymlink:
		return "symlink"
	}
	return "other"
}

var (
	_ vm.LogSource  = (*Driver)(nil)
	_ vm.Watcher    = (*Driver)(nil)
	_ vm.PortDialer = (*Driver)(nil)
)
