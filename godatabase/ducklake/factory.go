// 文件说明：factory.go — godatabase/ducklake 中 Factory、Open 等符号的实现
//
// 用法
//
//	(f *Factory) Open(ctx context.Context, db godatabase.Spec, mode ...  // Open 打开（或创建）指定 logical database 的 DuckLake 连接
//	(l Layout) WALFile(engine string) string  // WALFile 返回 duckdb 引擎的 WAL 路径（其他引擎为空）
//	同包调用：Close() → catalog_syncer.go, extension.go 等；Open() → catalog_syncer.go, inspect.go 等；layoutForEngine() → catalog_syncer.go, local_state.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/factory.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/blob, godatabase/sqlx
//	[同包] catalog_syncer.go, errors.go, extension.go, gated_conn.go, inspect.go 等 14 个
//	[关键引用] Close() ← catalog_syncer.go, extension.go, gated_conn.go
//
// 主要符号
//
//	type Factory struct {…}
//	(f *Factory) Open(ctx context.Context, db godatabase.Spec, mode ...
//	type Layout struct {…}
//	(l Layout) WALFile(engine string) string
//	(f *Factory) AfterWrite(ctx context.Context, db godatabase.Spec, sqlDB *sql.DB) ...
//	(f *Factory) BeforeClose(ctx context.Context, dbID string, sqlDB *sql.DB) error
//	... 另有 28 个顶层符号

package ducklake

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"go.uber.org/zap"
)

// Factory 实现 godatabase.Factory：每库一个 DuckDB 实例（:memory: + ATTACH DuckLake）。
// 本地模式（Remote.Enabled=false）下 catalog 与 data 都落在 DataRoot；
// 远端模式（Enabled=true）下 DATA_PATH 指向 S3，CacheDir 只放 catalog 工作副本与 tmp。
type Factory struct {
	CacheDir string
	// DataRoot 是本地持久根（storage.data_dir）。为空时退化为 CacheDir（仅测试用）。
	DataRoot string
	Options  Options
	Syncer   Syncer
	Remote   RemoteStorage
	Blobs    blob.Store
	Logger   godatabase.Logger
	// Observer 接收 catalog 指标；可为 nil。
	Observer godatabase.Observer
}

// layoutFor 返回该库在当前存储模式下的本地目录布局。
func (f *Factory) layoutFor(db godatabase.Spec, engine string) Layout {
	return layoutAt(f.DataRoot, f.CacheDir, db.ProjectID, db.ID, engine, !f.Remote.Enabled)
}

// layoutAt 计算单库目录布局。
//
//	dataLocal=true （本地模式）：catalog 与 data 落在 {dataRoot}/{project}/databases/{id}/，
//	                             tmp 落在 {cacheDir}/{id}/tmp。
//	dataLocal=false（远端模式）：catalog 工作副本与 tmp 都在 cacheDir（可从远端重建），
//	                             data 由 S3 URI 提供。
func layoutAt(dataRoot, cacheDir, projectID, databaseID, engine string, dataLocal bool) Layout {
	ext, _ := blob.CatalogFileExt(engine)
	workRoot := filepath.Join(cacheDir, databaseID)
	if dataLocal && dataRoot != "" && projectID != "" {
		workRoot = filepath.Join(dataRoot, projectID, "databases", databaseID)
	}
	return Layout{
		Root:        workRoot,
		CatalogFile: filepath.Join(workRoot, "catalog", "catalog"+ext),
		DataDir:     filepath.Join(workRoot, "data"),
		TempDir:     filepath.Join(cacheDir, databaseID, "tmp"),
	}
}

func (f *Factory) observeOpen(stage string, duration time.Duration, err error) {
	if f.Logger == nil {
		return
	}
	fields := []zap.Field{zap.String("stage", stage), zap.Duration("duration", duration)}
	if err != nil && !IsLocalAhead(err) {
		fields = append(fields, zap.Bool("error", true))
	}
	f.Logger.Debug("ducklake open stage", fields...)
}

// openGuardMu/openGuardPaths 是进程级「同一 catalog 文件只允许打开一次」守卫
// （ducklake-duckdb-catalog-plan §7-1）：POSIX 文件锁以进程为单位，
// 拦不住同进程的第二次打开，DuckDB 会导致文件损坏。放在包级而非 Factory
// 字段上，保证用户库/系统库等多个 Factory 实例之间同样互斥。
var (
	openGuardMu    sync.Mutex
	openGuardPaths = map[string]bool{}
)

// markOpening 登记 catalog 文件路径；已登记时返回错误。
func (f *Factory) markOpening(absPath string) error {
	openGuardMu.Lock()
	defer openGuardMu.Unlock()
	if openGuardPaths[absPath] {
		return fmt.Errorf("ducklake: catalog file %s is already opened by this process (POSIX lock cannot guard same-process double-open)", absPath)
	}
	openGuardPaths[absPath] = true
	return nil
}

