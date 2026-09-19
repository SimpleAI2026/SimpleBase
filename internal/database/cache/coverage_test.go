package cache

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/observability"
)

type recMetrics struct{ bytes, evicts atomic.Int64 }

func (m *recMetrics) ObserveCacheBytes(b int64) { m.bytes.Store(b) }
func (m *recMetrics) IncCacheEvictions()        { m.evicts.Add(1) }

type errCloser struct{ err error }

func (e errCloser) CloseDatabase(ctx context.Context, id string) error { return e.err }

func TestUsageMissingRootAndFiles(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	m, err := NewManager(Options{Root: missing, Metrics: &recMetrics{}})
	if err != nil {
		t.Fatal(err)
	}
	u, err := m.Usage(context.Background())
	if err != nil || u.TotalBytes != 0 {
		t.Fatalf("missing root: %+v %v", u, err)
	}

	dir := t.TempDir()
	met := &recMetrics{}
	log := observability.NewLogger("debug", "json", io.Discard)
	m, err = NewManager(Options{Root: dir, Logger: log, Metrics: met, MaxBytes: 100, MaxDatabases: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-a-dir"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := "550e8400-e29b-41d4-a716-446655440000"
	p, _ := m.Path(id)
	_ = os.MkdirAll(p, 0o755)
	_ = os.WriteFile(filepath.Join(p, "f"), []byte("hello"), 0o644)
	u, err = m.Usage(context.Background())
	if err != nil || u.DatabaseDirs != 1 || u.TotalBytes != 5 {
		t.Fatalf("%+v %v", u, err)
	}
	if met.bytes.Load() != 5 {
		t.Fatalf("metrics bytes %d", met.bytes.Load())
	}
}

func TestEvictBranches(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	m, _ := NewManager(Options{Root: missing})
	res, err := m.Evict(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Achieved {
		t.Fatal(res)
	}

	dir := t.TempDir()
	reg := &fakeRegistry{active: map[string]bool{}}
	log := observability.NewLogger("debug", "json", io.Discard)
	met := &recMetrics{}
	m, _ = NewManager(Options{Root: dir, Registry: reg, Closer: errCloser{err: errors.New("close")}, Logger: log, Metrics: met})
	_ = os.WriteFile(filepath.Join(dir, "file"), []byte("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "not-uuid"), 0o755)
	id := "550e8400-e29b-41d4-a716-446655440000"
	p, _ := m.Path(id)
	_ = os.MkdirAll(p, 0o755)
	_ = os.WriteFile(filepath.Join(p, "d"), make([]byte, 20), 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Evict(ctx, 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}

	res, err = m.Evict(context.Background(), 5)
	if err != nil || !res.Achieved || met.evicts.Load() < 1 {
		t.Fatalf("evict %+v %v evicts=%d", res, err, met.evicts.Load())
	}
}

func TestEnsureCapacityAndRemoveMissing(t *testing.T) {
	dir := t.TempDir()
	id := "660e8400-e29b-41d4-a716-446655440000"
	reg := &fakeRegistry{active: map[string]bool{}}
	m, _ := NewManager(Options{Root: dir, MaxBytes: 100, Registry: reg, Closer: reg})
	if err := m.EnsureCapacity(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	p, _ := m.Path(id)
	_ = os.MkdirAll(p, 0o755)
	_ = os.WriteFile(filepath.Join(p, "d"), make([]byte, 80), 0o644)
	if err := m.EnsureCapacity(context.Background(), 50); err != nil {
		t.Fatalf("should evict: %v", err)
	}
	if err := m.Remove("not-a-uuid"); err == nil {
		t.Fatal("invalid id")
	}
	if err := removeAll(filepath.Join(dir, "does-not-exist")); err != nil {
		t.Fatal(err)
	}
}

func TestNewManagerDefaultsAndRoot(t *testing.T) {
	m, err := NewManager(Options{Root: "rel-cache-" + time.Now().Format("150405.000"), MaxBytes: -1, MaxDatabases: -1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(m.Root()) })
	if m.maxBytes != 1<<30 || m.maxDatabases != 256 {
		t.Fatal(m.maxBytes, m.maxDatabases)
	}
	if !filepath.IsAbs(m.Root()) {
		t.Fatal(m.Root())
	}
}
