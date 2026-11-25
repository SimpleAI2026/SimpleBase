// 文件说明：backpressure_test.go — gosandbox/box 的单元测试（TestCleanupDeliveredAfterFullQueue、TestBlockedRelayWriteEndsOnClose 等）
//
// 用法
//
//	go test ./gosandbox/box -run 'TestCleanupDeliveredAfterFullQueue|TestBlockedRelayWriteEndsOnClose|TestWatchCreationCancellation|TestSessionEndUnblocksWriterIO'
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/backpressure_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] copytree.go, deadline.go, egress_e2e_test.go, files.go, handle.go 等 26 个
//
// 主要符号
//
//	TestCleanupDeliveredAfterFullQueue(t *testing.T)
//	TestBlockedRelayWriteEndsOnClose(t *testing.T)
//	TestWatchCreationCancellation(t *testing.T)
//	type enteredWriteConn struct{…}
//	(c *enteredWriteConn) Write(p []byte) (int, error)
//	TestSessionEndUnblocksWriterIO(t *testing.T)

package box

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

func TestCleanupDeliveredAfterFullQueue(t *testing.T) {
	r, s := newRelayForTest(t, 1)
	for len(s.out) < cap(s.out) {
		s.out <- outFrame{msg: &proto.Msg{Kind: proto.KindPing}}
	}
	_ = r.Close()
	host, peer := net.Pipe()
	defer peer.Close()
	s.conn = host
	defer s.closeDone()
	done := make(chan struct{})
	go func() { defer close(done); s.writer(host) }()
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	for i := 0; i <= cap(s.out); i++ {
		var kind proto.Kind
		var payload []byte
		if err := proto.ReadFrame(peer, &kind, &payload); err != nil {
			t.Fatal(err)
		}
		want := proto.KindPing
		if i == cap(s.out) {
			want = proto.KindRelayClose
		}
		if kind != want {
			t.Fatalf("frame %d: %d, want %d", i, kind, want)
		}
	}
	s.closeDone()
	awaitClosed(t, done)
}

func TestBlockedRelayWriteEndsOnClose(t *testing.T) {
	for _, half := range []bool{false, true} {
		r, s := newRelayForTest(t, 1)
		for len(s.out) < cap(s.out) {
			s.out <- outFrame{msg: &proto.Msg{Kind: proto.KindPing}}
		}
		result := make(chan error, 1)
		go func() {
			if half {
				result <- r.CloseWrite()
				return
			}
			_, err := r.Write([]byte("x"))
			result <- err
		}()
		_ = r.Close()
		select {
		case err := <-result:
			if err != nil && !errors.Is(err, net.ErrClosed) {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("blocked write survived close")
		}
	}
}

func TestWatchCreationCancellation(t *testing.T) {
	_, s := newRelayForTest(t, 100)
	for len(s.out) < cap(s.out) {
		s.out <- outFrame{msg: &proto.Msg{Kind: proto.KindPing}}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := s.Watch(ctx, "/workspace", WatchOptions{}); result <- err }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("watch creation ignored cancellation")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.watches) != 0 {
		t.Fatal("watch retained")
	}
}

type enteredWriteConn struct {
	net.Conn
	entered chan struct{}
}

func (c *enteredWriteConn) Write(p []byte) (int, error) {
	select {
	case c.entered <- struct{}{}:
	default:
	}
	return c.Conn.Write(p)
}

func TestSessionEndUnblocksWriterIO(t *testing.T) {
	_, s := newRelayForTest(t, 1)
	host, peer := net.Pipe()
	defer peer.Close()
	conn := &enteredWriteConn{Conn: host, entered: make(chan struct{}, 1)}
	s.conn = conn
	done := make(chan struct{})
	go func() { defer close(done); s.writer(conn) }()
	s.out <- outFrame{msg: &proto.Msg{Kind: proto.KindPing}}
	select {
	case <-conn.entered:
	case <-time.After(time.Second):
		t.Fatal("writer never entered IO")
	}
	s.closeDone()
	awaitClosed(t, done)
}
