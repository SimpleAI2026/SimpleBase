// create_backup.go 实现备份任务处理器（plan7.md「恢复点与备份」）。
//
// 优先调用 Turso 官方支持的快照/备份机制；适配接口必须隐藏上游细节。
// 在官方能力不足前，不实现复制活跃 .db/.db-wal/.shm 文件的自研备份。
// 每个已发布恢复点必须保存 source/target IDs、创建时间、Turso 版本、校验信息、状态和不可变 manifest。
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// BackupPayload 是备份任务的参数。
type BackupPayload struct {
	// SnapshotRef 指向已通过 Turso 官方机制创建的快照标识。
	// 若为空，表示需在任务中触发快照创建（取决于 Snapshotter 能力）。
	SnapshotRef string `json:"snapshot_ref,omitempty"`
	// ManifestKey 备份 manifest 在 S3 中的 key（由 BackupService 构造）。
	ManifestKey string `json:"manifest_key,omitempty"`
}

// BackupManifest 是不可变的恢复点元数据，持久化到 S3。
type BackupManifest struct {
	BackupID      string    `json:"backup_id"`
	SourceDBID    string    `json:"source_database_id"`
	ProjectID     string    `json:"project_id"`
	TenantID      string    `json:"tenant_id"`
	SnapshotRef   string    `json:"snapshot_ref"`
	SnapshotHint string    `json:"snapshot_hint,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"`
	ContentSHA256 string    `json:"content_sha256,omitempty"`
}

// Snapshotter 抽象上游快照能力。首期可返回 ErrSnapshotNotSupported。
type Snapshotter interface {
	// Create 创建快照。destination 描述快照存储位置（由实现解释）。
	Create(ctx context.Context, source catalog.Database, destination string) (BackupManifest, error)
	// Restore 从快照恢复到目标库。
	Restore(ctx context.Context, manifest BackupManifest, target catalog.Database) error
}

// ErrSnapshotNotSupported 表示上游不支持快照能力。
var ErrSnapshotNotSupported = errors.New("jobs: snapshot not supported by upstream")

// BackupHandler 处理 backup 任务。
type BackupHandler struct {
	catalog    *catalog.Service
	snapshots  Snapshotter
	logger     observability.Logger
	now        func() time.Time
}

// NewBackupHandler 构造 handler。snapshots 可为 nil（返回 ErrSnapshotNotSupported）。
func NewBackupHandler(cat *catalog.Service, snaps Snapshotter, logger observability.Logger) *BackupHandler {
	return &BackupHandler{
		catalog:   cat,
		snapshots: snaps,
		logger:    logger,
		now:       time.Now,
	}
}

// Type 返回 backup。
func (h *BackupHandler) Type() catalog.JobType { return catalog.JobTypeBackup }

// Execute 执行备份任务。
func (h *BackupHandler) Execute(ctx context.Context, job catalog.Job) error {
	var payload BackupPayload
	if job.PayloadJSON != "" {
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return fmt.Errorf("jobs: unmarshal backup payload: %w", err)
		}
	}
	if h.snapshots == nil {
		return ErrSnapshotNotSupported
	}
	if h.catalog == nil {
		return errors.New("jobs: catalog service required for backup")
	}
	// 获取源库（通过 project 校验）。
	// 注意：worker 不持有 principal；使用 internal 获取方式。
	db, err := h.catalog.GetDatabaseForJob(ctx, job.ProjectID, job.DatabaseID)
	if err != nil {
		return fmt.Errorf("jobs: get source database: %w", err)
	}
	manifest, err := h.snapshots.Create(ctx, db, payload.SnapshotRef)
	if err != nil {
		return fmt.Errorf("jobs: create snapshot: %w", err)
	}
	if h.logger != nil {
		h.logger.Info("jobs: backup created",
			zap.String("backup_id", manifest.BackupID),
			zap.String("source", db.ID))
	}
	return nil
}

// RestoreHandler 处理 restore 任务。
type RestoreHandler struct {
	catalog    *catalog.Service
	snapshots  Snapshotter
	logger     observability.Logger
	now        func() time.Time
}

// NewRestoreHandler 构造 handler。
func NewRestoreHandler(cat *catalog.Service, snaps Snapshotter, logger observability.Logger) *RestoreHandler {
	return &RestoreHandler{
		catalog:   cat,
		snapshots: snaps,
		logger:    logger,
		now:       time.Now,
	}
}

// Type 返回 restore。
func (h *RestoreHandler) Type() catalog.JobType { return catalog.JobTypeRestore }

// RestorePayload 是恢复任务的参数。
type RestorePayload struct {
	SourceBackupID string `json:"source_backup_id"`
	ManifestJSON   string `json:"manifest_json"`
	TargetDBID     string `json:"target_database_id"`
}

// Execute 从指定恢复点恢复到新 target database。
func (h *RestoreHandler) Execute(ctx context.Context, job catalog.Job) error {
	var payload RestorePayload
	if job.PayloadJSON != "" {
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return fmt.Errorf("jobs: unmarshal restore payload: %w", err)
		}
	}
	if h.snapshots == nil {
		return ErrSnapshotNotSupported
	}
	var manifest BackupManifest
	if payload.ManifestJSON != "" {
		if err := json.Unmarshal([]byte(payload.ManifestJSON), &manifest); err != nil {
			return fmt.Errorf("jobs: unmarshal manifest: %w", err)
		}
	}
	// 获取 target database（应处于 recovering 状态）。
	target, err := h.catalog.GetDatabaseForJob(ctx, job.ProjectID, payload.TargetDBID)
	if err != nil {
		return fmt.Errorf("jobs: get target database: %w", err)
	}
	if err := h.snapshots.Restore(ctx, manifest, target); err != nil {
		// 失败：标记 degraded，保留证据与目标以便排查/重试。
		_ = h.catalog.SetDatabaseDegraded(ctx, target.ID, err)
		return fmt.Errorf("jobs: restore snapshot: %w", err)
	}
	// 成功：状态转 ready。
	if err := h.catalog.SetDatabaseReady(ctx, target.ID); err != nil {
		return fmt.Errorf("jobs: mark target ready: %w", err)
	}
	if h.logger != nil {
		h.logger.Info("jobs: restore completed",
			zap.String("target", target.ID),
			zap.String("source_backup", payload.SourceBackupID))
	}
	return nil
}

// VerifyRecoveryHandler 处理 verify_recovery 演练任务。
type VerifyRecoveryHandler struct {
	catalog    *catalog.Service
	snapshots  Snapshotter
	logger     observability.Logger
	now        func() time.Time
}

// NewVerifyRecoveryHandler 构造 handler。
func NewVerifyRecoveryHandler(cat *catalog.Service, snaps Snapshotter, logger observability.Logger) *VerifyRecoveryHandler {
	return &VerifyRecoveryHandler{
		catalog:   cat,
		snapshots: snaps,
		logger:    logger,
		now:       time.Now,
	}
}

// Type 返回 verify_recovery。
func (h *VerifyRecoveryHandler) Type() catalog.JobType { return catalog.JobTypeVerifyRecovery }

// Execute 从 S3 在临时 UUID + 空 cache 恢复，运行完整性检查，完成后只删除临时前缀。
func (h *VerifyRecoveryHandler) Execute(ctx context.Context, job catalog.Job) error {
	if h.snapshots == nil {
		return ErrSnapshotNotSupported
	}
	// 演练流程：创建临时 target → restore → integrity_check → 删除临时前缀。
	// 首期返回 ErrSnapshotNotSupported，待 Snapshotter 实现后补全。
	return ErrSnapshotNotSupported
}
