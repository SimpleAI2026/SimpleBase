package sandbox

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm/fake"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

// memStore 是带真实读写语义的内存 Store（c2Store 的 UpdateSandbox 是空操作，不适合验证状态）。
type memStore struct {
	mu   sync.Mutex
	rows map[string]Sandbox
}

func newMemStore() *memStore { return &memStore{rows: map[string]Sandbox{}} }

func (s *memStore) CreateSandbox(_ context.Context, sb Sandbox) (Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[sb.ID] = sb
	return sb, nil
}
func (s *memStore) GetSandbox(_ context.Context, p, id string) (Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rows[id]; ok && r.ProjectID == p && r.DeletedAt == nil {
		return r, nil
	}
	return Sandbox{}, sql.ErrNoRows
}
func (s *memStore) GetSandboxByID(_ context.Context, id string) (Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rows[id]; ok && r.DeletedAt == nil {
		return r, nil
	}
	return Sandbox{}, sql.ErrNoRows
}
func (s *memStore) GetSandboxByName(_ context.Context, p, name string) (Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rows {
		if r.ProjectID == p && r.Name == name && r.DeletedAt == nil {
			return r, nil
		}
	}
	return Sandbox{}, sql.ErrNoRows
}
func (s *memStore) GetSandboxByThread(context.Context, string, string) (Sandbox, error) {
	return Sandbox{}, sql.ErrNoRows
}
func (s *memStore) ListSandboxes(_ context.Context, p, _, _, _ string, _ int) ([]Sandbox, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Sandbox
	for _, r := range s.rows {
		if r.ProjectID == p && r.DeletedAt == nil {
			out = append(out, r)
		}
	}
	return out, "", nil
}
func (s *memStore) CountSandboxes(_ context.Context, p string) (int, error) {
	l, _, _ := s.ListSandboxes(context.Background(), p, "", "", "", 0)
	return len(l), nil
}
func (s *memStore) UpdateSandbox(_ context.Context, sb Sandbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rows[sb.ID]; !ok {
		return sql.ErrNoRows
	}
	s.rows[sb.ID] = sb
	return nil
}
func (s *memStore) SoftDeleteSandbox(_ context.Context, p, id, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[id]
	if !ok || r.ProjectID != p {
		return sql.ErrNoRows
	}
	now := time.Now()
	r.DeletedAt = &now
	s.rows[id] = r
	return nil
}
func (s *memStore) ListExpiredSandboxes(_ context.Context, now time.Time, _ int) ([]Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Sandbox
	for _, r := range s.rows {
		if r.DeletedAt == nil && (r.Status == "running" || r.Status == "pending") && r.ExpiresAt != nil && r.ExpiresAt.Before(now) {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *memStore) ListStaleRunSandboxes(context.Context, time.Time, int) ([]Sandbox, error) {
	return nil, nil
}
func (s *memStore) ListFailedSandboxRemovals(context.Context, int) ([]Sandbox, error) {
	return nil, nil
}
func (s *memStore) PurgeDeletedSandboxes(context.Context, time.Time) (int64, error) { return 0, nil }

func procManager(t *testing.T) (*Manager, *memStore) {
	t.Helper()
	cfg := config.SandboxConfig{Enabled: true, Workdir: "/workspace", Image: "img", CPUs: 1, MemoryMiB: 256,
		Network: "none", IdleTimeout: 10 * time.Minute, MaxDuration: time.Hour, MaxPerProject: 5,
		ExecTimeout: time.Second, ExecTimeoutMax: time.Minute, MaxFileBytes: 1024,
		Terminal: config.SandboxTerminalConfig{Enabled: true, MaxSessionsPerSandbox: 2, IdleTimeout: time.Minute}}
	st := newMemStore()
	return NewManager(cfg, st, fake.New()), st
}

func runningSandbox(t *testing.T, m *Manager, project, name string) Sandbox {
	t.Helper()
	sb, err := m.Create(context.Background(), project, CreateInput{Name: name, Start: true}, "u")
	if err != nil {
		t.Fatal(err)
	}
	return sb
}

func TestTerminalSessionLimitAndRelease(t *testing.T) {
	m, _ := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "t1")
	spec := StartSpec{Kind: ProcTerminal, PTY: &vm.PTYSize{Cols: 80, Rows: 24}}
	a, err := m.StartProc(ctx, "p", sb.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.StartProc(ctx, "p", sb.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.StartProc(ctx, "p", sb.ID, spec); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("third terminal must hit the per-sandbox cap, got %v", err)
	}
	_ = a.Close()
	// 关闭一个后名额释放。
	c, err := m.StartProc(ctx, "p", sb.ID, spec)
	if err != nil {
		t.Fatalf("slot not released: %v", err)
	}
	_ = b.Close()
	_ = c.Close()
	if n := m.ProcCount(sb.ID); n != 0 {
		t.Fatalf("procs leaked: %d", n)
	}
}

// 长会话不能占住沙盒锁：会话存在期间，同一沙盒的文件操作仍然可用。
func TestSessionDoesNotHoldSandboxLock(t *testing.T) {
	m, _ := procManager(t)
	m.lockWait = 100 * time.Millisecond
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "t2")
	h, err := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcTerminal, PTY: &vm.PTYSize{Cols: 80, Rows: 24}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if err := m.WriteFile(ctx, "p", sb.ID, "/workspace/a.txt", []byte("x")); err != nil {
		t.Fatalf("file write while a session is open: %v", err)
	}
	if _, err := m.ReadFile(ctx, "p", sb.ID, "/workspace/a.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestStopDeleteExpireCloseProcs(t *testing.T) {
	ctx := context.Background()
	for name, closeFn := range map[string]func(m *Manager, id string){
		"stop":   func(m *Manager, id string) { _, _ = m.Stop(ctx, "p", id) },
		"delete": func(m *Manager, id string) { _ = m.Delete(ctx, "p", id) },
		"reaper": func(m *Manager, id string) {
			past := m.now().Add(-time.Minute)
			r, _ := m.Get(ctx, "p", id, false)
			r.ExpiresAt = &past
			_ = m.store.UpdateSandbox(ctx, r)
			_ = m.ReapOnce(ctx)
		},
		"close": func(m *Manager, id string) { _ = m.Close() },
	} {
		t.Run(name, func(t *testing.T) {
			m, _ := procManager(t)
			sb := runningSandbox(t, m, "p", "x"+name)
			h, err := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcTerminal, PTY: &vm.PTYSize{Cols: 80, Rows: 24}})
			if err != nil {
				t.Fatal(err)
			}
			closeFn(m, sb.ID)
			select {
			case <-h.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("process still alive")
			}
			if n := m.ProcCount(sb.ID); n != 0 {
				t.Fatalf("procs left in table: %d", n)
			}
		})
	}
}

