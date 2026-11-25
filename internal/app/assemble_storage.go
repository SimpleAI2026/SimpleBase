// 文件说明：存储装配阶段：文件存储、用户库/系统库 factory、DuckLake 选项
package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/cache"
	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/api"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"go.uber.org/zap"
	"time"
)

// assembleFileStore 装配用户文件存储：按 storage.files 选择对象存储或本地目录。
// 位于 writable 提前返回之前，只读实例同样装配 fileStore。
func (a *App) assembleFileStore(ctx context.Context, cfg config.Config, s3cfg objectstore.Config, filesRemote bool) error {
	switch {
	case filesRemote:
		if s3cfg.Bucket == "" {
			return fmt.Errorf("storage.files=s3 requires s3.bucket")
		}
		if a.objectStore == nil {
			store, err := objectstore.NewClient(ctx, s3cfg, a.logger, a.metrics)
			if err != nil {
				return fmt.Errorf("create objectstore client: %w", err)
			}
			a.objectStore = store
		}
		fs, err := objectstore.NewS3FileStore(ctx, s3cfg, cfg.S3.Prefix, a.logger)
		if err != nil {
			return fmt.Errorf("create s3 file store: %w", err)
		}
		a.fileStore = fs
	default:
		fs, err := objectstore.NewLocalFileStore(cfg.Storage.DataDir)
		if err != nil {
			return fmt.Errorf("create local file store: %w", err)
		}
		a.fileStore = fs
		a.logger.Info("user files stored on local disk",
			zap.String("data_dir", cfg.Storage.DataDir))
	}
	return nil
}

// newUserDatabaseFactory 返回项目单文件用户库工厂（DuckDB / SQLite）。
// dbRemote 为真时使用本地工作副本 + 不可变快照同步；本地模式直接持久化。
func (a *App) newUserDatabaseFactory(cacheDir string) godatabase.Factory {
	cfg := a.cfg
	dbRemote := cfg.IsDBRemote()
	var syncer filedb.Syncer = filedb.NewLocalSyncer()
	if dbRemote && cfg.S3.Bucket != "" && a.objectStore != nil {
		if blobs, ok := a.objectStore.(objectstore.BlobStore); ok {
			fs := filedb.NewFileSyncer(blobs, objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix}.DB(), cacheDir,
				cfg.Database.DuckLake.CatalogSync.Interval, cfg.Database.DuckLake.CatalogSync.KeepVersions, dbbridge.LoggerFrom(a.logger))
			fs.Start(context.Background())
			syncer = fs
			a.registerCloser(fs)
		}
	}
	f := &filedb.Factory{
		DataRoot: cfg.Storage.DataDir,
		CacheDir: cacheDir,
		Remote:   dbRemote,
		Options: filedb.Options{
			MemoryLimit: cfg.Database.DuckLake.MemoryLimit,
			Threads:     cfg.Database.DuckLake.Threads,
		},
		Syncer: syncer,
		Logger: dbbridge.LoggerFrom(a.logger),
	}
	a.fileFactory = f
	a.fileSyncer = syncer
	return f
}

// newSystemDatabaseFactory 返回系统库专用 DuckLake Factory；与用户库无共享可变状态。
// 系统库 catalog 归到一个独立的缓存子目录，避免与用户库混用。
func (a *App) newSystemDatabaseFactory() *ducklake.Factory {
	cfg := a.cfg
	opts := duckLakeOptions(cfg.Database.DuckLake)
	f := &ducklake.Factory{
		CacheDir: filepath.Join(cfg.Database.CacheDir, "system", "dbs"),
		DataRoot: cfg.Storage.DataDir,
		Options:  opts,
		Logger:   dbbridge.LoggerFrom(a.logger),
		Observer: dbbridge.ObserverFrom(a.metrics),
	}
	if cfg.IsDBRemote() && cfg.S3.Bucket != "" {
		remote := ducklake.RemoteStorage{
			Enabled:        true,
			Endpoint:       cfg.S3.Endpoint,
			Region:         cfg.S3.Region,
			Bucket:         cfg.S3.Bucket,
			RootPrefix:     cfg.S3.Prefix,
			AccessKey:      cfg.S3.AccessKey,
			SecretKey:      cfg.S3.SecretKey,
			ForcePathStyle: cfg.S3.ForcePathStyle,
		}
		var blobs objectstore.BlobStore
		if a.objectStore != nil {
			if b, ok := a.objectStore.(objectstore.BlobStore); ok {
				blobs = b
			}
		}
		if blobs != nil {
			cs := ducklake.NewCatalogSyncer(blobs, remote, f.CacheDir, opts.CatalogSync, opts.CatalogEngine, dbbridge.LoggerFrom(a.logger), dbbridge.ObserverFrom(a.metrics))
			f.Remote = remote
			f.Blobs = blobs
			f.Syncer = cs
			a.catalogSyncer = cs
			a.registerCloser(syncerCloser{cs: cs})
		}
	}
	if f.Syncer == nil {
		f.Syncer = ducklake.NewLocalSyncer()
	}
	return f
}

