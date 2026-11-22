// 文件说明：存储装配阶段：文件存储、用户库/系统库 factory、DuckLake 选项（原 app.go 拆出）
package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/internal/api"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"go.uber.org/zap"
)

// assembleFileStore 装配用户文件存储（原 assembleDeps「文件存储选择」连续块，
// B4 提取）：按 storage.files 选择对象存储或本地目录。位于 writable 提前返回
// 之前——只读实例同样装配 fileStore。错误文本保持原样。
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
