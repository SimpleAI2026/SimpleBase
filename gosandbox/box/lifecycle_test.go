// 文件说明：lifecycle_test.go — gosandbox/box 的单元测试（TestWatchDeliveryAndClose、TestWatchEndsWithOwner 等）
//
// 用法
//
//	go test ./gosandbox/box -run 'TestWatchDeliveryAndClose|TestWatchEndsWithOwner|TestRelayListenerEndsOnClose|TestDialPortCancelledWhileConnecting'
//	同包调用：awaitClosed() → backpressure_test.go
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/lifecycle_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//	[关键引用] awaitClosed ← backpressure_test.go
//
// 主要符号
//
//	awaitClosed(t *testing.T, done <-chan struct{})
//	TestWatchDeliveryAndClose(t *testing.T)
//	TestWatchEndsWithOwner(t *testing.T)
//	TestRelayListenerEndsOnClose(t *testing.T)
//	TestDialPortCancelledWhileConnecting(t *testing.T)
//	TestDialPortCloseReleasesListener(t *testing.T)
//	TestFullQueueDoesNotBlockCancellation(t *testing.T)
//	TestSessionWriterEndsWithoutQueueClose(t *testing.T)

package box

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

func awaitClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lifecycle did not terminate")
	}
}

func TestWatchDeliveryAndClose(t *testing.T) {
	_, s := newRelayForTest(t, 100)
	w, err := s.Watch(context.Background(), "/workspace", WatchOptions{Buffer: 1})
	if err != nil {
		t.Fatal(err)
	}
	w.deliver([]proto.Entry{{Name: "first", Kind: proto.FileRegular}, {Name: "dropped"}})
	if !w.Overflowed() {
		t.Fatal("missing overflow")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); w.deliver([]proto.Entry{{Name: "racing"}}); _ = w.Close() }()
	}
	wg.Wait()
	awaitClosed(t, w.Done())
	w.deliver([]proto.Entry{{Name: "after close"}})
	if ev := <-w.Events(); ev.Path != "first" {
		t.Fatalf("event = %+v", ev)
	}
	if _, ok := <-w.Events(); ok {
		t.Fatal("events not closed")
	}
}

func TestWatchEndsWithOwner(t *testing.T) {
	for _, cancelSession := range []bool{false, true} {
		_, s := newRelayForTest(t, 100)
		ctx, cancel := context.WithCancel(context.Background())
		w, err := s.Watch(ctx, "/workspace", WatchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if cancelSession {
			s.closeDone()
		} else {
			cancel()
		}
		awaitClosed(t, w.Done())
		cancel()
	}
}

func TestRelayListenerEndsOnClose(t *testing.T) {
	for _, end := range []string{"local", "remote", "context", "session"} {
		t.Run(end, func(t *testing.T) {
			r, s := newRelayForTest(t, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go r.watchContext(ctx)
			if err := r.CloseWrite(); err != nil {
				t.Fatal(err)
			}
			r.deliverEOF()
			select {
			case <-r.done:
				t.Fatal("half-close ended connection")
			default:
			}
			switch end {
			case "local":
				_ = r.Close()
			case "remote":
				s.dispatch(proto.Msg{Kind: proto.KindRelayClose, ConnID: 1})
			case "context":
				cancel()
			case "session":
				s.closeDone()
			}
			awaitClosed(t, r.watchDone)
			awaitClosed(t, r.done)
			if s.relay(1) != nil {
				t.Fatal("relay retained")
			}
			if _, err := r.Write([]byte("x")); !errors.Is(err, net.ErrClosed) {
				t.Fatalf("write = %v", err)
			}
			_ = r.Close()
		})
	}
}

func TestDialPortCancelledWhileConnecting(t *testing.T) {
	_, s := newRelayForTest(t, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := s.DialPort(ctx, 8080); result <- err }()
	request := <-s.out
	if request.msg.Kind != proto.KindRelay {
		t.Fatal("no relay request")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("dial ignored cancellation")
	}
	if s.relay(request.msg.ConnID) != nil {
		t.Fatal("failed dial retained relay")
	}
}

func TestDialPortCloseReleasesListener(t *testing.T) {
	_, s := newRelayForTest(t, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		request := <-s.out
		s.dispatch(proto.Msg{Kind: proto.KindRelayData, ConnID: request.msg.ConnID})
	}()
	conn, err := s.DialPort(ctx, 8080)
	if err != nil {
		t.Fatal(err)
	}
	awaitClosed(t, peerDone)
	r := conn.(*relay)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	awaitClosed(t, r.watchDone)
	if ctx.Err() != nil {
		t.Fatal("connection closed caller context")
	}
	select {
	case <-s.done:
		t.Fatal("connection closed session")
	default:
	}
}

func TestFullQueueDoesNotBlockCancellation(t *testing.T) {
	_, s := newRelayForTest(t, 100)
	w, err := s.Watch(context.Background(), "/workspace", WatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for len(s.out) < cap(s.out) {
		s.out <- outFrame{msg: &proto.Msg{Kind: proto.KindPing}}
	}
	closed := make(chan struct{})
	go func() { defer close(closed); _ = w.Close() }()
	awaitClosed(t, closed)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := s.DialPort(ctx, 80); result <- err }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("full queue blocked cancellation")
	}
}

func TestSessionWriterEndsWithoutQueueClose(t *testing.T) {
	_, s := newRelayForTest(t, 1)
	done := make(chan struct{})
	go func() { defer close(done); s.writer(io.Discard) }()
	s.closeDone()
	awaitClosed(t, done)
}
