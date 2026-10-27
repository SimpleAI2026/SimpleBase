// 文件说明：logged.go — gosandbox/vm 中 LogFunc、WithLogging 等符号的实现
//
// 用法
//
//	WithLogging(d Driver, log LogFunc) Driver  // WithLogging 为任意 Driver 增加操作日志：操作名、VM 名、耗时与失败原因
//	type LogFunc（导出类型，同包与上层均可使用）
//	type loggedDriver（包内使用）
//	同包调用：Ensure() → driver.go；Exec() → driver.go；ReadFile() → driver.go
//
// 调用架构
//
//	[上游] gosandbox/vm/fake, gosandbox/vm/local, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/logged.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] driver.go, errors.go
//	[关键引用] Ensure() ← driver.go
//
// 主要符号
//
//	type LogFunc func(level, msg string, kv ...any)
//	WithLogging(d Driver, log LogFunc) Driver
//	... 另有 15 个顶层符号

package vm

import (
	"context"
	"time"
)

// LogFunc 是驱动层的日志回调，避免 gosandbox 引入具体日志库。
// level 取值 debug / info / warn / error；kv 为成对的 key, value。
type LogFunc func(level, msg string, kv ...any)

// WithLogging 为任意 Driver 增加操作日志：操作名、VM 名、耗时与失败原因。
// 不记录命令、环境变量与文件内容，只记录长度与路径。log 为 nil 时原样返回。
//
// 装饰器必须保留 Optional 能力：上层用 `driver.(Interactive)` 判断终端 / 流式输出是否可用，
// 若包装后断言失败，功能会静默降级。因此内层实现 Interactive 时返回的包装器也实现它。
func WithLogging(d Driver, log LogFunc) Driver {
	if d == nil || log == nil {
		return d
	}
	if inner, ok := d.(Interactive); ok {
		return &loggedInteractiveDriver{loggedDriver{inner: d, log: log}, inner}
	}
	return &loggedDriver{inner: d, log: log}
}

type loggedDriver struct {
	inner Driver
	log   LogFunc
}

func (l *loggedDriver) finish(level, op, name string, start time.Time, err error, kv ...any) {
	fields := append([]any{"op", op, "vm", name, "duration_ms", time.Since(start).Milliseconds()}, kv...)
	if err != nil {
		fields = append(fields, "error", err.Error())
		l.log("error", "sandbox driver call failed", fields...)
		return
	}
	l.log(level, "sandbox driver call ok", fields...)
}

func (l *loggedDriver) Ensure(ctx context.Context, t Target) error {
	start := time.Now()
	l.log("info", "sandbox driver ensure begin", "vm", t.Name, "image", t.Spec.Image, "cpus", t.Spec.CPUs,
		"memory_mib", t.Spec.MemoryMiB, "network", t.Spec.Network)
	err := l.inner.Ensure(ctx, t)
	l.finish("info", "ensure", t.Name, start, err)
	return err
}

func (l *loggedDriver) Exec(ctx context.Context, t Target, req ExecSpec) (ExecResult, error) {
	start := time.Now()
	res, err := l.inner.Exec(ctx, t, req)
	l.finish("info", "exec", t.Name, start, err, "shell", req.Shell != "", "argc", len(req.Args),
		"cmd_len", len(req.Cmd)+len(req.Shell), "timeout_s", int64(req.Timeout.Seconds()),
		"exit_code", res.ExitCode, "timed_out", res.TimedOut, "stdout_bytes", len(res.Stdout), "stderr_bytes", len(res.Stderr))
	return res, err
}

func (l *loggedDriver) ReadFile(ctx context.Context, t Target, path string) ([]byte, error) {
	start := time.Now()
	b, err := l.inner.ReadFile(ctx, t, path)
	l.finish("debug", "read_file", t.Name, start, err, "path", path, "bytes", len(b))
	return b, err
}

func (l *loggedDriver) WriteFile(ctx context.Context, t Target, path string, data []byte) error {
	start := time.Now()
	err := l.inner.WriteFile(ctx, t, path, data)
	l.finish("debug", "write_file", t.Name, start, err, "path", path, "bytes", len(data))
	return err
}

func (l *loggedDriver) RemoveFile(ctx context.Context, t Target, path string) error {
	start := time.Now()
	err := l.inner.RemoveFile(ctx, t, path)
	l.finish("debug", "remove_file", t.Name, start, err, "path", path)
	return err
}

func (l *loggedDriver) ListDir(ctx context.Context, t Target, path string) ([]FileEntry, error) {
	start := time.Now()
	entries, err := l.inner.ListDir(ctx, t, path)
	l.finish("debug", "list_dir", t.Name, start, err, "path", path, "entries", len(entries))
	return entries, err
}

func (l *loggedDriver) Status(ctx context.Context, name string) (string, error) {
	start := time.Now()
	kind, err := l.inner.Status(ctx, name)
	l.finish("debug", "status", name, start, err, "state", kind)
	return kind, err
}

func (l *loggedDriver) Stop(ctx context.Context, name string) error {
	start := time.Now()
	err := l.inner.Stop(ctx, name)
	l.finish("info", "stop", name, start, err)
	return err
}

func (l *loggedDriver) Remove(ctx context.Context, name string) error {
	start := time.Now()
	err := l.inner.Remove(ctx, name)
	l.finish("info", "remove", name, start, err)
	return err
}

// loggedInteractiveDriver 在内层实现 Interactive 时使用，除日志外原样转发 Start。
type loggedInteractiveDriver struct {
	loggedDriver
	inner Interactive
}

func (l *loggedInteractiveDriver) Start(ctx context.Context, t Target, req StartSpec) (Proc, error) {
	start := time.Now()
	p, err := l.inner.Start(ctx, t, req)
	l.finish("info", "start", t.Name, start, err, "argc", len(req.Args), "pty", req.PTY != nil,
		"cmd_len", len(req.Cmd)+len(req.Shell))
	return p, err
}

var (
	_ Driver      = (*loggedDriver)(nil)
	_ Interactive = (*loggedInteractiveDriver)(nil)
)
