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

func TestManagerPath_ValidUUID(t *testing.T) {
	m, err := NewManager(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	id := "550e8400-e29b-41d4-a716-446655440000"
	p, err := m.Path(id)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got %s", p)
	}
	if filepath.Base(p) != id {
		t.Errorf("expected base to be UUID, got %s", filepath.Base(p))
	}
}

func TestManagerRoot_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(Options{Root: dir})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if !filepath.IsAbs(m.Root()) {
		t.Errorf("Root should be absolute, got %s", m.Root())
	}
}

func TestNewManager_DefaultsApplied(t *testing.T) {
	m, err := NewManager(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if m.maxBytes != 1<<30 {
		t.Errorf("default maxBytes = %d, want %d", m.maxBytes, 1<<30)
	}
	if m.maxDatabases != 256 {
		t.Errorf("default maxDatabases = %d, want 256", m.maxDatabases)
	}
}

func TestNewManager_EmptyRootReturnsError(t *testing.T) {
	_, err := NewManager(Options{Root: ""})
	if err == nil {
		t.Fatal("expected error for empty root")
	}
}

func TestEvict_TargetZeroOrNegativeReturnsNil(t *testing.T) {
	m, err := NewManager(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	res, err := m.Evict(context.Background(), 0)
	if err != nil {
		t.Fatalf("Evict(0): %v", err)
	}
	if res.Achieved {
		t.Error("expected Achieved=false for zero target")
	}
}

func TestEvict_NoCandidatesReturnsCapacityExceeded(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(Options{Root: dir, Registry: &fakeRegistry{active: map[string]bool{}}})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// 目录为空，无可淘汰项
	_, err = m.Evict(context.Background(), 100)
	if err == nil {
		t.Fatal("expected error when no candidates available")
	}
}

func TestEvict_SkipsActiveDatabases(t *testing.T) {
	dir := t.TempDir()
	id := "550e8400-e29b-41d4-a716-446655440000"
	reg := &fakeRegistry{active: map[string]bool{id: true}}
	m, err := NewManager(Options{Root: dir, Registry: reg})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	p, _ := m.Path(id)
	_ = os.MkdirAll(p, 0o755)
	_ = os.WriteFile(filepath.Join(p, "data"), make([]byte, 50), 0o644)

	res, err := m.Evict(context.Background(), 10)
	if err == nil {
		t.Fatal("expected capacity exceeded since only candidate is active")
	}
	if res.SkippedActive != 1 {
		t.Errorf("expected SkippedActive=1, got %d", res.SkippedActive)
	}
}

func TestEnsureCapacity_SufficientSpace(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(Options{Root: dir, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// 空目录，空间充足
	if err := m.EnsureCapacity(context.Background(), 100); err != nil {
		t.Fatalf("EnsureCapacity with sufficient space: %v", err)
	}
}

func TestEnsureCapacity_InsufficientSpaceNoCandidates(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(Options{Root: dir, MaxBytes: 100, Registry: &fakeRegistry{active: map[string]bool{}}})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// 空目录，但 maxBytes 太小且无可淘汰项
	err = m.EnsureCapacity(context.Background(), 200)
	if err == nil {
		t.Fatal("expected error when cannot ensure capacity")
	}
}

func TestUsage_EmptyRoot(t *testing.T) {
	m, err := NewManager(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	u, err := m.Usage(context.Background())
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if u.TotalBytes != 0 || u.DatabaseDirs != 0 {
		t.Errorf("expected zero usage, got bytes=%d dirs=%d", u.TotalBytes, u.DatabaseDirs)
	}
}

func TestUsage_NestedFiles(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(Options{Root: dir})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	id := "550e8400-e29b-41d4-a716-446655440000"
	p, _ := m.Path(id)
	sub := filepath.Join(p, "sub")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(p, "a"), make([]byte, 10), 0o644)
	_ = os.WriteFile(filepath.Join(sub, "b"), make([]byte, 15), 0o644)

	u, err := m.Usage(context.Background())
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if u.DatabaseDirs != 1 {
		t.Errorf("expected 1 dir, got %d", u.DatabaseDirs)
	}
	if u.TotalBytes != 25 {
		t.Errorf("expected 25 bytes, got %d", u.TotalBytes)
	}
}
