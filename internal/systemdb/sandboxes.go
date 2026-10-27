// 文件说明：sandboxes.go — internal/systemdb 中 SandboxRow、CreateSandbox 等符号的实现
//
// 用法
//
//	(s *Store) CreateSandbox(ctx context.Context, r SandboxRow) (SandboxRow, error)  // CreateSandbox 插入一行
//	(s *Store) GetSandbox(ctx context.Context, projectID, id string) (SandboxRow, error)  // GetSandbox 按 (project, id) 取未删除行
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/sandboxes.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/sqlval, internal/utils/limit
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//
// 主要符号
//
//	type SandboxRow struct {…}
//	(s *Store) CreateSandbox(ctx context.Context, r SandboxRow) (SandboxRow, error)
//	(s *Store) GetSandbox(ctx context.Context, projectID, id string) (SandboxRow, error)
//	(s *Store) GetSandboxByID(ctx context.Context, id string) (SandboxRow, error)
//	(s *Store) GetSandboxByName(ctx context.Context, projectID, name string) ...
//	(s *Store) GetSandboxByThread(ctx context.Context, projectID, threadID string) ...
//	... 另有 13 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"github.com/linkxzhou/SimpleBase/internal/utils/sqlval"
	"time"

	"github.com/google/uuid"

	pagelimit "github.com/linkxzhou/SimpleBase/internal/utils/limit"
)

// SandboxRow 是 sys_sandboxes 一行（planv4.0 cloud-sandbox-plan §6）。
// 只存元数据；命令原文与输出不落库。
type SandboxRow struct {
	ID           string
	ProjectID    string
	Name         string
	CloudName    string
	Source       string // console | api | agent | run
	ThreadID     string
	Image        string
	CPUs         int
	MemoryMiB    int
	Network      string
	IdleTimeoutS int64
	MaxDurationS int64
	Status       string // pending | running | stopped | expired | error | deleted
	LastError    string
	CreatedBy    string
	CreatedAt    time.Time
	StartedAt    time.Time
	LastActiveAt time.Time
	ExpiresAt    time.Time
	DeletedAt    time.Time
	// Metadata 是用户自定义键值（E2B metadata）的 JSON 对象；空串等价于 "{}"。
	// 对应 v50 迁移新增的 metadata 列，追加在列清单末尾。
	Metadata string
}

const sandboxColumns = `id, project_id, name, cloud_name, source, thread_id, image, cpus, memory_mib, network,
	idle_timeout_s, max_duration_s, status, last_error, created_by, created_at, started_at, last_active_at,
	expires_at, deleted_at, metadata`

// CreateSandbox 插入一行；ID 为空时生成 uuid。名称唯一性由调用方先查后写保证。
func (s *Store) CreateSandbox(ctx context.Context, r SandboxRow) (SandboxRow, error) {
	if s == nil || s.db == nil {
		return SandboxRow{}, ErrUnavailable
	}
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_sandboxes(`+sandboxColumns+`)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, r.Name, r.CloudName, r.Source, sqlval.NullString(r.ThreadID), r.Image, r.CPUs, r.MemoryMiB, r.Network,
		r.IdleTimeoutS, r.MaxDurationS, r.Status, r.LastError, r.CreatedBy, r.CreatedAt.UTC(),
		sqlval.NullTimeValue(r.StartedAt), sqlval.NullTimeValue(r.LastActiveAt), sqlval.NullTimeValue(r.ExpiresAt), sqlval.NullTimeValue(r.DeletedAt),
		metadataOrEmpty(r.Metadata))
	if err != nil {
		return SandboxRow{}, err
	}
	s.notifyWrite(ctx)
	return r, nil
}

