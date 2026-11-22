// Package box 是沙盒的宿主侧：拉起一个常驻 agent、持有控制连接、按请求
// 把帧发给它，并把 agent 的异步事件（输出、退出）转给调用方。
//
// 与 host 包的区别：host 每条命令新起一个 bwrap，命令结束沙盒就不存在了；
// box 里 bwrap 常驻、agent 是沙盒的 PID 1，因此后台进程能跨请求存活、
// exec 与文件操作都在同一个命名空间里、暂停能保留进程状态。
package box

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/netx"
	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// ErrUnavailable 表示当前平台没有 box 后端（agent 只在 Linux 沙盒里跑）。
var ErrUnavailable = errors.New("box: only available on linux")

// Config 是拉起一个沙盒所需的一切。
type Config struct {
	// Name 是沙盒名（cgroup 与目录都用它）。
	Name string
	// Root 是沙盒目录：ws/ 是可写工作区，upper/work 是 overlay 的可写层。
	Root string
	// Workspace 是沙盒内的工作目录路径，默认 /workspace。
	Workspace string
	// Helper 是 gosandbox-inner 的绝对路径。
	Helper string
	// Bwrap 是 bwrap 的绝对路径；空表示不套 bwrap（测试用）。
	Bwrap string
	// Lower 是解包好的只读根目录；非空时以它作沙盒根（overlay 或 copy）。
	// 空表示宿主根只读（image=host）。
	Lower string
	// LogDir 是进程输出的落盘目录；空表示不落盘（只保留实时流）。
	// 落盘后进程退出仍能回放（E2B 的 logs 依赖它）。
	LogDir string
	// LogOptions 是日志轮转参数；零值取默认（10 MiB × 3）。
	LogOptions Options
	// Supervise 为真时把沙盒交给独立的 supervisor 进程托管：
	// 服务进程重启后能重新接管，沙盒与其中的进程不丢。
	//
	// 代价：断开的那一刻，agent 正在发但还没被读走的帧会丢（协议是帧级的）。
	// 开启前请确认调用方能重试。
	Supervise bool
	// Network 是出网策略；零值（Mode 为空）按 none 处理——最安全的一档。
	//
	// 在加这个字段之前，沙盒没有 --unshare-net，也就是说它直接共享宿主
	// 网络栈：能访问云元数据与私网。默认值必须是 none。
	Network netx.Policy
	// Policy 是交给 helper 的隔离策略（Landlock 规则）。
	Policy Policy
	// Env 是 agent 自己的环境（不是容器内命令的环境）。
	Env []string
	// Log 接收宿主侧日志；nil 表示不输出。
	Log func(level, msg string, kv ...any)
}

// Policy 是沙盒内 Landlock 的规则来源。字段与 host.Policy 的对应项一致，
// 这里单独定义是为了让 box 不依赖 host 的内部结构。
type Policy struct {
	// Mode 是 read-only 或 workspace-write。
	Mode string
	// Workspace 与 WritableRoots 是 Landlock 允许写的目录。
	Workspace     string
	WritableRoots []string
	// ScratchDirs 是 tmpfs 目录（/tmp 等），Landlock 同样放行写。
	ScratchDirs []string
	// Seccomp 是 seccomp 档位："host"（默认）或 "default"。
	Seccomp string
}

