// 文件说明：service.go — 管理操作审计（plan9.md）
//
// 用法
//
//	NewService(repo catalog.Repository, logger observability.Logger) *Service
//	(s *Service) Record(ctx context.Context, e Event) error  // Record 记录一条审计事件
//	(s *Service) ListOperations(ctx context.Context, q Query) ([]catalog.Operation ...  // ListOperations 返回审计事件列表
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/audit/service.go
//	         │ 依赖
//	         ▼
//	[下游] internal/catalog, internal/observability, internal/utils/limit
//
// 主要符号
//
//	type Service struct {…}
//	NewService(repo catalog.Repository, logger observability.Logger) *Service
//	type Event struct {…}
//	(s *Service) Record(ctx context.Context, e Event) error
//	type Query struct {…}
//	(s *Service) ListOperations(ctx context.Context, q Query) ([]catalog.Operation ...

package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/utils/limit"
)

// Service 提供审计事件记录与查询。
type Service struct {
	repo   catalog.Repository
	logger observability.Logger
	now    func() time.Time
}

// NewService 构造 Service。
func NewService(repo catalog.Repository, logger observability.Logger) *Service {
	return &Service{repo: repo, logger: logger, now: time.Now}
}

// Event 是审计事件输入。
type Event struct {
	DatabaseID  string
	ProjectID   string
	PrincipalID string
	Kind        string // "create"|"delete"|"open"|"close"|"restore"|"config_change"|"key_rotate"...
	RequestID   string
	Status      string // "ok"|"error"|"denied"
	Detail      string // 附加说明（已脱敏）
}

// Record 记录一条审计事件。不可变：插入后不修改。
func (s *Service) Record(ctx context.Context, e Event) error {
	if e.PrincipalID == "" {
		e.PrincipalID = "system"
	}
	if e.Kind == "" {
		return fmt.Errorf("audit: kind required")
	}
	op := catalog.Operation{
		ID:          uuid.NewString(),
		DatabaseID:  e.DatabaseID,
		ProjectID:   e.ProjectID,
		PrincipalID: e.PrincipalID,
		Kind:        e.Kind,
		RequestID:   e.RequestID,
		Status:      e.Status,
		CreatedAt:   s.now(),
	}
	if e.Detail != "" {
		// Detail 追加到 Status 字段（schema 无独立 detail 列）。
		op.Status = op.Status + ": " + e.Detail
	}
	return s.repo.AppendOperation(ctx, op)
}

// Query 按条件查询审计事件。
type Query struct {
	ProjectID  string
	DatabaseID string
	Kind       string
	Since      time.Time
	Limit      int
}

// ListOperations 返回审计事件列表。依赖 catalog.Repository.ListOperations。
func (s *Service) ListOperations(ctx context.Context, q Query) ([]catalog.Operation, error) {
	q.Limit = limit.Normalize(q.Limit, 50, 200)
	return s.repo.ListOperations(ctx, q.ProjectID, q.DatabaseID, q.Limit)
}