func TestProcRejectedForAgentSourceAndUnsupportedBackend(t *testing.T) {
	m, st := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "agentbox")
	r, _ := st.GetSandbox(ctx, "p", sb.ID)
	r.Source = "agent"
	_ = st.UpdateSandbox(ctx, r)
	if _, err := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcTerminal}); !errors.Is(err, ErrBusy) {
		t.Fatalf("agent sandboxes are read-only here, got %v", err)
	}
	// 终端被配置关闭：不支持。
	m2, _ := procManager(t)
	m2.cfg.Terminal.Enabled = false
	sb2 := runningSandbox(t, m2, "p", "off")
	if _, err := m2.StartProc(ctx, "p", sb2.ID, StartSpec{Kind: ProcTerminal}); !errors.Is(err, vm.ErrNotSupported) {
		t.Fatalf("disabled terminal: %v", err)
	}
	if m2.Interactive() || m2.Capabilities().Terminal {
		t.Fatal("capabilities must report terminal=false when disabled")
	}
	if !m.Interactive() || !m.Capabilities().Terminal {
		t.Fatal("fake backend with terminal enabled must report terminal=true")
	}
}

func TestExpiredAndCrossProjectProcRejected(t *testing.T) {
	m, st := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "e1")
	if _, err := m.StartProc(ctx, "other", sb.ID, StartSpec{Kind: ProcTerminal}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project must look like not found, got %v", err)
	}
	r, _ := st.GetSandbox(ctx, "p", sb.ID)
	r.Status = "expired"
	_ = st.UpdateSandbox(ctx, r)
	if _, err := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcTerminal}); !errors.Is(err, ErrGone) {
		t.Fatalf("expired: %v", err)
	}
}

