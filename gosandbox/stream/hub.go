// Package stream 把「只有一个读者」的输出源（进程 stdout、管道）转成多订阅者的
// 实时流，并保留一段固定长度的回放缓冲。
//
// 为什么需要它：vm.Proc.Output() 只能有一个读者。如果当前流的读协程与随后的
// Connect 各自去读，旧协程会抢走本该属于新连接的数据。由 Hub 持有唯一的读协程，
// 数据进入有上限的回放缓冲后推给所有订阅者，新订阅者先拿到缓冲里已有的内容
// （断开期间产生的输出），再接实时数据。
package stream

import (
	"io"
	"sync"
)

// ReplayCap 是回放缓冲保留的字节数。后台进程长期运行时缓冲不能无限增长；
// 超出的旧数据被丢弃，订阅者拿到的是「最近的输出」，与终端回滚缓冲语义一致。
const ReplayCap = 256 << 10

// Hub 是单一读取器 + 多订阅者广播。用 NewHub 构造后立即开始读取源。
type Hub struct {
	mu      sync.Mutex
	cond    *sync.Cond
	replay  []byte
	dropped int64 // 已被挤出缓冲的字节数，订阅者据此计算自己的位置
	closed  bool
}

// NewHub 从 r 持续读取并广播；r 返回错误（含 io.EOF）时标记结束，
// 所有订阅者的 Read 随后返回 io.EOF。
func NewHub(r io.Reader) *Hub {
	h := &Hub{}
	h.cond = sync.NewCond(&h.mu)
	go h.pump(r)
	return h
}

func (h *Hub) pump(r io.Reader) {
	buf := make([]byte, 16<<10)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			h.mu.Lock()
			h.replay = append(h.replay, buf[:n]...)
			if over := len(h.replay) - ReplayCap; over > 0 {
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

// Subscription 是一个订阅者在输出流里的读取位置。用完必须 Close。
type Subscription struct {
	h    *Hub
	pos  int64 // 下一个要读的绝对偏移（含已被挤出的部分）
	done chan struct{}
	once sync.Once
}

// Subscribe 创建订阅者。fromStart 为真时从缓冲里现存的最早数据开始
// （接回已运行的进程要看到历史）；为假时只收订阅之后的新数据。
func (h *Hub) Subscribe(fromStart bool) *Subscription {
	h.mu.Lock()
	defer h.mu.Unlock()
	pos := h.dropped + int64(len(h.replay))
	if fromStart {
		pos = h.dropped
	}
	return &Subscription{h: h, pos: pos, done: make(chan struct{})}
}

// Read 阻塞到有新数据、源结束（io.EOF）或订阅被 Close（io.ErrClosedPipe）。
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

// Close 结束订阅并唤醒阻塞中的 Read，可重复调用。
func (s *Subscription) Close() {
	s.once.Do(func() {
		close(s.done)
		s.h.mu.Lock()
		s.h.cond.Broadcast()
		s.h.mu.Unlock()
	})
}
