// 文件说明：watch.go — gosandbox/box 中 WatchOptions、FileEvent 等符号的实现
//
// 用法
//
//	type WatchOptions struct{…}  // WatchOptions 是监听的参数
//	type FileEvent struct{…}  // FileEvent 是一次文件变化
//	type Watch struct{…}  // Watch 是一个进行中的目录监听
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/watch.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//	[关键引用] WatchOptions ← backpressure_test.go, lifecycle_test.go
//
// 主要符号
//
//	type WatchOptions struct{…}
//	type FileEvent struct{…}
//	type Watch struct{…}
//	var ErrWatchOverflow
//	(s *Session) Watch(ctx context.Context, path string, opt WatchOptions) (*Watch, error)
//	(w *Watch) Events() <-chan FileEvent
//	(w *Watch) Done() <-chan struct{}
//	(w *Watch) Overflowed() bool
//	... 另有 4 个顶层符号

package box

import (
	"context"
	"errors"
	"io"
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opt.Buffer <= 0 {
		opt.Buffer = 64
	}
	id := newObjectID("w")
	w := &Watch{sess: s, id: id, events: make(chan FileEvent, opt.Buffer), done: make(chan struct{})}
	s.mu.Lock()
	if s.watches == nil {
		s.watches = map[string]*Watch{}
	}
	s.watches[id] = w
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		_ = w.Close()
		return nil, ctx.Err()
	case <-s.done:
		_ = w.Close()
		return nil, io.ErrClosedPipe
	case s.out <- outFrame{msg: &proto.Msg{Kind: proto.KindWatch, ID: id, Path: path, Recursive: opt.Recursive}}:
	}
	go func() {
		select {
		case <-ctx.Done():
		case <-s.done:
		case <-w.done:
			return
		}
		_ = w.Close()
	}()
	return w, nil
}

// Events 是事件通道。Close 之后通道关闭。
func (w *Watch) Events() <-chan FileEvent { return w.events }

// Done 在监听终止时关闭，不需要先消费积压事件。
func (w *Watch) Done() <-chan struct{} { return w.done }

// Overflowed 报告是否丢过事件。
func (w *Watch) Overflowed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.overflow
}

// deliver 收一条事件（由 Session 的读循环调用）。
func (w *Watch) deliver(entries []proto.Entry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
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
			w.overflow = true
		}
	}
}

// Close 停止监听。
func (w *Watch) Close() error {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		close(w.events)
		close(w.done)
		w.mu.Unlock()
		w.sess.dropWatch(w.id)
		w.sess.queueCleanup(&proto.Msg{Kind: proto.KindUnwatch, ID: w.id})
	})
	return nil
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
