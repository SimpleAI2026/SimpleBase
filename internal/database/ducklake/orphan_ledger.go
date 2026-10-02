package ducklake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// OrphanRecord 是孤儿文件账本的一项（multi-instance-consistency-plan §4.7）。
// 所有删除动作（维护任务删数据文件、删快照、删 manifest）都必须先记账后删除：
// 记账对象 PutIfAbsent 成功后，实际删除失败也不丢账——恢复流程按账本重扫。
//
// 账本按日分目录（orphans/{yyyy-mm-dd}/{uuid}.json）便于 lifecycle 自动清理
// 过期账目（与 delete_older_than 对齐）。
type OrphanRecord struct {
	FileKey    string `json:"file_key"` // 被删对象（或待删候选）
	DatabaseID string `json:"database_id"`
	TenantID   string `json:"tenant_id"`
	Reason     string `json:"reason"` // expire_snapshots | delete_orphaned_files | rewrite_delete | prune_manifest
	Status     string `json:"status"` // pending | deleted
	CreatedAt  string `json:"created_at"`
}

// OrphanLedger 把删除动作记到对象存储账本。
type OrphanLedger struct {
	Store objectstore.BlobStore
	Keys  objectstore.KeyBuilder
}

// Record 写一条 pending 账目（PutIfAbsent + uuid key，天然幂等）。
func (l *OrphanLedger) Record(ctx context.Context, tenantID, databaseID, fileKey, reason string) error {
	rec := OrphanRecord{
		FileKey:    fileKey,
		DatabaseID: databaseID,
		TenantID:   tenantID,
		Reason:     reason,
		Status:     "pending",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	key, err := l.Keys.DuckLakeOrphanLedgerKey(tenantID, databaseID, time.Now().UTC().Format("2006-01-02"), uuid.NewString())
	if err != nil {
		return err
	}
	body, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := l.Store.PutIfAbsent(ctx, key, body, "application/json"); err != nil {
		return fmt.Errorf("ducklake: record orphan %s: %w", fileKey, err)
	}
	return nil
}

// MaintenanceGate 是维护任务的门禁（§4.7 部署矩阵）。
// 两个条件都通过才允许执行破坏性维护（expire/delete/rewrite）：
//  1. hasLease：本实例持有所涉库的写租约（避免误删他人文件）；
//  2. syncCaughtUp：catalog 同步水位追平（Sync lag <= maxLag），
//     避免「本地以为文件是孤儿，实际是未同步提交引用的文件」。
type MaintenanceGate struct {
	// LeaseValid 返回 dbID 是否持租；nil 表示未启用租约（单实例部署）→ 视为通过。
	LeaseValid func(dbID string) bool
	// SyncLag 返回 dbID 的同步滞后 snapshot 数。
	SyncLag func(dbID string) int64
	// MaxLag 允许的最大滞后；<=0 默认 0（必须完全追平）。
	MaxLag int64
}

// ErrGateBlocked 维护被门禁拦截。
var ErrGateBlocked = errors.New("ducklake: maintenance blocked by gate")

// Check 对指定库执行门禁检查；通过返回 nil，否则 ErrGateBlocked 包装原因。
func (g *MaintenanceGate) Check(dbID string) error {
	if g == nil {
		return nil
	}
	if g.LeaseValid != nil && !g.LeaseValid(dbID) {
		return fmt.Errorf("%w: write lease not held for %s", ErrGateBlocked, dbID)
	}
	if g.SyncLag != nil {
		maxLag := g.MaxLag
		if maxLag <= 0 {
			maxLag = 0
		}
		if lag := g.SyncLag(dbID); lag > maxLag {
			return fmt.Errorf("%w: sync lag %d > %d for %s", ErrGateBlocked, lag, maxLag, dbID)
		}
	}
	return nil
}

// BlockedReason 解析门禁拒绝原因（no_lease / sync_lag），供指标打标签。
func BlockedReason(err error) string {
	if err == nil || !errors.Is(err, ErrGateBlocked) {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "lease") {
		return "no_lease"
	}
	if strings.Contains(msg, "lag") {
		return "sync_lag"
	}
	return "other"
}
