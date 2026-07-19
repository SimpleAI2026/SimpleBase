// tursofactory.go 提供 database.Factory 的默认实现：把 catalog.Database
// 转换为 turso.OpenOptions 并调用 turso.Open。它是 Registry 与底层 Turso
// 驱动之间唯一的桥接点（见 plan4.md）。
package database

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database/turso"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// StorageConfig 是构造 TursoFactory 所需的 S3 存储配置。
// 字段与 config.S3Config 对应，由 app 装配层填充，本包不直接依赖 config 包
// 以避免循环依赖。
type StorageConfig struct {
	Endpoint       string
	Region         string
	Bucket         string
	KMSKeyID       string
	ForcePathStyle bool
}

// TursoFactory 是 Factory 的默认实现。
type TursoFactory struct {
	Storage  StorageConfig
	CacheDir string // 本地缓存根目录；空表示纯远程，不使用本地缓存
	Pool     turso.PoolOptions
	Logger   observability.Logger
	Metrics  *observability.Metrics
}

// Open 实现 Factory。databaseID 必须是 UUID（由 catalog 保证），
// 缓存路径限制在 CacheDir 内，不接受用户可控路径。
func (f *TursoFactory) Open(ctx context.Context, db catalog.Database, mode AccessMode) (*sql.DB, error) {
	var cachePath string
	if f.CacheDir != "" {
		cachePath = filepath.Join(f.CacheDir, db.ID)
	}

	opts := turso.OpenOptions{
		DatabaseID: db.ID,
		CachePath:  cachePath,
		Writable:   mode == ReadWrite,
		Storage: turso.StorageConfig{
			Endpoint:       f.Storage.Endpoint,
			Region:         f.Storage.Region,
			Bucket:         f.Storage.Bucket,
			Prefix:         db.StoragePrefix,
			KMSKeyID:       f.Storage.KMSKeyID,
			ForcePathStyle: f.Storage.ForcePathStyle,
		},
	}

	return turso.Open(ctx, opts, f.Pool, f.Logger, f.Metrics, nil)
}
