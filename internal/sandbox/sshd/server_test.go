package sshd

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm/fake"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

// ---- 测试夹具 ----

type testStore struct {
	mu   sync.Mutex
	rows map[string]sandbox.Sandbox
}

func (s *testStore) CreateSandbox(_ context.Context, sb sandbox.Sandbox) (sandbox.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[sb.ID] = sb
	return sb, nil
}
func (s *testStore) GetSandbox(_ context.Context, p, id string) (sandbox.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rows[id]; ok && r.ProjectID == p {
		return r, nil
	}
	return sandbox.Sandbox{}, sql.ErrNoRows
}
func (s *testStore) GetSandboxByID(_ context.Context, id string) (sandbox.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rows[id]; ok {
		return r, nil
	}
	return sandbox.Sandbox{}, sql.ErrNoRows
}
func (s *testStore) GetSandboxByName(context.Context, string, string) (sandbox.Sandbox, error) {
	return sandbox.Sandbox{}, sql.ErrNoRows
}
func (s *testStore) GetSandboxByThread(context.Context, string, string) (sandbox.Sandbox, error) {
	return sandbox.Sandbox{}, sql.ErrNoRows
}
func (s *testStore) ListSandboxes(context.Context, string, string, string, string, int) ([]sandbox.Sandbox, string, error) {
	return nil, "", nil
}
func (s *testStore) CountSandboxes(context.Context, string) (int, error) { return 0, nil }
func (s *testStore) UpdateSandbox(_ context.Context, sb sandbox.Sandbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[sb.ID] = sb
	return nil
}
func (s *testStore) SoftDeleteSandbox(context.Context, string, string, string) error { return nil }
func (s *testStore) ListExpiredSandboxes(context.Context, time.Time, int) ([]sandbox.Sandbox, error) {
	return nil, nil
}
func (s *testStore) ListStaleRunSandboxes(context.Context, time.Time, int) ([]sandbox.Sandbox, error) {
	return nil, nil
}
func (s *testStore) ListFailedSandboxRemovals(context.Context, int) ([]sandbox.Sandbox, error) {
	return nil, nil
}
func (s *testStore) PurgeDeletedSandboxes(context.Context, time.Time) (int64, error) { return 0, nil }

// staticAuth 只认一个沙盒 + 一个密码；其余一律同一个错误。
type staticAuth struct {
	project, sandboxID, password string
	calls                        int
	mu                           sync.Mutex
}

var errNo = errors.New("denied")

func (a *staticAuth) Authenticate(_ context.Context, user, secret string, _ net.Addr) (Identity, error) {
	a.mu.Lock()
	a.calls++
	a.mu.Unlock()
	if user == a.sandboxID && secret == a.password {
		return Identity{ProjectID: a.project, SandboxID: a.sandboxID, Actor: "tester"}, nil
	}
	return Identity{}, errNo
}

type fixture struct {
	srv  *Server
	mgr  *sandbox.Manager
	sb   sandbox.Sandbox
	auth *staticAuth
	// audits 收集会话审计。
	mu     sync.Mutex
	audits []string
}

func newFixture(t *testing.T, sessions int) *fixture {
	t.Helper()
	cfg := config.SandboxConfig{Enabled: true, Workdir: "/workspace", Image: "img", CPUs: 1, MemoryMiB: 256,
		Network: "none", IdleTimeout: time.Minute, MaxDuration: time.Hour, MaxPerProject: 5,
		ExecTimeout: time.Second, ExecTimeoutMax: time.Minute, MaxFileBytes: 1024,
		Terminal: config.SandboxTerminalConfig{Enabled: true, MaxSessionsPerSandbox: sessions, IdleTimeout: time.Minute}}
	mgr := sandbox.NewManager(cfg, &testStore{rows: map[string]sandbox.Sandbox{}}, fake.New())
	sb, err := mgr.Create(context.Background(), "proj", sandbox.CreateInput{Name: "box", Start: true}, "u")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{mgr: mgr, sb: sb, auth: &staticAuth{project: "proj", sandboxID: sb.ID, password: "s3cret"}}
	key, err := LoadOrCreateHostKey(t.TempDir() + "/hk")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{HostKey: key, Authenticator: f.auth, Sessions: mgr,
		Audit: func(_ context.Context, id Identity, kind string, _ time.Duration, exit int) {
			f.mu.Lock()
			f.audits = append(f.audits, kind+":"+id.SandboxID)
			f.mu.Unlock()
		}})
	if err != nil {
		t.Fatal(err)
	}
	f.srv = srv
	t.Cleanup(func() { _ = srv.Close(); _ = mgr.Close() })
	return f
}

