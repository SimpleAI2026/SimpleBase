// 文件说明：evict_b13_test.go — godatabase/cache 的单元测试（TestEvictProjectLayoutShortIDsNeverTargetsProjectsRoot、TestEvictProjectLayoutEmptyShellRecovered 等）
//
// 用法
//
//	go test ./godatabase/cache -run 'TestEvictProjectLayoutShortIDsNeverTargetsProjectsRoot|TestEvictProjectLayoutEmptyShellRecovered|TestUsageCountsProjectDirsNotProjectsRoot|TestEnsureCapacityCountLimitDrivesCount'
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/cache/evict_b13_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] coverage_test.go, evict.go, manager.go, manager_test.go, more_test.go
//
// 主要符号
//
//	type recordingCloser struct{…}
//	(r *recordingCloser) CloseDatabase(ctx context.Context, id string) error
//	TestEvictProjectLayoutShortIDsNeverTargetsProjectsRoot(t *testing.T)
//	TestEvictProjectLayoutEmptyShellRecovered(t *testing.T)
//	TestUsageCountsProjectDirsNotProjectsRoot(t *testing.T)
//	TestEnsureCapacityCountLimitDrivesCount(t *testing.T)

package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase/internal/testutil"
)

// recordingCloser 记录 CloseDatabase 收到的 ID（区分按 projectID 还是 UUID）。
type recordingCloser struct {
	fakeRegistry
	closed []string
}

func (r *recordingCloser) CloseDatabase(ctx context.Context, id string) error {
	r.closed = append(r.closed, id)
	r.active[id] = false
	return nil
}

// TestEvictProjectLayoutShortIDsNeverTargetsProjectsRoot 复现 B13：真实布局
// projects/{8位短项目ID}/database.ext 下，合法短 ID 目录必须各自独立参与
// 淘汰；projects/ 根目录绝不因为「识别不出 UUID 子目录」被当作空壳候选
// 整体删除，活跃项目也必须保留。
func TestEvictProjectLayoutShortIDsNeverTargetsProjectsRoot(t *testing.T) {
	dir := t.TempDir()
	reg := &recordingCloser{fakeRegistry: fakeRegistry{active: map[string]bool{
		"abcd1234": true, // 项目 A 活跃（registry 按 projectID 报告）
	}}}
	m, err := NewManager(Options{
		Root:     dir,
		MaxBytes: 1 << 30,
		Registry: reg,
		Closer:   reg,
		Logger:   &testutil.RecordingLogger{},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	mkProject := func(id, content string) string {
		p := filepath.Join(dir, "projects", id)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", id, err)
		}
		if err := os.WriteFile(filepath.Join(p, "database.duckdb"), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", id, err)
		}
		return p
	}
	active := mkProject("abcd1234", "active-db-content")
	idle := mkProject("wxyz9876", "idle-db-content")

	res, err := m.Evict(context.Background(), int64(len("idle-db-content")))
	if err != nil {
		t.Fatalf("Evict: %v", err)
	}
	if res.EvictedCount != 1 || !res.Achieved {
		t.Fatalf("expected exactly 1 eviction achieved, got %+v", res)
	}
	if res.SkippedActive != 1 {
		t.Fatalf("expected active project skipped, got %+v", res)
	}
	// 活跃项目目录必须保留。
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active project dir must survive: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(active, "database.duckdb")); err != nil || string(b) != "active-db-content" {
		t.Fatalf("active project content must survive, got %q err=%v", b, err)
	}
	// 空闲项目目录被淘汰，且 closer 以 projectID 收到关闭请求。
	if _, err := os.Stat(idle); !os.IsNotExist(err) {
		t.Fatalf("idle project dir must be evicted, stat err=%v", err)
	}
	if len(reg.closed) != 1 || reg.closed[0] != "wxyz9876" {
		t.Fatalf("closer must receive projectID of evicted dir, got %v", reg.closed)
	}
	// projects/ 根目录必须保留（仍有活跃子目录）。
	if _, err := os.Stat(filepath.Join(dir, "projects")); err != nil {
		t.Fatalf("projects root must survive: %v", err)
	}
}

