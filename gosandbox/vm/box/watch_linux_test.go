//go:build linux

// 文件说明：watch_linux_test.go — gosandbox/vm/box 的单元测试（TestFileWatchStableChannelAndBlockedClose、TestFileWatchSourceEnd 等）
//
// 用法
//
//	go test ./gosandbox/vm/box -run 'TestFileWatchStableChannelAndBlockedClose|TestFileWatchSourceEnd'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/watch_linux_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] capabilities.go, contracts_linux_test.go, diskuse_linux.go, driver_linux.go, driver_other.go 等 15 个
//
// 主要符号
//
//	type testWatchSource struct{…}
//	(w *testWatchSource) Events() <-chan backend.FileEvent
//	(w *testWatchSource) Done() <-chan struct{}
//	(w *testWatchSource) Overflowed() bool
//	(w *testWatchSource) Close() error
//	TestFileWatchStableChannelAndBlockedClose(t *testing.T)
//	TestFileWatchSourceEnd(t *testing.T)

package box

import (
	"sync"
	"testing"
	"time"

	backend "github.com/linkxzhou/SimpleBase/gosandbox/box"
	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

type testWatchSource struct {
	events chan backend.FileEvent
	done   chan struct{}
	once   sync.Once
}

func (w *testWatchSource) Events() <-chan backend.FileEvent { return w.events }
func (w *testWatchSource) Done() <-chan struct{}            { return w.done }
func (w *testWatchSource) Overflowed() bool                 { return false }
func (w *testWatchSource) Close() error                     { w.once.Do(func() { close(w.done) }); return nil }

func TestFileWatchStableChannelAndBlockedClose(t *testing.T) {
	w := &testWatchSource{events: make(chan backend.FileEvent), done: make(chan struct{})}
	f := newFileWatch(w)
	defer f.Close()
	events := f.Events()
	for i := 0; i < 10; i++ {
		if f.Events() != events {
			t.Fatal("unstable event channel")
		}
	}
	for i := 0; i < 65; i++ {
		select {
		case w.events <- backend.FileEvent{Path: "file", Kind: proto.FileRegular}:
		case <-time.After(time.Second):
			t.Fatal("forwarder did not consume expected input")
		}
	}
	// The 65th receive has completed while the output still contains 64 unread events.
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.done:
	case <-time.After(time.Second):
		t.Fatal("blocked forwarder leaked")
	}
	for ev := range f.Events() {
		if ev.Path != "file" || ev.Kind != "file" {
			t.Fatalf("mapping = %+v", ev)
		}
	}
}

func TestFileWatchSourceEnd(t *testing.T) {
	w := &testWatchSource{events: make(chan backend.FileEvent), done: make(chan struct{})}
	f := newFileWatch(w)
	close(w.events)
	select {
	case <-f.done:
	case <-time.After(time.Second):
		t.Fatal("source end ignored")
	}
}