// snapshotFromFileFactory 返回项目文件同步水位（databaseID 或 projectID 均可）。
func snapshotFromFileFactory(f *filedb.Factory, databaseID string) *api.DatabaseSnapshot {
	last, lag := f.SnapshotStatus(databaseID)
	if last == 0 && lag == 0 {
		return nil
	}
	return &api.DatabaseSnapshot{LastSyncedSnapshot: last, SyncLag: lag}
}

func duckLakeOptions(cfg config.DuckLakeConfig) ducklake.Options {
	return ducklake.Options{
		CatalogEngine:        cfg.CatalogEngine,
		MemoryLimit:          cfg.MemoryLimit,
		Threads:              cfg.Threads,
		ExtensionDir:         cfg.ExtensionDir,
		DataInliningRowLimit: cfg.DataInliningRowLimit,
		ParquetCompression:   cfg.ParquetCompression,
		TargetFileSize:       cfg.TargetFileSize,
		RequireCommitMessage: cfg.RequireCommitMessage,
		CatalogSync: ducklake.CatalogSyncOptions{
			Mode:         cfg.CatalogSync.Mode,
			Debounce:     cfg.CatalogSync.Debounce,
			Interval:     cfg.CatalogSync.Interval,
			MaxLag:       cfg.CatalogSync.MaxLag,
			KeepVersions: cfg.CatalogSync.KeepVersions,
		},
		Maintenance: ducklake.MaintenanceOptions{
			CheckpointInterval:     cfg.Maintenance.CheckpointInterval,
			ExpireOlderThan:        cfg.Maintenance.ExpireOlderThan,
			DeleteOlderThan:        cfg.Maintenance.DeleteOlderThan,
			RewriteDeleteThreshold: cfg.Maintenance.RewriteDeleteThreshold,
		},
	}
}

func boolGauge(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// connectObjectStore 在可写实例且持久层落到 S3 时创建对象存储客户端并跑条件写探针。
// 探针失败且 instance.lease.enabled=true 时拒绝启动（§3.3）。
func (a *App) connectObjectStore(ctx context.Context, cfg config.Config, s3cfg objectstore.Config, dbRemote, filesRemote bool) error {
	if !cfg.Instance.Writable || !(dbRemote || filesRemote) || s3cfg.Bucket == "" {
		return nil
	}
	a.logger.Info("connecting object storage", zap.String("endpoint", s3cfg.Endpoint), zap.String("bucket", s3cfg.Bucket))
	store, err := objectstore.NewClient(ctx, s3cfg, a.logger, a.metrics)
	if err != nil {
		return fmt.Errorf("create objectstore client: %w", err)
	}
	a.objectStore = store
	a.step("object storage client ready")
	if dbRemote {
		if err := a.probeCreateIfAbsent(ctx, cfg, store); err != nil {
			return err
		}
	}
	a.step("object storage probe done", zap.Bool("cas_supported", a.casSupported))
	return nil
}

// probeCreateIfAbsent 是启动期的 create-if-absent 条件写探针（§4.5）：
// 数据库落到远端时验证对象存储是否真的支持条件写。
// 探针失败 → 记 Error + 指标归零 → 禁用一切依赖互斥的能力，/health/ready 暴露原因；
// 不让代码以为自己有保护。配置要求写租约时直接阻止启动——不得退化到
// 静默无保护的单写（可写实例 + 远端同步器 + 无互斥，§3.3）。
func (a *App) probeCreateIfAbsent(ctx context.Context, cfg config.Config, store objectstore.Client) error {
	bs, ok := store.(objectstore.BlobStore)
	if !ok {
		return nil
	}
	probePrefix := objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix}.CatalogPrefix()
	res := objectstore.ProbePutIfAbsent(ctx, bs, probePrefix)
	a.casSupported = res.CASSupported
	if a.metrics != nil {
		a.metrics.CASSupported.Set(boolGauge(res.CASSupported))
	}
	if res.CASSupported {
		a.logger.Info("objectstore create-if-absent probe passed")
		return nil
	}
	a.logger.Error("objectstore create-if-absent probe FAILED; mutual-exclusion capabilities disabled",
		zap.Error(res.Err),
		zap.String("hint", "endpoint does not enforce create-if-absent (bucket versioning enabled on COS?)"),
	)
	if cfg.Instance.Lease.Enabled {
		return fmt.Errorf("objectstore create-if-absent probe failed: %w; "+
			"instance.lease.enabled=true 要求对象存储支持条件写，拒绝在无单写保护下启动可写实例 "+
			"(hint: COS 存储桶开启版本控制会使 x-cos-forbid-overwrite 失效)", res.Err)
	}
	return nil
}