// unmarkOpening 注销（关闭后允许重新打开）。
func (f *Factory) unmarkOpening(absPath string) {
	openGuardMu.Lock()
	defer openGuardMu.Unlock()
	delete(openGuardPaths, absPath)
}

// ensureOpening 返回目录布局并登记守卫；返回的释放函数在连接关闭后调用。
func (f *Factory) ensureOpening(layout Layout) (release func(), err error) {
	abs, aerr := filepath.Abs(layout.CatalogFile)
	if aerr != nil {
		abs = layout.CatalogFile
	}
	if err := f.markOpening(abs); err != nil {
		return nil, err
	}
	return func() { f.unmarkOpening(abs) }, nil
}

// Open 打开（或创建）指定 logical database 的 DuckLake 连接。
func (f *Factory) Open(ctx context.Context, db godatabase.Spec, mode godatabase.AccessMode) (*sql.DB, error) {
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
	engine := opts.CatalogEngine
	if !IsValidCatalogEngine(engine) {
		return nil, fmt.Errorf("ducklake: unsupported catalog engine %q (want duckdb or sqlite)", engine)
	}
	layout := f.layoutFor(db, engine)

	// §4.4 防御：本地残留另一种引擎的 catalog 文件 → 拒绝打开。
	if err := checkForeignEngineFile(f.layoutFor(db, otherEngine(engine))); err != nil {
		return nil, err
	}
	if err := layout.ensure(); err != nil {
		return nil, fmt.Errorf("ducklake: create cache dirs: %w", err)
	}

	releaseOpen, err := f.ensureOpening(layout)
	if err != nil {
		return nil, err
	}

	if f.Remote.Enabled {
		catalogStart := time.Now()
		_, catalogErr := EnsureLocalCatalog(ctx, f.Blobs, f.Remote, f.CacheDir, db, engine)
		f.observeOpen("catalog_reconcile", time.Since(catalogStart), catalogErr)
		if err := catalogErr; err != nil {
			if IsLocalAhead(err) {
				// 本地领先远端（§3.8）：保留本地版本，告警并计数；
				// Open 成功后由调用方触发 Sync 推上去。
				if f.Logger != nil {
					f.Logger.Warn("ducklake local catalog ahead of remote; keep local copy",
						zap.String("database_id", db.ID), zap.Error(err))
				}
				if f.Observer != nil {
					f.Observer.CatalogLocalAhead()
				}
			} else {
				releaseOpen()
				return nil, err
			}
		}
	}

	dataPath, err := f.dataPathFor(db, layout)
	if err != nil {
		releaseOpen()
		return nil, err
	}

	boot := buildBootSQL(layout, opts, f.Remote, dataPath)
	// 配置锁与 INSTALL/SET/ATTACH 只允许在第一条连接上执行；
	// ATTACH 在共享 DuckDB 实例内全局可见，其余连接只需设置 USE。
	var bootMu sync.Mutex
	booted := false
	connector, err := duckdb.NewConnector("", func(execer driver.ExecerContext) error {
		bootMu.Lock()
		defer bootMu.Unlock()
		queries := boot
		if booted {
			queries = []string{"USE " + sqlx.Ident(opts.LakeAlias)}
		}
		initCtx := ctx
		if booted {
			// 新连接可能在 Open 返回很久后创建，不能复用已结束的启动 context。
			initCtx = context.Background()
		}
		for _, q := range queries {
			if _, err := execer.ExecContext(initCtx, q, nil); err != nil {
				return fmt.Errorf("ducklake: boot %s: %w", summarizeSQL(q), err)
			}
		}
		booted = true
		return nil
	})
	if err != nil {
		releaseOpen()
		return nil, fmt.Errorf("ducklake: new connector: %w", err)
	}

	// 守卫释放挂在 connector.Close 上：sql.DB.Close 在所有连接关闭后调用它，
	// 覆盖 BeforeClose 与失租 closeNoFlush 两条路径；once 保证只释放一次。
	// 写锁与连接池绑定：读可并行，写事务与 sync flush/copy 互斥。
	// 系统库额外对 Transaction conflict 做有限次重放（提交失败，语句未生效）。
	gate := &writeGate{retryConflicts: db.Kind == godatabase.KindSystem}
	gc := &guardedConnector{Connector: connector, release: releaseOpen, gate: gate}
	sqlDB := sql.OpenDB(gc)
	gc.db = sqlDB
	registerDBGate(sqlDB, gate)
	// 多连接共享一个 DuckDB 实例；每条连接均执行 ATTACH/USE，配置只在首连接设置。
	// 不预建空闲连接，避免大量库同时打开时占用过多内存。
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(2)

	bootStart := time.Now()
	pingErr := sqlDB.PingContext(ctx)
	f.observeOpen("boot_ping", time.Since(bootStart), pingErr)
	if pingErr != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ducklake: ping: %w", pingErr)
	}

	checkStart := time.Now()
	if _, err := AssertDuckDBVersion(ctx, sqlDB); err != nil {
		f.observeOpen("extension_check", time.Since(checkStart), err)
		_ = sqlDB.Close()
		return nil, err
	}
	if err := AssertExtensionsLoaded(ctx, sqlDB, engine); err != nil {
		f.observeOpen("extension_check", time.Since(checkStart), err)
		_ = sqlDB.Close()
		return nil, err
	}
	f.observeOpen("extension_check", time.Since(checkStart), nil)

	if f.Remote.Enabled {
		pathStart := time.Now()
		pathErr := validateRemoteDataPath(ctx, sqlDB, opts.LakeAlias, dataPath)
		f.observeOpen("data_path_check", time.Since(pathStart), pathErr)
		if pathErr != nil {
			_ = sqlDB.Close()
			return nil, pathErr
		}
	}

	// §3.5：Bind 携带本 Factory 的缓存目录，同步器的水位/暂存文件按库
	// 定位（系统库与用户库目录布局不同，混用会导致水位错写/丢失保护）。
	if ls, ok := f.Syncer.(interface {
		BindWithCacheDir(dbID, cacheDir string, sqlDB *sql.DB, meta godatabase.Spec, alias string)
	}); ok {
		ls.BindWithCacheDir(db.ID, f.CacheDir, sqlDB, db, opts.LakeAlias)
	} else if bs, ok := f.Syncer.(BindingSyncer); ok {
		bs.Bind(db.ID, sqlDB, db, opts.LakeAlias)
	}

	if f.Logger != nil {
		f.Logger.Info("ducklake database opened",
			zap.String("database_id", db.ID),
			zap.String("mode", mode.String()),
			zap.String("alias", opts.LakeAlias),
			zap.String("catalog_engine", engine),
			zap.Bool("remote", f.Remote.Enabled),
		)
	}
	return sqlDB, nil
}

