// 文件说明：output_hub.go — internal/sandbox 中 Subscription、Read 等符号的实现
//
// 用法
//
//	(s *Subscription) Read(p []byte) (int, error)  // Read 阻塞到有新数据、进程输出结束（io.EOF）或订阅被 Close（io.ErrClosedPipe）
//	(s *Subscription) Close()  // Close 结束订阅，唤醒阻塞中的 Read
//	同包调用：Read() → manager.go, proc.go；Close() → manager.go, proc.go；outputHub → proc.go
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/sandbox/sshd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/output_hub.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] errors.go, manager.go, paths.go, proc.go, pty.go
//	[关键引用] Read() ← manager.go, proc.go
//
// 主要符号
//
//	type Subscription struct {…}
//	(s *Subscription) Read(p []byte) (int, error)
//	(s *Subscription) Close()
//	... 另有 5 个顶层符号

package sandbox

import (
	"io"
	"sync"
)

// outputHub 是进程输出的单一读取器 + 多订阅者广播。
//
// 一个进程的 Output() 只能有一个读者；如果 process.Start 的流和之后的 process.Connect 各自去读，
// 旧流的读协程会抢走本该属于新连接的数据。所以由 hub 持有唯一的读协程，把数据写进
// 有上限的回放缓冲，并推给所有订阅者；新订阅者先拿到缓冲里已有的内容（断开期间产生的输出），
// 再接实时数据。
//
// 缓冲只保留最近 replayCap 字节：后台进程长期运行时不能无限增长；超出的旧数据被丢弃，
// 订阅者拿到的是「最近的输出」，与终端回滚缓冲的语义一致。
type outputHub struct {
	mu      sync.Mutex
	cond    *sync.Cond
	replay  []byte
	dropped int64 // 已被挤出缓冲的字节数，订阅者据此计算自己的位置
	closed  bool
}

const replayCap = 256 << 10

func newOutputHub(r io.Reader) *outputHub {
	h := &outputHub{}
	h.cond = sync.NewCond(&h.mu)
	go h.pump(r)
	return h
}

func (h *outputHub) pump(r io.Reader) {
	buf := make([]byte, 16<<10)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			h.mu.Lock()
			h.replay = append(h.replay, buf[:n]...)
			if over := len(h.replay) - replayCap; over > 0 {
				h.replay = append([]byte(nil), h.replay[over:]...)
				h.dropped += int64(over)
			}
			h.cond.Broadcast()
			h.mu.Unlock()
		}
		if err != nil {
			h.mu.Lock()
			h.closed = true
			h.cond.Broadcast()
			h.mu.Unlock()
			return
		}
	}
}

// Subscription 是一个订阅者在输出流里的读取位置。
type Subscription struct {
	h    *outputHub
	pos  int64 // 下一个要读的绝对偏移（含已被挤出的部分）
	done chan struct{}
	once sync.Once
}

// Subscribe 创建订阅者。fromStart 为真时从缓冲里现存的最早数据开始（Connect 到已运行的进程要看到历史）；
// 为假时只收订阅之后的新数据。
func (h *outputHub) Subscribe(fromStart bool) *Subscription {
	h.mu.Lock()
	defer h.mu.Unlock()
	pos := h.dropped + int64(len(h.replay))
	if fromStart {
		pos = h.dropped
	}
	return &Subscription{h: h, pos: pos, done: make(chan struct{})}
}

// Read 阻塞到有新数据、进程输出结束（io.EOF）或订阅被 Close（io.ErrClosedPipe）。
// 订阅者落后到数据已被挤出缓冲时，直接从现存最早的数据继续（丢掉的部分无法找回）。
func (s *Subscription) Read(p []byte) (int, error) {
	h := s.h
	h.mu.Lock()
	defer h.mu.Unlock()
	for {
		select {
		case <-s.done:
			return 0, io.ErrClosedPipe
		default:
		}
		if s.pos < h.dropped {
			s.pos = h.dropped
		}
		if avail := h.dropped + int64(len(h.replay)) - s.pos; avail > 0 {
			n := copy(p, h.replay[s.pos-h.dropped:])
			s.pos += int64(n)
			return n, nil
		}
		if h.closed {
			return 0, io.EOF
		}
		h.cond.Wait()
	}
}

// Close 结束订阅，唤醒阻塞中的 Read。可重复调用。
func (s *Subscription) Close() {
	s.once.Do(func() {
		close(s.done)
		s.h.mu.Lock()
		s.h.cond.Broadcast()
		s.h.mu.Unlock()
	})
}
