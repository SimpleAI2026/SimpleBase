// 文件说明：server.go — 云沙盒的进程内 SSH 服务端：把 SSH 会话落到沙盒里的 PTY / 命令上
//
// 用法
//
//	New(o Options) (*Server, error)
//	(s *Server) ServeConn(nc net.Conn)  // ServeConn 在一个已建立的连接上完成 SSH 握手并处理会话，阻塞到连接结束
//	(s *Server) Serve(l net.Listener) error  // Serve 在 listener 上接受连接，直到 Close
//	同包调用：Close() → membuf.go, session.go；ServeConn() → membuf.go；Identity → session.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/sshd/server.go
//	         │ 依赖
//	         ▼
//	[下游] internal/sandbox, internal/utils/atomicfile
//	[同包] membuf.go, session.go
//	[关键引用] Close() ← membuf.go, session.go
//
// 主要符号
//
//	type Identity struct {…}
//	type Authenticator interface {…}
//	type Sessions interface {…}
//	type Audit func(ctx context.Context, id Identity, kind string, duration ...
//	type Server struct {…}
//	type Options struct {…}
//	... 另有 24 个顶层符号

package sshd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/utils/atomicfile"
)

// Identity 是认证通过后确定的访问主体：哪个项目的哪个沙盒。
type Identity struct {
	ProjectID string
	SandboxID string
	// Actor 用于审计，不含密钥。
	Actor string
}

// Authenticator 校验 SSH 登录。username 是沙盒 ID，secret 是密码（API Key 或一次性票据）。
//
// 实现必须保证：沙盒不存在、无权限、密钥错误对外是同一种失败（返回同一个错误），不泄露存在性。
type Authenticator interface {
	Authenticate(ctx context.Context, username, secret string, remote net.Addr) (Identity, error)
}

// Sessions 是 sshd 需要的沙盒能力，由 *sandbox.Manager 满足。
type Sessions interface {
	StartProc(ctx context.Context, projectID, id string, spec sandbox.StartSpec) (*sandbox.ProcHandle, error)
}

// Audit 在会话结束时被调用。只含元数据，不含键入内容与输出。
type Audit func(ctx context.Context, id Identity, kind string, duration time.Duration, exit int)

// Server 是 SSH 服务端。
type Server struct {
	cfg     *ssh.ServerConfig
	auth    Authenticator
	sess    Sessions
	audit   Audit
	limiter *failLimiter
	now     func() time.Time

	mu       sync.Mutex
	closed   bool
	conns    map[*ssh.ServerConn]struct{}
	listener net.Listener
	wg       sync.WaitGroup
}

// Options 配置服务端。
type Options struct {
	HostKey       ssh.Signer
	Authenticator Authenticator
	Sessions      Sessions
	Audit         Audit
	// MaxAuthTries 每个连接允许的密码尝试次数，默认 3。
	MaxAuthTries int
	// Now 便于测试注入。
	Now func() time.Time
}

// New 构造服务端。
func New(o Options) (*Server, error) {
	if o.HostKey == nil || o.Authenticator == nil || o.Sessions == nil {
		return nil, errors.New("sshd: host key, authenticator and sessions are required")
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	tries := o.MaxAuthTries
	if tries <= 0 {
		tries = 3
	}
	s := &Server{auth: o.Authenticator, sess: o.Sessions, audit: o.Audit, now: now,
		limiter: newFailLimiter(now), conns: map[*ssh.ServerConn]struct{}{}}
	cfg := &ssh.ServerConfig{
		MaxAuthTries: tries,
		// 只开放密码认证：用户名是沙盒 ID，密码是 API Key / 票据。
		PasswordCallback: s.passwordCallback,
		// 不泄露服务端版本细节。
		ServerVersion: "SSH-2.0-SimpleBase",
	}
	cfg.AddHostKey(o.HostKey)
	s.cfg = cfg
	return s, nil
}

const (
	permProject = "project"
	permSandbox = "sandbox"
	permActor   = "actor"
)

func (s *Server) passwordCallback(conn ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
	ip := hostOnly(conn.RemoteAddr())
	if !s.limiter.allow(ip) {
		return nil, errAuthFailed
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, err := s.auth.Authenticate(ctx, conn.User(), string(pw), conn.RemoteAddr())
	if err != nil {
		s.limiter.fail(ip)
		// 对外不区分原因：不存在 / 无权限 / 密钥错误都是同一个失败。
		return nil, errAuthFailed
	}
	s.limiter.reset(ip)
	return &ssh.Permissions{Extensions: map[string]string{
		permProject: id.ProjectID, permSandbox: id.SandboxID, permActor: id.Actor}}, nil
}

var errAuthFailed = errors.New("permission denied")

func hostOnly(a net.Addr) string {
	if a == nil {
		return ""
	}
	h, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	return h
}

// ServeConn 在一个已建立的连接上完成 SSH 握手并处理会话，阻塞到连接结束。
func (s *Server) ServeConn(nc net.Conn) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		nc.Close()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	defer nc.Close()

	// 握手限时，避免半开连接占住协程。
	// 握手限时用真实时钟：s.now 只用于限速与审计，测试里会被替换成固定时间。
	_ = nc.SetDeadline(time.Now().Add(20 * time.Second))
	sc, chans, reqs, err := ssh.NewServerConn(nc, s.cfg)
	if err != nil {
		return
	}
	_ = nc.SetDeadline(time.Time{})
	s.mu.Lock()
	s.conns[sc] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, sc)
		s.mu.Unlock()
		sc.Close()
	}()
	id := Identity{ProjectID: sc.Permissions.Extensions[permProject], SandboxID: sc.Permissions.Extensions[permSandbox],
		Actor: sc.Permissions.Extensions[permActor]}

	// 全局请求（如 tcpip-forward、keepalive）：只回应 keepalive，其余一律拒绝。
	go func() {
		for r := range reqs {
			if r.WantReply {
				_ = r.Reply(false, nil)
			}
		}
	}()
	var cwg sync.WaitGroup
	for nch := range chans {
		if nch.ChannelType() != "session" {
			// direct-tcpip、x11、forwarded-tcpip 等一律拒绝。
			_ = nch.Reject(ssh.UnknownChannelType, "only session channels are allowed")
			continue
		}
		ch, creqs, err := nch.Accept()
		if err != nil {
			continue
		}
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			s.handleSession(id, ch, creqs)
		}()
	}
	cwg.Wait()
}

