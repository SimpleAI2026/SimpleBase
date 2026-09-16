package ducklake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
// DuckLake 扩展存在两套形态：
//  1) 旧：行式 option_name / value / scope
//  2) 新：宽表，列即配置项（如 data_path、extension_version）
// Open 路径的 data_path 校验依赖本函数，必须兼容当前已加载的扩展。
func ListSettings(ctx context.Context, db *sql.DB, alias string) ([]Setting, error) {
	if !isSafeIdent(alias) {
		return nil, fmt.Errorf("ducklake: invalid lake alias")
	}
	q := fmt.Sprintf("SELECT * FROM %s.settings()", quoteIdent(alias))
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("ducklake: settings: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("ducklake: settings columns: %w", err)
	}
	if len(cols) == 0 {
		return nil, nil
	}

	// 行式：option_name + value（scope 可选）
	lower := make([]string, len(cols))
	idx := map[string]int{}
	for i, c := range cols {
		lower[i] = strings.ToLower(c)
		idx[lower[i]] = i
	}
	if _, okName := idx["option_name"]; okName {
		if _, okVal := idx["value"]; okVal {
			var out []Setting
			for rows.Next() {
				raw := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range raw {
					ptrs[i] = &raw[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					return nil, err
				}
				s := Setting{
					Key:   fmt.Sprint(raw[idx["option_name"]]),
					Value: fmt.Sprint(raw[idx["value"]]),
				}
				if i, ok := idx["scope"]; ok && raw[i] != nil {
					s.Scope = fmt.Sprint(raw[i])
				}
				out = append(out, s)
			}
			return out, rows.Err()
		}
	}

	// 宽表：每一列是一项设置，通常一行。
	var out []Setting
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, name := range lower {
			val := ""
			if raw[i] != nil {
				val = fmt.Sprint(raw[i])
			}
			out = append(out, Setting{Key: name, Value: val})
		}
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
