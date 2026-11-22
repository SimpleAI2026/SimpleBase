//go:build linux

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

// Err 实现 vm.FileWatch：正常关闭返回 nil，丢事件时要求调用方重扫。
func (f *fileWatch) Err() error {
	if f.w.Overflowed() {
		return gosandboxbox.ErrWatchOverflow
	}
	return nil
}

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
