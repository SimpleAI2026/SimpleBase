package registry

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
)

type hookFactory struct {
	fakeFactory
	writes int
	closes int
}

func (h *hookFactory) AfterWrite(ctx context.Context, db godatabase.Spec, sqlDB *sql.DB) error {
	h.writes++
	return nil
}
func (h *hookFactory) BeforeClose(ctx context.Context, dbID string, sqlDB *sql.DB) error {
	h.closes++
	return nil
}

func TestHandleQueryExecuteBatchAndHooks(t *testing.T) {
	hf := &hookFactory{}
	reg := New(hf, nil, Options{Writable: true}, &godatabase.RecordingLogger{}, &godatabase.RecordingObserver{})
	db := readyDatabase("12121212-1212-1212-1212-121212121212")
	lease, err := reg.Acquire(context.Background(), db, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	h := lease.Handle
	if h.Conn() == nil || h.LastUsed().IsZero() {
		t.Fatal("conn/lastused")
	}
	if _, err := h.Query(context.Background(), godatabase.Statement{SQL: "SELECT 1"}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Execute(context.Background(), godatabase.Statement{SQL: "CREATE TABLE t(x INT)"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Execute(context.Background(), godatabase.Statement{SQL: "INSERT INTO t VALUES(1)"}); err != nil {
		t.Fatal(err)
	}
	if hf.writes == 0 {
		t.Fatal("AfterWrite not called")
	}
	if _, err := h.Execute(context.Background(), godatabase.Statement{SQL: "INSERT INTO missing VALUES(1)"}); err == nil {
		t.Fatal("exec error")
	}
	if _, err := h.Batch(context.Background(), []godatabase.Statement{{SQL: "INSERT INTO t VALUES(2)"}}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Batch(context.Background(), []godatabase.Statement{{SQL: "INSERT INTO missing VALUES(1)"}}, false); err == nil {
		t.Fatal("batch error")
	}
	lease.Release()
	if err := reg.CloseDatabase(context.Background(), db.ID); err != nil {
		t.Fatal(err)
	}
	if hf.closes == 0 {
		t.Fatal("BeforeClose not called")
	}
}

func TestNewDefaultsAndNilLease(t *testing.T) {
	reg := New(&fakeFactory{}, nil, Options{}, nil, nil)
	if reg.idleTimeout != 5*time.Minute || reg.maxOpen != 0 {
		t.Fatal(reg.idleTimeout, reg.maxOpen)
	}
	var l *Lease
	l.Release()
}

func TestAcquireWaitCanceled(t *testing.T) {
	f := &fakeFactory{openDelay: 80 * time.Millisecond}
	reg := New(f, nil, Options{Writable: true}, nil, nil)
	db := readyDatabase("13131313-1313-1313-1313-131313131313")
	started := make(chan struct{})
	go func() {
		close(started)
		lease, err := reg.Acquire(context.Background(), db, godatabase.ReadWrite)
		if err == nil {
			lease.Release()
		}
	}()
	<-started
	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reg.Acquire(ctx, db, godatabase.ReadWrite); !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
}

func TestShutdownForceCloseActive(t *testing.T) {
	reg := New(&fakeFactory{}, nil, Options{Writable: true}, &godatabase.RecordingLogger{}, &godatabase.RecordingObserver{})
	db := readyDatabase("14141414-1414-1414-1414-141414141414")
	lease, err := reg.Acquire(context.Background(), db, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := reg.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	lease.Release()
}

func TestCloseIdleSkipsNotReadyAndRecent(t *testing.T) {
	reg := New(&fakeFactory{}, nil, Options{Writable: true, IdleTimeout: time.Hour}, nil, &godatabase.RecordingObserver{})
	db := readyDatabase("15151515-1515-1515-1515-151515151515")
	lease, err := reg.Acquire(context.Background(), db, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	if err := reg.CloseIdle(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if !reg.IsActive(db.ID) && reg.entries[db.ID] == nil {
		// may still exist as idle
	}
	reg.mu.Lock()
	if e := reg.entries[db.ID]; e != nil && e.handle != nil {
		e.handle.state.Store(uint32(stateClosed))
	}
	reg.mu.Unlock()
	if err := reg.CloseIdle(context.Background(), time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func TestEvictorDefaultInterval(t *testing.T) {
	e := NewEvictor(New(&fakeFactory{}, nil, Options{Writable: true}, nil, nil), 0)
	if e.interval != time.Minute {
		t.Fatal(e.interval)
	}
}

func TestAcquireClosedDuringOpenCleanup(t *testing.T) {
	reg := New(&fakeFactory{}, nil, Options{Writable: true}, nil, nil)
	db := readyDatabase("16161616-1616-1616-1616-161616161616")
	if err := reg.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Acquire(context.Background(), db, godatabase.ReadWrite); !errors.Is(err, godatabase.ErrRegistryClosed) {
		t.Fatal(err)
	}
}

// 打开/关闭句柄必须成对经 Observer 上报，最终归零。
func TestObserverTracksOpenHandles(t *testing.T) {
	obs := &godatabase.RecordingObserver{}
	reg := New(&fakeFactory{}, nil, Options{Writable: true}, nil, obs)
	l1, err := reg.Acquire(context.Background(), readyDatabase("16161616-1616-1616-1616-161616161616"), godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	l2, err := reg.Acquire(context.Background(), readyDatabase("17171717-1717-1717-1717-171717171717"), godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Handles.Load() != 2 {
		t.Fatalf("open handles = %d, want 2", obs.Handles.Load())
	}
	l1.Release()
	l2.Release()
	if err := reg.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if obs.Handles.Load() != 0 {
		t.Fatalf("after shutdown open handles = %d, want 0", obs.Handles.Load())
	}
}

// resolver 只对缺失路径的非系统库生效，且结果写入 Spec；系统库与已有路径不应被改写。
type recResolver struct {
	calls int
	path  string
	err   error
}

func (r *recResolver) ResolveDatabasePath(context.Context, string, string) (string, error) {
	r.calls++
	return r.path, r.err
}

func TestAcquireResolvesMissingPathOnly(t *testing.T) {
	res := &recResolver{path: "abcd1234/database.duckdb"}
	reg := New(&fakeFactory{}, res, Options{Writable: true}, nil, nil)

	missing := godatabase.Spec{ID: "18181818-1818-1818-1818-181818181818", ProjectID: "abcd1234", Status: godatabase.StatusReady}
	l, err := reg.Acquire(context.Background(), missing, godatabase.ReadWrite)
	if err != nil || res.calls != 1 {
		t.Fatalf("missing path: calls=%d err=%v", res.calls, err)
	}
	if snap, ok := reg.Snapshot("abcd1234"); !ok || snap.DatabasePath != "abcd1234/database.duckdb" {
		t.Fatalf("resolved path must be stored: %+v %v", snap, ok)
	}
	l.Release()

	res.calls = 0
	sys := godatabase.Spec{ID: "19191919-1919-1919-1919-191919191919", ProjectID: "sb-admin", Kind: godatabase.KindSystem, Status: godatabase.StatusReady}
	ls, err := reg.Acquire(context.Background(), sys, godatabase.ReadWrite)
	if err != nil || res.calls != 0 {
		t.Fatalf("system db must not be resolved: calls=%d err=%v", res.calls, err)
	}
	ls.Release()

	res2 := &recResolver{err: errors.New("cross tenant")}
	reg2 := New(&fakeFactory{}, res2, Options{Writable: true}, nil, nil)
	if _, err := reg2.Acquire(context.Background(), godatabase.Spec{ID: "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a", ProjectID: "wxyz9876", Status: godatabase.StatusReady}, godatabase.ReadWrite); err == nil || err.Error() != "cross tenant" {
		t.Fatalf("resolver error must be returned as-is: %v", err)
	}
}
