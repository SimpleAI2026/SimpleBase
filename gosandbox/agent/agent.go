// Package agent 是沙盒内的 PID 1：一个常驻进程，替宿主执行命令、读写文件、
// 开 PTY、报告用量，并把宿主的入站连接中继到沙盒内的 127.0.0.1。
//
// 为什么需要一个常驻 agent：bwrap 的进程树一旦建好，宿主无法再把新进程放进
// 同一组命名空间（rootless 下 setns 进别人的 userns 需要该 userns 的权限）。
// 因此「后台进程跨请求存活」「exec 进同一沙盒」「文件在沙盒视角读写」只能由
// 沙盒内的 PID 1 自己 fork 出来做，这也正是 kiri 让 guest agent 当 PID 1 的
// 原因（thirdparty/kiri/crates/kiri-init/src/linux/agent.rs）。
//
// 本文件是服务端骨架：读循环按帧分发、写侧单写者、信号与 PID 1 的收尸。
package agent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// Server 是 agent 的状态：控制连接、进程表、写侧串行化。
type Server struct {
	// in 是读侧：帧与 FsWrite 的裸字节都从它读。
	in io.Reader
	// ctrl 是写侧：所有帧从它写出。socketpair 下与 in 是同一个 fd。
	ctrl io.Writer
	log  io.Writer
	// out 是单写者通道：所有帧经它串行写出，避免交错。
	out chan *proto.Msg
	// done 在写侧退出（连接坏掉）时关闭，让 send 立刻返回而不是永久阻塞。
	done chan struct{}
	// procs 是进程表。读循环增删，收尸 goroutine 也删（进程自己退出），
	// 所以用 mu 保护；单个 proc 内部的字段各自有同步。
	mu    sync.Mutex
	procs map[string]*proc
	// relays 是正在中继的连接，按 ConnID 索引。
	//
	// relayMu 只保护这张表：读循环建/删条目，每条连接的输出泵在结束时也删，
	// 不隔离就是并发 map 读写（Go 运行时会直接 fatal）。
	relayMu sync.Mutex
	relays  map[uint32]*relayConn
	// watches 是进行中的目录监听，按监听 id 索引。
	// 类型在平台文件里定义（Linux 有 inotify，其它平台没有）。
	watches map[string]*watcher
}

// 网络档位由宿主通过 --network 传入，与 netx.Mode 的取值一致。
const (
	networkNone   = "none"
	networkEgress = "egress"
)

// Main 是 agent 的入口：解析 --agent --ctrl N，然后在那个 fd 上服务。
//
// 失败返回非零退出码；正常路径只在收到 Shutdown 或控制连接断开时返回。
func Main(args []string, _ []string) int {
	ctrl, mapping, network, err := parseAgentArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gosandbox-inner: %v\n", err)
		return 2
	}
	// 把宿主给的映射范围交给 user.go：切 uid 之前要先知道「这个 id 存不存在」。
	SetUserMapping(mapping)
	// 网络隔离之后 lo 是 DOWN 的：入站中继与白名单 DNS 都依赖它。
	// 失败只警告——沙盒仍能用（只是 127.0.0.1 不可达），不该因此起不来。
	if err := bringUpLoopback(); err != nil {
		fmt.Fprintf(os.Stderr, "gosandbox-inner: cannot bring up loopback: %v\n", err)
	}
	// egress 档：出站由 pasta 在宿主侧透明转发，pasta 不做目标地址过滤，
	// 所以元数据与私网必须由 agent 在自己的 netns 里用黑洞路由挡掉。
	// 拦不上就不启动（fail-closed）：放它跑等于用户以为隔离了，实际能读云凭据。
	if network == networkEgress {
		if err := blockPrivateNetworks(); err != nil {
			fmt.Fprintf(os.Stderr, "gosandbox-inner: cannot install egress filters: %v\n", err)
			return 3
		}
	}
	// 网络配置到此为止：把 CAP_NET_ADMIN 丢掉，用户命令不能改沙盒网络
	// （否则它可以删掉黑洞路由，把私网放回来）。失败只警告——最坏情况是
	// 沙盒内保留了这份 capability，而它只作用于沙盒自己的 netns。
	if err := dropNetAdmin(); err != nil {
		fmt.Fprintf(os.Stderr, "gosandbox-inner: cannot drop CAP_NET_ADMIN: %v\n", err)
	}
	if err := Serve(ctrl, ctrl, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "gosandbox-inner: agent: %v\n", err)
		return 1
	}
	return 0
}

// parseAgentArgs 解析 agent 模式的参数。控制用的 fd 由宿主通过 ExtraFiles 传入，
// 不经 argv 传值以外的任何东西。
func parseAgentArgs(args []string) (*os.File, *UserMapping, string, error) {
	var fd = -1
	var mapping *UserMapping
	network := networkNone
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--ctrl":
			if i+1 >= len(args) {
				return nil, nil, "", errors.New("--ctrl needs a file descriptor")
			}
			n := 0
			if _, err := fmt.Sscanf(args[i+1], "%d", &n); err != nil || n < 3 {
				return nil, nil, "", fmt.Errorf("invalid --ctrl %q", args[i+1])
			}
			fd = n
			i++
		case "--network":
			if i+1 >= len(args) {
				return nil, nil, "", errors.New("--network needs a mode")
			}
			switch args[i+1] {
			case networkNone, networkEgress:
				network = args[i+1]
			default:
				return nil, nil, "", fmt.Errorf("invalid --network %q", args[i+1])
			}
			i++
		case "--uidmap":
			if i+1 >= len(args) {
				return nil, nil, "", errors.New("--uidmap needs a spec")
			}
			m, perr := parseUserMapping(args[i+1])
			if perr != nil {
				return nil, nil, "", perr
			}
			mapping = m
			i++
		default:
			return nil, nil, "", fmt.Errorf("unexpected argument %q", args[i])
		}
	}
	if fd < 0 {
		return nil, nil, "", errors.New("usage: gosandbox-inner --agent --ctrl N [--network none|egress] [--uidmap SPEC]")
	}
	// 控制通道不能让沙盒内的任何进程继承：os.NewFile 不会加 FD_CLOEXEC，
	// 而 ExtraFiles 传进来的 fd 在 fork 时是可继承的。不设它，用户命令
	// （以及它拉起的每一个子孙进程）都能读写控制面，甚至往里写垃圾字节
	// 把协议流带偏。
	syscall.CloseOnExec(fd)
	// 接管这个 fd：它由宿主通过 ExtraFiles 传入，已经是打开的。
	return os.NewFile(uintptr(fd), "ctrl"), mapping, network, nil
}