// Session 是一个运行中的沙盒：一条到 agent 的控制连接。
type Session struct {
	cfg Config

	// out 是单写者通道：所有帧串行写出，避免交错。
	// raw 非空表示该帧之后紧跟这段裸字节（FSWrite 用），
	// 必须与帧一起经同一个 writer 发出，否则对端会把后续帧当文件内容。
	out chan outFrame
	// done 在读侧退出时关闭。
	done chan struct{}
	// writeErr 是写侧第一次失败的原因。
	writeErr atomic.Value

	// mu 保护 handles 与 seq。
	mu      sync.Mutex
	seq     uint32
	handles map[string]*Handle
	pending map[uint32]chan proto.Msg
	relays  map[uint32]*relay
	// sinks 是进程输出的落盘池；nil 表示不落盘。（不叫 logs：与 Logs() 方法重名）
	sinks *logSink
	// watches 是进行中的目录监听，按监听 id 索引。
	watches map[string]*Watch
	// egress 是 pasta 进程；nil 表示没有出网（none 档或没接上）。
	egress *netx.Runner

	// conn 是控制通道的宿主侧，Close 时用来收尾。
	conn net.Conn
	// stats 是最后一次 Stats() 的结果，供 Driver 的 Metrics 用。
	stats atomic.Pointer[proto.Stats]
	// closed 表示已 Close。
	closed atomic.Bool
	// doneOnce 保证 done 只关一次：reader / writer / 错误路径都会关它，
	// 重连（supervisor）时两条路径并存会 panic: close of closed channel。
	doneOnce sync.Once
}

// Start 拉起 bwrap + agent 并等 Hello。
func Start(ctx context.Context, cfg Config) (*Session, error) {
	if cfg.Name == "" {
		return nil, errors.New("box: name is required")
	}
	if cfg.Workspace == "" {
		cfg.Workspace = "/workspace"
	}
	conn, err := launch(ctx, cfg)
	if err != nil {
		return nil, err
	}
	s, err := newSessionOn(ctx, cfg, conn)
	if err != nil {
		return nil, err
	}
	s.egress = s.startEgress(ctx)
	return s, nil
}

// startEgress 在 agent 就绪之后把沙盒的 netns 交给 pasta。
//
// 必须等 Hello 之后再启动：cmd.Start 返回时 bwrap 可能还没完成 unshare，
// 此时 /proc/<pid>/ns/net 还指向上一个命名空间（或打不开）。早启动会让
// pasta 拿着错误的 netns 配置半天，最后表现为「网络配好了却连不上」。
//
// 失败不致命——沙盒仍能跑（只是没有出网），但必须报出来。
func (s *Session) startEgress(ctx context.Context) *netx.Runner {
	cfg := s.cfg
	if cfg.Network.Mode == "" || cfg.Network.Mode == netx.ModeNone {
		return nil
	}
	pid, ok := s.conn.(interface{ Pid() int })
	if !ok || pid.Pid() <= 0 {
		// supervisor 模式下控制面是 unix socket，拿不到沙盒进程 pid：
		// egress 与 supervise 目前不能同时开启，如实报出来。
		if cfg.Log != nil {
			cfg.Log("warn", "sandbox egress is unavailable: this control channel does not expose the sandbox pid (egress cannot be combined with supervise)")
		}
		return nil
	}
	r, err := netx.Start(ctx, netx.StartOptions{
		NetnsPID: pid.Pid(), Mode: cfg.Network.Mode,
		Log: func(line string) {
			if cfg.Log != nil {
				cfg.Log("warn", "pasta: "+line)
			}
		},
	})
	if err != nil {
		if cfg.Log != nil {
			cfg.Log("warn", "sandbox egress is unavailable; the sandbox runs without network",
				"error", err.Error())
		}
		return nil
	}
	return r
}

// closeDone 只关闭一次 done。
func (s *Session) closeDone() {
	s.doneOnce.Do(func() { close(s.done) })
}

// newObjectID 生成宿主侧唯一的对象 id。
//
// 不能用自增序号：supervisor 重连后新 Session 的 seq 从头开始，会与 agent
// 里仍活着的旧进程撞名（agent 返回 "process p1 already exists"）。
func newObjectID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(b[:])
}

