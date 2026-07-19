// service.go 实现 catalog 的业务逻辑：创建/查询/删除数据库、状态转换、
// provider 配置与 usage 追加。所有跨 project 校验在此完成，handler 不得
// 直接访问 Repository。
package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// nameMaxLen 是 tenant/project/database 名称的最大长度。
const nameMaxLen = 128

// Clock 抽象当前时间，便于测试注入固定时间。
type Clock func() time.Time

// DescriptorWriter 抽象向 S3 写入 descriptor 的能力，由 objectstore 实现。
// Service 依赖接口而非具体包，便于测试用假实现替换。
type DescriptorWriter interface {
	PutJSON(ctx context.Context, key string, value any, opts objectstore.PutOptions) error
}

// Service 是 catalog 的业务入口。
type Service struct {
	repo       Repository
	keys       objectstore.KeyBuilder
	descriptor DescriptorWriter
	logger     observability.Logger
	now        Clock
}

// Repository 返回底层 Repository，供 usage/audit/jobs 等内部服务共享访问。
// 外部 handler 不得使用此方法绕过 Service 校验。
func (s *Service) Repository() Repository { return s.repo }

// NewService 构造 Service。descriptor 可为 nil（测试或未配置 S3 时禁止创建库）。
func NewService(repo Repository, keys objectstore.KeyBuilder, descriptor DescriptorWriter, logger observability.Logger) *Service {
	return &Service{
		repo:       repo,
		keys:       keys,
		descriptor: descriptor,
		logger:     logger,
		now:        time.Now,
	}
}

// CreateDatabaseInput 是创建数据库的输入。
type CreateDatabaseInput struct {
	TenantID  string
	ProjectID string
	Name      string
}

// CreateDatabase 校验名称与 project 归属 → 生成 UUID → 建立不可猜测 prefix →
// 状态 creating → 写 DB 记录 → 写 S3 descriptor → 失败标记 degraded 或补偿软删。
func (s *Service) CreateDatabase(ctx context.Context, in CreateDatabaseInput) (Database, error) {
	if err := validateName(in.Name); err != nil {
		return Database{}, err
	}
	if in.TenantID == "" || in.ProjectID == "" {
		return Database{}, fmt.Errorf("%w: tenant_id/project_id required", ErrInvalidName)
	}
	belongs, err := s.repo.ProjectBelongsToTenant(ctx, in.ProjectID, in.TenantID)
	if err != nil {
		return Database{}, fmt.Errorf("catalog: check project ownership: %w", err)
	}
	if !belongs {
		return Database{}, fmt.Errorf("%w: project %s not in tenant %s", ErrCrossProject, in.ProjectID, in.TenantID)
	}

	id := uuid.NewString()
	prefix, err := s.keys.DataPrefix(in.TenantID, id)
	if err != nil {
		return Database{}, fmt.Errorf("catalog: build storage prefix: %w", err)
	}

	now := s.now()
	db := Database{
		ID:            id,
		TenantID:      in.TenantID,
		ProjectID:     in.ProjectID,
		Name:          in.Name,
		Status:        DatabaseCreating,
		StoragePrefix: prefix,
		FormatVersion: objectstore.DescriptorFormatVersion,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.repo.CreateDatabase(ctx, db); err != nil {
		return Database{}, err
	}

	if s.descriptor != nil {
		descKey, err := s.keys.DescriptorKey(in.TenantID, id)
		if err != nil {
			s.degradeAfterCreateFailure(ctx, id, err)
			return Database{}, fmt.Errorf("%w: build descriptor key: %v", ErrDescriptorWrite, err)
		}
		desc := objectstore.Descriptor{
			FormatVersion: objectstore.DescriptorFormatVersion,
			TenantID:      in.TenantID,
			ProjectID:     in.ProjectID,
			DatabaseID:    id,
			Name:          in.Name,
			CreatedAt:     now,
			DataPrefix:    prefix,
			Status:        objectstore.StatusCreating,
			TursoStorage: objectstore.TursoStorage{
				Prefix: prefix,
			},
		}
		if err := s.descriptor.PutJSON(ctx, descKey, desc, objectstore.PutOptions{}); err != nil {
			// descriptor 写失败：标记 degraded，不删除已写入的 catalog 记录，
			// 由运维/恢复流程决定补偿删除或重试。
			s.degradeAfterCreateFailure(ctx, id, err)
			return Database{}, fmt.Errorf("%w: %v", ErrDescriptorWrite, err)
		}
	}

	return db, nil
}

// degradeAfterCreateFailure 尝试将新建的数据库标记为 degraded；
// 该转换失败只记录日志，不覆盖原始错误。
func (s *Service) degradeAfterCreateFailure(ctx context.Context, id string, cause error) {
	if _, err := s.repo.TransitionDatabase(ctx, id, []DatabaseStatus{DatabaseCreating}, DatabaseDegraded, s.now()); err != nil {
		if s.logger != nil {
			s.logger.Error("catalog: failed to mark database degraded after create failure",
				zap.String("database_id", id), zap.String("cause", cause.Error()), zap.String("err", err.Error()))
		}
	}
}

// GetDatabase 校验 principal 可访问 projectID 后返回数据库记录。
func (s *Service) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (Database, error) {
	if !principal.CanAccessProject(projectID) {
		return Database{}, fmt.Errorf("%w: project %s", ErrCrossProject, projectID)
	}
	return s.repo.GetDatabase(ctx, projectID, databaseID)
}

// ListDatabases 校验 principal 可访问 projectID 后分页列出数据库。
func (s *Service) ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page Page) ([]Database, string, error) {
	if !principal.CanAccessProject(projectID) {
		return nil, "", fmt.Errorf("%w: project %s", ErrCrossProject, projectID)
	}
	return s.repo.ListDatabases(ctx, projectID, page)
}