func TestStartProcValidation(t *testing.T) {
	m, _ := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "v1")
	bad := map[string]StartSpec{
		"cwd outside":   {Shell: "true", Cwd: "/etc"},
		"env reserved":  {Shell: "true", Env: map[string]string{"SIMPLEBASE_X": "1"}},
		"env bad key":   {Shell: "true", Env: map[string]string{"lower": "1"}},
		"too many args": {Cmd: "x", Args: make([]string, 129)},
		"nul in arg":    {Cmd: "x", Args: []string{"a\x00b"}},
	}
	for name, spec := range bad {
		if _, err := m.StartProc(ctx, "p", sb.ID, spec); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}

func TestProcTagLookupAndDuplicateTag(t *testing.T) {
	m, _ := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "tag1")
	h, err := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcCommand, Tag: "job", PTY: &vm.PTYSize{Cols: 80, Rows: 24}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if got, ok := m.FindProc("p", sb.ID, 0, "job"); !ok || got != h {
		t.Fatal("tag lookup failed")
	}
	if _, ok := m.FindProc("p", sb.ID, h.PID(), ""); !ok {
		t.Fatal("pid lookup failed")
	}
	if _, ok := m.FindProc("other", sb.ID, 0, "job"); ok {
		t.Fatal("lookup must be scoped to the project")
	}
	if _, err := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcCommand, Tag: "job"}); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("duplicate tag: %v", err)
	}
	if procs := m.Procs(sb.ID); len(procs) != 1 || procs[0].Tag != "job" {
		t.Fatalf("Procs = %+v", procs)
	}
	// 终端会话不出现在 E2B 的进程列表里。
	term, _ := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcTerminal})
	defer term.Close()
	if procs := m.Procs(sb.ID); len(procs) != 1 {
		t.Fatalf("terminals must not be listed as E2B processes: %+v", procs)
	}
}

func TestProcSelfExitRemovesFromTable(t *testing.T) {
	m, _ := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "self1")
	h, err := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcCommand, Shell: "echo done"})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	_ = h.CopyOutput(&out)
	if !strings.Contains(out.String(), "done") {
		t.Fatalf("output = %q", out.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for m.ProcCount(sb.ID) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("exited process was not removed from the table")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestIdleTerminalReaped(t *testing.T) {
	m, _ := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "idle1")
	h, err := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcTerminal, PTY: &vm.PTYSize{Cols: 80, Rows: 24}})
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcCommand, Tag: "bg", PTY: &vm.PTYSize{Cols: 80, Rows: 24}})
	if err != nil {
		t.Fatal(err)
	}
	defer cmd.Close()
	base := m.now()
	m.now = func() time.Time { return base.Add(2 * time.Minute) } // 超过 1 分钟空闲上限
	m.reapProcs(ctx)
	select {
	case <-h.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("idle terminal was not closed")
	}
	select {
	case <-cmd.Done():
		t.Fatal("background commands must not be reaped by the terminal idle timer")
	default:
	}
}

