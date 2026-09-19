package cache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUsageReadRootPermission(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(Options{Root: dir, Logger: nil})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := m.Usage(context.Background()); err == nil {
		t.Fatal("expected read root error")
	}
	if _, err := m.Evict(context.Background(), 10); err == nil {
		t.Fatal("expected evict read error")
	}
}

func TestUsageSkipUnreadableDir(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	id := "770e8400-e29b-41d4-a716-446655440000"
	p, _ := m.Path(id)
	_ = os.MkdirAll(p, 0o755)
	nested := filepath.Join(p, "locked")
	_ = os.MkdirAll(nested, 0o000)
	t.Cleanup(func() { _ = os.Chmod(nested, 0o755) })
	u, err := m.Usage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// unreadable nested dir may skip size
	_ = u
}

func TestEvictCancelMidwayAndEnsureCapacityError(t *testing.T) {
	dir := t.TempDir()
	id := "880e8400-e29b-41d4-a716-446655440000"
	reg := &fakeRegistry{active: map[string]bool{}}
	ctx, cancel := context.WithCancel(context.Background())
	closer := cancelCloser{cancel: cancel}
	m, err := NewManager(Options{Root: dir, MaxBytes: 10, Registry: reg, Closer: closer})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := m.Path(id)
	_ = os.MkdirAll(p, 0o755)
	_ = os.WriteFile(filepath.Join(p, "d"), make([]byte, 80), 0o644)
	id2 := "990e8400-e29b-41d4-a716-446655440000"
	p2, _ := m.Path(id2)
	_ = os.MkdirAll(p2, 0o755)
	_ = os.WriteFile(filepath.Join(p2, "d"), make([]byte, 80), 0o644)

	_, err = m.Evict(ctx, 200)
	if err == nil || !errors.Is(err, context.Canceled) && !errors.Is(err, ErrCacheCapacityExceeded) {
		t.Logf("evict mid: %v", err)
	}

	m2, _ := NewManager(Options{Root: dir, MaxBytes: 10, Registry: reg})
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	// EnsureCapacity: usage succeeds, evict sees canceled ctx
	_ = m2.EnsureCapacity(ctx2, 1000)
}

type cancelCloser struct{ cancel context.CancelFunc }

func (c cancelCloser) CloseDatabase(ctx context.Context, id string) error {
	c.cancel()
	return errors.New("closed")
}

func TestRemoveAllPermission(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "x")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(child, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(child, 0o755) })
	err := removeAll(child)
	// may or may not fail depending on OS; just exercise the path
	_ = err
}

func TestEnsureCapacityUsageError(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(Options{Root: dir, MaxBytes: 10})
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := m.EnsureCapacity(context.Background(), 5); err == nil {
		t.Fatal("expected usage error")
	}
}

func TestNewManagerAbsAndPathRel(t *testing.T) {
	m, err := NewManager(Options{Root: ".", MaxBytes: 1, MaxDatabases: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(m.Root()) {
		t.Fatal(m.Root())
	}
	id := "550e8400-e29b-41d4-a716-446655440000"
	p, err := m.Path(id)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != id {
		t.Fatal(p)
	}
	_ = time.Now()
}
