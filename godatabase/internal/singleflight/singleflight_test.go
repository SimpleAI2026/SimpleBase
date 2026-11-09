package singleflight

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitInFlight(t *testing.T, g *Group[int], key string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !g.InFlight(key) {
		if time.Now().After(deadline) {
			t.Fatal("leader never became in-flight")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDoSharesResultAcrossWaiters(t *testing.T) {
	g := &Group[int]{}
	var execs int32
	release := make(chan struct{})

	var wg sync.WaitGroup
	vals := make([]int, 8)
	errs := make([]error, 8)
	wg.Add(1)
	go func() {
		defer wg.Done()
		vals[0], errs[0] = g.Do("k", func() (int, error) {
			atomic.AddInt32(&execs, 1)
			<-release
			return 42, nil
		})
	}()
	waitInFlight(t, g, "k")
	for i := 1; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			vals[i], errs[i] = g.Do("k", func() (int, error) {
				atomic.AddInt32(&execs, 1)
				return -1, nil
			})
		}(i)
	}
	time.Sleep(30 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&execs); got != 1 {
		t.Fatalf("execs = %d, want 1", got)
	}
	for i := range vals {
		if vals[i] != 42 || errs[i] != nil {
			t.Fatalf("[%d] = (%d, %v), want (42, nil)", i, vals[i], errs[i])
		}
	}
	if g.InFlight("k") {
		t.Fatal("key must be cleared after completion")
	}
}

func TestDoPropagatesError(t *testing.T) {
	g := &Group[string]{}
	sentinel := errors.New("boom")
	_, err := g.Do("k", func() (string, error) { return "", sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestTryDoBusyThenIdle(t *testing.T) {
	g := &Group[int]{}
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = g.TryDo("k", func() (int, error) { <-release; return 1, nil })
	}()
	waitInFlight(t, g, "k")

	if _, err := g.TryDo("k", func() (int, error) { return 2, nil }); !errors.Is(err, ErrInFlight) {
		t.Fatalf("busy TryDo = %v, want ErrInFlight", err)
	}
	if _, err := g.TryDo("other", func() (int, error) { return 3, nil }); err != nil {
		t.Fatalf("distinct key = %v", err)
	}
	close(release)
	<-done
	if v, err := g.TryDo("k", func() (int, error) { return 4, nil }); err != nil || v != 4 {
		t.Fatalf("idle TryDo = (%d, %v)", v, err)
	}
}

// leader panic 时等待者必须被唤醒（不得永久阻塞），key 必须被清理。
func TestLeaderPanicWakesWaiters(t *testing.T) {
	g := &Group[int]{}
	release := make(chan struct{})
	leaderDone := make(chan any, 1)
	go func() {
		defer func() { leaderDone <- recover() }()
		_, _ = g.Do("k", func() (int, error) {
			<-release
			panic("kaboom")
		})
	}()
	waitInFlight(t, g, "k")

	waiterErr := make(chan error, 1)
	go func() {
		_, err := g.Do("k", func() (int, error) { return 0, nil })
		waiterErr <- err
	}()
	time.Sleep(30 * time.Millisecond)
	close(release)

	select {
	case err := <-waiterErr:
		if !errors.Is(err, errPanicked) {
			t.Fatalf("waiter err = %v, want errPanicked", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter blocked forever after leader panic")
	}
	if r := <-leaderDone; r != "kaboom" {
		t.Fatalf("leader recover = %v, want kaboom", r)
	}
	if g.InFlight("k") {
		t.Fatal("key must be cleared after panic")
	}
}

// Forget 之后新 leader 登记的记录，不得被旧 leader 结束时误删。
func TestForgetDoesNotLetOldLeaderEraseNewLeader(t *testing.T) {
	g := &Group[int]{}
	oldRelease := make(chan struct{})
	oldDone := make(chan struct{})
	go func() {
		defer close(oldDone)
		_, _ = g.Do("k", func() (int, error) { <-oldRelease; return 1, nil })
	}()
	waitInFlight(t, g, "k")
	g.Forget("k")

	newRelease := make(chan struct{})
	newDone := make(chan struct{})
	go func() {
		defer close(newDone)
		_, _ = g.Do("k", func() (int, error) { <-newRelease; return 2, nil })
	}()
	waitInFlight(t, g, "k")

	close(oldRelease)
	<-oldDone
	if !g.InFlight("k") {
		t.Fatal("old leader finishing erased the new leader's record")
	}
	close(newRelease)
	<-newDone
}

func TestNilGroupRunsDirectly(t *testing.T) {
	var g *Group[int]
	if v, err := g.Do("k", func() (int, error) { return 7, nil }); err != nil || v != 7 {
		t.Fatalf("Do = (%d, %v)", v, err)
	}
	if v, err := g.TryDo("k", func() (int, error) { return 8, nil }); err != nil || v != 8 {
		t.Fatalf("TryDo = (%d, %v)", v, err)
	}
	g.Forget("k")
	if g.InFlight("k") {
		t.Fatal("nil group has nothing in flight")
	}
}

func TestZeroValueUsable(t *testing.T) {
	var g Group[int]
	if v, err := g.Do("k", func() (int, error) { return 5, nil }); err != nil || v != 5 {
		t.Fatalf("Do = (%d, %v)", v, err)
	}
}