// Serve 在 listener 上接受连接，直到 Close。
func (s *Server) Serve(l net.Listener) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return net.ErrClosed
	}
	s.listener = l
	s.mu.Unlock()
	for {
		nc, err := l.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}
		go s.ServeConn(nc)
	}
}

// Close 停止接受新连接并断开现有会话。
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	l := s.listener
	conns := make([]*ssh.ServerConn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	if l != nil {
		_ = l.Close()
	}
	for _, c := range conns {
		_ = c.Close()
	}
	s.wg.Wait()
	return nil
}

// ---- 主机密钥 ----

// LoadOrCreateHostKey 读取 Ed25519 主机密钥；不存在时生成并以 0600 原子写入。
func LoadOrCreateHostKey(path string) (ssh.Signer, error) {
	if b, err := os.ReadFile(path); err == nil {
		key, perr := ssh.ParseRawPrivateKey(b)
		if perr != nil {
			return nil, fmt.Errorf("sshd: parse host key %s: %w", path, perr)
		}
		return ssh.NewSignerFromKey(key)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := atomicfile.Write(path, pemBytes, 0o600); err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(priv)
}

// Fingerprint 返回主机公钥的 SHA256 指纹，格式同 ssh-keygen -l（"SHA256:..."）。
func Fingerprint(s ssh.Signer) string { return ssh.FingerprintSHA256(s.PublicKey()) }

// ---- 登录失败限速 ----

// failLimiter 按来源 IP 做滑动窗口限速：窗口内失败达到上限就拒绝后续尝试。
type failLimiter struct {
	mu     sync.Mutex
	now    func() time.Time
	window time.Duration
	max    int
	fails  map[string][]time.Time
}

func newFailLimiter(now func() time.Time) *failLimiter {
	return &failLimiter{now: now, window: time.Minute, max: 10, fails: map[string][]time.Time{}}
}

func (l *failLimiter) prune(ip string) []time.Time {
	cut := l.now().Add(-l.window)
	kept := l.fails[ip][:0]
	for _, t := range l.fails[ip] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.fails, ip)
		return nil
	}
	l.fails[ip] = kept
	return kept
}

func (l *failLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(ip)) < l.max
}

func (l *failLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.fails) > 4096 { // 防止被大量不同来源撑爆内存
		l.fails = map[string][]time.Time{}
	}
	l.fails[ip] = append(l.prune(ip), l.now())
}

func (l *failLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}

// ---- 会话 ----

type ptyReq struct {
	Term             string
	Cols, Rows, W, H uint32
	Modes            string
}

type windowChange struct{ Cols, Rows, W, H uint32 }

type execReq struct{ Command string }

type envReq struct{ Name, Value string }

// allowedEnv 判断 env 请求是否可接受：只接 TERM、LANG、LC_*。
func allowedEnv(name string) bool {
	return name == "TERM" || name == "LANG" || strings.HasPrefix(name, "LC_")
}

func isErr(err, target error) bool { return errors.Is(err, target) }
