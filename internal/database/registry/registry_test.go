package registry

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"

	_ "github.com/uglyer/go-sqlite3"
)

// fakeFactory 记录 Open 调用次数，每次返回一个新的内存 sqlite *sql.DB。
// 用于验证并发 Acquire 只触发一次真正的打开。
type fakeFactory struct {
	openCount atomic.Int64
	failNext  atomic.Bool
	openDelay time.Duration
}

func (f *fakeFactory) Open(ctx context.Context, db catalog.Database, mode database.AccessMode) (*sql.DB, error) {
	f.openCount.Add(1)
	if f.openDelay > 0 {
		time.Sleep(f.openDelay)
	}
	if f.failNext.CompareAndSwap(true, false) {
		return nil, errors.New("simulated open failure")
	}
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func readyDatabase(id string) catalog.Database {
	return catalog.Database{ID: id, Status: catalog.DatabaseReady}
}

func TestAcquire_ConcurrentOnlyOpensOnce(t *testing.T) {
	factory := &fakeFactory{openDelay: 20 * time.Millisecond}
	reg := New(factory, nil, Options{Writable: true}, nil, nil)
	db := readyDatabase("11111111-1111-1111-1111-111111111111")

	const n = 100
	var wg sync.WaitGroup
	leases := make([]*Lease, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			lease, err := reg.Acquire(context.Background(), db, database.ReadWrite)
			leases[idx] = lease
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("acquire %d failed: %v", i, err)
		}
	}
	if got := factory.openCount.Load(); got != 1 {
		t.Fatalf("expected factory.Open called once, got %d", got)
	}
	for _, l := range leases {
		l.Release()
	}
}

func TestAcquire_OpenFailure_AllWaitersSameError(t *testing.T) {
	factory := &fakeFactory{}
	factory.failNext.Store(true)
	reg := New(factory, nil, Options{Writable: true}, nil, nil)
	db := readyDatabase("22222222-2222-2222-2222-222222222222")

	_, err := reg.Acquire(context.Background(), db, database.ReadWrite)
	if err == nil {
		t.Fatal("expected error from failing factory")
	}

	// 下一次 Acquire 应可以重试并成功（entry 已被清理）。
	lease, err := reg.Acquire(context.Background(), db, database.ReadWrite)
	if err != nil {
		t.Fatalf("retry after failure should succeed: %v", err)
	}
	lease.Release()
}

func TestAcquire_ReadWriteRejectedWhenNotWritable(t *testing.T) {
	factory := &fakeFactory{}
	reg := New(factory, nil, Options{Writable: false}, nil, nil)
	db := readyDatabase("33333333-3333-3333-3333-333333333333")

	_, err := reg.Acquire(context.Background(), db, database.ReadWrite)
	if !errors.Is(err, database.ErrWriterUnavailable) {
		t.Fatalf("expected ErrWriterUnavailable, got %v", err)
	}
}

func TestAcquire_RejectsDeletingDatabase(t *testing.T) {
	factory := &fakeFactory{}
	reg := New(factory, nil, Options{Writable: true}, nil, nil)
	db := catalog.Database{ID: "44444444-4444-4444-4444-444444444444", Status: catalog.DatabaseDeleting}

	_, err := reg.Acquire(context.Background(), db, database.ReadOnly)
	if !errors.Is(err, database.ErrDatabaseDeleting) {
		t.Fatalf("expected ErrDatabaseDeleting, got %v", err)
	}
}

func TestLease_ReleaseIdempotent(t *testing.T) {
	factory := &fakeFactory{}
	reg := New(factory, nil, Options{Writable: true}, nil, nil)
	db := readyDatabase("55555555-5555-5555-5555-555555555555")

	lease, err := reg.Acquire(context.Background(), db, database.ReadWrite)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if got := lease.Handle.ActiveCount(); got != 1 {
		t.Fatalf("expected active=1, got %d", got)
	}
	lease.Release()
	lease.Release() // 幂等
	if got := lease.Handle.ActiveCount(); got != 0 {
		t.Fatalf("expected active=0 after release, got %d", got)
	}
}

func TestCloseIdle_DoesNotCloseActiveHandle(t *testing.T) {
	factory := &fakeFactory{}
	reg := New(factory, nil, Options{Writable: true, IdleTimeout: time.Millisecond}, nil, nil)
	db := readyDatabase("66666666-6666-6666-6666-666666666666")

	lease, err := reg.Acquire(context.Background(), db, database.ReadWrite)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := reg.CloseIdle(context.Background(), time.Now()); err != nil {
		t.Fatalf("CloseIdle: %v", err)
	}

	// handle 仍应可用（active>0，未被关闭）。
	if _, err := lease.Handle.Query(context.Background(), database.Statement{SQL: "SELECT 1"}, 0); err != nil {
		t.Fatalf("query after CloseIdle on active handle: %v", err)
	}
	lease.Release()
}

func TestCloseIdle_ClosesIdleHandle(t *testing.T) {
	reg := New(&fakeFactory{}, nil, Options{Writable: true}, nil, nil)
	reg.idleTimeout = time.Millisecond
	db := readyDatabase("77777777-7777-7777-7777-777777777777")

	lease, err := reg.Acquire(context.Background(), db, database.ReadWrite)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	lease.Release()
	time.Sleep(5 * time.Millisecond)

	if err := reg.CloseIdle(context.Background(), time.Now()); err != nil {
		t.Fatalf("CloseIdle: %v", err)
	}

	reg.mu.Lock()
	_, exists := reg.entries[db.ID]
	reg.mu.Unlock()
	if exists {
		t.Fatalf("expected idle handle to be removed from registry")
	}
}

func TestShutdown_ClosesAllHandles(t *testing.T) {
	reg := New(&fakeFactory{}, nil, Options{Writable: true}, nil, nil)
	db1 := readyDatabase("88888888-8888-8888-8888-888888888888")
	db2 := readyDatabase("99999999-9999-9999-9999-999999999999")

	l1, err := reg.Acquire(context.Background(), db1, database.ReadWrite)
	if err != nil {
		t.Fatalf("acquire1: %v", err)
	}
	l2, err := reg.Acquire(context.Background(), db2, database.ReadWrite)
	if err != nil {
		t.Fatalf("acquire2: %v", err)
	}
	l1.Release()
	l2.Release()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := reg.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if _, err := reg.Acquire(context.Background(), db1, database.ReadWrite); !errors.Is(err, database.ErrRegistryClosed) {
		t.Fatalf("expected ErrRegistryClosed after shutdown, got %v", err)
	}
}
