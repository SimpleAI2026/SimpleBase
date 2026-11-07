// harness_test.go 为「DuckLake 系统库读路径」基准提供可复现的本地环境。
//
// 为什么需要这个 harness：生产环境系统库在远端（COS）上，每次读取都要为
// 小 parquet 文件付远端读取代价。本地基准无法复现网络，但可以复现「读放大」——
// 即「存活文件数 → 查询耗时」的关系：把 DataInliningRowLimit 设为 0 模拟最坏情况
// （不是当前生产设置，当前配置为 1000，但 catalog 同步前 flush 仍可能产生小文件），
// 每次 INSERT 就落一个独立小 parquet 文件。
//
// godatabase 不得 import 根模块，因此本包不依赖 systemdb / catalog / auth：
// 表结构与查询语句按 internal/systemdb、internal/auth 的同形 SQL 在此内联，
// 只经 ducklake.Factory 打开系统库。
package bench

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
)

const benchProject = "benchprj"

// 与 internal/systemdb/schema.go 中同名表的 DDL 保持同形。
var benchSchema = []string{
	`CREATE TABLE IF NOT EXISTS sys_databases (
		id VARCHAR NOT NULL,
		tenant_id VARCHAR NOT NULL,
		project_id VARCHAR NOT NULL,
		name VARCHAR NOT NULL,
		kind VARCHAR NOT NULL,
		status VARCHAR NOT NULL,
		storage_prefix VARCHAR NOT NULL,
		format_version BIGINT NOT NULL,
		deleted_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		data_model VARCHAR DEFAULT 'collection'
	)`,
	`CREATE TABLE IF NOT EXISTS sys_metric_samples (
		id VARCHAR NOT NULL,
		project_id VARCHAR NOT NULL,
		name VARCHAR NOT NULL,
		value_double DOUBLE NOT NULL,
		labels_json VARCHAR NOT NULL,
		occurred_at TIMESTAMP NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS sys_log_events (
		id VARCHAR NOT NULL,
		project_id VARCHAR,
		level VARCHAR NOT NULL,
		logger VARCHAR NOT NULL,
		message VARCHAR NOT NULL,
		fields_json VARCHAR NOT NULL,
		request_id VARCHAR NOT NULL,
		occurred_at TIMESTAMP NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS sys_users (
		id VARCHAR NOT NULL,
		username VARCHAR NOT NULL,
		password_hash VARCHAR NOT NULL,
		role VARCHAR NOT NULL,
		display_name VARCHAR NOT NULL DEFAULT '',
		email VARCHAR NOT NULL DEFAULT '',
		status VARCHAR NOT NULL,
		must_change_password BIGINT NOT NULL DEFAULT 0,
		created_by VARCHAR NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		last_login_at TIMESTAMP,
		disabled_at TIMESTAMP
	)`,
}

// storeOpts 控制基准环境的构造方式。
type storeOpts struct {
	// Inlining 对应 ducklake.data_inlining_row_limit。
	// 0 = 每次 INSERT 独立成文件（读放大最坏情况）；
	// 1000 = 当前生产配置的内联阈值（同步前 flush 仍会形成文件）。
	Inlining int
}

