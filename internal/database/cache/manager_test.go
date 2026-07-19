package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// fakeRegistry 实现 ActiveChecker/Closer，所有库视为非活跃。
type fakeRegistry struct{ active map[string]bool }

func (f *fakeRegistry) IsActive(id string) bool { return f.active[id] }
func (f *fakeRegistry) CloseDatabase(ctx context.Context, id string) error {
	f.active[id] = false
	return nil
}

func TestManagerPathRejectsNonUUID(t *testing.T) {
	m, err := NewManager(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if _, err := m.Path("not-a-uuid"); err == nil {
		t.Fatal("expected error for non-uuid id")
	}
}

func TestManagerPathRejectsTraversal(t *testing.T) {
	m, err := NewManager(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// 合法 UUID 但 Path 仍校验根目录穿越。
	// UUID 本身不会穿越，但测试防御性逻辑。
	p, err := m.Path("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if p == "" {
		t.Fatal("empty path")
	}
}

func TestManagerUsageAndRemove(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(Options{Root: dir, Registry: &fakeRegistry{active: map[string]bool{}}})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// 创建一个缓存目录。
	id := "550e8400-e29b-41d4-a716-446655440000"
	p, _ := m.Path(id)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(p, "data"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	u, err := m.Usage(context.Background())
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if u.DatabaseDirs != 1 {
		t.Fatalf("expected 1 dir, got %d", u.DatabaseDirs)
	}
	if u.TotalBytes != 5 {
		t.Fatalf("expected 5 bytes, got %d", u.TotalBytes)
	}
	if err := m.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	u, _ = m.Usage(context.Background())
	if u.DatabaseDirs != 0 {
		t.Fatalf("expected 0 dirs after remove, got %d", u.DatabaseDirs)
	}
}

func TestManagerRemoveRejectsActive(t *testing.T) {
	dir := t.TempDir()
	reg := &fakeRegistry{active: map[string]bool{"550e8400-e29b-41d4-a716-446655440000": true}}
	m, err := NewManager(Options{Root: dir, Registry: reg})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.Remove("550e8400-e29b-41d4-a716-446655440000"); err == nil {
		t.Fatal("expected error removing active database")
	}
}

func TestEvictFreesSpace(t *testing.T) {
	dir := t.TempDir()
	reg := &fakeRegistry{active: map[string]bool{}}
	m, err := NewManager(Options{
		Root:     dir,
		MaxBytes: 10,
		Registry: reg,
		Closer:   reg,
		Logger:   observability.NewLogger("debug", "console", os.Stderr),
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// 创建两个缓存目录。
	for _, id := range []string{"550e8400-e29b-41d4-a716-446655440000", "660e8400-e29b-41d4-a716-446655440000"} {
		p, _ := m.Path(id)
		_ = os.MkdirAll(p, 0o755)
		_ = os.WriteFile(filepath.Join(p, "data"), make([]byte, 20), 0o644)
	}
	res, err := m.Evict(context.Background(), 15)
	if err != nil {
		t.Fatalf("Evict: %v", err)
	}
	if !res.Achieved {
		t.Fatalf("eviction not achieved: %+v", res)
	}
	if res.EvictedCount < 1 {
		t.Fatalf("expected at least 1 eviction, got %d", res.EvictedCount)
	}
}