// newSession 在已建立的控制通道上起一个 Session，等 agent 的 Hello。
//
// 单独拆出来是为了让测试能直接给一对内存管道，不必真的 fork 一个 agent。
func newSessionOn(ctx context.Context, cfg Config, conn net.Conn) (*Session, error) {
	s := &Session{
		cfg:     cfg,
		conn:    conn,
		out:     make(chan outFrame, 256),
		done:    make(chan struct{}),
		handles: map[string]*Handle{},
		pending: map[uint32]chan proto.Msg{},
		relays:  map[uint32]*relay{},
	}
	// 落盘是旁路：失败只警告，不影响沙盒。
	s.sinks = newLogSink(cfg.LogDir, cfg.LogOptions, func(msg string, kv ...any) {
		if cfg.Log != nil {
			cfg.Log("warn", msg, kv...)
		}
	})
	go s.writer(conn)
	// 第一帧必须是 Hello：版本不一致直接拒绝。
	var kind proto.Kind
	var payload []byte
	if err := setReadDeadline(conn, 30*time.Second); err != nil {
		conn.Close()
		s.closeDone()
		return nil, err
	}
	if err := proto.ReadFrame(conn, &kind, &payload); err != nil {
		conn.Close()
		s.closeDone()
		return nil, fmt.Errorf("box: agent did not say hello: %w", err)
	}
	hello, err := proto.Decode(kind, payload)
	if err != nil || hello.Kind != proto.KindHello {
		conn.Close()
		s.closeDone()
		return nil, fmt.Errorf("box: unexpected first frame")
	}
	if hello.Version != proto.Version {
		conn.Close()
		s.closeDone()
		return nil, fmt.Errorf("box: agent speaks protocol %d, want %d", hello.Version, proto.Version)
	}
	_ = setReadDeadline(conn, 0)
	go s.reader(conn)
	return s, nil
}

// outFrame 是写侧的一帧，可带一段紧随其后的裸字节。
type outFrame struct {
	msg *proto.Msg
	raw []byte
}

// writer 是唯一的写入者。帧与它的裸字节之间不能插别的东西，
// 所以两者在同一次循环里写出。
func (s *Session) writer(w io.Writer) {
	for f := range s.out {
		if err := proto.Send(w, f.msg); err != nil {
			s.writeErr.Store(err)
			s.closeDone()
			return
		}
		if len(f.raw) > 0 {
			if _, err := w.Write(f.raw); err != nil {
				s.writeErr.Store(err)
				s.closeDone()
				return
			}
		}
	}
}

// reader 读 agent 的帧并分发：进程事件进对应的 Handle，文件回复进 pending，
// 其余（统计、中继）就地处理。
func (s *Session) reader(r io.Reader) {
	defer func() {
		if !s.closed.Load() {
			s.closeDone()
		}
	}()
	var kind proto.Kind
	var payload []byte
	for {
		if err := proto.ReadFrame(r, &kind, &payload); err != nil {
			return
		}
		m, err := proto.Decode(kind, payload)
		if err != nil {
			return
		}
		s.dispatch(m)
	}
}

// dispatch 把一帧送到它的去处。
func (s *Session) dispatch(m proto.Msg) {
	switch m.Kind {
	case proto.KindOutput, proto.KindStarted, proto.KindExited, proto.KindInputConsumed:
		if h := s.handle(m.ID); h != nil {
			h.deliver(m)
		}
	case proto.KindError, proto.KindAck, proto.KindFsStatReply, proto.KindFsListReply,
		proto.KindFsReadReply, proto.KindFsWriteAck:
		if ch := s.takePending(m.Seq); ch != nil {
			ch <- m
		}
	case proto.KindStats:
		st := m.Stats
		s.stats.Store(&st)
	case proto.KindListening, proto.KindTop:
		// agent 回带 Seq，按序号找回等待的调用方。
		if ch := s.takePending(m.Seq); ch != nil {
			ch <- m
		}
	case proto.KindRelayData:
		if rc := s.relay(m.ConnID); rc != nil {
			rc.deliver(m.Data)
		}
	case proto.KindRelayHalfClose:
		// agent 侧半关闭：它不再发数据，但宿主还可以继续写。
		if m.Flags&halfCloseWrite != 0 {
			if rc := s.relay(m.ConnID); rc != nil {
				rc.deliverEOF()
			}
		}
		if m.Flags&halfCloseRead != 0 {
			// agent 不再读：宿主的写方向也断了，避免写进一个没人收的管道。
			if rc := s.relay(m.ConnID); rc != nil {
				_ = rc.Close()
			}
		}
	case proto.KindRelayClose:
		if rc := s.relay(m.ConnID); rc != nil {
			if m.Flags&proto.RelayConnectFailed != 0 {
				// 沙盒内没人 listen：让 DialPort 报错，而不是返回一条「立刻 EOF」的连接。
				rc.abort(fmt.Errorf("box: nothing is listening on 127.0.0.1:%d in the sandbox", rc.port))
			} else {
				rc.deliverEOF()
			}
		}
	case proto.KindWatchEvent:
		if w := s.watch(m.ID); w != nil {
			w.deliver(m.Entries)
		}
	}
}