// TestEvictProjectLayoutEmptyShellRecovered：projects/ 下无任何子目录时，
// 空壳 projects/ 本身允许回收（无数据可损失）。
func TestEvictProjectLayoutEmptyShellRecovered(t *testing.T) {
	dir := t.TempDir()
	reg := &recordingCloser{fakeRegistry: fakeRegistry{active: map[string]bool{}}}
	m, err := NewManager(Options{Root: dir, MaxBytes: 1 << 30, Registry: reg, Closer: reg})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	projects := filepath.Join(dir, "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// 空壳无字节可释放，用数量驱动（与 evictOverCount 同路径）。
	res, err := m.evict(context.Background(), 0, 1)
	if err != nil {
		t.Fatalf("evict: %v", err)
	}
	if res.EvictedCount != 1 {
		t.Fatalf("expected empty projects shell evicted, got %+v", res)
	}
	if _, err := os.Stat(projects); !os.IsNotExist(err) {
		t.Fatalf("empty projects shell must be removed, stat err=%v", err)
	}
}

// TestUsageCountsProjectDirsNotProjectsRoot：Usage 的目录数口径必须与淘汰
// 单位一致：projects/ 布局按项目子目录计数，而不是整个 projects/ 算 1。
func TestUsageCountsProjectDirsNotProjectsRoot(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(Options{Root: dir})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	for _, id := range []string{"abcd1234", "wxyz9876", "mnop3456"} {
		p := filepath.Join(dir, "projects", id)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(p, "local-state.json"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	u, err := m.Usage(context.Background())
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if u.DatabaseDirs != 3 {
		t.Fatalf("expected 3 project dirs, got %d", u.DatabaseDirs)
	}
	if u.TotalBytes != 6 {
		t.Fatalf("expected 6 bytes, got %d", u.TotalBytes)
	}
}

// TestEnsureCapacityCountLimitDrivesCount：B4 回归——数量上限必须按「超额
// 目录数」驱动：3 个大小悬殊的目录、上限 1，必须淘汰 2 个而不是删 1 个
// 大目录就停（旧算法按 2*1024 字节目标，删最大目录即达标）。
func TestEnsureCapacityCountLimitDrivesCount(t *testing.T) {
	dir := t.TempDir()
	reg := &recordingCloser{fakeRegistry: fakeRegistry{active: map[string]bool{}}}
	m, err := NewManager(Options{
		Root:         dir,
		MaxBytes:     1 << 30,
		MaxDatabases: 1,
		Registry:     reg,
		Closer:       reg,
		Logger:       &testutil.RecordingLogger{},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	big := filepath.Join(dir, "11111111-1111-1111-1111-111111111111")
	small1 := filepath.Join(dir, "22222222-2222-2222-2222-222222222222")
	small2 := filepath.Join(dir, "33333333-3333-3333-3333-333333333333")
	for _, p := range []string{big, small1, small2} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(big, "blob"), make([]byte, 8192), 0o644); err != nil {
		t.Fatalf("write big: %v", err)
	}
	for _, p := range []string{small1, small2} {
		if err := os.WriteFile(filepath.Join(p, "tiny"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write tiny: %v", err)
		}
	}
	if err := m.EnsureCapacity(context.Background(), 1); err != nil {
		t.Fatalf("EnsureCapacity: %v", err)
	}
	// 上限 1：应恰好保留 1 个目录（LRU 下两个 tiny 先被淘汰，或至少淘汰
	// 恰好 2 个）。big 留下与否取决于 mtime 顺序，但删除数必须等于 2。
	remaining := 0
	for _, p := range []string{big, small1, small2} {
		if _, err := os.Stat(p); err == nil {
			remaining++
		}
	}
	if remaining != 1 {
		t.Fatalf("expected exactly 1 remaining dir, got %d", remaining)
	}
}
