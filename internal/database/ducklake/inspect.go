package ducklake

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Snapshot 是 lake.snapshots() 的一行。
type Snapshot struct {
	ID         int64
	Time       time.Time
	SchemaVer  int64
	Changes    string
	Author     string
	Message    string
	ExtraInfo  string
}

// Setting 是 lake.settings() 的一行。
type Setting struct {
	Key   string
	Value string
	Scope string
}

// TableFile 是 ducklake_list_files 的一行。
type TableFile struct {
	DataFile   string
	DeleteFile string
	FileSize   int64
}

// CurrentSnapshot 返回 lake.current_snapshot() 的最新快照 id。
func CurrentSnapshot(ctx context.Context, db *sql.DB, alias string) (int64, error) {
	if !isSafeIdent(alias) {
		return 0, fmt.Errorf("ducklake: invalid lake alias")
	}
	var id int64
	err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT id FROM %s.current_snapshot()", quoteIdent(alias))).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("ducklake: current_snapshot: %w", err)
	}
	return id, nil
}

// ListSnapshots 返回全部快照（管理面 / 审计流水）。
func ListSnapshots(ctx context.Context, db *sql.DB, alias string) ([]Snapshot, error) {
	if !isSafeIdent(alias) {
		return nil, fmt.Errorf("ducklake: invalid lake alias")
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT snapshot_id, snapshot_time, schema_version,
		       CAST(changes AS VARCHAR),
		       COALESCE(author, ''),
		       COALESCE(commit_message, ''),
		       COALESCE(CAST(commit_extra_info AS VARCHAR), '')
		FROM %s.snapshots()
		ORDER BY snapshot_id`, quoteIdent(alias)))
	if err != nil {
		return nil, fmt.Errorf("ducklake: snapshots: %w", err)
	}
	defer rows.Close()

	var out []Snapshot
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(&s.ID, &s.Time, &s.SchemaVer, &s.Changes, &s.Author, &s.Message, &s.ExtraInfo); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListSettings 返回 lake.settings()。
func ListSettings(ctx context.Context, db *sql.DB, alias string) ([]Setting, error) {
	if !isSafeIdent(alias) {
		return nil, fmt.Errorf("ducklake: invalid lake alias")
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		"SELECT CAST(option_name AS VARCHAR), CAST(value AS VARCHAR), COALESCE(CAST(scope AS VARCHAR), '') FROM %s.settings()",
		quoteIdent(alias)))
	if err != nil {
		return nil, fmt.Errorf("ducklake: settings: %w", err)
	}
	defer rows.Close()

	var out []Setting
	for rows.Next() {
		var s Setting
		if err := rows.Scan(&s.Key, &s.Value, &s.Scope); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListTableFiles 列出表的数据/删除文件。
func ListTableFiles(ctx context.Context, db *sql.DB, alias, table string) ([]TableFile, error) {
	if !isSafeIdent(alias) || !isSafeIdent(table) {
		return nil, fmt.Errorf("ducklake: invalid identifier")
	}
	rows, err := db.QueryContext(ctx,
		"SELECT CAST(data_file AS VARCHAR), COALESCE(CAST(delete_file AS VARCHAR), ''), COALESCE(file_size_bytes, 0) FROM ducklake_list_files(?, ?)",
		alias, table)
	if err != nil {
		return nil, fmt.Errorf("ducklake: list_files: %w", err)
	}
	defer rows.Close()

	var out []TableFile
	for rows.Next() {
		var f TableFile
		if err := rows.Scan(&f.DataFile, &f.DeleteFile, &f.FileSize); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
