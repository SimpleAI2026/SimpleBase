// 文件说明：watch.go — Session 的目录监听客户端
//
// 用法
//
//	w, err := s.Watch(ctx, "/workspace", box.WatchOptions{Recursive: true})
//	for ev := range w.Events() { … }   // ev.Path / ev.Kind
//	w.Close()
//
// 调用架构
//
//	[上游] gosandbox/vm/box、internal/api（envd WatchDir）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/watch.go
//	         │
//	         ▼
//	[下游] gosandbox/proto
//	[同包] session.go
//
// 主要符号
//
//	type WatchOptions struct {…}
//	type FileEvent struct {…}
//	type Watch struct {…}
//	(s *Session) Watch(ctx context.Context, path string, opt WatchOptions) (*Watch, error)
//	(w *Watch) Events() <-chan FileEvent
//	(w *Watch) Close() error

package box

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// WatchOptions 是监听的参数。
type WatchOptions struct {
	// Recursive 为真时连子目录一起看（运行中新建的子树也会被覆盖）。
	Recursive bool
	// Buffer 是事件通道的缓冲；0 取 64。
	// 满了会丢事件并给出 ErrWatchOverflow —— 宁可丢也不能让 agent 的输出泵阻塞。
	Buffer int
}

// FileEvent 是一次文件变化。
type FileEvent struct {
	// Path 是绝对路径（沙盒内视角）。
	Path string
	// Kind 是变化后的类型（目录/文件/其它）。
	Kind proto.FileKind
}

// Watch 是一个进行中的目录监听。
type Watch struct {
	sess *Session
	id   string

	events chan FileEvent
	done   chan struct{}
	// overflow 为真表示至少丢过一次事件（调用方应据此重新扫一遍目录）。
	overflow bool

	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
}

// ErrWatchOverflow 表示事件太多，通道放不下，已经丢了一些。
var ErrWatchOverflow = errors.New("box: file watch overflowed; rescan the directory")

// Watch 开始监听一个沙盒内路径。
func (s *Session) Watch(ctx context.Context, path string, opt WatchOptions) (*Watch, error) {
	if path == "" {
		return nil, errors.New("box: empty watch path")
	}
	if opt.Buffer <= 0 {
		opt.Buffer = 64
	}
	id := fmt.Sprintf("w%d", s.nextSeq())
	w := &Watch{sess: s, id: id, events: make(chan FileEvent, opt.Buffer), done: make(chan struct{})}
	s.mu.Lock()
	if s.watches == nil {
		s.watches = map[string]*Watch{}
	}
	s.watches[id] = w
	s.mu.Unlock()

	if err := s.send(&proto.Msg{Kind: proto.KindWatch, ID: id, Path: path, Recursive: opt.Recursive}); err != nil {
		s.dropWatch(id)
		w.Close()
		return nil, err
	}
	// ctx 取消时自动关掉，避免调用方忘了。
	if ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = w.Close()
			case <-w.done:
			case <-s.done:
			}
		}()
	}
	return w, nil
}

// Events 是事件通道。Close 之后通道关闭。
func (w *Watch) Events() <-chan FileEvent { return w.events }

// Overflowed 报告是否丢过事件。
func (w *Watch) Overflowed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.overflow
}

// deliver 收一条事件（由 Session 的读循环调用）。
func (w *Watch) deliver(entries []proto.Entry) {
	for _, e := range entries {
		path := e.Target
		if path == "" {
			path = e.Name
		}
		select {
		case w.events <- FileEvent{Path: path, Kind: e.Kind}:
		default:
			// 通道满：丢掉这一条并标记。让 agent 阻塞在写帧上会拖住
			// 整个沙盒的事件循环，比丢事件严重得多。
			w.mu.Lock()
			w.overflow = true
			w.mu.Unlock()
		}
	}
}

// Close 停止监听。
func (w *Watch) Close() error {
	var err error
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		w.mu.Unlock()
		w.sess.dropWatch(w.id)
		close(w.events)
		close(w.done)
		// 通知 agent 停掉内核侧的监听；连接已断时忽略。
		err = w.sess.send(&proto.Msg{Kind: proto.KindUnwatch, ID: w.id})
	})
	if errors.Is(err, errClosedPipe) {
		return nil
	}
	return err
}

// dropWatch 从 Session 的表里删掉。
func (s *Session) dropWatch(id string) {
	s.mu.Lock()
	delete(s.watches, id)
	s.mu.Unlock()
}

// watch 按 id 找监听。
func (s *Session) watch(id string) *Watch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watches[id]
}

// errClosedPipe 是 send 在连接关闭时返回的错误（与 io.ErrClosedPipe 同义）。
var errClosedPipe = errors.New("box: closed pipe")
