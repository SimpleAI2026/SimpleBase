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
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// Factory 实现 database.Factory：每库一个 DuckDB 实例（:memory: + ATTACH DuckLake）。
// Remote.Enabled 时 DATA_PATH 指向 S3，并在 Open 时冷启动下载 catalog。
type Factory struct {
	CacheDir string
	Options  Options
	Syncer   Syncer
	Remote   RemoteStorage
	Blobs    objectstore.BlobStore
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
	if err := f.Remote.validate(); err != nil {
		return nil, err
	}

	opts := f.Options.normalized()
	layout := layoutFor(f.CacheDir, db.ID)
	if err := layout.ensure(); err != nil {
		return nil, fmt.Errorf("ducklake: create cache dirs: %w", err)
	}

	if f.Remote.Enabled {
		if _, err := EnsureLocalCatalog(ctx, f.Blobs, f.Remote, f.CacheDir, db); err != nil {
			return nil, err
		}
	}

	dataPath, err := f.dataPathFor(db, layout)
	if err != nil {
		return nil, err
	}

	boot := buildBootSQL(layout, opts, f.Remote, dataPath)
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

	if f.Remote.Enabled {
		if err := validateRemoteDataPath(ctx, sqlDB, opts.LakeAlias, dataPath); err != nil {
			_ = sqlDB.Close()
			return nil, err
		}
	}

	if bs, ok := f.Syncer.(BindingSyncer); ok {
		bs.Bind(db.ID, sqlDB, db, opts.LakeAlias)
	}

	if f.Logger != nil {
		f.Logger.Info("ducklake database opened",
			zap.String("database_id", db.ID),
			zap.String("mode", mode.String()),
			zap.String("alias", opts.LakeAlias),
			zap.Bool("remote", f.Remote.Enabled),
		)
	}
	return sqlDB, nil
}

func (f *Factory) dataPathFor(db catalog.Database, layout Layout) (string, error) {
	if !f.Remote.Enabled {
		return layout.dataPathArg(), nil
	}
	return buildDataURI(f.Remote, db.TenantID, db.ID)
}

func buildBootSQL(layout Layout, opts Options, remote RemoteStorage, dataPath string) []string {
	alias := opts.LakeAlias
	out := extensionBootSQL(opts)
	if remote.Enabled {
		out = append(out, buildCreateSecretSQL(remote))
	}
	out = append(out,
		"SET memory_limit = "+quoteSQLString(opts.MemoryLimit),
		"SET threads = "+strconv.Itoa(opts.Threads),
		"SET temp_directory = "+sqlPath(layout.TempDir),
		"SET allowed_directories = ["+strings.Join(allowedDirectorySQL(layout, opts), ", ")+"]",
	)

	attachOpts := fmt.Sprintf(
		"DATA_PATH %s, DATA_INLINING_ROW_LIMIT %d, AUTOMATIC_MIGRATION false",
		quoteSQLString(dataPath),
		opts.DataInliningRowLimit,
	)
	if remote.Enabled {
		// catalog 可能记录了旧 data_path；冷启动用 OVERRIDE 对齐当前 bucket/prefix。
		attachOpts += ", OVERRIDE_DATA_PATH true"
	}
	attach := fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS %s (%s)",
		filepathToSlash(layout.CatalogFile),
		quoteIdent(alias),
		attachOpts,
	)
	out = append(out, attach, "USE "+quoteIdent(alias))
	out = append(out, optionSQL(alias, opts)...)
	// 本地盘：ATTACH 后关闭外部访问，仅靠 allowed_directories 白名单。
	// 远端 S3 DATA_PATH：写 parquet 仍需外部访问；关闭会导致
	// "file system operations are disabled by configuration"。
	if !remote.Enabled {
		out = append(out, "SET enable_external_access = false")
	}
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

func validateRemoteDataPath(ctx context.Context, db *sql.DB, alias, expectURI string) error {
	settings, err := ListSettings(ctx, db, alias)
	if err != nil {
		return err
	}
	for _, s := range settings {
		if strings.EqualFold(s.Key, "data_path") {
			got := strings.TrimSpace(s.Value)
			want := strings.TrimSpace(expectURI)
			if got != "" && got != want && !strings.HasPrefix(got, want) && !strings.HasPrefix(want, got) {
				return fmt.Errorf("ducklake: data_path mismatch: catalog=%q expect=%q", got, want)
			}
			return nil
		}
	}
	return nil
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

func summarizeSQL(q string) string {
	q = strings.TrimSpace(q)
	// 避免把 SECRET 凭据打进错误摘要
	upper := strings.ToUpper(q)
	if strings.Contains(upper, "SECRET") && strings.Contains(upper, "KEY_ID") {
		return "CREATE OR REPLACE SECRET ..."
	}
	if len(q) > 80 {
		return q[:80] + "..."
	}
	return q
}



// AfterWrite 在 Registry 写成功后推进 CatalogSyncer 水位。
func (f *Factory) AfterWrite(ctx context.Context, db catalog.Database, sqlDB *sql.DB) error {
	if f == nil || f.Syncer == nil {
		return nil
	}
	alias := f.Options.normalized().LakeAlias
	snap, err := CurrentSnapshot(ctx, sqlDB, alias)
	if err != nil {
		return err
	}
	f.Syncer.MarkDirty(db.ID, snap)
	return nil
}

// BeforeClose 在关闭连接前强制同步并解绑。
func (f *Factory) BeforeClose(ctx context.Context, dbID string, sqlDB *sql.DB) error {
	_ = sqlDB
	if f == nil || f.Syncer == nil {
		return nil
	}
	if bs, ok := f.Syncer.(BindingSyncer); ok {
		return bs.Unbind(ctx, dbID)
	}
	return f.Syncer.Flush(ctx, dbID)
}

// DurabilityFor 返回写响应应声明的持久化级别（§4.4）。
func (f *Factory) DurabilityFor(dbID string) string {
	if f == nil || !f.Remote.Enabled {
		return DurabilityCommittedLocal
	}
	cs, ok := f.Syncer.(*CatalogSyncer)
	if !ok || cs == nil {
		return DurabilityCommittedLocal
	}
	if f.Options.CatalogSync.Mode == "sync_on_commit" && cs.SyncLag(dbID) == 0 && cs.LastSynced(dbID) > 0 {
		return DurabilitySyncedS3
	}
	return DurabilityCommittedLocal
}

// SnapshotStatus 返回已同步快照与滞后（管理面）。
func (f *Factory) SnapshotStatus(dbID string) (lastSynced, lag int64) {
	if f == nil || f.Syncer == nil {
		return 0, 0
	}
	lastSynced = f.Syncer.LastSynced(dbID)
	if cs, ok := f.Syncer.(*CatalogSyncer); ok {
		lag = cs.SyncLag(dbID)
	}
	return lastSynced, lag
}
