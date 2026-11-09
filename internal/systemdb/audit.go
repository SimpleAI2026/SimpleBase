// 文件说明：audit.go — 系统级审计事件（production-readiness-plan §8.2.5 / §8.3.3）
//
// 用法
//
//	(s *Store) RecordAudit(ev AuditEvent)                 // 缓冲一条审计（满批后台刷写）
//	(s *Store) FlushAudits(ctx) error                     // 批量落 sys_audit_events
//	(s *Store) QueryAudits(ctx, q AuditQuery) ([]AuditEvent, error)
//
// 写入路径与 RecordLog 同款：请求路径只入队，后台 flusher 批量写；
// 失败丢弃并计数，绝不阻塞业务请求。
package systemdb

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/utils/limit"
)

// AuditEvent 是 sys_audit_events 一行。
// Actor 域分隔：user:<id> | key:<id> | enduser:<id> | system。
type AuditEvent struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	Actor      string    `json:"actor"`
	ProjectID  string    `json:"project_id"`
	Kind       string    `json:"kind"`
	Detail     string    `json:"detail,omitempty"`
}

// AuditQuery 是审计查询条件。
type AuditQuery struct {
	Actor     string
	ProjectID string
	Kind      string
	From      time.Time
	Limit     int
}

// auditDetailMax 是 detail 截断长度（§8.3.3 脱敏：只存结构化短字符串）。
const auditDetailMax = 512

// RecordAudit 缓冲一条系统审计事件（§8.3.3：与 RecordLog 同款缓冲 + 后台批量刷）。
func (s *Store) RecordAudit(ev AuditEvent) {
	if s == nil {
		return
	}
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	if ev.Actor == "" {
		ev.Actor = "system"
	}
	ev.Kind = strings.TrimSpace(ev.Kind)
	if ev.Kind == "" {
		return
	}
	// 脱敏：detail 只保留短字符串并截断，杜绝塞入请求体。
	ev.Detail = strings.TrimSpace(ev.Detail)
	if len(ev.Detail) > auditDetailMax {
		ev.Detail = ev.Detail[:auditDetailMax]
	}
	s.mu.Lock()
	if len(s.auditBuf) >= flushQueueCap {
		s.droppedAudit++
		s.mu.Unlock()
		return
	}
	s.auditBuf = append(s.auditBuf, ev)
	n := len(s.auditBuf)
	f := s.async
	closed := s.closed
	s.mu.Unlock()
	if n >= 64 {
		switch {
		case f != nil:
			f.notify()
		case !closed:
			_ = s.FlushAudits(context.Background())
		}
	}
}

// FlushAudits 将缓冲审计写入 sys_audit_events（同步兜底路径；后台 flusher
// 走 flushAllAsync 同一事务的 insertAudits）。
func (s *Store) FlushAudits(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	batch := s.drainAudits()
	if len(batch) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.requeueAudits(batch)
		return err
	}
	if err := insertAudits(ctx, tx, batch); err != nil {
		_ = tx.Rollback()
		s.requeueAudits(batch)
		return err
	}
	if err := tx.Commit(); err != nil {
		s.requeueAudits(batch)
		return err
	}
	s.NotifyWrite(ctx)
	return nil
}

// drainAudits 取走全部缓冲审计（调用方负责落盘或 requeue）。
func (s *Store) drainAudits() []AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	batch := s.auditBuf
	s.auditBuf = nil
	return batch
}

// insertAudits 在事务内批量插入审计行。
func insertAudits(ctx context.Context, ex execer, batch []AuditEvent) error {
	for _, ev := range batch {
		if _, err := ex.ExecContext(ctx, `INSERT INTO sys_audit_events (id, occurred_at, actor, project_id, kind, detail) VALUES (?, ?, ?, ?, ?, ?)`,
			ev.ID, ev.OccurredAt, ev.Actor, ev.ProjectID, ev.Kind, ev.Detail); err != nil {
			return err
		}
	}
	return nil
}

// requeueAudits 把失败批次放回队首（保持顺序；容量满则丢弃最旧）。
func (s *Store) requeueAudits(batch []AuditEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	merged := append(batch, s.auditBuf...)
	if len(merged) > flushQueueCap {
		merged = merged[len(merged)-flushQueueCap:]
	}
	s.auditBuf = merged
}

// QueryAudits 按条件查询系统审计事件（时间倒序）。
func (s *Store) QueryAudits(ctx context.Context, q AuditQuery) ([]AuditEvent, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	q.Limit = limit.Normalize(q.Limit, 50, 200)
	sqlStr := `SELECT id, occurred_at, actor, project_id, kind, detail FROM sys_audit_events WHERE 1=1`
	args := []any{}
	if q.Actor != "" {
		sqlStr += " AND actor = ?"
		args = append(args, q.Actor)
	}
	if q.ProjectID != "" && !IsAdminProject(q.ProjectID) {
		sqlStr += " AND project_id = ?"
		args = append(args, q.ProjectID)
	}
	if q.Kind != "" {
		sqlStr += " AND kind = ?"
		args = append(args, q.Kind)
	}
	if !q.From.IsZero() {
		sqlStr += " AND occurred_at >= ?"
		args = append(args, q.From.UTC())
	}
	sqlStr += " ORDER BY occurred_at DESC LIMIT ?"
	args = append(args, int64(q.Limit))
	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var ev AuditEvent
		if err := rows.Scan(&ev.ID, &ev.OccurredAt, &ev.Actor, &ev.ProjectID, &ev.Kind, &ev.Detail); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
