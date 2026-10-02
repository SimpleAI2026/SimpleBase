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

// openGuardMu/openGuardPaths 是进程级「同一 catalog 文件只允许打开一次」守卫
//（ducklake-duckdb-catalog-plan §7-1）：POSIX 文件锁以进程为单位，
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
	engine := opts.CatalogEngine
	if !IsValidCatalogEngine(engine) {
		return nil, fmt.Errorf("ducklake: unsupported catalog engine %q (want duckdb or sqlite)", engine)
	}
	layout := layoutForEngine(f.CacheDir, db.ID, engine)

	// §4.4 防御：本地残留另一种引擎的 catalog 文件 → 拒绝打开。
	if err := checkForeignEngineFile(layoutForEngine(f.CacheDir, db.ID, otherEngine(engine))); err != nil {
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
		if _, err := EnsureLocalCatalog(ctx, f.Blobs, f.Remote, f.CacheDir, db, engine); err != nil {
			if IsLocalAhead(err) {
				// 本地领先远端（§3.8）：保留本地版本，告警并计数；
				// Open 成功后由调用方触发 Sync 推上去。
				if f.Logger != nil {
					f.Logger.Warn("ducklake local catalog ahead of remote; keep local copy",
						zap.String("database_id", db.ID), zap.Error(err))
				}
				if f.Metrics != nil && f.Metrics.CatalogLocalAhead != nil {
					f.Metrics.CatalogLocalAhead.Inc()
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
	connector, err := duckdb.NewConnector("", func(execer driver.ExecerContext) error {
		for _, q := range boot {
			if _, err := execer.ExecContext(ctx, q, nil); err != nil {
				return fmt.Errorf("ducklake: boot %s: %w", summarizeSQL(q), err)
			}
		}
		return nil
	})
	if err != nil {
		releaseOpen()
		return nil, fmt.Errorf("ducklake: new connector: %w", err)
	}

	// 守卫释放挂在 connector.Close 上：sql.DB.Close 在所有连接关闭后调用它，
	// 覆盖 BeforeClose 与失租 closeNoFlush 两条路径；once 保证只释放一次。
	sqlDB := sql.OpenDB(&guardedConnector{Connector: connector, release: releaseOpen})
	// §7.2 P6 实测结论：连接上限必须保持 1——boot 序列含 ATTACH+USE+
	// SET lock_configuration，均为连接级语义，第二连接要么配置被锁
	//（memory_limit 不可 SET）要么丢失 lake search_path；且 DuckLake
	// 是单写者模型。系统库的并发瓶颈改由缓存（P1）与异步写（P3）消除。
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
	if err := AssertExtensionsLoaded(ctx, sqlDB, engine); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	if f.Remote.Enabled {
		if err := validateRemoteDataPath(ctx, sqlDB, opts.LakeAlias, dataPath); err != nil {
			_ = sqlDB.Close()
			return nil, err
		}
	}

	// §3.5：Bind 携带本 Factory 的缓存目录，同步器的水位/暂存文件按库
	// 定位（系统库与用户库目录布局不同，混用会导致水位错写/丢失保护）。
	if ls, ok := f.Syncer.(interface {
		BindWithCacheDir(dbID, cacheDir string, sqlDB *sql.DB, meta catalog.Database, alias string)
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
}

func (c *guardedConnector) Close() error {
	var err error
	if cl, ok := c.Connector.(io.Closer); ok {
		err = cl.Close()
	}
	c.once.Do(func() {
		if c.release != nil {
			c.release()
		}
	})
	return err
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

	// 远端模式强制关闭 data inlining（multi-instance-consistency-plan §4.2）：
	// 内联会把用户数据行写进 catalog，使 catalog 覆盖/丢失从「丢指针」
	// 升级为「丢数据」（§3.1 放大因素 1）。本地/DevMode 保留配置值。
	if remote.Enabled {
		opts.DataInliningRowLimit = 0
	}

	attachOpts := fmt.Sprintf(
		"DATA_PATH %s, DATA_INLINING_ROW_LIMIT %d, AUTOMATIC_MIGRATION false",
		quoteSQLString(dataPath),
		opts.DataInliningRowLimit,
	)
	if remote.Enabled {
		// catalog 可能记录了旧 data_path；冷启动用 OVERRIDE 对齐当前 bucket/prefix。
		attachOpts += ", OVERRIDE_DATA_PATH true"
	}
	engine := NormalizeCatalogEngine(opts.CatalogEngine)
	// duckdb: 'ducklake:{path}'；sqlite: 'ducklake:sqlite:{path}'
	dsn := "ducklake:" + filepathToSlash(layout.CatalogFile)
	if p := enginePrefix(engine); p != "" {
		dsn = "ducklake:" + p + ":" + filepathToSlash(layout.CatalogFile)
	}
	attach := fmt.Sprintf(
		"ATTACH '%s' AS %s (%s)",
		dsn,
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
	return layoutForEngine(cacheDir, databaseID, EngineDuckDB)
}

// layoutForEngine 按引擎生成布局；engine 必须先经 NormalizeCatalogEngine。
func layoutForEngine(cacheDir, databaseID, engine string) Layout {
	root := filepath.Join(cacheDir, databaseID)
	ext, _ := objectstore.CatalogFileExt(engine)
	return Layout{
		Root:        root,
		CatalogFile: filepath.Join(root, "catalog", "catalog"+ext),
		DataDir:     filepath.Join(root, "data"),
		TempDir:     filepath.Join(root, "tmp"),
	}
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



// AfterWrite 在 Registry 写成功后推进 CatalogSyncer 水位与 local-state.json
//（multi-instance-consistency-plan §4.3：EnsureLocalCatalog 依赖它判定本地领先）。
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

	// 推进本地水位文件（失败不阻塞写路径）。
	if f.CacheDir != "" {
		st, _ := ReadLocalState(f.CacheDir, db.ID)
		if snap > st.SnapshotID {
			st.SnapshotID = snap
			if err := SaveLocalState(f.CacheDir, db.ID, st); err != nil && f.Logger != nil {
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
