package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
)

type fakeExec struct {
	mu      sync.Mutex
	queries []string
	args    [][]any
	failOn  string // 含该子串的 query 返回错误
}

func (f *fakeExec) run(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	f.args = append(f.args, args)
	if f.failOn != "" && strings.Contains(query, f.failOn) {
		return nil, errors.New("boom")
	}
	return nil, nil
}

func newMaintRunner(f *fakeExec, writable func() bool) *MaintenanceRunner {
	r := NewMaintenanceRunner(nil, ducklake.MaintenanceOptions{
		CheckpointInterval: time.Hour,
		ExpireOlderThan:    7 * 24 * time.Hour,
		DeleteOlderThan:    7 * 24 * time.Hour,
	}, writable, nil)
	r.exec = f.run
	return r
}

// 三轮步骤按序执行；cleanup 默认 dry-run。
func TestMaintenanceRunnerStepsInOrder(t *testing.T) {
	f := &fakeExec{}
	r := newMaintRunner(f, func() bool { return true })
	r.runOnce(context.Background())

	if len(f.queries) != 3 {
		t.Fatalf("steps=%d want 3: %v", len(f.queries), f.queries)
	}
	if f.queries[0] != "CALL ducklake_merge_adjacent_files(?)" {
		t.Fatalf("step1: %s", f.queries[0])
	}
	if f.queries[1] != "CALL ducklake_expire_snapshots(?, older_than => ?)" {
		t.Fatalf("step2: %s", f.queries[1])
	}
	if f.queries[2] != "CALL ducklake_cleanup_old_files(?, older_than => ?, dry_run => ?)" {
		t.Fatalf("step3: %s", f.queries[2])
	}
	// dry-run 默认开启
	last := f.args[2]
	if dry, ok := last[len(last)-1].(bool); !ok || !dry {
		t.Fatalf("cleanup dry_run arg=%v want true", last[len(last)-1])
	}
	// 不做 delete_orphaned_files
	for _, q := range f.queries {
		if strings.Contains(q, "delete_orphaned") {
			t.Fatalf("must never call delete_orphaned_files: %s", q)
		}
	}
}

// 非 writer 直接跳过。
func TestMaintenanceRunnerSkipsWhenNotWriter(t *testing.T) {
	f := &fakeExec{}
	r := newMaintRunner(f, func() bool { return false })
	r.runOnce(context.Background())
	if len(f.queries) != 0 {
		t.Fatalf("non-writer must skip, got %v", f.queries)
	}
}

// 单步失败不 panic、不中断后续步骤。
func TestMaintenanceRunnerStepErrorContinues(t *testing.T) {
	f := &fakeExec{failOn: "expire_snapshots"}
	r := newMaintRunner(f, func() bool { return true })
	r.runOnce(context.Background())
	if len(f.queries) != 3 {
		t.Fatalf("steps=%d want 3（失败步骤不应中断后续）", len(f.queries))
	}
}

// Start/Close 可中断：初始延迟 60s 内 Close 必须快速返回。
func TestMaintenanceRunnerCloseStopsLoop(t *testing.T) {
	f := &fakeExec{}
	r := newMaintRunner(f, func() bool { return true })
	r.Start()
	done := make(chan struct{})
	go func() {
		_ = r.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close 被初始延迟卡住")
	}
	// 幂等
	_ = r.Close()
}

// exec 为 nil（无连接）时 Start 不启动循环。
func TestMaintenanceRunnerNoDB(t *testing.T) {
	r := NewMaintenanceRunner(nil, ducklake.MaintenanceOptions{}, nil, nil)
	r.Start() // no-op，不 panic
	_ = r.Close()
	if r.interval() != time.Hour {
		t.Fatalf("default interval: %v", r.interval())
	}
}

// —— compactAuthTables（planv5.0 §4 P2.2）——

func newCompactRunner(f *fakeExec, writable func() bool, liveFor map[string]int) *MaintenanceRunner {
	r := newMaintRunner(f, writable)
	r.liveFiles = func(_ context.Context, table string) int {
		return liveFor[table]
	}
	return r
}

// 达到阈值的表按序 merge，阈值外/统计失败的表跳过。
func TestCompactAuthTablesMergesOnlyHotTables(t *testing.T) {
	f := &fakeExec{}
	liveFor := map[string]int{
		"sys_user_sessions":  18,
		"sys_project_owners": 19,
		"sys_users":          2,  // 低于阈值：跳过
	}
	r := newCompactRunner(f, func() bool { return true }, liveFor)
	r.compactAuthTables(context.Background())

	if len(f.queries) != 2 {
		t.Fatalf("merge calls=%d want 2: %v", len(f.queries), f.queries)
	}
	for i, want := range []string{"sys_user_sessions", "sys_project_owners"} {
		if got := f.args[i][1]; got != want {
			t.Fatalf("merge #%d table=%v want %v", i, got, want)
		}
		if f.queries[i] != "CALL ducklake_merge_adjacent_files(?, ?)" {
			t.Fatalf("merge #%d query=%s", i, f.queries[i])
		}
	}
}

// 统计失败的表（liveFiles 返回 -1）跳过但不阻塞后续表。
func TestCompactAuthTablesSkipsOnStatsFailure(t *testing.T) {
	f := &fakeExec{}
	liveFor := map[string]int{
		"sys_user_sessions":  -1, // 统计失败
		"sys_project_owners": 10,
	}
	r := newCompactRunner(f, func() bool { return true }, liveFor)
	r.compactAuthTables(context.Background())
	if len(f.queries) != 1 || f.args[0][1] != "sys_project_owners" {
		t.Fatalf("must skip failed stats and merge rest: %v", f.queries)
	}
}

// 非 writer 直接跳过。
func TestCompactAuthTablesSkipsWhenNotWriter(t *testing.T) {
	f := &fakeExec{}
	r := newCompactRunner(f, func() bool { return false }, map[string]int{"sys_users": 10})
	r.compactAuthTables(context.Background())
	if len(f.queries) != 0 {
		t.Fatal("non-writer must skip compactAuthTables")
	}
}

// liveFiles 为 nil（无连接）时不 panic、不发 merge。
func TestCompactAuthTablesSkipsWithoutLiveFiles(t *testing.T) {
	f := &fakeExec{}
	r := newMaintRunner(f, func() bool { return true })
	r.compactAuthTables(context.Background())
	if len(f.queries) != 0 {
		t.Fatal("nil liveFiles must skip compactAuthTables")
	}
}

// 单表 merge 失败不中断后续表。
func TestCompactAuthTablesStepErrorContinues(t *testing.T) {
	f := &fakeExec{failOn: "merge_adjacent_files"}
	liveFor := map[string]int{
		"sys_user_sessions":  10,
		"sys_project_owners": 10,
	}
	r := newCompactRunner(f, func() bool { return true }, liveFor)
	r.compactAuthTables(context.Background())
	if len(f.queries) != 2 {
		t.Fatalf("merge calls=%d want 2（失败不中断）", len(f.queries))
	}
}
