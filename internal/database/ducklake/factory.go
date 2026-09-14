package ducklake

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	duckdb "github.com/duckdb/duckdb-go/v2"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// Factory 实现 database.Factory：每库一个 DuckDB 实例（:memory: + ATTACH DuckLake）。
// Phase 1 使用本地 DATA_PATH，不写 S3。
type Factory struct {
	CacheDir string
	Options  Options
	Syncer   Syncer
	Logger   observability.Logger
	Metrics  *observability.Metrics
}

// Open 打开（或创建）指定 logical database 的 DuckLake 连接。
func (f *Factory) Open(ctx context.Context, db catalog.Database, mode database.AccessMode) (*sql.DB, error) {
	if _, err := uuid.Parse(db.ID); err != nil {
		return nil, fmt.Errorf("ducklake: invalid database id: %w", err)
	}
	if f.CacheDir == "" {
		return nil, fmt.Errorf("ducklake: cache dir is required")
	}

	opts := f.Options.normalized()
	layout := layoutFor(f.CacheDir, db.ID)
	if err := layout.ensure(); err != nil {
		return nil, fmt.Errorf("ducklake: create cache dirs: %w", err)
	}

	boot := buildBootSQL(layout, opts)
	connector, err := duckdb.NewConnector("", func(execer driver.ExecerContext) error {
		for _, q := range boot {
			if _, err := execer.ExecContext(ctx, q, nil); err != nil {
				return fmt.Errorf("ducklake: boot %s: %w", summarizeSQL(q), err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ducklake: new connector: %w", err)
	}

	sqlDB := sql.OpenDB(connector)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ducklake: ping: %w", err)
	}

	if _, err := AssertDuckDBVersion(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := AssertExtensionsLoaded(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	if f.Logger != nil {
		f.Logger.Info("ducklake database opened",
			zap.String("database_id", db.ID),
			zap.String("mode", mode.String()),
			zap.String("alias", opts.LakeAlias),
		)
	}
	return sqlDB, nil
}

func buildBootSQL(layout Layout, opts Options) []string {
	alias := opts.LakeAlias
	out := extensionBootSQL(opts)
	out = append(out,
		"SET memory_limit = "+quoteSQLString(opts.MemoryLimit),
		"SET threads = "+strconv.Itoa(opts.Threads),
		"SET temp_directory = "+sqlPath(layout.TempDir),
		"SET allowed_directories = ["+strings.Join(allowedDirectorySQL(layout, opts), ", ")+"]",
	)

	attach := fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS %s (DATA_PATH %s, DATA_INLINING_ROW_LIMIT %d, AUTOMATIC_MIGRATION false)",
		filepathToSlash(layout.CatalogFile),
		quoteIdent(alias),
		sqlPath(layout.dataPathArg()),
		opts.DataInliningRowLimit,
	)
	out = append(out, attach, "USE "+quoteIdent(alias))
	out = append(out, optionSQL(alias, opts)...)
	// allowed_directories 是 enable_external_access=false 时的白名单（DuckDB 安全模型）。
	// 必须在 ATTACH 之后关闭外部访问，已挂载的 catalog / DATA_PATH 仍可读写。
	out = append(out, "SET enable_external_access = false")
	out = append(out, "SET lock_configuration = true")
	return out
}

func allowedDirectorySQL(layout Layout, opts Options) []string {
	dirs := []string{sqlPath(layout.Root)}
	if opts.ExtensionDir != "" {
		dirs = append(dirs, sqlPath(opts.ExtensionDir))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, sqlPath(filepath.Join(home, ".duckdb")))
	}
	return dirs
}

func optionSQL(alias string, opts Options) []string {
	target := quoteIdent(alias)
	out := []string{
		fmt.Sprintf("CALL %s.set_option('parquet_compression', %s)", target, quoteSQLString(opts.ParquetCompression)),
		fmt.Sprintf("CALL %s.set_option('target_file_size', %s)", target, quoteSQLString(opts.TargetFileSize)),
		fmt.Sprintf("CALL %s.set_option('data_inlining_row_limit', %s)", target, quoteSQLString(strconv.Itoa(opts.DataInliningRowLimit))),
	}
	if opts.RequireCommitMessage {
		out = append(out, fmt.Sprintf("CALL %s.set_option('require_commit_message', 'true')", target))
	}
	return out
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

func summarizeSQL(q string) string {
	q = strings.TrimSpace(q)
	if len(q) > 80 {
		return q[:80] + "..."
	}
	return q
}