func TestSetExpiryAndExtend(t *testing.T) {
	m, _ := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "exp1")
	base := m.now()
	m.now = func() time.Time { return base }
	r, err := m.SetExpiry(ctx, "p", sb.ID, 120)
	if err != nil || r.ExpiresAt == nil || !r.ExpiresAt.Equal(base.UTC().Add(120*time.Second)) {
		t.Fatalf("SetExpiry = %+v %v", r.ExpiresAt, err)
	}
	// 重新计时：可以缩短。
	r, _ = m.SetExpiry(ctx, "p", sb.ID, 30)
	if !r.ExpiresAt.Equal(base.UTC().Add(30 * time.Second)) {
		t.Fatalf("SetExpiry must restart the clock, got %v", r.ExpiresAt)
	}
	// 超过 max_duration 时截断。
	r, _ = m.SetExpiry(ctx, "p", sb.ID, 999999)
	if !r.ExpiresAt.Equal(base.UTC().Add(time.Hour)) {
		t.Fatalf("not clamped: %v", r.ExpiresAt)
	}
	// ExtendExpiry 只延长不缩短。
	m.SetExpiry(ctx, "p", sb.ID, 600)
	r, _ = m.ExtendExpiry(ctx, "p", sb.ID, 60)
	if !r.ExpiresAt.Equal(base.UTC().Add(600 * time.Second)) {
		t.Fatalf("Extend shortened the TTL: %v", r.ExpiresAt)
	}
	r, _ = m.ExtendExpiry(ctx, "p", sb.ID, 1200)
	if !r.ExpiresAt.Equal(base.UTC().Add(1200 * time.Second)) {
		t.Fatalf("Extend did not lengthen: %v", r.ExpiresAt)
	}
	if _, err := m.SetExpiry(ctx, "p", sb.ID, -1); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("negative: %v", err)
	}
	if _, err := m.SetExpiry(ctx, "other", sb.ID, 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project: %v", err)
	}
}

func TestGetByID(t *testing.T) {
	m, _ := procManager(t)
	sb := runningSandbox(t, m, "p", "byid")
	got, err := m.GetByID(context.Background(), sb.ID)
	if err != nil || got.ProjectID != "p" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := m.GetByID(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestEnvsAreInMemoryAndMergedUnderRequestEnv(t *testing.T) {
	m, _ := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "env1")
	m.SetEnvs(sb.ID, map[string]string{"A": "1", "B": "2"})
	h, err := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcCommand, Shell: "echo $A$B", Env: map[string]string{"B": "9"}})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	_ = h.CopyOutput(&out)
	if strings.TrimSpace(out.String()) != "19" {
		t.Fatalf("request env must override sandbox env: %q", out.String())
	}
	// 返回的是副本：改它不影响存量。
	got := m.Envs(sb.ID)
	got["A"] = "x"
	if m.Envs(sb.ID)["A"] != "1" {
		t.Fatal("Envs must return a copy")
	}
	m.ClearEnvs(sb.ID)
	if len(m.Envs(sb.ID)) != 0 {
		t.Fatal("ClearEnvs did not clear")
	}
}

func TestSignalViaProcHandle(t *testing.T) {
	m, _ := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "sig1")
	h, _ := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcCommand, PTY: &vm.PTYSize{Cols: 80, Rows: 24}})
	if err := m.KillProc(h, syscall.SIGHUP); err == nil {
		t.Fatal("SIGHUP must be rejected")
	}
	if err := m.KillProc(h, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("SIGTERM did not stop the process")
	}
	if e := h.Wait(); !e.Signaled {
		t.Fatalf("exit = %+v", e)
	}
}

func TestCopyOutputStopsOnWriteError(t *testing.T) {
	m, _ := procManager(t)
	ctx := context.Background()
	sb := runningSandbox(t, m, "p", "co1")
	h, _ := m.StartProc(ctx, "p", sb.ID, StartSpec{Kind: ProcCommand, Shell: "echo hello"})
	defer h.Close()
	if err := h.CopyOutput(errWriter{}); err == nil {
		t.Fatal("write error must surface")
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestValidateMetadata(t *testing.T) {
	if err := validateMetadata(map[string]string{"a": "b", "k.1": "v"}); err != nil {
		t.Fatal(err)
	}
	for name, md := range map[string]map[string]string{
		"reserved prefix": {"simplebase.x": "1"},
		"reserved name":   {"name": "x"},
		"del char":        {"a": "b\x7f"},
	} {
		if err := validateMetadata(md); !errors.Is(err, ErrInvalidSpec) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
