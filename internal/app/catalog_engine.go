// 文件说明：catalog_engine.go — internal/app 的内部实现（bootID、checkCatalogEngine 等）
//
// 用法
//
//	同包调用：checkCatalogEngine() → app.go；bootID → reset.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/catalog_engine.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/ducklake, internal/objectstore
//	[同包] app.go, cron_runner.go, identity.go, project_purger.go, reset.go 等 6 个
//	[关键引用] checkCatalogEngine() ← app.go
//
// 主要符号
//
//	var bootID
//	(a *App) checkCatalogEngine(ctx context.Context) error

package app

import (
	"context"
	"os"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"go.uber.org/zap"
)

// bootID 区分同一 instance.id 的不同进程（写入实例标记的 created_by）。
var bootID = uuid.NewString()

// checkCatalogEngine 执行实例级 catalog 引擎校验（ducklake-duckdb-catalog-plan §4.2）。
// storage.database=s3 且 objectStore 为 BlobStore 时同时校验远端标记。
func (a *App) checkCatalogEngine(ctx context.Context) error {
	cfg := a.cfg
	in := ducklake.FormatCheckInput{
		Engine:    cfg.Database.DuckLake.CatalogEngine,
		DataRoot:  cfg.Storage.DataDir,
		Keys:      objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix}.DB(),
		CreatedBy: cfg.Instance.ID + "/" + bootID,
		Bucket:    cfg.S3.Bucket,
	}
	if cfg.IsDBRemote() && a.objectStore != nil {
		if bs, ok := a.objectStore.(objectstore.BlobStore); ok {
			in.Store = bs
		}
	}
	f, err := ducklake.EnsureInstanceFormat(ctx, in)
	if err != nil {
		if a.metrics != nil && a.metrics.CatalogEngineMismatch != nil {
			a.metrics.CatalogEngineMismatch.Set(1)
		}
		if a.logger != nil {
			// 报错信息只含 bucket/prefix/引擎名，不含任何凭据。
			a.logger.Error("catalog engine check failed; refusing to start", zap.Error(err))
		}
		return err
	}
	if a.metrics != nil && a.metrics.CatalogEngineMismatch != nil {
		a.metrics.CatalogEngineMismatch.Set(0)
	}
	if a.logger != nil {
		a.logger.Info("catalog engine check passed",
			zap.String("catalog_engine", f.CatalogEngine),
			zap.Int("format_version", f.FormatVersion),
			zap.Bool("remote", in.Store != nil),
			zap.Int("pid", os.Getpid()),
		)
	}
	return nil
}
