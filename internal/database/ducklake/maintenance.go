package ducklake

import (
	"context"
	"database/sql"
	"fmt"
)

// FlushInlined 把 catalog 内联数据物化为 Parquet。
func FlushInlined(ctx context.Context, db *sql.DB, alias string) error {
	return callLake(ctx, db, alias, "ducklake_flush_inlined_data", false)
}

// ExpireSnapshots 按 older_than 过期快照。dryRun 只返回将删除的快照。
func ExpireSnapshots(ctx context.Context, db *sql.DB, alias, olderThan string, dryRun bool) error {
	if !isSafeIdent(alias) {
		return fmt.Errorf("ducklake: invalid lake alias")
	}
	q := fmt.Sprintf("CALL ducklake_expire_snapshots(%s, older_than => %s, dry_run => %s)",
		quoteSQLString(alias), quoteSQLString(olderThan), boolSQL(dryRun))
	_, err := db.ExecContext(ctx, q)
	if err != nil {
		return fmt.Errorf("ducklake: expire_snapshots: %w", err)
	}
	return nil
}

// MergeAdjacentFiles 合并小文件（保留 time travel）。
func MergeAdjacentFiles(ctx context.Context, db *sql.DB, alias string) error {
	return callLake(ctx, db, alias, "ducklake_merge_adjacent_files", false)
}

// RewriteDataFiles 重写删除比例超阈值的文件。
func RewriteDataFiles(ctx context.Context, db *sql.DB, alias string, dryRun bool) error {
	return callLake(ctx, db, alias, "ducklake_rewrite_data_files", dryRun)
}

// CleanupOldFiles 清理已调度删除的文件。
func CleanupOldFiles(ctx context.Context, db *sql.DB, alias, olderThan string, dryRun bool) error {
	if !isSafeIdent(alias) {
		return fmt.Errorf("ducklake: invalid lake alias")
	}
	q := fmt.Sprintf("CALL ducklake_cleanup_old_files(%s, older_than => %s, dry_run => %s)",
		quoteSQLString(alias), quoteSQLString(olderThan), boolSQL(dryRun))
	_, err := db.ExecContext(ctx, q)
	if err != nil {
		return fmt.Errorf("ducklake: cleanup_old_files: %w", err)
	}
	return nil
}

// DeleteOrphanedFiles 清理不被 catalog 追踪的孤儿文件。
func DeleteOrphanedFiles(ctx context.Context, db *sql.DB, alias, olderThan string, dryRun bool) error {
	if !isSafeIdent(alias) {
		return fmt.Errorf("ducklake: invalid lake alias")
	}
	q := fmt.Sprintf("CALL ducklake_delete_orphaned_files(%s, older_than => %s, dry_run => %s)",
		quoteSQLString(alias), quoteSQLString(olderThan), boolSQL(dryRun))
	_, err := db.ExecContext(ctx, q)
	if err != nil {
		return fmt.Errorf("ducklake: delete_orphaned_files: %w", err)
	}
	return nil
}

func callLake(ctx context.Context, db *sql.DB, alias, fn string, dryRun bool) error {
	if !isSafeIdent(alias) {
		return fmt.Errorf("ducklake: invalid lake alias")
	}
	q := fmt.Sprintf("CALL %s(%s", fn, quoteSQLString(alias))
	if dryRun {
		q += ", dry_run => true"
	}
	q += ")"
	if _, err := db.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("ducklake: %s: %w", fn, err)
	}
	return nil
}

func boolSQL(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