// dial 经 net.Pipe 连接服务端（与 WebSocket 桥的接入方式相同）。
func (f *fixture) dial(t *testing.T, user, pw string) (*ssh.Client, error) {
	t.Helper()
	cc, sc := NewPipe()
	go f.srv.ServeConn(sc)
	conn, chans, reqs, err := ssh.NewClientConn(cc, "pipe", &ssh.ClientConfig{
		User: user, Auth: []ssh.AuthMethod{ssh.Password(pw)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
	})
	if err != nil {
		cc.Close()
		return nil, err
	}
	return ssh.NewClient(conn, chans, reqs), nil
}

func readUntil(t *testing.T, r io.Reader, want string) string {
	t.Helper()
	var sb strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 256)
		for !strings.Contains(sb.String(), want) {
			n, err := r.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatalf("timeout waiting for %q; got %q", want, sb.String())
	}
	return sb.String()
}

// ---- 认证 ----

func TestAuthSuccessAndUniformFailure(t *testing.T) {
	f := newFixture(t, 4)
	c, err := f.dial(t, f.sb.ID, "s3cret")
	if err != nil {
		t.Fatalf("valid login: %v", err)
	}
	c.Close()

	// 错误密码 / 不存在的沙盒 / 空密码：外部看到的错误完全一样，不泄露存在性。
	var msgs []string
	for _, tc := range [][2]string{{f.sb.ID, "wrong"}, {"no-such-sandbox", "s3cret"}, {f.sb.ID, ""}} {
		_, err := f.dial(t, tc[0], tc[1])
		if err == nil {
			t.Fatalf("login %v must fail", tc)
		}
		msgs = append(msgs, err.Error())
	}
	if msgs[0] != msgs[1] || msgs[1] != msgs[2] {
		t.Fatalf("failure messages differ, leaks existence: %q", msgs)
	}
}

func TestFailureRateLimit(t *testing.T) {
	f := newFixture(t, 4)
	now := time.Now()
	f.srv.limiter.now = func() time.Time { return now }
	f.srv.now = func() time.Time { return now }
	for i := 0; i < f.srv.limiter.max; i++ {
		f.srv.limiter.fail("mem")
	}
	before := f.auth.calls
	if _, err := f.dial(t, f.sb.ID, "s3cret"); err == nil {
		t.Fatal("rate-limited source must be refused even with the right password")
	}
	if f.auth.calls != before {
		t.Fatal("authenticator must not be consulted once the limit is hit")
	}
	// 窗口过去后恢复。
	now = now.Add(2 * time.Minute)
	if c, err := f.dial(t, f.sb.ID, "s3cret"); err != nil {
		t.Fatalf("limit must expire: %v", err)
	} else {
		c.Close()
	}
}

func TestPublicKeyAndNoneAuthRejected(t *testing.T) {
	f := newFixture(t, 4)
	cc, sc := NewPipe()
	go f.srv.ServeConn(sc)
	_, _, _, err := ssh.NewClientConn(cc, "pipe", &ssh.ClientConfig{User: f.sb.ID,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 3 * time.Second})
	if err == nil {
		t.Fatal("login without any credential must fail")
	}
}

