package vm

import (
	"bytes"
	"context"
	"io"
	"sync"
	"syscall"
	"testing"
)

// stubDriver 实现 Driver；interactive 子类型额外实现 Interactive。
type stubDriver struct{ calls int }

func (s *stubDriver) Ensure(context.Context, Target) error { s.calls++; return nil }
func (s *stubDriver) Exec(context.Context, Target, ExecSpec) (ExecResult, error) {
	s.calls++
	return ExecResult{Stdout: []byte("ok")}, nil
}
func (s *stubDriver) ReadFile(context.Context, Target, string) ([]byte, error) {
	s.calls++
	return []byte("x"), nil
}
func (s *stubDriver) WriteFile(context.Context, Target, string, []byte) error { s.calls++; return nil }
func (s *stubDriver) RemoveFile(context.Context, Target, string) error        { s.calls++; return nil }
func (s *stubDriver) ListDir(context.Context, Target, string) ([]FileEntry, error) {
	s.calls++
	return nil, nil
}
func (s *stubDriver) Status(context.Context, string) (string, error) {
	s.calls++
	return "running", nil
}
func (s *stubDriver) Stop(context.Context, string) error   { s.calls++; return nil }
func (s *stubDriver) Remove(context.Context, string) error { s.calls++; return nil }

type stubProc struct{ done chan struct{} }

func (p *stubProc) PID() int                    { return 1 }
func (p *stubProc) Stdin() io.WriteCloser       { return nopWriteCloser{new(bytes.Buffer)} }
func (p *stubProc) Output() io.Reader           { return new(bytes.Buffer) }
func (p *stubProc) Resize(PTYSize) error        { return nil }
func (p *stubProc) Signal(syscall.Signal) error { return nil }
func (p *stubProc) Wait() ProcExit              { return ProcExit{} }
func (p *stubProc) Done() <-chan struct{}       { return p.done }
func (p *stubProc) Close() error                { return nil }

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

type stubInteractive struct{ stubDriver }

func (s *stubInteractive) Start(context.Context, Target, StartSpec) (Proc, error) {
	s.calls++
	return &stubProc{done: make(chan struct{})}, nil
}

// 关键回归：WithLogging 不能吃掉 Interactive 能力，否则上层会把终端判为不可用。
func TestWithLoggingKeepsInteractive(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	log := func(level, msg string, kv ...any) {
		mu.Lock()
		lines = append(lines, msg)
		mu.Unlock()
	}
	wrapped := WithLogging(&stubInteractive{}, log)
	it, ok := wrapped.(Interactive)
	if !ok {
		t.Fatal("WithLogging must preserve the Interactive capability")
	}
	p, err := it.Start(context.Background(), Target{Name: "vm"}, StartSpec{PTY: &PTYSize{Cols: 80, Rows: 24}})
	if err != nil || p == nil {
		t.Fatalf("Start: %v", err)
	}
	if _, ok := p.(Proc); !ok {
		t.Fatal("Start must return a Proc")
	}
	mu.Lock()
	n := len(lines)
	mu.Unlock()
	if n == 0 {
		t.Fatal("Start through the wrapper must be logged")
	}
	// 非 Interactive 的内层仍然只是 Driver。
	if _, ok := WithLogging(&stubDriver{}, log).(Interactive); ok {
		t.Fatal("a non-interactive driver must not become interactive")
	}
	// log 为 nil 时原样返回（保持指针同一性，避免多余分配影响断言）。
	d := &stubInteractive{}
	if WithLogging(d, nil) != Driver(d) {
		t.Fatal("WithLogging with a nil logger must return the driver unchanged")
	}
	if WithLogging(nil, log) != nil {
		t.Fatal("WithLogging(nil) must stay nil")
	}
}