// GetSandbox 按 (project, id) 取未删除行；无则 sql.ErrNoRows。
func (s *Store) GetSandbox(ctx context.Context, projectID, id string) (SandboxRow, error) {
	if s == nil || s.db == nil {
		return SandboxRow{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sandboxColumns+` FROM sys_sandboxes
		 WHERE id = ? AND project_id = ? AND deleted_at IS NULL`, id, projectID)
	return scanSandbox(row)
}

// GetSandboxByID 不带项目条件按 id 取未删除行。只供「请求里没有项目」的入口使用
// （SSH 登录按沙盒 ID 反查），调用方取到后必须再校验主体对 row.ProjectID 的访问权。
func (s *Store) GetSandboxByID(ctx context.Context, id string) (SandboxRow, error) {
	if s == nil || s.db == nil {
		return SandboxRow{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sandboxColumns+` FROM sys_sandboxes WHERE id = ? AND deleted_at IS NULL`, id)
	return scanSandbox(row)
}

// GetSandboxByName 按项目内名称取未删除行。
func (s *Store) GetSandboxByName(ctx context.Context, projectID, name string) (SandboxRow, error) {
	if s == nil || s.db == nil {
		return SandboxRow{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sandboxColumns+` FROM sys_sandboxes
		 WHERE project_id = ? AND name = ? AND deleted_at IS NULL
		 ORDER BY created_at ASC LIMIT 1`, projectID, name)
	return scanSandbox(row)
}

// GetSandboxByThread 取某 agent thread 的未删除沙盒行。
func (s *Store) GetSandboxByThread(ctx context.Context, projectID, threadID string) (SandboxRow, error) {
	if s == nil || s.db == nil {
		return SandboxRow{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sandboxColumns+` FROM sys_sandboxes
		 WHERE project_id = ? AND thread_id = ? AND deleted_at IS NULL
		 ORDER BY created_at ASC LIMIT 1`, projectID, threadID)
	return scanSandbox(row)
}

// ErrSandboxCursor 表示分页游标不属于该项目或对应行已被物理清理。
var ErrSandboxCursor = errors.New("systemdb: invalid sandbox cursor")

// ListSandboxes 列出项目未删除行（创建时间倒序，id 倒序兜底）。status/source 为空表示不过滤。
// cursor 为上一页最后一行的 id，空表示第一页；返回的 next 为空表示没有更多数据。
func (s *Store) ListSandboxes(ctx context.Context, projectID, status, source, cursor string, limit int) ([]SandboxRow, string, error) {
	if s == nil || s.db == nil {
		return nil, "", ErrUnavailable
	}
	limit = pagelimit.Normalize(limit, 500, 500)
	q := `SELECT ` + sandboxColumns + ` FROM sys_sandboxes WHERE project_id = ? AND deleted_at IS NULL`
	args := []any{projectID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	if source != "" {
		q += ` AND source = ?`
		args = append(args, source)
	}
	if cursor != "" {
		// 游标行可能已软删（仍可定位）；被物理清理或跨项目时拒绝，而不是静默返回空页。
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sys_sandboxes WHERE id = ? AND project_id = ?`, cursor, projectID).Scan(&n); err != nil {
			return nil, "", err
		}
		if n == 0 {
			return nil, "", ErrSandboxCursor
		}
		// 比较在库内完成，避免时间戳经 Go 往返后的精度差异。
		const at = `(SELECT created_at FROM sys_sandboxes WHERE id = ? AND project_id = ?)`
		q += ` AND (created_at < ` + at + ` OR (created_at = ` + at + ` AND id < ?))`
		args = append(args, cursor, projectID, cursor, projectID, cursor)
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.querySandboxes(ctx, q, args...)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = rows[limit-1].ID
	}
	return rows, next, nil
}

// CountSandboxes 统计项目未删除行数（项目上限用）。
func (s *Store) CountSandboxes(ctx context.Context, projectID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_sandboxes WHERE project_id = ? AND deleted_at IS NULL`, projectID).Scan(&n)
	return n, err
}

// UpdateSandbox 写回可变字段（名称、空闲超时、状态、错误、时间戳）。
func (s *Store) UpdateSandbox(ctx context.Context, r SandboxRow) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_sandboxes SET name=?, idle_timeout_s=?, status=?, last_error=?,
			started_at=?, last_active_at=?, expires_at=?
		 WHERE id=? AND project_id=? AND deleted_at IS NULL`,
		r.Name, r.IdleTimeoutS, r.Status, r.LastError,
		sqlval.NullTimeValue(r.StartedAt), sqlval.NullTimeValue(r.LastActiveAt), sqlval.NullTimeValue(r.ExpiresAt),
		r.ID, r.ProjectID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return nil
}

// SoftDeleteSandbox 标 status=deleted 并写 deleted_at。
func (s *Store) SoftDeleteSandbox(ctx context.Context, projectID, id, lastError string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_sandboxes SET status='deleted', last_error=?, deleted_at=?
		 WHERE id=? AND project_id=? AND deleted_at IS NULL`, lastError, now, id, projectID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return nil
}

// ListExpiredSandboxes 取 running/pending 且 expires_at < now 的行（reaper 规则 1）。
func (s *Store) ListExpiredSandboxes(ctx context.Context, now time.Time, limit int) ([]SandboxRow, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	return s.querySandboxes(ctx,
		`SELECT `+sandboxColumns+` FROM sys_sandboxes
		 WHERE deleted_at IS NULL AND status IN ('running', 'pending')
		   AND expires_at IS NOT NULL AND expires_at < ?
		 ORDER BY expires_at ASC LIMIT ?`, now.UTC(), limit)
}

// ListStaleRunSandboxes 取 source=run 且创建早于 before 的未删除行（reaper 规则 2）。
func (s *Store) ListStaleRunSandboxes(ctx context.Context, before time.Time, limit int) ([]SandboxRow, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	return s.querySandboxes(ctx,
		`SELECT `+sandboxColumns+` FROM sys_sandboxes
		 WHERE deleted_at IS NULL AND source = 'run' AND created_at < ?
		 ORDER BY created_at ASC LIMIT ?`, before.UTC(), limit)
}

// ListFailedSandboxRemovals 取用户删除 Cloud VM 失败后的待重试资源。
func (s *Store) ListFailedSandboxRemovals(ctx context.Context, limit int) ([]SandboxRow, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	return s.querySandboxes(ctx,
		`SELECT `+sandboxColumns+` FROM sys_sandboxes
		 WHERE deleted_at IS NULL AND last_error = 'cloud removal failed'
		 ORDER BY created_at ASC LIMIT ?`, limit)
}

// PurgeDeletedSandboxes 物理删除 deleted_at 早于 before 的行（reaper 规则 3）。
func (s *Store) PurgeDeletedSandboxes(ctx context.Context, before time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM sys_sandboxes WHERE deleted_at IS NOT NULL AND deleted_at < ?`, before.UTC())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		s.notifyWrite(ctx)
	}
	return n, nil
}

func (s *Store) querySandboxes(ctx context.Context, q string, args ...any) ([]SandboxRow, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SandboxRow, 0)
	for rows.Next() {
		r, err := scanSandbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanSandbox(sc rowScanner) (SandboxRow, error) {
	var r SandboxRow
	var thread sql.NullString
	var cpus, mem int64
	var started, active, expires, deleted sql.NullTime
	var meta sql.NullString
	err := sc.Scan(&r.ID, &r.ProjectID, &r.Name, &r.CloudName, &r.Source, &thread, &r.Image, &cpus, &mem, &r.Network,
		&r.IdleTimeoutS, &r.MaxDurationS, &r.Status, &r.LastError, &r.CreatedBy, &r.CreatedAt,
		&started, &active, &expires, &deleted, &meta)
	if errors.Is(err, sql.ErrNoRows) {
		return SandboxRow{}, sql.ErrNoRows
	}
	if err != nil {
		return SandboxRow{}, err
	}
	r.ThreadID = thread.String
	r.Metadata = metadataOrEmpty(meta.String)
	r.CPUs, r.MemoryMiB = int(cpus), int(mem)
	if started.Valid {
		r.StartedAt = started.Time
	}
	if active.Valid {
		r.LastActiveAt = active.Time
	}
	if expires.Valid {
		r.ExpiresAt = expires.Time
	}
	if deleted.Valid {
		r.DeletedAt = deleted.Time
	}
	return r, nil
}

// metadataOrEmpty 把空串归一为 "{}"，保证库里始终是合法 JSON 对象。
func metadataOrEmpty(m string) string {
	if m == "" {
		return "{}"
	}
	return m
}