// ---- 会话 ----

func TestShellWithPTYEchoAndExit(t *testing.T) {
	f := newFixture(t, 4)
	c, err := f.dial(t, f.sb.ID, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.RequestPty("xterm-256color", 24, 80, ssh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	in, _ := s.StdinPipe()
	out, _ := s.StdoutPipe()
	if err := s.Shell(); err != nil {
		t.Fatal(err)
	}
	io.WriteString(in, "echo hello-from-box\n")
	readUntil(t, out, "hello-from-box")
	io.WriteString(in, "exit 5\n")
	err = s.Wait()
	var ee *ssh.ExitError
	if !errors.As(err, &ee) || ee.ExitStatus() != 5 {
		t.Fatalf("exit status: %v", err)
	}
	waitAudit(t, f, "terminal:"+f.sb.ID)
}

func waitAudit(t *testing.T, f *fixture, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, a := range f.audits {
			if a == want {
				f.mu.Unlock()
				return
			}
		}
		f.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("audit %q never recorded; got %v", want, f.audits)
}

func TestExecRunsCommandWithoutPTY(t *testing.T) {
	f := newFixture(t, 4)
	c, _ := f.dial(t, f.sb.ID, "s3cret")
	defer c.Close()
	s, _ := c.NewSession()
	defer s.Close()
	out, err := s.Output("echo from-exec")
	if err != nil {
		t.Fatalf("exec: %v (out=%q)", err, out)
	}
	if strings.TrimSpace(string(out)) != "from-exec" {
		t.Fatalf("exec output = %q", out)
	}
	waitAudit(t, f, "exec:"+f.sb.ID)
}

func TestWindowChangeReachesProcess(t *testing.T) {
	f := newFixture(t, 4)
	c, _ := f.dial(t, f.sb.ID, "s3cret")
	defer c.Close()
	s, _ := c.NewSession()
	defer s.Close()
	_ = s.RequestPty("xterm", 24, 80, ssh.TerminalModes{})
	in, _ := s.StdinPipe()
	_, _ = s.StdoutPipe()
	if err := s.Shell(); err != nil {
		t.Fatal(err)
	}
	if err := s.WindowChange(50, 132); err != nil {
		t.Fatalf("window-change must be accepted once the shell runs: %v", err)
	}
	io.WriteString(in, "exit\n")
}

// ---- 拒绝项 ----

func TestForwardingAndOtherChannelsRejected(t *testing.T) {
	f := newFixture(t, 4)
	c, _ := f.dial(t, f.sb.ID, "s3cret")
	defer c.Close()

	// direct-tcpip（端口转发）：沙盒没有网络，也不能把主机网络当跳板。
	if conn, err := c.Dial("tcp", "127.0.0.1:80"); err == nil {
		conn.Close()
		t.Fatal("direct-tcpip must be rejected")
	}
	// tcpip-forward（远程转发）。
	if l, err := c.Listen("tcp", "127.0.0.1:0"); err == nil {
		l.Close()
		t.Fatal("tcpip-forward must be rejected")
	}
	for name, typ := range map[string]string{"x11": "x11-req", "agent": "auth-agent-req@openssh.com"} {
		s, _ := c.NewSession()
		ok, _ := s.SendRequest(typ, true, nil)
		if ok {
			t.Errorf("%s request must be rejected", name)
		}
		s.Close()
	}
	// subsystem（含 sftp）。
	s, _ := c.NewSession()
	if err := s.RequestSubsystem("sftp"); err == nil {
		t.Fatal("sftp subsystem must be rejected")
	}
	s.Close()
	// 非 session 通道。
	if _, _, err := c.OpenChannel("direct-streamlocal@openssh.com", nil); err == nil {
		t.Fatal("unknown channel types must be rejected")
	}
}

func TestEnvRequestWhitelist(t *testing.T) {
	f := newFixture(t, 4)
	c, _ := f.dial(t, f.sb.ID, "s3cret")
	defer c.Close()
	s, _ := c.NewSession()
	defer s.Close()
	if err := s.Setenv("LANG", "C.UTF-8"); err != nil {
		t.Errorf("LANG must be accepted: %v", err)
	}
	if err := s.Setenv("LC_ALL", "C"); err != nil {
		t.Errorf("LC_* must be accepted: %v", err)
	}
	for _, name := range []string{"LD_PRELOAD", "PATH", "SIMPLEBASE_TOKEN", "BASH_ENV"} {
		if err := s.Setenv(name, "x"); err == nil {
			t.Errorf("env %s must be rejected", name)
		}
	}
}

func TestSessionLimitReported(t *testing.T) {
	f := newFixture(t, 1)
	c, _ := f.dial(t, f.sb.ID, "s3cret")
	defer c.Close()
	open := func() (*ssh.Session, io.WriteCloser, error) {
		s, err := c.NewSession()
		if err != nil {
			return nil, nil, err
		}
		_ = s.RequestPty("xterm", 24, 80, ssh.TerminalModes{})
		in, _ := s.StdinPipe()
		_, _ = s.StdoutPipe()
		return s, in, s.Shell()
	}
	s1, in1, err := open()
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	s2, _, err := open()
	if err == nil {
		t.Fatal("second interactive session must be refused at the per-sandbox cap")
	}
	s2.Close()
	io.WriteString(in1, "exit\n")
	_ = s1.Wait()
	// 名额释放后可再开。
	deadline := time.Now().Add(3 * time.Second)
	for {
		s3, in3, err := open()
		if err == nil {
			io.WriteString(in3, "exit\n")
			s3.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("slot not released: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCloseDropsLiveSessions(t *testing.T) {
	f := newFixture(t, 4)
	c, _ := f.dial(t, f.sb.ID, "s3cret")
	s, _ := c.NewSession()
	_ = s.RequestPty("xterm", 24, 80, ssh.TerminalModes{})
	_, _ = s.StdinPipe()
	_, _ = s.StdoutPipe()
	if err := s.Shell(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = s.Wait(); close(done) }()
	_ = f.srv.Close()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("Close must drop live sessions")
	}
	if n := f.mgr.ProcCount(f.sb.ID); n != 0 {
		t.Fatalf("processes leaked after Close: %d", n)
	}
	// 关闭后不再接受新连接。
	cc, sc := NewPipe()
	go f.srv.ServeConn(sc)
	if _, _, _, err := ssh.NewClientConn(cc, "pipe", &ssh.ClientConfig{User: f.sb.ID,
		Auth: []ssh.AuthMethod{ssh.Password("s3cret")}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: time.Second}); err == nil {
		t.Fatal("closed server must refuse connections")
	}
}

// ---- 主机密钥 ----

func TestHostKeyCreatedOnceAndStable(t *testing.T) {
	path := t.TempDir() + "/sub/hk"
	k1, err := LoadOrCreateHostKey(path)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := LoadOrCreateHostKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if Fingerprint(k1) != Fingerprint(k2) || !strings.HasPrefix(Fingerprint(k1), "SHA256:") {
		t.Fatalf("host key must persist: %s vs %s", Fingerprint(k1), Fingerprint(k2))
	}
	if err := writeGarbage(path); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateHostKey(path); err == nil {
		t.Fatal("a corrupt key file must be an error, not silently regenerated")
	}
}

func TestNewRequiresDependencies(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("New must reject missing dependencies")
	}
}

func TestValidTermAndEnvAllowList(t *testing.T) {
	for _, ok := range []string{"xterm", "xterm-256color", "screen.linux", "vt100"} {
		if !validTerm(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "x y", "a;b", "$(id)", strings.Repeat("a", 33), "a\nb"} {
		if validTerm(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}
