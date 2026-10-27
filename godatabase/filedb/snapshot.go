// 文件说明：snapshot.go — godatabase/filedb 中 SnapshotFile、ValidateSnapshot 等符号的实现
//
// 用法
//
//	SnapshotFile(ctx context.Context, db *sql.DB, source, engine, dst string) ...
//	ValidateSnapshot(ctx context.Context, path, engine string) error
//
// 调用架构
//
//	[上游] internal/app, internal/backup
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/filedb/snapshot.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/sqlx
//	[同包] factory.go, path.go, remote.go, restore.go, restore_remote.go 等 6 个
//
// 主要符号
//
//	SnapshotFile(ctx context.Context, db *sql.DB, source, engine, dst string) ...
//	ValidateSnapshot(ctx context.Context, path, engine string) error

package filedb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
)

func SnapshotFile(ctx context.Context, db *sql.DB, source, engine, dst string) (godatabase.SnapshotInfo, error) {
	info := godatabase.SnapshotInfo{Engine: engine, FormatVer: 1}
	if db == nil {
		return info, fmt.Errorf("filedb: snapshot database is nil")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return info, fmt.Errorf("filedb: prepare snapshot: %w", err)
	}
	if _, err := os.Stat(dst); err == nil {
		return info, fmt.Errorf("filedb: snapshot destination exists")
	}
	switch engine {
	case godatabase.EngineSQLite:
		if _, err := db.ExecContext(ctx, "VACUUM INTO "+sqlx.Literal(dst)); err != nil {
			return info, fmt.Errorf("filedb: sqlite snapshot: %w", err)
		}
	case godatabase.EngineDuckDB:
		if _, err := db.ExecContext(ctx, "CHECKPOINT"); err != nil {
			return info, fmt.Errorf("filedb: duckdb checkpoint: %w", err)
		}
		if err := copyFile(source, dst); err != nil {
			return info, err
		}
	default:
		return info, fmt.Errorf("filedb: unsupported snapshot engine")
	}
	if err := ValidateSnapshot(ctx, dst, engine); err != nil {
		_ = os.Remove(dst)
		return info, err
	}
	stat, err := os.Stat(dst)
	if err != nil {
		return info, fmt.Errorf("filedb: stat snapshot: %w", err)
	}
	digest, err := fileSHA256(dst)
	if err != nil {
		return info, fmt.Errorf("filedb: hash snapshot: %w", err)
	}
	info.SizeBytes, info.SHA256, info.CompletedAt = stat.Size(), digest, time.Now().UTC()
	return info, nil
}

func ValidateSnapshot(ctx context.Context, path, engine string) error {
	var db *sql.DB
	switch engine {
	case godatabase.EngineSQLite:
		db = sql.OpenDB(&sqliteConnector{filePath: path})
		db.SetMaxOpenConns(1)
	case godatabase.EngineDuckDB:
		connector, err := duckdb.NewConnector(path, nil)
		if err != nil {
			return fmt.Errorf("filedb: validate duckdb snapshot: %w", err)
		}
		db = sql.OpenDB(connector)
	default:
		return fmt.Errorf("filedb: unsupported snapshot engine")
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("filedb: invalid snapshot: %w", err)
	}
	if engine == godatabase.EngineSQLite {
		var result string
		if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil || result != "ok" {
			return fmt.Errorf("filedb: snapshot integrity check failed")
		}
	}
	return nil
}
