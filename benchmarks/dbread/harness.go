// harness.go 为「系统库读路径」基准提供可复现的本地环境。
//
// 为什么需要这个 harness：生产环境系统库在远端（COS）上，每次读取都要为
// 小 parquet 文件付远端读取代价。本地基准无法复现网络，
// 但可以复现「读放大」——即「存活文件数 → 查询耗时」的关系：
// 把 DataInliningRowLimit 设为 0 模拟最坏情况（不是当前生产设置；
// 当前配置为 1000，但 catalog 同步前 flush 仍可能产生小文件），
// 每次 INSERT 就落一个独立小 parquet 文件。
package main

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// storeOpts 控制基准环境的构造方式。
type storeOpts struct {
	// Inlining 对应 ducklake.data_inlining_row_limit。
	// 0 = 每次 INSERT 独立成文件（读放大最坏情况）；
	// 1000 = 当前生产配置的内联阈值（同步前 flush 仍会形成文件）。
	Inlining int
}

// newBenchStore 在临时目录上 bootstrap 一个系统库。
// 返回的 store 由测试框架自动关闭。
func newBenchStore(tb testing.TB, opts storeOpts) *systemdb.Store {
	tb.Helper()
	dir := tb.TempDir()
	dopts := ducklake.DefaultOptions()
	dopts.DataInliningRowLimit = opts.Inlining
	f := &ducklake.Factory{
		CacheDir: filepath.Join(dir, "system", "dbs"),
		Options:  dopts,
		Syncer:   ducklake.NewLocalSyncer(),
	}
	store, err := systemdb.Bootstrap(context.Background(), systemdb.BootstrapInput{
		LocatorDir: filepath.Join(dir, "system"),
		Name:       systemdb.DefaultName,
		Factory:    f,
		Keys:       objectstore.KeyBuilder{RootPrefix: "simplebase", Environment: "bench"},
	})
	if err != nil {
		tb.Fatalf("dbread: bootstrap systemdb: %v", err)
	}
	tb.Cleanup(func() { _ = store.Close() })
	return store
}

// seedProject 建一个 tenant + project 行。
func seedProject(tb testing.TB, store *systemdb.Store, projectID string) {
	tb.Helper()
	repo := catalog.NewSQLRepository(store.DB())
	ctx := context.Background()
	if err := repo.CreateTenant(ctx, catalog.Tenant{
		ID: "tenant-bench", Name: "bench", CreatedAt: time.Now().UTC(),
	}); err != nil && !catalog.IsAlreadyExists(err) {
		tb.Fatalf("dbread: create tenant: %v", err)
	}
	if err := repo.CreateProject(ctx, catalog.Project{
		ID: projectID, TenantID: "tenant-bench", Name: "bench", CreatedAt: time.Now().UTC(),
	}); err != nil && !catalog.IsAlreadyExists(err) {
		tb.Fatalf("dbread: create project: %v", err)
	}
}

// seedLogFlushes 写入 n 条日志，每条单独 flush 一轮。
// 每轮 = 一个事务 = 一个 parquet 文件（inlining=0 时），用于复现小文件堆积。
func seedLogFlushes(tb testing.TB, store *systemdb.Store, projectID string, n int) {
	tb.Helper()
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Duration(n) * time.Minute)
	for i := 0; i < n; i++ {
		store.RecordLog(systemdb.LogEvent{
			ProjectID:  projectID,
			Level:      "info",
			Logger:     "bench",
			Message:    fmt.Sprintf("bench log line %d", i),
			FieldsJSON: `{"status":200,"duration_ms":12}`,
			OccurredAt: base.Add(time.Duration(i) * time.Minute),
		})
		if err := store.FlushLogs(ctx); err != nil {
			tb.Fatalf("dbread: flush logs #%d: %v", i, err)
		}
	}
}

// seedMetricFlushes 写入 n 轮指标（每轮一次 flush）。
func seedMetricFlushes(tb testing.TB, store *systemdb.Store, projectID string, n int) {
	tb.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		store.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "http_requests", Value: 10})
		store.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "http_latency_ms", Value: float64(5 + i)})
		if err := store.FlushMetrics(ctx); err != nil {
			tb.Fatalf("dbread: flush metrics #%d: %v", i, err)
		}
	}
}

// seedUsersAndKeys 写入 n 个用户（复刻 auth 身份点查的读放大）。
func seedUsersAndKeys(tb testing.TB, store *systemdb.Store, n int) {
	tb.Helper()
	ctx := context.Background()
	users := auth.NewSQLUserRepository(store.DB())
	for i := 0; i < n; i++ {
		u := auth.User{
			ID:           fmt.Sprintf("user-%04d", i),
			Username:     fmt.Sprintf("bench-user-%04d", i),
			PasswordHash: "x",
			Role:         auth.RoleUser,
			Status:       auth.UserStatusActive,
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		}
		if err := users.Create(ctx, u); err != nil {
			tb.Fatalf("dbread: create user #%d: %v", i, err)
		}
	}
}

// liveDataFiles 统计 catalog 中存活（未被任何快照删除）数据文件数。
// 这是读放大的直接度量。DuckLake 元数据表位于 __ducklake_metadata_<alias> schema。
func liveDataFiles(tb testing.TB, store *systemdb.Store) (int, int64) {
	tb.Helper()
	row := store.DB().QueryRowContext(context.Background(),
		`SELECT count(*), COALESCE(sum(file_size_bytes), 0)
		 FROM __ducklake_metadata_lake.ducklake_data_file WHERE end_snapshot IS NULL`)
	var files int
	var bytes int64
	if err := row.Scan(&files, &bytes); err != nil {
		tb.Fatalf("dbread: count data files: %v", err)
	}
	return files, bytes
}

// snapshotCount 返回 catalog 中的快照总数（写放大度量）。
func snapshotCount(tb testing.TB, store *systemdb.Store) int {
	tb.Helper()
	var n int
	if err := store.DB().QueryRowContext(context.Background(),
		`SELECT count(*) FROM __ducklake_metadata_lake.ducklake_snapshot`).Scan(&n); err != nil {
		tb.Fatalf("dbread: count snapshots: %v", err)
	}
	return n
}