// handle 按 id 找进程句柄。
func (s *Session) handle(id string) *Handle {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handles[id]
}

// takePending 取走一个等待回复的通道（一次性）。
func (s *Session) takePending(seq uint32) chan proto.Msg {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := s.pending[seq]
	delete(s.pending, seq)
	return ch
}

// relay 按 id 找中继连接。
func (s *Session) relay(id uint32) *relay {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.relays[id]
}

// send 投一帧给写侧。
func (s *Session) send(m *proto.Msg) error {
	if s.closed.Load() {
		return io.ErrClosedPipe
	}
	select {
	case s.out <- outFrame{msg: m}:
		return nil
	case <-s.done:
		if err, ok := s.writeErr.Load().(error); ok {
			return err
		}
		return io.ErrClosedPipe
	}
}

// nextSeq 返回下一个请求序号。
func (s *Session) nextSeq() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return s.seq
}

// request 发一帧并等它的回复。
func (s *Session) request(m *proto.Msg, timeout time.Duration) (proto.Msg, error) {
	seq := s.nextSeq()
	ch := make(chan proto.Msg, 1)
	s.mu.Lock()
	s.pending[seq] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, seq)
		s.mu.Unlock()
	}()
	// 用 seq 作为关联键：文件回复带 Seq，Ack/Error 也带。
	m.Seq = seq
	if err := s.send(m); err != nil {
		return proto.Msg{}, err
	}
	select {
	case r := <-ch:
		if r.Kind == proto.KindError {
			return r, errors.New(r.Err)
		}
		return r, nil
	case <-time.After(timeout):
		return proto.Msg{}, fmt.Errorf("box: %d timed out", m.Kind)
	case <-s.done:
		return proto.Msg{}, io.ErrClosedPipe
	}
}

// Close 关掉沙盒：先请 agent 自己退出，超时就杀。
func (s *Session) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	// 尽力发一帧 Shutdown；连接已坏时忽略。
	select {
	case s.out <- outFrame{msg: &proto.Msg{Kind: proto.KindShutdown}}:
	default:
	}
	close(s.out)
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
	}
	// 先关监听：它们的 Close 会给 agent 发 Unwatch，必须在 out 关闭之前。
	s.mu.Lock()
	watches := s.watches
	s.watches = map[string]*Watch{}
	s.mu.Unlock()
	for _, w := range watches {
		_ = w.Close()
	}
	// pasta 在 netns 销毁后本会自己退出，但控制通道先关可能让它
	// 还挂着：显式停掉，避免残留进程。
	if s.egress != nil {
		_ = s.egress.Stop()
	}
	closeSession(s)
	if s.sinks != nil {
		s.sinks.closeAll()
	}
	return nil
}

// Done 在会话结束时关闭。
func (s *Session) Done() <-chan struct{} { return s.done }

// statsPtr 返回最近一次 Stats 的结果。
func (s *Session) statsPtr() *proto.Stats { return s.stats.Load() }
