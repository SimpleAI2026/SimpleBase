package ducklake

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
)

func TestIsMutatingSQL(t *testing.T) {
	if isMutatingSQL("  SELECT 1") || isMutatingSQL("EXPLAIN SELECT 1") || isMutatingSQL("WITH c AS (SELECT 1) SELECT * FROM c") {
		t.Fatal("reads must not take the write gate")
	}
	if !isMutatingSQL("INSERT INTO t VALUES (1)") || !isMutatingSQL("CALL ducklake_flush_inlined_data('lake')") {
		t.Fatal("writes must take the write gate")
	}
	if !isMutatingSQL("WITH c AS (SELECT 1) INSERT INTO t SELECT * FROM c") {
		t.Fatal("write CTE must take the write gate")
	}
	if isMutatingSQL("   ") {
		t.Fatal("empty")
	}
}

func TestRetryOnConflict(t *testing.T) {
	conflict := errors.New(`Transaction conflict - attempting to insert into table with index "32" - but another transaction has deleted from it`)

	t.Run("succeeds on retry", func(t *testing.T) {
		var n int
		err := RetryOnConflict(context.Background(), func() error {
			n++
			if n < 3 {
				return conflict
			}
			return nil
		})
		if err != nil || n != 3 {
			t.Fatalf("err=%v n=%d", err, n)
		}
	})

	t.Run("stops after limited attempts", func(t *testing.T) {
		var n int
		err := RetryOnConflict(context.Background(), func() error {
			n++
			return conflict
		})
		if !IsTransactionConflict(err) || n != conflictRetryAttempts+1 {
			t.Fatalf("err=%v n=%d", err, n)
		}
	})

	t.Run("does not retry other errors", func(t *testing.T) {
		var n int
		err := RetryOnConflict(context.Background(), func() error {
			n++
			return errors.New("syntax error")
		})
		if err == nil || n != 1 || IsTransactionConflict(err) {
			t.Fatalf("err=%v n=%d", err, n)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		called := false
		err := RetryOnConflict(ctx, func() error {
			called = true
			return nil
		})
		if !errors.Is(err, context.Canceled) || called {
			t.Fatalf("err=%v called=%v", err, called)
		}
	})

	t.Run("cancel during backoff", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var n int
		done := make(chan error, 1)
		go func() {
			done <- RetryOnConflict(ctx, func() error {
				n++
				cancel()
				return conflict
			})
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) || n != 1 {
				t.Fatalf("err=%v n=%d", err, n)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("retry did not observe cancellation")
		}
	})
}

func TestPausePeriodicDefersSchedule(t *testing.T) {
	opts := DefaultOptions().CatalogSync
	opts.Mode = "interval"
	opts.Interval = time.Hour
	opts.MaxLag = time.Hour
	cs := NewCatalogSyncer(nil, RemoteStorage{Enabled: true}, t.TempDir(), opts, "", nil, nil)
	cs.MarkDirty("db1", 2)
	cs.mu.Lock()
	if cs.timers["db1"] == nil || cs.dirty["db1"] != 2 {
		t.Fatal("interval timer should arm on first dirty")
	}
	cs.mu.Unlock()

	cs.PausePeriodic("db1")
	cs.MarkDirty("db1", 4)
	cs.mu.Lock()
	if cs.timers["db1"] != nil {
		t.Fatal("pause must cancel and suppress the timer")
	}
	if cs.dirty["db1"] != 4 {
		t.Fatalf("dirty=%d", cs.dirty["db1"])
	}
	cs.mu.Unlock()

	cs.ResumePeriodic("db1")
	cs.mu.Lock()
	armed := cs.timers["db1"] != nil
	cs.mu.Unlock()
	if !armed {
		t.Fatal("resume should schedule the accumulated dirty snapshot")
	}
	if err := cs.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWriteGateSerializesFlushAndWrites(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.ExtensionDir = filepath.Join(dir, "extensions")
	f := &Factory{CacheDir: dir, Options: opts}
	meta := catalog.Database{ID: uuid.NewString(), Kind: catalog.DatabaseKindSystem}
	db, err := f.Open(context.Background(), meta, database.ReadWrite)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE gate_t (x INTEGER)`); err != nil {
		t.Fatal(err)
	}

	held, release := holdWriteGate(ctx, db)
	blocked := make(chan error, 1)
	go func() {
		_, err := db.ExecContext(context.Background(), `INSERT INTO gate_t VALUES (1)`)
		blocked <- err
	}()
	select {
	case err := <-blocked:
		release()
		t.Fatalf("insert did not wait for the write gate: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	// 同一 goroutine 持锁后再写，不能自己死锁（sync flush 走这条路径）。
	if _, err := db.ExecContext(held, `INSERT INTO gate_t VALUES (2)`); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	select {
	case err := <-blocked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("insert still blocked after releasing the write gate")
	}

	// 显式事务里的语句必须复用连接上已持有的写锁，不能二次加锁死锁。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO gate_t VALUES (7)`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 15; j++ {
				if _, err := db.ExecContext(ctx, `INSERT INTO gate_t VALUES (?)`, n*100+j); err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}
	for k := 0; k < 6; k++ {
		if _, err := db.ExecContext(ctx, `CALL ducklake_flush_inlined_data(?)`, DefaultLakeAlias); err != nil {
			t.Fatalf("flush: %v", err)
		}
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent write: %v", err)
	}
	g := gateFor(db)
	if g == nil {
		t.Fatal("missing write gate")
	}
	if got := g.maxHolders.Load(); got != 1 {
		t.Fatalf("overlapping write holders = %d, want 1", got)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gate_t`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	// 持锁插入 1 行 + 被挡住后完成的 1 行 + 显式事务 1 行 + 4*15 并发插入。
	if n != 4*15+3 {
		t.Fatalf("rows=%d", n)
	}
}
