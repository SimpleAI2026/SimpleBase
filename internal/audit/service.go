// Package audit 实现管理操作审计（plan9.md）。
//
// 所有写操作（创建/删除库、配置变更、密钥轮换、恢复）必须记录不可变审计事件。
// 审计事件复用 catalog.Operation 表；本包提供便捷的 Service 和查询 API。
// 审计日志禁止回显密钥原文、SQL 参数中的敏感数据。
package audit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
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
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	return s.repo.ListOperations(ctx, q.ProjectID, q.DatabaseID, q.Limit)
}

// Redact 对敏感字符串做脱敏（保留前2后2字符）。
func Redact(s string) string {
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}