// newBenchDB 在临时目录上打开一个系统库（kind=system）并建好基准用表。
// 返回的 *sql.DB 由测试框架自动关闭。
func newBenchDB(tb testing.TB, opts storeOpts) *sql.DB {
	tb.Helper()
	dir := tb.TempDir()
	dopts := ducklake.DefaultOptions()
	dopts.DataInliningRowLimit = opts.Inlining
	f := &ducklake.Factory{
		CacheDir: filepath.Join(dir, "system", "dbs"),
		DataRoot: dir,
		Options:  dopts,
		Syncer:   ducklake.NewLocalSyncer(),
	}
	spec := godatabase.Spec{
		ID:        uuid.NewString(),
		ProjectID: "sb-admin",
		Kind:      godatabase.KindSystem,
		Status:    godatabase.StatusReady,
	}
	ctx := context.Background()
	db, err := f.Open(ctx, spec, godatabase.ReadWrite)
	if err != nil {
		tb.Fatalf("bench: open system db: %v", err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	for _, ddl := range benchSchema {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			tb.Fatalf("bench: create schema: %v", err)
		}
	}
	return db
}

// seedLogFlushes 写入 n 条日志，每条单独一个事务。
// inlining=0 时每个事务落一个 parquet 文件，用于复现小文件堆积。
func seedLogFlushes(tb testing.TB, db *sql.DB, projectID string, n int) {
	tb.Helper()
	base := time.Now().UTC().Add(-time.Duration(n) * time.Minute)
	for i := 0; i < n; i++ {
		insertLog(tb, db, projectID, fmt.Sprintf("bench log line %d", i), base.Add(time.Duration(i)*time.Minute))
	}
}

func insertLog(tb testing.TB, db *sql.DB, projectID, message string, at time.Time) {
	tb.Helper()
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO sys_log_events(id, project_id, level, logger, message, fields_json, request_id, occurred_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		uuid.NewString(), projectID, "info", "bench", message,
		`{"status":200,"duration_ms":12}`, "", at.UTC()); err != nil {
		tb.Fatalf("bench: insert log: %v", err)
	}
}

// seedMetricFlushes 写入 n 轮指标（每轮一个事务，含请求数、耗时、耗时计数与直方图行）。
func seedMetricFlushes(tb testing.TB, db *sql.DB, projectID string, n int) {
	tb.Helper()
	ctx := context.Background()
	// 每轮一条多行 INSERT：与 systemdb 的批量刷写一致，inlining=0 时每轮恰好一个文件。
	const stmt = `INSERT INTO sys_metric_samples(id, project_id, name, value_double, labels_json, occurred_at)
		VALUES(?,?,?,?,?,?),(?,?,?,?,?,?),(?,?,?,?,?,?),(?,?,?,?,?,?)`
	for i := 0; i < n; i++ {
		now := time.Now().UTC()
		if _, err := db.ExecContext(ctx, stmt,
			uuid.NewString(), projectID, "http_requests", 10.0, "{}", now,
			uuid.NewString(), projectID, "http_latency_ms", float64(5+i), "{}", now,
			uuid.NewString(), projectID, "http_latency_ms_count", 10.0, "{}", now,
			uuid.NewString(), projectID, "http_latency_histogram_ms", 10.0, `{"v":1}`, now,
		); err != nil {
			tb.Fatalf("bench: insert metrics #%d: %v", i, err)
		}
	}
}

// seedUsers 写入 n 个用户，复刻 auth 身份点查的读放大。
func seedUsers(tb testing.TB, db *sql.DB, n int) {
	tb.Helper()
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		if _, err := db.ExecContext(context.Background(),
			`INSERT INTO sys_users(id, username, password_hash, role, display_name, email, status,
				must_change_password, created_by, created_at, updated_at, last_login_at, disabled_at)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("user-%04d", i), fmt.Sprintf("bench-user-%04d", i), "x", "user", "", "", "active",
			0, "", now, now, nil, nil); err != nil {
			tb.Fatalf("bench: insert user #%d: %v", i, err)
		}
	}
}

// queryLogs 与 systemdb.Store.QueryLogs 同形：近 24h、项目过滤、按时间倒序取 limit 条。
func queryLogs(ctx context.Context, db *sql.DB, projectID string, limit int) (int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, project_id, level, logger, message, fields_json, request_id, occurred_at
		 FROM sys_log_events WHERE 1=1 AND (project_id = ? OR project_id IS NULL)
		 AND occurred_at >= ? ORDER BY occurred_at DESC LIMIT ?`,
		projectID, time.Now().UTC().Add(-24*time.Hour), int64(limit))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, level, logger, message, fields, requestID string
		var project sql.NullString
		var at time.Time
		if err := rows.Scan(&id, &project, &level, &logger, &message, &fields, &requestID, &at); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

// metricsSummary 与 systemdb.Store.MetricsSummary 同形：条件聚合 + 库计数 + 直方图三次查询。
func metricsSummary(ctx context.Context, db *sql.DB, projectID string) error {
	since := time.Now().UTC().Add(-24 * time.Hour)
	var requests, errs, latencySum, latencyCount, latencyAvg float64
	if err := db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(CASE WHEN name = 'http_requests' THEN value_double END), 0),
			COALESCE(SUM(CASE WHEN name = 'http_errors' THEN value_double END), 0),
			COALESCE(SUM(CASE WHEN name = 'http_latency_ms' THEN value_double END), 0),
			COALESCE(SUM(CASE WHEN name = 'http_latency_ms_count' THEN value_double END), 0),
			COALESCE(AVG(CASE WHEN name = 'http_latency_ms' THEN value_double END), 0)
		 FROM sys_metric_samples
		 WHERE occurred_at >= ? AND name IN ('http_requests', 'http_errors', 'http_latency_ms', 'http_latency_ms_count')
		 AND project_id = ?`, since, projectID,
	).Scan(&requests, &errs, &latencySum, &latencyCount, &latencyAvg); err != nil {
		return err
	}
	var active int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_databases WHERE project_id = ? AND deleted_at IS NULL AND kind = 'user'`,
		projectID).Scan(&active); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT labels_json, value_double FROM sys_metric_samples
		 WHERE occurred_at >= ? AND name = ? AND project_id = ?`,
		since, "http_latency_histogram_ms", projectID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var labels string
		var total float64
		if err := rows.Scan(&labels, &total); err != nil {
			return err
		}
	}
	return rows.Err()
}

// userByID 与 auth.sqlUserRepository.GetByID 同形的身份点查。
func userByID(ctx context.Context, db *sql.DB, id string) error {
	var (
		uid, username, hash, role, display, email, status, createdBy string
		mustChange                                                   int64
		createdAt, updatedAt                                         time.Time
		lastLogin, disabled                                          sql.NullTime
	)
	return db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, display_name, email, status,
			must_change_password, created_by, created_at, updated_at, last_login_at, disabled_at
		 FROM sys_users WHERE id = ?`, id,
	).Scan(&uid, &username, &hash, &role, &display, &email, &status,
		&mustChange, &createdBy, &createdAt, &updatedAt, &lastLogin, &disabled)
}

