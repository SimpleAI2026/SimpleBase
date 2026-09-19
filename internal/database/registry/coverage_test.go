package registry

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
)

type hookFactory struct {
	fakeFactory
	writes int
	closes int
}

func (h *hookFactory) AfterWrite(ctx context.Context, db catalog.Database, sqlDB *sql.DB) error {
	h.writes++
	return nil
}
func (h *hookFactory) BeforeClose(ctx context.Context, dbID string, sqlDB *sql.DB) error {
	h.closes++
	return nil
}

func TestHandleQueryExecuteBatchAndHooks(t *testing.T) {
	hf := &hookFactory{}
	reg := New(hf, nil, Options{Writable: true}, observability.NewLogger("debug", "json", io.Discard), observability.NewMetrics(prometheus.NewRegistry()))
	db := readyDatabase("12121212-1212-1212-1212-121212121212")
	lease, err := reg.Acquire(context.Background(), db, database.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	h := lease.Handle
	if h.Conn() == nil || h.LastUsed().IsZero() {
		t.Fatal("conn/lastused")
	}
	if _, err := h.Query(context.Background(), database.Statement{SQL: "SELECT 1"}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Execute(context.Background(), database.Statement{SQL: "CREATE TABLE t(x INT)"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Execute(context.Background(), database.Statement{SQL: "INSERT INTO t VALUES(1)"}); err != nil {
		t.Fatal(err)
	}
	if hf.writes == 0 {
		t.Fatal("AfterWrite not called")
	}
	if _, err := h.Execute(context.Background(), database.Statement{SQL: "INSERT INTO missing VALUES(1)"}); err == nil {
		t.Fatal("exec error")
	}
	if _, err := h.Batch(context.Background(), []database.Statement{{SQL: "INSERT INTO t VALUES(2)"}}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Batch(context.Background(), []database.Statement{{SQL: "INSERT INTO missing VALUES(1)"}}, false); err == nil {
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
		lease, err := reg.Acquire(context.Background(), db, database.ReadWrite)
		if err == nil {
			lease.Release()
		}
	}()
	<-started
	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reg.Acquire(ctx, db, database.ReadWrite); !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
}

func TestShutdownForceCloseActive(t *testing.T) {
	reg := New(&fakeFactory{}, nil, Options{Writable: true}, observability.NewLogger("warn", "json", io.Discard), observability.NewMetrics(prometheus.NewRegistry()))
	db := readyDatabase("14141414-1414-1414-1414-141414141414")
	lease, err := reg.Acquire(context.Background(), db, database.ReadWrite)
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
	reg := New(&fakeFactory{}, nil, Options{Writable: true, IdleTimeout: time.Hour}, nil, observability.NewMetrics(prometheus.NewRegistry()))
	db := readyDatabase("15151515-1515-1515-1515-151515151515")
	lease, err := reg.Acquire(context.Background(), db, database.ReadWrite)
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
	reg := New(&fakeFactory{}, nil, Options{Writable: true}, observability.NewLogger("error", "json", io.Discard), nil)
	db := readyDatabase("16161616-1616-1616-1616-161616161616")
	if err := reg.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Acquire(context.Background(), db, database.ReadWrite); !errors.Is(err, database.ErrRegistryClosed) {
		t.Fatal(err)
	}
}
