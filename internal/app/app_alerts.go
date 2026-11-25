// 文件说明：app_alerts.go — internal/app 的内部实现（assembleAlertBus、watchDiskFreeRatio、backupReporter）
//
// 用法
//
//	(a *App) assembleAlertBus(cfg config.Config, store *systemdb.Store)（包内使用）
//	同包调用：assembleAlertBus → app.go；backupReporter → cron_runner.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/app_alerts.go
//	         │ 依赖
//	         ▼
//	[下游] internal/alerts, internal/backup, internal/systemdb
//	[同包] app.go, cron_runner.go 等 7 个
//	[关键引用] assembleAlertBus ← app.go；backupReporter ← cron_runner.go
//
// 主要符号
//
//	(a *App) assembleAlertBus(cfg config.Config, store *systemdb.Store)
//	(a *App) watchDiskFreeRatio(ctx context.Context, cfg config.Config, bus *alerts.Bus)
//	(a *App) backupReporter() backup.Reporter
//	type backupReporter struct {…}

package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/alerts"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/backup"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"go.uber.org/zap"
)

// assembleAlertBus 装配进程内告警总线（§8.3.5）与磁盘水位巡检（§8.3.4）。
// 订阅方：审计记录（sys_audit_events，kind=alert.*）+ metrics 计数；
// 巡检：启动时 + 每 5min 检查 data_dir 所在卷，<10% 发 critical、<20% 发 warning（状态翻转去重）。
func (a *App) assembleAlertBus(cfg config.Config, store *systemdb.Store) {
	bus := alerts.NewBus(a.logger)
	a.alertBus = bus

	// 订阅方 1：审计记录。
	bus.Subscribe(func(sig alerts.Signal) {
		store.RecordAudit(systemdb.AuditEvent{
			Actor: auth.ActorSystem,
			Kind:  "alert." + sig.Kind,
			Detail: strings.TrimSpace("severity=" + sig.Severity +
				" project=" + sig.Project + " " + sig.Detail),
		})
	})
	// 订阅方 2：metrics 计数。
	if a.metrics != nil {
		bus.Subscribe(func(sig alerts.Signal) {
			a.metrics.AlertsTotal.WithLabelValues(sig.Kind, sig.Severity).Inc()
		})
	}

	// 磁盘水位巡检（§8.3.4）：启动即查一次，其后每 5min。
	if a.schedulerBaseCtx != nil {
		go a.watchDiskFreeRatio(a.schedulerBaseCtx, cfg, bus)
	}
}

// watchDiskFreeRatio 周期采样磁盘可用比例；越限发告警信号（状态翻转时才发，避免刷屏）。
func (a *App) watchDiskFreeRatio(ctx context.Context, cfg config.Config, bus *alerts.Bus) {
	dataDir := cfg.Storage.DataDir
	if dataDir == "" {
		return
	}
	const (
		checkEvery = 5 * time.Minute
		warnBelow  = 0.20
		critBelow  = 0.10
	)
	var lastState int // 0=ok 1=warn 2=crit
	tick := time.NewTicker(checkEvery)
	defer tick.Stop()
	check := func() {
		ratio, err := alerts.DiskFreeRatio(dataDir)
		if err != nil {
			a.logger.Error("disk stat unavailable", zap.String("err", err.Error()))
			return
		}
		if a.metrics != nil {
			a.metrics.DiskFreeRatio.Set(ratio)
		}
		switch {
		case ratio < critBelow:
			if lastState != 2 {
				bus.Emit(alerts.Signal{Kind: "disk.critical", Severity: alerts.SeverityCritical,
					Detail: fmt.Sprintf("free %.1f%% below %.0f%%", ratio*100, critBelow*100)})
			}
			lastState = 2
		case ratio < warnBelow:
			if lastState != 1 {
				bus.Emit(alerts.Signal{Kind: "disk.warning", Severity: alerts.SeverityWarning,
					Detail: fmt.Sprintf("free %.1f%% below %.0f%%", ratio*100, warnBelow*100)})
			}
			lastState = 1
		default:
			lastState = 0
		}
	}
	check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			check()
		}
	}
}

// backupReporter 返回备份结果上报器（§8.3.2）：
// 成功 → 记录时刻（readiness「上次成功 < 48h」用）+ 审计 backup.succeeded；
// 失败 → 告警信号 backup.failed + 审计 backup.failed + 错误日志。
func (a *App) backupReporter() backup.Reporter {
	return backupReporter{app: a}
}

type backupReporter struct{ app *App }

func (r backupReporter) BackupSucceeded(at time.Time, projects int) {
	if r.app.systemStore != nil {
		r.app.systemStore.RecordAudit(systemdb.AuditEvent{
			Actor: auth.ActorSystem, Kind: "backup.succeeded",
			Detail: fmt.Sprintf("projects=%d at=%s", projects, at.Format(time.RFC3339)),
		})
	}
	r.app.logger.Info("backup round succeeded", zap.Int("projects", projects))
}

func (r backupReporter) BackupFailed(at time.Time, err error) {
	if r.app.alertBus != nil {
		r.app.alertBus.Emit(alerts.Signal{
			Kind: "backup.failed", Severity: alerts.SeverityCritical,
			Detail: err.Error(),
		})
	}
	if r.app.systemStore != nil {
		r.app.systemStore.RecordAudit(systemdb.AuditEvent{
			Actor: auth.ActorSystem, Kind: "backup.failed",
			Detail: err.Error(),
		})
	}
	r.app.logger.Error("backup round failed", zap.String("err", err.Error()))
}