// Serve 在同一条连接上服务到关闭：in 读、ctrl 写。
//
// 读循环单 goroutine：进程表因此不需要锁，Start 与 Signal 的先后顺序也确定。
// 每个进程的输出泵是独立 goroutine，但只往 out 通道投递，不直接写连接。
func Serve(in io.Reader, ctrl io.Writer, log io.Writer) error {
	s := &Server{
		in:      in,
		ctrl:    ctrl,
		log:     log,
		out:     make(chan *proto.Msg, 256),
		done:    make(chan struct{}),
		procs:   map[string]*proc{},
		relays:  map[uint32]*relayConn{},
		watches: map[string]*watcher{},
	}

	// 写侧：唯一的写入者。它退出时关闭 done，让仍在 send 的 goroutine 立刻返回。
	writerDone := make(chan error, 1)
	go func() {
		defer close(s.done)
		for m := range s.out {
			if err := proto.Send(ctrl, m); err != nil {
				writerDone <- err
				return
			}
		}
		writerDone <- nil
	}()

	// 第一帧是 Hello：版本不一致直接拒绝，避免后续帧被误解析。
	if err := s.send(&proto.Msg{Kind: proto.KindHello, Version: proto.Version, Agent: "gosandbox-inner"}); err != nil {
		return err
	}

	readErr := s.loop()
	s.killAll()
	s.stopWatches()
	close(s.out)
	// 等写侧把已入队的帧写完。
	if werr := <-writerDone; werr != nil && readErr == nil {
		readErr = werr
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	return nil
}

// loop 是读循环：读一帧、分发、看是否需要退出。
func (s *Server) loop() error {
	var kind proto.Kind
	var payload []byte
	for {
		if err := proto.ReadFrame(s.in, &kind, &payload); err != nil {
			return err
		}
		m, err := proto.Decode(kind, payload)
		if err != nil {
			// 协议错误不可恢复：告诉宿主并断开，让它重建沙盒。
			_ = s.send(&proto.Msg{Kind: proto.KindError, Err: err.Error()})
			return err
		}
		if done, err := s.dispatch(m); done || err != nil {
			return err
		}
	}
}

// dispatch 处理一帧。返回 done 表示该关闭连接。
func (s *Server) dispatch(m proto.Msg) (bool, error) {
	switch m.Kind {
	case proto.KindPing:
		_ = s.send(&proto.Msg{Kind: proto.KindPong, Nonce: m.Nonce})
		return false, nil
	case proto.KindPong:
		return false, nil
	case proto.KindShutdown:
		return true, nil

	case proto.KindStart:
		return false, s.start(m)
	case proto.KindInput:
		s.input(m)
		return false, nil
	case proto.KindCloseStdin:
		s.closeStdin(m)
		return false, nil
	case proto.KindResize:
		s.resize(m)
		return false, nil
	case proto.KindSignal:
		s.signal(m)
		return false, nil

	case proto.KindFsStat, proto.KindFsList, proto.KindFsRead, proto.KindFsRemove, proto.KindFsMkdir, proto.KindFsRename:
		s.fsOp(m)
		return false, nil
	case proto.KindFsWrite:
		// 写请求后面跟 Size 字节裸数据，必须先读出来再处理，
		// 否则后续帧会被当成文件内容。
		return false, s.fsWrite(m)

	case proto.KindStatsQuery:
		s.sendStats()
		return false, nil
	case proto.KindTopQuery:
		s.sendTop(m)
		return false, nil
	case proto.KindListenQuery:
		s.sendListening(m)
		return false, nil

	case proto.KindRelay:
		s.openRelay(m)
		return false, nil
	case proto.KindRelayData:
		s.relayData(m)
		return false, nil
	case proto.KindRelayHalfClose:
		s.relayHalfClose(m)
	case proto.KindRelayClose:
		s.relayClose(m)
		return false, nil

	case proto.KindWatch:
		s.watch(m)
		return false, nil
	case proto.KindUnwatch:
		s.unwatch(m)
		return false, nil
	}
	// 未知或方向不对的帧：回错误但不退出，宿主可能是新版本。
	_ = s.send(&proto.Msg{Kind: proto.KindError, Err: fmt.Sprintf("unsupported frame kind %d", m.Kind)})
	return false, nil
}

// send 把帧投给写侧。通道满时阻塞：这是背压，不是错误。
// 写侧退出（连接坏掉）时立刻返回 io.ErrClosedPipe，而不是永久阻塞。
func (s *Server) send(m *proto.Msg) error {
	select {
	case s.out <- m:
		return nil
	case <-s.done:
		return io.ErrClosedPipe
	}
}
