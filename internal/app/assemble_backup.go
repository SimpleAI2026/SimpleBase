// 文件说明：备份装配阶段：assembleBackup（原 app.go 拆出）
package app

import (
	"fmt"
	"github.com/linkxzhou/SimpleBase/internal/api/backuphttp"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"

	"github.com/linkxzhou/SimpleBase/internal/backup"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"go.uber.org/zap"
)

func (a *App) assembleBackup(cfg config.Config) (*backuphttp.Handler, error) {
	if !cfg.Backup.Enabled || a.systemStore == nil || a.registry == nil || a.fileFactory == nil {
		if a.logger != nil {
			a.logger.Info("backup disabled",
				zap.Bool("backup_enabled", cfg.Backup.Enabled),
				zap.Bool("system_store", a.systemStore != nil),
				zap.Bool("registry", a.registry != nil),
				zap.Bool("file_factory", a.fileFactory != nil),
			)
		}
		return nil, nil
	}
	repo := backup.NewRepository(a.systemStore.DB(), a.systemStore.NotifyWrite)
	var blobs objectstore.BlobStore
	if a.objectStore != nil {
		blobs, _ = a.objectStore.(objectstore.BlobStore)
	}
	service := backup.NewService(cfg.Backup, cfg.Instance.ID, a.catalog, a.registry, a.fileFactory, repo, blobs, objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix})
	backupHandler := backuphttp.New(service, httpkit.NewAuditService(a.auditSvc), cfg.Instance.Writable)
	// §8.3.6：readiness「48h 内有成功备份」检查的数据源。
	a.backupStoreReader = repo
	if cfg.Backup.Schedule != "" {
		// §8.3.2：catchup + 错误上报（告警总线在 seedAndStartMaintenance 装配，
		// 此处仅当已存在时接线；nil Reporter 退化为无上报）。
		reporter := a.backupReporter()
		scheduler, err := backup.NewScheduler(service, repo, cfg.Backup.Schedule, backup.SchedulerOptions{
			MissedPolicy: cfg.Backup.EffectiveMissedPolicy(),
			Reporter:     reporter,
		})
		if err != nil {
			return nil, fmt.Errorf("backup schedule: %w", err)
		}
		scheduler.Start(a.ensureSchedulerCtx())
		a.logger.Info("backup scheduler started",
			zap.String("schedule", cfg.Backup.Schedule),
			zap.String("dir", cfg.Backup.Dir),
			zap.String("missed_policy", cfg.Backup.EffectiveMissedPolicy()),
		)
	}
	return backupHandler, nil
}