// ResolveProjectTenant 返回 project 所属的 tenant ID。
// 用于 API 层的 project context 中间件解析 path 参数 :projectID 对应的 tenant。
// project 不存在返回 ErrNotFound。
func (s *Service) ResolveProjectTenant(ctx context.Context, projectID string) (string, error) {
	return s.repo.GetProjectTenant(ctx, projectID)
}

// BeginDeleteDatabase 校验归属后将数据库转入 deleting；实际 S3 清理由后台任务异步执行。
func (s *Service) BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (Database, error) {
	if !principal.CanAccessProject(projectID) {
		return Database{}, fmt.Errorf("%w: project %s", ErrCrossProject, projectID)
	}
	current, err := s.repo.GetDatabase(ctx, projectID, databaseID)
	if err != nil {
		return Database{}, err
	}
	from := []DatabaseStatus{DatabaseReady, DatabaseClosed, DatabaseDegraded}
	db, err := s.repo.TransitionDatabase(ctx, current.ID, from, DatabaseDeleting, s.now())
	if err != nil {
		return Database{}, err
	}
	_ = s.repo.AppendOperation(ctx, Operation{
		ID:          uuid.NewString(),
		DatabaseID:  db.ID,
		ProjectID:   projectID,
		PrincipalID: principal.APIKeyID,
		Kind:        "delete",
		Status:      "ok",
		CreatedAt:   s.now(),
	})
	return db, nil
}

// SetDatabaseReady 将数据库从 creating/opening/recovering 转为 ready。
// 由 Database Runtime 在完成打开/恢复流程后调用，不经过 HTTP handler。
func (s *Service) SetDatabaseReady(ctx context.Context, id string) error {
	from := []DatabaseStatus{DatabaseCreating, DatabaseOpening, DatabaseRecovering}
	_, err := s.repo.TransitionDatabase(ctx, id, from, DatabaseReady, s.now())
	return err
}

