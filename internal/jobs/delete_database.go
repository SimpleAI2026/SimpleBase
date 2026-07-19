// delete_database.go 实现删除数据库的后台任务（plan7.md「删除流程」）。
//
// 流程：关闭 registry handle → 检查保留截止时间 → 删除已知 database prefix
// （含备份按策略）→ 标记 deleted。任何中断可安全重试。
//
// 禁止 handler 同步 DeletePrefix；删除只在 worker 中执行。
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// DeleteDatabasePayload 是删除任务的参数。
type DeleteDatabasePayload struct {
	// RetentionUntil 删除保留期截止时间；此时间之前不真正删除 S3 对象。
	// 零值表示立即删除（用于无保留期策略）。
	RetentionUntil time.Time `json:"retention_until,omitempty"`
	// StoragePrefix 待删除的 S3 data 前缀（来自 catalog.Database.StoragePrefix）。
	StoragePrefix string `json:"storage_prefix"`
	// TenantID 用于 objectstore.DeletePrefix 的 key 构建。
	TenantID string `json:"tenant_id"`
}

// DeleteDatabaseHandler 处理 delete_database 任务。
type DeleteDatabaseHandler struct {
	catalog    *catalog.Service
	registry   *registry.Registry
	objects    objectstore.Deleter
	logger     observability.Logger
	now        func() time.Time
}

// Deleter 接口由 objectstore.Client 满足（DeletePrefix 方法）。
// 这里重新声明以避免 jobs 包直接依赖 objectstore.Client 具体类型。
type Deleter = objectstore.Deleter

// NewDeleteDatabaseHandler 构造 handler。
func NewDeleteDatabaseHandler(
	cat *catalog.Service,
	reg *registry.Registry,
	objs objectstore.Deleter,
	logger observability.Logger,
) *DeleteDatabaseHandler {
	return &DeleteDatabaseHandler{
		catalog:  cat,
		registry: reg,
		objects:  objs,
		logger:   logger,
		now:      time.Now,
	}
}

// Type 返回 delete_database。
func (h *DeleteDatabaseHandler) Type() catalog.JobType { return catalog.JobTypeDeleteDatabase }

// Execute 执行删除任务。幂等：任何步骤失败可重试。
func (h *DeleteDatabaseHandler) Execute(ctx context.Context, job catalog.Job) error {
	var payload DeleteDatabasePayload
	if job.PayloadJSON != "" {
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return fmt.Errorf("jobs: unmarshal delete payload: %w", err)
		}
	}

	// 1) 关闭 registry handle（若存在）。
	if h.registry != nil {
		if err := h.registry.CloseDatabase(ctx, job.DatabaseID); err != nil {
			// 活跃引用存在时无法关闭；延迟重试。
			return fmt.Errorf("jobs: close registry: %w", err)
		}
	}

	// 2) 检查保留截止时间。
	if !payload.RetentionUntil.IsZero() && h.now().Before(payload.RetentionUntil) {
		// 仍在保留期内：不执行删除，返回特定错误触发延迟重试。
		return &retryAfterError{
			after: payload.RetentionUntil.Sub(h.now()),
			msg:   fmt.Sprintf("jobs: database %s in retention until %s", job.DatabaseID, payload.RetentionUntil),
		}
	}

	// 3) 删除 S3 prefix（data 对象）。
	// 注意：不自行复制/拼接 Turso 在线 data/ 对象；DeletePrefix 直接删除整个前缀。
	if h.objects != nil && payload.StoragePrefix != "" {
		if err := h.objects.DeletePrefix(ctx, payload.StoragePrefix); err != nil {
			// S3/DNS/KMS 权限失败不得误标 completed。
			return fmt.Errorf("jobs: delete prefix %s: %w", payload.StoragePrefix, err)
		}
		if h.logger != nil {
			h.logger.Info("jobs: deleted storage prefix",
				zap.String("database_id", job.DatabaseID),
				zap.String("prefix", payload.StoragePrefix))
		}
	}

	// 4) catalog 标记 deleted。
	if h.catalog != nil {
		if err := h.catalog.MarkDatabaseDeleted(ctx, job.DatabaseID, h.now()); err != nil {
			return fmt.Errorf("jobs: mark deleted: %w", err)
		}
	}
	return nil
}

// retryAfterError 表示任务应在指定时间后重试。
type retryAfterError struct {
	after time.Duration
	msg   string
}

func (e *retryAfterError) Error() string { return e.msg }

// IsRetryAfter 让 worker 可识别此错误并调整 RunAfter。
// 当前 worker 的 backoff 已处理通用重试；此接口为未来精确调度预留。
var _ interface{ IsRetryAfter() time.Duration } = (*retryAfterError)(nil)

func (e *retryAfterError) IsRetryAfter() time.Duration { return e.after }

// ErrNoHandler 用于内部断言，不在外部 API 暴露。
var _ = errors.New
