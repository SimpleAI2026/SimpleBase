// 文件说明：app_lease.go — internal/app 的内部实现（assembleWriteLease）
//
// 用法
//
//	(a *App) assembleWriteLease(ctx context.Context, cfg config.Config, regOpts registry.Options)（包内使用）
//	同包调用：assembleWriteLease → app.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/app_lease.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/lease, godatabase/filedb, godatabase/ducklake, internal/objectstore
//	[同包] app.go 等 7 个
//	[关键引用] assembleWriteLease ← app.go；leaseCloser ← app.go
//
// 主要符号
//
//	(a *App) assembleWriteLease(ctx context.Context, cfg config.Config, regOpts registry.Options)

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/lease"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"go.uber.org/zap"
)

// assembleWriteLease 装配 per-database 写租约（原 assembleDeps「lease manager
// 与回调」连续块，B4 提取）。仅 writable + storage.database=s3 + 探针通过时启用；
// 失租回调与 WriteGate 的闭包捕获时机、注册 closer 顺序保持原样。
func (a *App) assembleWriteLease(ctx context.Context, cfg config.Config, regOpts registry.Options) {
	dbRemote := cfg.IsDBRemote()
	if cfg.Instance.Writable && dbRemote && cfg.Instance.Lease.Enabled && a.casSupported {
		if blobs, ok := a.objectStore.(objectstore.BlobStore); ok {
			keys := objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix}
			lm := lease.NewManager(blobs, keys.DB(), lease.Config{
				Enabled:       true,
				TTL:           cfg.Instance.Lease.TTL,
				RenewInterval: cfg.Instance.Lease.RenewInterval,
				Grace:         cfg.Instance.Lease.Grace,
			})
			// C7：observability.Logger 与 godatabase.Logger 形状一致，直接注入。
			lm.SetLogger(a.logger)
			lm.EnableProjectScope()
			a.leaseMgr = lm
			a.leaseKeys = keys
			a.leaseGate = lease.NewGate()
			a.registerCloser(leaseCloser{m: lm})
			a.logger.Info("per-database write lease enabled",
				zap.String("ttl", cfg.Instance.Lease.TTL.String()),
				zap.String("on_lost", cfg.Instance.Lease.OnLost),
			)

			// 失租回调：停同步 PUT + 释放句柄（§4.6 onLost 顺序）。
			a.onLostLease = func(scope string) {
				a.logger.Error("write lease LOST: releasing database handle and disabling further writes",
					zap.String("database_id", scope))
				if a.metrics != nil && a.metrics.LeaseLost != nil {
					a.metrics.LeaseLost.WithLabelValues(scope).Inc()
				}
				if a.metrics != nil && a.metrics.LeaseState != nil {
					a.metrics.LeaseState.WithLabelValues(scope).Set(0)
				}
				// 同步器立即进入失租模式：后续 Flush/Close 不再 PUT（§4.6 第 1 步）。
				if cs, ok := a.catalogSyncer.(*ducklake.CatalogSyncer); ok {
					cs.CloseNoFlush(scope)
				}
				if fs, ok := a.fileSyncer.(*filedb.FileSyncer); ok {
					fs.CloseNoFlush(scope)
				}
				if strings.EqualFold(cfg.Instance.Lease.OnLost, "exit") {
					a.logger.Fatal("write lease lost and on_lost=exit: terminating process")
				}
				a.registry.Remove(scope)
				// perf §1.5：失租后清除本进程的写门记录，允许后续重新竞争租约。
				a.leaseGate.Clear(scope)
			}

			// WriteGate：ReadWrite Acquire 前必须持租（惰性首次获取）。
			// 失租（ValidFor=false 且已有租约记录）→ 拒绝新写。
			// perf §1.5：同库并发首写在 leaseGate 上按 dbID 串行化，
			// 只触发一次 Acquire，其余请求复用结果（修复并发首写 45s + 500）。
			regOpts.WriteGate = func(db godatabase.Spec) error {
				scope := lm.ScopeKey(db.ProjectID, db.ID)
				if lm.ValidFor(scope) {
					return nil
				}
				// 已有租约记录但未持租 = 失租后：拒绝。
				if lm.Has(scope) {
					return fmt.Errorf("instance: write lease lost for database %s; writes rejected until lease recovered", db.ID)
				}
				// 首次写：获取租约。他人持租 → 拒绝（该实例应降级只读）。
				if err := a.leaseGate.TryDo(scope, func() error {
					if lm.ValidFor(scope) {
						return nil
					}
					if lm.Has(scope) {
						return fmt.Errorf("instance: write lease lost for database %s; writes rejected until lease recovered", db.ID)
					}
					if _, err := lm.Acquire(ctx, db.ProjectID, scope, cfg.Instance.ID, a.onLostLease); err != nil {
						if lease.IsHeld(err) {
							if a.metrics != nil && a.metrics.LeaseHeldRejected != nil {
								a.metrics.LeaseHeldRejected.WithLabelValues(db.ID).Inc()
							}
							return fmt.Errorf("instance: write lease held by another instance for database %s: %w", db.ID, err)
						}
						return fmt.Errorf("instance: acquire write lease: %w", err)
					}
					return nil
				}); err != nil {
					if errors.Is(err, lease.ErrAcquiring) {
						// 同库首租获取在途：立即返回可识别状态，不排队挂等 TTL+Grace。
						return fmt.Errorf("instance: write lease acquisition in progress for database %s: %w", db.ID, err)
					}
					return err
				}
				if a.metrics != nil {
					if a.metrics.LeaseState != nil {
						a.metrics.LeaseState.WithLabelValues(db.ID).Set(1)
					}
					if a.metrics.LeaseEpoch != nil {
						a.metrics.LeaseEpoch.WithLabelValues(db.ID).Set(float64(lm.EpochFor(scope)))
					}
				}
				if cs, ok := a.catalogSyncer.(*ducklake.CatalogSyncer); ok {
					cs.SetWriterEpoch(db.ID, lm.EpochFor(scope))
				}
				if fs, ok := a.fileSyncer.(*filedb.FileSyncer); ok {
					fs.SetWriterEpoch(db.ProjectID, lm.EpochFor(scope))
				}
				return nil
			}
		}
	} else if cfg.Instance.Writable && dbRemote && cfg.Instance.Lease.Enabled && !a.casSupported {
		a.logger.Warn("write lease requested but disabled: create-if-absent probe failed",
			zap.String("impact", "single-writer enforcement is NOT active; replicas must stay 1"))
	}
}