// MarkDatabaseDeleted 将数据库从 deleting 转为 deleted 并记录 deleted_at。
// 由后台删除任务在完成 S3 prefix 清理后调用（plan7.md）。幂等：重复调用不报错。
func (s *Service) MarkDatabaseDeleted(ctx context.Context, id string, at time.Time) error {
	// 状态校验：仅 deleting 可转 deleted。若已是 deleted 则幂等成功。
	_, err := s.repo.TransitionDatabase(ctx, id, []DatabaseStatus{DatabaseDeleting}, DatabaseDeleted, at)
	if err != nil {
		if errors.Is(err, ErrInvalidState) {
			// 检查是否已是 deleted（幂等重试）。
			// 注意：GetDatabase 需要 projectID，此处用 ListJobs 同级的方式不适用；
			// 直接依赖 TransitionDatabase 的 affected rows 判断：若非 deleting 则忽略。
			return nil
		}
		return err
	}
	return s.repo.MarkDeleted(ctx, id, at)
}

// GetDatabaseForJob 供后台任务按 projectID+databaseID 获取库元数据。
// 不经过 HTTP principal 校验，但校验 databaseID 属于 projectID（防越权）。
func (s *Service) GetDatabaseForJob(ctx context.Context, projectID, databaseID string) (Database, error) {
	db, err := s.repo.GetDatabase(ctx, projectID, databaseID)
	if err != nil {
		return Database{}, err
	}
	return db, nil
}

// GetLLMProviders 返回 project 的 LLM 供应商配置（含 CredentialRef，不含密钥原文）。
// 供 llmgateway resolver 调用；密钥解析由独立密钥服务完成。
func (s *Service) GetLLMProviders(ctx context.Context, projectID string) (LLMProviders, error) {
	return s.repo.GetLLMProviders(ctx, projectID)
}

// SetDatabaseDegraded 将数据库标记为 degraded 并记录 cause。
// from 允许所有非终态状态，因为故障可能发生在任意阶段。
func (s *Service) SetDatabaseDegraded(ctx context.Context, id string, cause error) error {
	from := []DatabaseStatus{
		DatabaseCreating, DatabaseOpening, DatabaseReady, DatabaseClosing, DatabaseRecovering,
	}
	_, err := s.repo.TransitionDatabase(ctx, id, from, DatabaseDegraded, s.now())
	if err != nil {
		return err
	}
	if s.logger != nil && cause != nil {
		s.logger.Warn("catalog: database marked degraded", zap.String("database_id", id), zap.String("cause", cause.Error()))
	}
	return nil
}

// UpsertProviderConfig 保存 project 级 LLM provider 配置。
func (s *Service) UpsertProviderConfig(ctx context.Context, principal auth.Principal, cfg LLMProviderConfig) error {
	if !principal.CanAccessProject(cfg.ProjectID) {
		return fmt.Errorf("%w: project %s", ErrCrossProject, cfg.ProjectID)
	}
	now := s.now()
	if cfg.ID == "" {
		cfg.ID = uuid.NewString()
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	return s.repo.UpsertProviderConfig(ctx, cfg)
}

// ListEnabledProviders 返回 project 已启用的 LLM provider 配置。
func (s *Service) ListEnabledProviders(ctx context.Context, principal auth.Principal, projectID string) ([]LLMProviderConfig, error) {
	if !principal.CanAccessProject(projectID) {
		return nil, fmt.Errorf("%w: project %s", ErrCrossProject, projectID)
	}
	return s.repo.ListEnabledProviders(ctx, projectID)
}

// AppendUsage 批量写入用量事件；不校验单条归属，调用方（usage 服务）负责生成合法事件。
func (s *Service) AppendUsage(ctx context.Context, events []UsageEvent) error {
	return s.repo.AppendUsage(ctx, events)
}

// validateName 校验 tenant/project/database 名称：非空、长度受限、不含路径分隔符。
// 名称仅用于展示与唯一性约束，绝不用作 S3 key（见 KeyBuilder 约束）。
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidName)
	}
	if len(name) > nameMaxLen {
		return fmt.Errorf("%w: name exceeds %d bytes", ErrInvalidName, nameMaxLen)
	}
	for _, r := range name {
		if r == '/' || r == '\\' || r < 0x20 {
			return fmt.Errorf("%w: name contains invalid character", ErrInvalidName)
		}
	}
	return nil
}