// guardedConnector 包装 DuckDB connector：sql.DB.Close 会调用实现了
// io.Closer 的 connector 的 Close（Go ≥1.17），此时先关闭底层数据库，
// 再释放进程级打开守卫（只释放一次）。
type guardedConnector struct {
	driver.Connector
	release func()
	once    sync.Once
	gate    *writeGate
	db      *sql.DB
}

func (c *guardedConnector) Connect(ctx context.Context) (driver.Conn, error) {
	raw, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return newGatedConn(raw, c.gate), nil
}

func (c *guardedConnector) Close() error {
	var err error
	if cl, ok := c.Connector.(io.Closer); ok {
		err = cl.Close()
	}
	c.once.Do(func() {
		unregisterDBGate(c.db)
		if c.release != nil {
			c.release()
		}
	})
	return err
}

func (f *Factory) dataPathFor(db godatabase.Spec, layout Layout) (string, error) {
	if !f.Remote.Enabled {
		return layout.dataPathArg(), nil
	}
	return buildDataURI(f.Remote, db.ProjectID, db.ID)
}

func buildBootSQL(layout Layout, opts Options, remote RemoteStorage, dataPath string) []string {
	alias := opts.LakeAlias
	out := extensionBootSQL(opts)
	if remote.Enabled {
		out = append(out, buildCreateSecretSQL(remote))
	}
	out = append(out,
		"SET memory_limit = "+sqlx.Literal(opts.MemoryLimit),
		"SET threads = "+strconv.Itoa(opts.Threads),
		"SET temp_directory = "+sqlPath(layout.TempDir),
		"SET allowed_directories = ["+strings.Join(allowedDirectorySQL(layout, opts), ", ")+"]",
	)

	// 远端模式允许内联小写入；上传 catalog 快照前由同步器刷出内联数据。

	out = append(out, bootAttachSQL(layout, opts, remote, dataPath), "USE "+sqlx.Ident(alias))
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

func bootAttachSQL(layout Layout, opts Options, remote RemoteStorage, dataPath string) string {
	attachOpts := fmt.Sprintf(
		"DATA_PATH %s, DATA_INLINING_ROW_LIMIT %d, AUTOMATIC_MIGRATION false",
		sqlx.Literal(dataPath), opts.DataInliningRowLimit,
	)
	if remote.Enabled {
		attachOpts += ", OVERRIDE_DATA_PATH true"
	}
	engine := NormalizeCatalogEngine(opts.CatalogEngine)
	dsn := "ducklake:" + filepathToSlash(layout.CatalogFile)
	if p := enginePrefix(engine); p != "" {
		dsn = "ducklake:" + p + ":" + filepathToSlash(layout.CatalogFile)
	}
	return fmt.Sprintf("ATTACH '%s' AS %s (%s)", dsn, sqlx.Ident(opts.LakeAlias), attachOpts)
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
	target := sqlx.Ident(alias)
	out := []string{
		fmt.Sprintf("CALL %s.set_option('parquet_compression', %s)", target, sqlx.Literal(opts.ParquetCompression)),
		fmt.Sprintf("CALL %s.set_option('target_file_size', %s)", target, sqlx.Literal(opts.TargetFileSize)),
		fmt.Sprintf("CALL %s.set_option('data_inlining_row_limit', %s)", target, sqlx.Literal(strconv.Itoa(opts.DataInliningRowLimit))),
	}
	if opts.RequireCommitMessage {
		out = append(out, fmt.Sprintf("CALL %s.set_option('require_commit_message', 'true')", target))
	}
	return out
}

// enginePrefix 返回 ducklake DSN 中引擎段：duckdb 为空（原生默认），sqlite 为 "sqlite"。
func enginePrefix(engine string) string {
	if NormalizeCatalogEngine(engine) == EngineSQLite {
		return EngineSQLite
	}
	return ""
}

// otherEngine 返回另一种引擎名（duckdb ↔ sqlite）。
func otherEngine(engine string) string {
	if NormalizeCatalogEngine(engine) == EngineSQLite {
		return EngineDuckDB
	}
	return EngineSQLite
}

// checkForeignEngineFile 检查另一种引擎的本地 catalog 文件残留（§4.4）。
func checkForeignEngineFile(foreign Layout) error {
	if _, err := os.Stat(foreign.CatalogFile); err == nil {
		return fmt.Errorf("%w: local catalog file of a different engine exists at %s (config engine mismatch or leftover from previous engine)",
			ErrCatalogEngineMismatch, foreign.CatalogFile)
	}
	return nil
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

// —— 本地缓存目录布局（原 paths.go）——

// Layout 描述单个 logical database 在本地缓存中的目录布局。
// 与 S3 前缀镜像：catalog/catalog.{ducklake|sqlite} + data/。
// duckdb 引擎另有同名 .wal（DuckDB 活文件旁的预写日志）。
type Layout struct {
	Root        string
	CatalogFile string
	DataDir     string
	TempDir     string
}

func layoutFor(cacheDir, databaseID string) Layout {
	return layoutAt("", cacheDir, "", databaseID, EngineDuckDB, false)
}

// layoutForEngine 按引擎生成布局（catalog 工作副本在 cacheDir）；engine 必须先经 NormalizeCatalogEngine。
func layoutForEngine(cacheDir, databaseID, engine string) Layout {
	return layoutAt("", cacheDir, "", databaseID, engine, false)
}

// WALFile 返回 duckdb 引擎的 WAL 路径（其他引擎为空）。
func (l Layout) WALFile(engine string) string {
	if NormalizeCatalogEngine(engine) != EngineDuckDB {
		return ""
	}
	return l.CatalogFile + ".wal"
}

func (l Layout) ensure() error {
	for _, dir := range []string{
		filepath.Dir(l.CatalogFile),
		l.DataDir,
		l.TempDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (l Layout) dataPathArg() string {
	p := filepath.ToSlash(l.DataDir)
	if p != "" && p[len(p)-1] != '/' {
		p += "/"
	}
	return p
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

// AfterWrite 在 Registry 写成功后标记待同步水位；写路径不访问对象存储。
func (f *Factory) AfterWrite(ctx context.Context, db godatabase.Spec, sqlDB *sql.DB) error {
	if f == nil || f.Syncer == nil {
		return nil
	}
	alias := f.Options.normalized().LakeAlias
	snap, err := CurrentSnapshot(ctx, sqlDB, alias)
	if err != nil {
		return err
	}
	f.Syncer.MarkDirty(db.ID, snap)

	// 保留每次提交后的本地水位：重启时不得以过期状态覆盖尚未同步的 catalog。
	if f.CacheDir != "" {
		st, _ := ReadLocalState(filepath.Dir(f.layoutFor(db, f.Options.normalized().CatalogEngine).CatalogFile), db.ID)
		if snap > st.SnapshotID {
			st.SnapshotID = snap
			if err := SaveLocalState(filepath.Dir(f.layoutFor(db, f.Options.normalized().CatalogEngine).CatalogFile), db.ID, st); err != nil && f.Logger != nil {
				f.Logger.Warn("ducklake local-state save after write failed",
					zap.String("database_id", db.ID), zap.Error(err))
			}
		}
	}
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
func (f *Factory) DurabilityFor(_ string) string {
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
