// 文件说明：factory.go — 每项目一个 DuckDB/SQLite 主文件的用户数据库运行时
//
// 用法
//
//	(f *Factory) DatabaseFilePath(meta godatabase.Spec) (string, error)
//	(f *Factory) Open(ctx context.Context, meta godatabase.Spec, mode ...  // Open 打开项目单文件数据库
//	同包调用：filePath() → path.go, remote.go 等；Close() → remote.go, snapshot.go；Open() → remote.go
//
// 调用架构
//
//	[上游] internal/app, internal/backup
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/filedb/factory.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/sqlx
//	[同包] path.go, remote.go, restore.go, restore_remote.go, snapshot.go 等 6 个
//	[关键引用] filePath() ← path.go, remote.go, restore.go
//
// 主要符号
//
//	type Options struct {…}
//	type Factory struct {…}
//	(f *Factory) DatabaseFilePath(meta godatabase.Spec) (string, error)
//	(f *Factory) Open(ctx context.Context, meta godatabase.Spec, mode ...
//	(f *Factory) AfterWrite(_ context.Context, meta godatabase.Spec, _ *sql.DB) error
//	(f *Factory) BeforeClose(ctx context.Context, projectID string, _ *sql.DB) error
//	... 另有 19 个顶层符号
//	Package filedb 实现每项目一个 DuckDB/SQLite 主文件的用户数据库运行时。

package filedb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	duckdb "github.com/duckdb/duckdb-go/v2"
	sqlite3 "github.com/uglyer/go-sqlite3"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
)

type Options struct {
	MemoryLimit string
	Threads     int
}

type Factory struct {
	DataRoot string
	CacheDir string
	Remote   bool
	Options  Options
	Syncer   Syncer
	Logger   godatabase.Logger
}

var openFiles = struct {
	sync.Mutex
	paths map[string]bool
}{paths: map[string]bool{}}

func markOpen(filePath string) error {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}
	openFiles.Lock()
	defer openFiles.Unlock()
	if openFiles.paths[abs] {
		return fmt.Errorf("filedb: project database file is already open")
	}
	openFiles.paths[abs] = true
	return nil
}

func unmarkOpen(filePath string) {
	abs, _ := filepath.Abs(filePath)
	openFiles.Lock()
	delete(openFiles.paths, abs)
	openFiles.Unlock()
}

func (f *Factory) filePath(meta godatabase.Spec) (string, error) {
	return localDatabaseFile(f.DataRoot, f.CacheDir, meta.ProjectID, meta.DatabasePath, f.Remote)
}

func (f *Factory) DatabaseFilePath(meta godatabase.Spec) (string, error) {
	return f.filePath(meta)
}

// Open 打开项目单文件数据库。返回的错误文本已脱敏：驱动与 os 错误常带物理路径，
// 而未映射错误会被 API 层原文写入日志。
func (f *Factory) Open(ctx context.Context, meta godatabase.Spec, mode godatabase.AccessMode) (*sql.DB, error) {
	sqlDB, err := f.open(ctx, meta, mode)
	if err != nil {
		return nil, f.redactError(err, meta)
	}
	return sqlDB, nil
}

type redactedError struct{ msg string }

func (e *redactedError) Error() string { return e.msg }

// redactError 把错误文本中的数据根、缓存目录与项目文件路径替换为占位符。
// 保留 context 取消语义，其余错误链有意断开，避免 %w 链重新暴露原文。
func (f *Factory) redactError(err error, meta godatabase.Spec) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var paths []string
	add := func(p string) {
		if p == "" {
			return
		}
		paths = append(paths, p)
		if r, e := filepath.EvalSymlinks(p); e == nil && r != p {
			paths = append(paths, r)
		}
	}
	if fp, e := f.filePath(meta); e == nil {
		add(fp)
	}
	add(meta.DatabasePath)
	add(f.DataRoot)
	add(f.CacheDir)
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	msg := err.Error()
	for _, p := range paths {
		msg = strings.ReplaceAll(msg, p, "<redacted>")
	}
	return &redactedError{msg: msg}
}