// bootSystemDatabase 启动系统库并创建 catalog 服务（含启动修复）。
func (a *App) bootSystemDatabase(ctx context.Context, cfg config.Config, keys objectstore.KeyBuilder, sysFactory *ducklake.Factory) (*systemdb.Store, error) {
	sysName := cfg.SystemDatabase.Name
	if sysName == "" {
		sysName = systemdb.DefaultName
	}
	a.logger.Info("booting system database",
		zap.String("name", sysName),
		zap.String("locator_dir", filepath.Join(cfg.Storage.DataDir, "_system")),
	)
	store, err := systemdb.Bootstrap(ctx, systemdb.BootstrapInput{
		// locator 记录系统库 UUID，必须与 data_dir 同寿命：
		// 放进 cache_dir 会在清缓存后重新生成一个空系统库。
		LocatorDir: filepath.Join(cfg.Storage.DataDir, "_system"),
		Name:       sysName,
		Factory:    sysFactory,
		Keys:       keys,
		Logger:     a.logger,
	})
	if err != nil {
		return nil, fmt.Errorf("bootstrap system database: %w", err)
	}
	a.step("system database bootstrapped",
		zap.String("database_id", store.Meta().ID),
		zap.String("project_id", store.Meta().ProjectID),
	)

	if err := a.assembleCatalog(ctx, cfg, store, keys); err != nil {
		return nil, err
	}
	return store, nil
}

// assembleCatalog 创建 catalog 服务并修复启动时的数据库可用性状态。
func (a *App) assembleCatalog(ctx context.Context, cfg config.Config, store *systemdb.Store, keys objectstore.KeyBuilder) error {
	repo := store.CatalogRepo()
	var descWriter catalog.DescriptorWriter
	if a.objectStore != nil {
		descWriter = a.objectStore
	}
	ducklakeStore := duckLakeStorageFrom(cfg)
	a.catalog = catalog.NewService(repo, keys, descWriter, ducklakeStore, a.logger)
	a.catalog.SetDefaultEngine(cfg.Database.Engine)
	if err := a.catalog.RepairDatabasesOnStartup(ctx); err != nil {
		return fmt.Errorf("repair database availability: %w", err)
	}
	a.step("catalog service ready", zap.String("default_engine", cfg.Database.Engine))
	return nil
}

// assembleRegistryAndCache 创建写租约、registry、cache 管理器与空闲回收器。
func (a *App) assembleRegistryAndCache(ctx context.Context, cfg config.Config, userFactory godatabase.Factory) error {
	// P1-1：per-database 写租约（multi-instance-consistency-plan §4.6）。
	// 仅 writable + storage.database=s3 + 探针通过时启用；失租 → 释放该库句柄。
	regOpts := registry.Options{
		IdleTimeout: cfg.Database.IdleTimeout,
		MaxOpen:     cfg.Database.MaxOpen,
		Writable:    cfg.Instance.Writable,
	}
	a.assembleWriteLease(ctx, cfg, regOpts)

	a.registry = registry.New(userFactory, dbbridge.ResolverFrom(a.catalog), regOpts, dbbridge.LoggerFrom(a.logger), dbbridge.ObserverFrom(a.metrics))
	a.step("registry ready",
		zap.Int("max_open", cfg.Database.MaxOpen),
		zap.String("idle_timeout", cfg.Database.IdleTimeout.String()),
		zap.Bool("write_lease", a.leaseMgr != nil),
	)

	var err error
	// cache.Manager 只依赖 registry，先于 Evictor 创建以便接入容量淘汰。
	a.cacheMgr, err = cache.NewManager(cache.Options{
		Root:         cfg.Database.CacheDir,
		DataRoot:     cfg.Storage.DataDir,
		MaxBytes:     cfg.Database.CacheMaxBytes,
		MaxDatabases: cfg.Database.CacheMaxDatabases,
		Registry:     a.registry,
		Closer:       a.registry,
		Logger:       dbbridge.LoggerFrom(a.logger),
		Observer:     dbbridge.ObserverFrom(a.metrics),
	})
	if err != nil {
		return fmt.Errorf("create cache manager: %w", err)
	}
	a.step("cache manager ready",
		zap.String("root", cfg.Database.CacheDir),
		zap.Int64("max_bytes", cfg.Database.CacheMaxBytes),
		zap.Int("max_databases", cfg.Database.CacheMaxDatabases),
	)

	// R1：周期回收空闲句柄，防止 max_open 打满后其他项目永久不可用。
	// Shutdown 时先 Stop（停止新回收），再由 Close → registry.Shutdown 收尾。
	// R1 第 3 条：cacheMgr 接入同一周期，缓存字节/数量超限按 LRU 淘汰
	// （cache_max_bytes / cache_max_databases 配置自此生效）。
	a.handleEvictor = registry.NewEvictor(a.registry, time.Minute, a.cacheMgr)
	go a.handleEvictor.Run(a.ensureSchedulerCtx())
	return nil
}