// liveTableFiles 只统计目标表的活跃 Parquet，避免以全库文件数代替查询触达数。
func liveTableFiles(tb testing.TB, db *sql.DB, table string) int {
	tb.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM __ducklake_metadata_lake.ducklake_data_file
		 WHERE end_snapshot IS NULL AND table_id =
		 (SELECT table_id FROM __ducklake_metadata_lake.ducklake_table WHERE table_name = ?)`, table).Scan(&count); err != nil {
		tb.Fatalf("bench: count live table files: %v", err)
	}
	return count
}

// liveDataFiles 统计 catalog 中存活（未被任何快照删除）数据文件数与总字节数，
// 是读放大的直接度量。DuckLake 元数据表位于 __ducklake_metadata_<alias> schema。
func liveDataFiles(tb testing.TB, db *sql.DB) (int, int64) {
	tb.Helper()
	var files int
	var bytes int64
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*), COALESCE(sum(file_size_bytes), 0)
		 FROM __ducklake_metadata_lake.ducklake_data_file WHERE end_snapshot IS NULL`).Scan(&files, &bytes); err != nil {
		tb.Fatalf("bench: count data files: %v", err)
	}
	return files, bytes
}

// snapshotCount 返回 catalog 中的快照总数（写放大度量）。
func snapshotCount(tb testing.TB, db *sql.DB) int {
	tb.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM __ducklake_metadata_lake.ducklake_snapshot`).Scan(&n); err != nil {
		tb.Fatalf("bench: count snapshots: %v", err)
	}
	return n
}
