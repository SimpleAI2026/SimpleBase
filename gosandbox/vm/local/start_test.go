//go:build unix

package local

import (
	"context"
	"errors"
	"io"
	"strings"
	"syscall"
	"testing"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

type stubProc struct{ closed bool }

func (*stubProc) PID() int                    { return 1 }
func (*stubProc) Stdin() io.WriteCloser       { return nil }
func (*stubProc) Output() io.Reader           { return strings.NewReader("") }
func (*stubProc) Resize(host.PTYSize) error   { return nil }
func (*stubProc) Signal(syscall.Signal) error { return nil }
func (*stubProc) Wait() host.ExitInfo         { return host.ExitInfo{} }
func (*stubProc) Done() <-chan struct{}       { c := make(chan struct{}); close(c); return c }
func (s *stubProc) Close() error              { s.closed = true; return nil }

func TestStartBuildsArgvAndEnv(t *testing.T) {
	d := newTestDriver(t, nil)
	var gotPolicy host.Policy
	var gotSpec host.StartSpec
	d.start = func(_ context.Context, p host.Policy, s host.StartSpec) (host.Proc, error) {
		gotPolicy, gotSpec = p, s
		return &stubProc{}, nil
	}
	p, err := d.Start(context.Background(), target("sbx-s"), vm.StartSpec{PTY: &vm.PTYSize{Cols: 90, Rows: 30}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if strings.Join(gotSpec.Command.Argv, " ") != "/bin/sh -i" {
		t.Fatalf("default argv = %v", gotSpec.Command.Argv)
	}
	if gotSpec.PTY == nil || gotSpec.PTY.Cols != 90 || gotSpec.PTY.Rows != 30 {
		t.Fatalf("pty = %+v", gotSpec.PTY)
	}
	if gotPolicy.MaxOutputBytes != 0 {
		t.Fatalf("long sessions must not truncate output, got %d", gotPolicy.MaxOutputBytes)
	}
	if gotPolicy.Mode != host.ModeWorkspaceWrite || !gotPolicy.Enabled {
		t.Fatalf("policy = %+v", gotPolicy)
	}
	joined := strings.Join(gotSpec.Command.Env, "\n")
	if !strings.Contains(joined, "TERM=xterm-256color") {
		t.Fatalf("PTY sessions need a TERM: %v", gotSpec.Command.Env)
	}
	for _, e := range gotSpec.Command.Env {
		if strings.HasPrefix(e, "SIMPLEBASE_") {
			t.Fatalf("host env leaked: %s", e)
		}
	}
}

func TestStartKeepsCallerTERM(t *testing.T) {
	d := newTestDriver(t, nil)
	var gotSpec host.StartSpec
	d.start = func(_ context.Context, _ host.Policy, s host.StartSpec) (host.Proc, error) {
		gotSpec = s
		return &stubProc{}, nil
	}
	if _, err := d.Start(context.Background(), target("sbx-s"), vm.StartSpec{Shell: "true", PTY: &vm.PTYSize{Cols: 1, Rows: 1}, Env: map[string]string{"TERM": "vt100"}}); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range gotSpec.Command.Env {
		if strings.HasPrefix(e, "TERM=") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want exactly one TERM, env=%v", gotSpec.Command.Env)
	}
}

// 隔离不可用时返回 ErrBackend，不会退回裸执行。
func TestStartFailClosed(t *testing.T) {
	d := newTestDriver(t, nil)
	d.start = func(context.Context, host.Policy, host.StartSpec) (host.Proc, error) {
		return nil, host.ErrIsolationUnavailable
	}
	_, err := d.Start(context.Background(), target("sbx-s"), vm.StartSpec{Shell: "true"})
	if !errors.Is(err, vm.ErrBackend) {
		t.Fatalf("want ErrBackend, got %v", err)
	}
	if strings.Contains(err.Error(), "isolation") {
		t.Fatalf("internal detail leaked: %v", err)
	}
}

func TestStartRejectsPublicNetworkAndBadCwd(t *testing.T) {
	d := newTestDriver(t, nil)
	d.start = func(context.Context, host.Policy, host.StartSpec) (host.Proc, error) {
		t.Fatal("must not start")
		return nil, nil
	}
	tg := target("sbx-s")
	tg.Spec.Network = "public"
	if _, err := d.Start(context.Background(), tg, vm.StartSpec{Shell: "true"}); !errors.Is(err, vm.ErrBackend) {
		t.Fatalf("public network: %v", err)
	}
	if _, err := d.Start(context.Background(), target("sbx-s"), vm.StartSpec{Shell: "true", Cwd: "/etc"}); !errors.Is(err, vm.ErrBackend) {
		t.Fatalf("cwd outside workspace: %v", err)
	}
}

func TestStartUnknownCommand(t *testing.T) {
	d := newTestDriver(t, nil)
	d.start = func(context.Context, host.Policy, host.StartSpec) (host.Proc, error) {
		t.Fatal("must not start")
		return nil, nil
	}
	if _, err := d.Start(context.Background(), target("sbx-s"), vm.StartSpec{Cmd: "definitely-not-a-binary-xyz"}); !errors.Is(err, vm.ErrBackend) {
		t.Fatalf("got %v", err)
	}
}

func TestStartContextCanceledIsNotBackendError(t *testing.T) {
	d := newTestDriver(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.start = func(context.Context, host.Policy, host.StartSpec) (host.Proc, error) { return nil, errors.New("boom") }
	if _, err := d.Start(ctx, target("sbx-s"), vm.StartSpec{Shell: "true"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestListDirReportsModeAndTime(t *testing.T) {
	d := newTestDriver(t, nil)
	ctx := context.Background()
	tg := target("sbx-m")
	if err := d.WriteFile(ctx, tg, "/workspace/a.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	entries, err := d.ListDir(ctx, tg, "/workspace")
	if err != nil || len(entries) != 1 {
		t.Fatalf("%v %v", entries, err)
	}
	if entries[0].Mode != 0o644 || entries[0].ModTime.IsZero() {
		t.Fatalf("mode/time not filled: %+v", entries[0])
	}
}
