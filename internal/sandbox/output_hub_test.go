package sandbox

import (
	"io"
	"strings"
	"testing"
	"time"
)

func readAll(t *testing.T, s *Subscription, want string) string {
	t.Helper()
	var sb strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 64)
		for !strings.Contains(sb.String(), want) {
			n, err := s.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for %q, got %q", want, sb.String())
	}
	return sb.String()
}

func TestHubReplaysHistoryToLateSubscriber(t *testing.T) {
	pr, pw := io.Pipe()
	h := newOutputHub(pr)
	early := h.Subscribe(true)
	pw.Write([]byte("before "))
	readAll(t, early, "before")
	// 晚到的订阅者要看到断开期间产生的输出。
	late := h.Subscribe(true)
	pw.Write([]byte("after"))
	got := readAll(t, late, "after")
	if !strings.Contains(got, "before after") {
		t.Fatalf("late subscriber lost history: %q", got)
	}
	// 只订阅新数据的订阅者看不到历史。
	fresh := h.Subscribe(false)
	pw.Write([]byte("!"))
	if got := readAll(t, fresh, "!"); strings.Contains(got, "before") {
		t.Fatalf("fresh subscriber saw history: %q", got)
	}
	early.Close()
	late.Close()
	fresh.Close()
	pw.Close()
}

// 多个订阅者互不抢数据——这正是引入 hub 的原因。
func TestHubSubscribersDoNotStealFromEachOther(t *testing.T) {
	pr, pw := io.Pipe()
	h := newOutputHub(pr)
	a, b := h.Subscribe(true), h.Subscribe(true)
	pw.Write([]byte("one two three"))
	if !strings.Contains(readAll(t, a, "three"), "one two three") || !strings.Contains(readAll(t, b, "three"), "one two three") {
		t.Fatal("both subscribers must receive the full stream")
	}
	a.Close()
	b.Close()
	pw.Close()
}

func TestHubEOFAndCloseUnblock(t *testing.T) {
	pr, pw := io.Pipe()
	h := newOutputHub(pr)
	s := h.Subscribe(true)
	errc := make(chan error, 1)
	go func() { _, err := s.Read(make([]byte, 8)); errc <- err }()
	time.Sleep(30 * time.Millisecond)
	s.Close() // 唤醒阻塞中的 Read
	select {
	case err := <-errc:
		if err != io.ErrClosedPipe {
			t.Fatalf("closed subscription: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not unblock Read")
	}
	s.Close() // 可重复调用
	s2 := h.Subscribe(true)
	pw.Write([]byte("tail"))
	pw.Close() // 进程输出结束
	readAll(t, s2, "tail")
	if _, err := s2.Read(make([]byte, 8)); err != io.EOF {
		t.Fatalf("after the output ends: %v", err)
	}
}

func TestHubBoundedReplayDropsOldest(t *testing.T) {
	pr, pw := io.Pipe()
	h := newOutputHub(pr)
	chunk := strings.Repeat("x", 100<<10)
	for i := 0; i < 4; i++ { // 400 KiB > 256 KiB 上限
		pw.Write([]byte(chunk))
	}
	pw.Write([]byte("END"))
	s := h.Subscribe(true)
	got := readAll(t, s, "END")
	if len(got) > replayCap+16 {
		t.Fatalf("replay buffer not bounded: %d bytes", len(got))
	}
	if !strings.HasSuffix(got, "END") {
		t.Fatalf("the newest output must survive trimming")
	}
	// 落后到数据被挤出的订阅者不会卡死，从现存最早的数据继续。
	slow := h.Subscribe(false)
	for i := 0; i < 4; i++ {
		pw.Write([]byte(chunk))
	}
	pw.Write([]byte("LAST"))
	readAll(t, slow, "LAST")
	s.Close()
	slow.Close()
	pw.Close()
}
