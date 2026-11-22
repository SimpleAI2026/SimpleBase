package testutil

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRecordingLogger(t *testing.T) {
	logger := &RecordingLogger{}
	logger.Debug("d")
	logger.Info("registry: opened", zap.String("k", "v"))
	logger.Warn("w")
	logger.Error("registry: failed")
	if len(logger.Entries()) != 4 {
		t.Fatalf("entries = %d", len(logger.Entries()))
	}
	if logger.Count("", "registry:") != 2 || logger.Count("error", "registry:") != 1 || logger.Count("warn", "registry:") != 0 {
		t.Fatalf("count mismatch: %+v", logger.Entries())
	}
}

func TestRecordingObserver(t *testing.T) {
	observer := &RecordingObserver{}
	observer.OpenHandleDelta(1)
	observer.OpenHandleDelta(1)
	observer.OpenHandleDelta(-1)
	observer.CacheBytes(42)
	observer.CacheEvicted()
	observer.CatalogLocalAhead()
	observer.CatalogSync("ok", time.Second, false)
	observer.CatalogSync("error", time.Millisecond, true)
	observer.CatalogSyncLag("db", 3)
	observer.CatalogSyncConflict("db")
	observer.CatalogSize("db", 99)
	if observer.Handles.Load() != 1 || observer.Bytes.Load() != 42 || observer.Evictions.Load() != 1 || observer.LocalAhead.Load() != 1 {
		t.Fatalf("counters: %+v", observer)
	}
	if observer.SyncOK.Load() != 1 || observer.SyncFailed.Load() != 1 || observer.LastOutcome() != "error" || observer.Conflicts.Load() != 1 {
		t.Fatalf("sync counters: ok=%d failed=%d outcome=%s", observer.SyncOK.Load(), observer.SyncFailed.Load(), observer.LastOutcome())
	}
	if value, ok := observer.Lag("db"); !ok || value != 3 {
		t.Fatalf("lag = %d %v", value, ok)
	}
	if value, ok := observer.CatalogBytes("db"); !ok || value != 99 {
		t.Fatalf("catalog bytes = %d %v", value, ok)
	}
	if _, ok := observer.Lag("none"); ok {
		t.Fatal("unknown db must report not found")
	}
}