func (f *Factory) open(ctx context.Context, meta godatabase.Spec, mode godatabase.AccessMode) (*sql.DB, error) {
	if meta.IsSystem() {
		return nil, fmt.Errorf("filedb: system database must use system factory")
	}
	engine, err := godatabase.EngineFromPath(meta.DatabasePath)
	if err != nil {
		return nil, err
	}
	filePath, err := f.filePath(meta)
	if err != nil {
		return nil, err
	}
	if err := recoverRestoreFiles(filePath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(f.CacheDir, "projects", meta.ProjectID, "tmp"), 0o755); err != nil {
		return nil, fmt.Errorf("filedb: create temp directory: %w", err)
	}
	if err := markOpen(filePath); err != nil {
		return nil, err
	}
	release := sync.OnceFunc(func() { unmarkOpen(filePath) })
	if f.Syncer != nil {
		if err := f.Syncer.Restore(ctx, meta, filePath); err != nil {
			release()
			return nil, err
		}
	}

	var base driver.Connector
	var initSQL []string
	switch engine {
	case godatabase.EngineDuckDB:
		base, err = duckdb.NewConnector(filePath, nil)
		if err == nil {
			opts := f.Options
			if opts.MemoryLimit == "" {
				opts.MemoryLimit = "512MB"
			}
			if opts.Threads <= 0 {
				opts.Threads = 2
			}
			initSQL = []string{
				"SET memory_limit = " + sqlx.Literal(opts.MemoryLimit),
				"SET threads = " + strconv.Itoa(opts.Threads),
				"SET temp_directory = " + sqlx.Literal(filepath.Join(f.CacheDir, "projects", meta.ProjectID, "tmp")),
				"SET lock_configuration = true",
			}
		}
	case godatabase.EngineSQLite:
		base = &sqliteConnector{filePath: filePath}
		initSQL = []string{
			"PRAGMA busy_timeout = 5000",
			"PRAGMA journal_mode = WAL",
			"PRAGMA synchronous = FULL",
			"PRAGMA foreign_keys = ON",
		}
	}
	if err != nil {
		release()
		return nil, fmt.Errorf("filedb: create connector: %w", err)
	}
	connector := &trackedConnector{base: base, initSQL: initSQL, release: release}
	sqlDB := sql.OpenDB(connector)
	connector.db = sqlDB
	if engine == godatabase.EngineSQLite {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(2)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("filedb: open %s database: %w", engine, err)
	}
	if engine == godatabase.EngineSQLite {
		var result string
		if err := sqlDB.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil || result != "ok" {
			_ = sqlDB.Close()
			if err != nil {
				return nil, fmt.Errorf("filedb: sqlite integrity check: %w", err)
			}
			return nil, fmt.Errorf("filedb: sqlite integrity check failed: %s", result)
		}
	}
	if f.Syncer != nil {
		f.Syncer.Bind(meta, sqlDB, filePath)
	}
	if f.Logger != nil {
		f.Logger.Info("project database opened",
			zap.String("project_id", meta.ProjectID), zap.String("database_id", meta.ID),
			zap.String("engine", engine), zap.String("mode", mode.String()), zap.Bool("remote", f.Remote))
	}
	return sqlDB, nil
}

func (f *Factory) AfterWrite(_ context.Context, meta godatabase.Spec, _ *sql.DB) error {
	if f != nil && f.Syncer != nil {
		f.Syncer.MarkDirty(meta.ProjectID)
	}
	return nil
}

// BeforeClose 的 ID 是项目 ID；用户库物理资源均以项目为边界。
func (f *Factory) BeforeClose(ctx context.Context, projectID string, _ *sql.DB) error {
	if f == nil || f.Syncer == nil {
		return nil
	}
	return f.Syncer.Unbind(ctx, projectID)
}

func (f *Factory) SnapshotStatus(projectID string) (lastSynced, lag int64) {
	if f == nil || f.Syncer == nil {
		return 0, 0
	}
	return f.Syncer.LastSynced(projectID), f.Syncer.SyncLag(projectID)
}

// DurabilityFor 返回写响应应声明的持久化级别。项目文件始终先落本地工作副本。
func (f *Factory) DurabilityFor(string) string {
	return "committed_local"
}

func (f *Factory) PurgeProject(meta godatabase.Spec) error {
	filePath, err := f.filePath(meta)
	if err != nil {
		return err
	}
	engine, err := godatabase.EngineFromPath(meta.DatabasePath)
	if err != nil {
		return err
	}
	for _, name := range sidecarFiles(filePath, engine) {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("filedb: remove project database: %w", err)
		}
	}
	return nil
}

// PurgeProjectFiles 按项目规范位置删除两种引擎的主文件与伴生文件。
// 删除默认数据库时不再持有该库元数据，因此不依赖 DatabasePath。
func (f *Factory) PurgeProjectFiles(projectID string) error {
	for _, engine := range []string{godatabase.EngineDuckDB, godatabase.EngineSQLite} {
		databasePath, err := godatabase.PathForProject(projectID, engine)
		if err != nil {
			return err
		}
		if err := f.PurgeProject(godatabase.Spec{ProjectID: projectID, DatabasePath: databasePath}); err != nil {
			return err
		}
	}
	return nil
}

type sqliteConnector struct{ filePath string }

func (c *sqliteConnector) Connect(context.Context) (driver.Conn, error) {
	return (&sqlite3.SQLiteDriver{}).Open(c.filePath)
}
func (c *sqliteConnector) Driver() driver.Driver { return &sqlite3.SQLiteDriver{} }

type trackedConnector struct {
	base    driver.Connector
	initSQL []string
	release func()
	once    sync.Once
	db      *sql.DB
}

func (c *trackedConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	for _, query := range c.initSQL {
		if _, err := conn.(driver.ExecerContext).ExecContext(ctx, query, nil); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("filedb: initialize connection: %w", err)
		}
	}
	return conn, nil
}

func (c *trackedConnector) Driver() driver.Driver { return c.base.Driver() }

func (c *trackedConnector) Close() error {
	var err error
	if closer, ok := c.base.(io.Closer); ok {
		err = closer.Close()
	}
	c.once.Do(c.release)
	return err
}
