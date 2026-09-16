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
	// storage 是写入 descriptor.ducklake_storage 时的模板（Prefix 按库填充）。
	// DevMode 下 descriptor 为 nil 时可不设置。
	storage objectstore.DuckLakeStorage
	logger  observability.Logger
	now     Clock
}

// Repository 返回底层 Repository，供 usage/audit/jobs 等内部服务共享访问。
// 外部 handler 不得使用此方法绕过 Service 校验。
func (s *Service) Repository() Repository { return s.repo }

// NewService 构造 Service。descriptor 可为 nil（DevMode / 测试未配置 S3 时跳过 descriptor 写入）。
// storage 在 descriptor 非 nil 时应提供 bucket/region 等（Prefix 按库生成）。
func NewService(repo Repository, keys objectstore.KeyBuilder, descriptor DescriptorWriter, storage objectstore.DuckLakeStorage, logger observability.Logger) *Service {
	return &Service{
		repo:       repo,
		keys:       keys,
		descriptor: descriptor,
		storage:    storage,
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
// 状态 creating → 写 DB 记录 → 写 S3 descriptor → 同步转为 ready；失败标记 degraded。
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
		st := s.storage
		st.Prefix = prefix
		desc := objectstore.Descriptor{
			FormatVersion:   objectstore.DescriptorFormatVersion,
			TenantID:        in.TenantID,
			ProjectID:       in.ProjectID,
			DatabaseID:      id,
			Name:            in.Name,
			CreatedAt:       now,
			DataPrefix:      prefix,
			Status:          objectstore.StatusCreating,
			Engine:          "ducklake",
			DuckLakeStorage: st,
		}
		if err := desc.Validate(); err != nil {
			s.degradeAfterCreateFailure(ctx, id, err)
			return Database{}, fmt.Errorf("%w: invalid descriptor: %v", ErrDescriptorWrite, err)
		}
		if err := s.descriptor.PutJSON(ctx, descKey, desc, objectstore.PutOptions{}); err != nil {
			// descriptor 写失败：标记 degraded，不删除已写入的 catalog 记录，
			// 由运维/恢复流程决定补偿删除或重试。
			s.degradeAfterCreateFailure(ctx, id, err)
			return Database{}, fmt.Errorf("%w: %v", ErrDescriptorWrite, err)
		}
	}

	// DuckLake 创建无异步开通：catalog（+可选 descriptor）写成功即视为就绪。
	// 否则会永久停在 creating（历史上依赖未接线的 Runtime 回调）。
	if err := s.SetDatabaseReady(ctx, id); err != nil {
		s.degradeAfterCreateFailure(ctx, id, err)
		return Database{}, fmt.Errorf("catalog: mark ready after create: %w", err)
	}
	db.Status = DatabaseReady
	db.UpdatedAt = s.now()
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

// ListProjects 返回当前 principal 可见的项目列表。
// ProjectAdmin：租户下全部项目；否则仅返回 API key 授权的 ProjectIDs（并补全 name）。
func (s *Service) ListProjects(ctx context.Context, principal auth.Principal) ([]Project, error) {
	if principal.TenantID == "" {
		return nil, fmt.Errorf("%w: tenant required", ErrInvalidName)
	}
	all, err := s.repo.ListProjectsByTenant(ctx, principal.TenantID)
	if err != nil {
		return nil, err
	}
	if principal.HasPermission(auth.ProjectAdmin) {
		return all, nil
	}
	out := make([]Project, 0, len(all))
	for _, p := range all {
		if principal.CanAccessProject(p.ID) {
			out = append(out, p)
		}
	}
	return out, nil
}

// BeginDeleteDatabase 校验归属后将数据库转入 deleting。
func (s *Service) BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (Database, error) {
	if !principal.CanAccessProject(projectID) {
		return Database{}, fmt.Errorf("%w: project %s", ErrCrossProject, projectID)
	}
	current, err := s.repo.GetDatabase(ctx, projectID, databaseID)
	if err != nil {
		return Database{}, err
	}
	from := []DatabaseStatus{DatabaseCreating, DatabaseOpening, DatabaseReady, DatabaseClosed, DatabaseDegraded, DatabaseRecovering}
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

// StoragePurger 抽象同步删除平面 B（DuckLake data/catalog/descriptor 所在前缀）。
type StoragePurger interface {
	DeletePrefix(ctx context.Context, prefix string) error
}

// DeleteDatabaseSync 软删并同步清理存储：deleting → 关闭连接 → DeletePrefix(库前缀) → deleted。
// purger / closer 可为 nil（DevMode 无 S3 时只更新 catalog）。
// 产品决策：不走 jobs 异步队列完成本轮清理。
func (s *Service) DeleteDatabaseSync(
	ctx context.Context,
	principal auth.Principal,
	projectID, databaseID string,
	closer func(ctx context.Context, databaseID string) error,
	purger StoragePurger,
) (Database, error) {
	db, err := s.BeginDeleteDatabase(ctx, principal, projectID, databaseID)
	if err != nil {
		return Database{}, err
	}
	if closer != nil {
		if err := closer(ctx, db.ID); err != nil && s.logger != nil {
			s.logger.Warn("catalog: close registry during delete",
				zap.String("database_id", db.ID), zap.Error(err))
		}
	}
	if purger != nil {
		prefix := db.StoragePrefix
		if dbPrefix, err := s.keys.DatabasePrefix(db.TenantID, db.ID); err == nil && dbPrefix != "" {
			prefix = dbPrefix
		}
		if prefix != "" {
			if err := purger.DeletePrefix(ctx, prefix); err != nil {
				return db, fmt.Errorf("catalog: sync purge storage prefix %s: %w", prefix, err)
			}
		}
	}
	if err := s.MarkDatabaseDeleted(ctx, db.ID, s.now()); err != nil {
		return db, err
	}
	db.Status = DatabaseDeleted
	return db, nil
}

// SetDatabaseReady 将数据库从 creating/opening/recovering 转为 ready。
// 已是 ready 时幂等成功（open / 重复回调安全）。
func (s *Service) SetDatabaseReady(ctx context.Context, id string) error {
	from := []DatabaseStatus{DatabaseCreating, DatabaseOpening, DatabaseRecovering}
	_, err := s.repo.TransitionDatabase(ctx, id, from, DatabaseReady, s.now())
	if err != nil && errors.Is(err, ErrInvalidState) {
		return nil // 已是 ready 或其他终态外的不可转换：创建路径已就绪时忽略
	}
	return err
}

// MarkDatabaseDeleted 将数据库从 deleting 转为 deleted 并记录 deleted_at。
// 由 DeleteDatabaseSync（或遗留 jobs handler）在完成存储清理后调用。幂等：重复调用不报错。
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
