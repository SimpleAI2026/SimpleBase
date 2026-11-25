// 文件说明：app_lease.go — internal/app 的内部实现（assembleWriteLease）
//
// 用法
//
//	(a *App) assembleWriteLease(ctx context.Context, cfg config.Config, regOpts registry.Options)（包内使用）
//	同包调用：assembleWriteLease → app.go；其余符号只被 assembleWriteLease 及其闭包使用
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
//	(a *App) newLeaseOnLost(cfg) func(scope string) / newLeaseWriteGate(ctx, cfg, lm) func(godatabase.Spec) error
//	leaseLost(...) / (*App).leaseAcquireError(...) / (*App).markLeaseHeld(...)

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

// assembleWriteLease 装配 per-database 写租约。仅 writable + storage.database=s3 +
// instance.lease.enabled 且条件写探针通过时启用；探针未过只告警，不建立任何互斥。
// 顺序：先准备失租回调，再挂 WriteGate，最后注册 closer（Close 时反向释放）。
func (a *App) assembleWriteLease(ctx context.Context, cfg config.Config, regOpts registry.Options) {
	if !cfg.Instance.Writable || !cfg.IsDBRemote() || !cfg.Instance.Lease.Enabled {
		return
	}
	if !a.casSupported {
		a.logger.Warn("write lease requested but disabled: create-if-absent probe failed",
			zap.String("impact", "single-writer enforcement is NOT active; replicas must stay 1"))
		return
	}
	blobs, ok := a.objectStore.(objectstore.BlobStore)
	if !ok {
		return
	}
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
	a.onLostLease = a.newLeaseOnLost(cfg)
	regOpts.WriteGate = a.newLeaseWriteGate(ctx, cfg, lm)
}

// newLeaseOnLost 返回失租回调，按 §4.6 的 onLost 顺序执行：记录指标 → 同步器进入失租模式
// （后续 Flush/Close 不再 PUT）→ 释放句柄；on_lost=exit 时直接终止进程。
// 最后清掉本进程的写门记录，允许后续重新竞争租约（perf §1.5）。
func (a *App) newLeaseOnLost(cfg config.Config) func(scope string) {
	return func(scope string) {
		a.logger.Error("write lease LOST: releasing database handle and disabling further writes",
			zap.String("database_id", scope))
		if a.metrics != nil {
			if a.metrics.LeaseLost != nil {
				a.metrics.LeaseLost.WithLabelValues(scope).Inc()
			}
			if a.metrics.LeaseState != nil {
				a.metrics.LeaseState.WithLabelValues(scope).Set(0)
			}
		}
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
		a.leaseGate.Clear(scope)
	}
}

// newLeaseWriteGate 返回 registry 的写门：ReadWrite Acquire 之前必须持租，首次写时惰性获取。
//   - 已持租 → 放行；
//   - 有租约记录但未持租 = 失租后 → 拒绝新写；
//   - 首次写 → 获取租约，他人持租即拒绝（该实例应降级只读）。
//
// 同库并发首写在 leaseGate 上按 dbID 串行化，只触发一次 Acquire，其余请求复用结果
// （修复并发首写 45s + 500，perf §1.5）。
func (a *App) newLeaseWriteGate(ctx context.Context, cfg config.Config, lm *lease.Manager) func(godatabase.Spec) error {
	return func(db godatabase.Spec) error {
		scope := lm.ScopeKey(db.ProjectID, db.ID)
		if lm.ValidFor(scope) {
			return nil
		}
		if err := leaseLost(lm, scope, db.ID); err != nil {
			return err
		}
		if err := a.leaseGate.TryDo(scope, func() error {
			// 排队期间同库的另一次首写可能已经拿到租约，或本库已经失租。
			if lm.ValidFor(scope) {
				return nil
			}
			if err := leaseLost(lm, scope, db.ID); err != nil {
				return err
			}
			if _, err := lm.Acquire(ctx, db.ProjectID, scope, cfg.Instance.ID, a.onLostLease); err != nil {
				return a.leaseAcquireError(db.ID, err)
			}
			return nil
		}); err != nil {
			if errors.Is(err, lease.ErrAcquiring) {
				// 同库首租获取在途：立即返回可识别状态，不排队挂等 TTL+Grace。
				return fmt.Errorf("instance: write lease acquisition in progress for database %s: %w", db.ID, err)
			}
			return err
		}
		a.markLeaseHeld(db, lm, scope)
		return nil
	}
}

// leaseLost 在「有租约记录但当前未持租」（失租后）时返回拒绝写的原因；
// 持租或从未取租返回 nil。调用方先判 ValidFor 再调它，顺序与失租语义一致。
func leaseLost(lm *lease.Manager, scope, dbID string) error {
	if lm.ValidFor(scope) || !lm.Has(scope) {
		return nil
	}
	return fmt.Errorf("instance: write lease lost for database %s; writes rejected until lease recovered", dbID)
}

// leaseAcquireError 把取租失败映射成写门的错误；他人持租时另计一个指标。
func (a *App) leaseAcquireError(dbID string, err error) error {
	if lease.IsHeld(err) {
		if a.metrics != nil && a.metrics.LeaseHeldRejected != nil {
			a.metrics.LeaseHeldRejected.WithLabelValues(dbID).Inc()
		}
		return fmt.Errorf("instance: write lease held by another instance for database %s: %w", dbID, err)
	}
	return fmt.Errorf("instance: acquire write lease: %w", err)
}

// markLeaseHeld 在首次取租成功后把租约状态与 epoch 写进指标和两个同步器。
func (a *App) markLeaseHeld(db godatabase.Spec, lm *lease.Manager, scope string) {
	epoch := lm.EpochFor(scope)
	if a.metrics != nil {
		if a.metrics.LeaseState != nil {
			a.metrics.LeaseState.WithLabelValues(db.ID).Set(1)
		}
		if a.metrics.LeaseEpoch != nil {
			a.metrics.LeaseEpoch.WithLabelValues(db.ID).Set(float64(epoch))
		}
	}
	if cs, ok := a.catalogSyncer.(*ducklake.CatalogSyncer); ok {
		cs.SetWriterEpoch(db.ID, epoch)
	}
	if fs, ok := a.fileSyncer.(*filedb.FileSyncer); ok {
		fs.SetWriterEpoch(db.ProjectID, epoch)
	}
}
