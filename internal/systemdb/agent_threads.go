// 文件说明：agent_threads.go — 云助手会话（AgentThread）与消息（AgentMessage）的持久化与分页
//
// 用法
//
//	AgentThread、AgentMessage、CreateAgentThread（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] godatabase/bench, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/agent_threads.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/ducklake, godatabase/limit, internal/utils/strutil
//	[同包] async_flush.go, bootstrap.go, cron_jobs.go, function_triggers.go 等 22 个
//
// 主要符号
//
//	type AgentThread struct {…}
//	type AgentMessage struct {…}
//	func (s *Store) CreateAgentThread(ctx context.Context, t AgentThread) (AgentThread, error)
//	func (s *Store) ListAgentThreads(ctx context.Context, projectID, agentID string, limit int) ([]AgentThread, er
//	func (s *Store) GetAgentThread(ctx context.Context, projectID, id string) (AgentThread, error)
//	func (s *Store) RenameAgentThread(ctx context.Context, projectID, id, title string) (AgentThread, error)
//	type AgentThreadFilter struct {…}
//	func (s *Store) PageAgentThreads(ctx context.Context, projectID, agentID, cursor string, limit int) ([]AgentTh
//	... 另有 5 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	pagelimit "github.com/linkxzhou/SimpleBase/godatabase/limit"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// AgentThread 是 sys_agent_threads 一行。
type AgentThread struct {
	ID        string
	ProjectID string
	Title     string
	// AgentID 是会话归属的助手（planv6.0 §3.7 修订：每个助手一份会话）。
	// 空串表示未绑定，读取时按「默认助手」处理，兼容 v47 之前的历史数据。
	AgentID   string
	CreatedBy string
	// StarterTemplate 非空表示这是首页「一句话建应用」的搭建会话（home-starter-prompts-plan §2.4）。
	// 只由服务端在创建时校验并登记，run 时据此换算技能；不接受消息内容改写。
	StarterTemplate string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// AgentMessage 是 sys_agent_messages 一行。
type AgentMessage struct {
	ID            string
	ThreadID      string
	ProjectID     string
	Role          string
	Content       string
	AgentID       string
	MentionsJSON  string
	ToolCallsJSON string
	RunID         string
	RunStatus     string
	ErrorCode     string
	CreatedAt     time.Time
}

// CreateAgentThread 创建会话线程（不写工作台元数据，旧语义保留给
// 定时任务、旧 SDK 与既有测试；planv6.7 AGC-NAV §7.2）。
func (s *Store) CreateAgentThread(ctx context.Context, t AgentThread) (AgentThread, error) {
	return s.CreateAgentThreadWithWorkspace(ctx, t, AgentThreadWorkspace{})
}

// ListAgentThreads 列出未归档线程。agentID 非空时只返回该助手的会话；
// 空串表示不过滤（保留给内部调用与既有测试）。
func (s *Store) ListAgentThreads(ctx context.Context, projectID, agentID string, limit int) ([]AgentThread, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, connErr
	}

	limit = pagelimit.Normalize(limit, 50, 200)
	query := `SELECT id, project_id, title, COALESCE(agent_id, ''), created_by, created_at, updated_at
		 FROM sys_agent_threads WHERE project_id = ? AND archived_at IS NULL`
	args := []any{projectID}
	if agentID != "" {
		query += ` AND COALESCE(agent_id, '') = ?`
		args = append(args, agentID)
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, int64(limit))
	rows, err := sqlDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentThread, 0)
	for rows.Next() {
		var t AgentThread
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.AgentID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetAgentThread 按 ID 取未归档线程。
func (s *Store) GetAgentThread(ctx context.Context, projectID, id string) (AgentThread, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentThread{}, connErr
	}

	var t AgentThread
	err := sqlDB.QueryRowContext(ctx,
		`SELECT id, project_id, title, COALESCE(agent_id, ''), created_by, created_at, updated_at
		 FROM sys_agent_threads WHERE id = ? AND project_id = ? AND archived_at IS NULL`,
		id, projectID).Scan(&t.ID, &t.ProjectID, &t.Title, &t.AgentID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentThread{}, sql.ErrNoRows
	}
	return t, err
}

// RenameAgentThread 仅修改项目内未归档会话标题。
func (s *Store) RenameAgentThread(ctx context.Context, projectID, id, title string) (AgentThread, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentThread{}, connErr
	}

	res, err := sqlDB.ExecContext(ctx, `UPDATE sys_agent_threads SET title=?, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL`, title, time.Now().UTC(), id, projectID)
	if err != nil {
		return AgentThread{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return AgentThread{}, sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return s.GetAgentThread(ctx, projectID, id)
}

// ErrAgentThreadCursor 表示游标不存在或属于其它项目。
var ErrAgentThreadCursor = errors.New("systemdb: invalid agent thread cursor")

// AgentThreadFilter 是会话分页过滤条件（planv6.7 AGC-NAV §7.3）；零值等价于旧行为。
type AgentThreadFilter struct {
	AgentID string
	// TagID 非空时只返回该标签的会话。
	TagID string
	// ThreadKind 非空时只返回该类型的会话（chat/alert/statistics/scheduled）。
	ThreadKind string
}

// PageAgentThreads 按更新时间/id 倒序 keyset 分页（旧签名，薄包装）。
// agentID 非空时只返回该助手的会话；空串不过滤。
func (s *Store) PageAgentThreads(ctx context.Context, projectID, agentID, cursor string, limit int) ([]AgentThread, string, error) {
	return s.PageAgentThreadsFiltered(ctx, projectID, AgentThreadFilter{AgentID: agentID}, cursor, limit)
}

// PageAgentThreadsFiltered 在 PageAgentThreads 基础上支持标签与会话类型过滤。
func (s *Store) PageAgentThreadsFiltered(ctx context.Context, projectID string, f AgentThreadFilter, cursor string, limit int) ([]AgentThread, string, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, "", connErr
	}

	limit = pagelimit.Normalize(limit, 50, 200)
	query := `SELECT t.id, t.project_id, t.title, COALESCE(t.agent_id, ''), t.created_by, t.created_at, t.updated_at
		FROM sys_agent_threads t
		LEFT JOIN sys_agent_thread_workspace w ON w.project_id = t.project_id AND w.thread_id = t.id
		WHERE t.project_id=? AND t.archived_at IS NULL`
	if f.ThreadKind != "" {
		// 只有按类型过滤时才 join 协作表：旧库（未建协作表）的普通列表必须继续可用，
		// 未升级环境由 collab_schema 能力探测与写入口返回 workspace_upgrade_required。
		query = `SELECT t.id, t.project_id, t.title, COALESCE(t.agent_id, ''), t.created_by, t.created_at, t.updated_at
			FROM sys_agent_threads t
			LEFT JOIN sys_agent_thread_workspace w ON w.project_id = t.project_id AND w.thread_id = t.id
			LEFT JOIN sys_agent_thread_collab c ON c.project_id = t.project_id AND c.thread_id = t.id
			WHERE t.project_id=? AND t.archived_at IS NULL`
	}
	args := []any{projectID}
	if f.AgentID != "" {
		query += ` AND COALESCE(t.agent_id, '') = ?`
		args = append(args, f.AgentID)
	}
	if f.TagID != "" {
		query += ` AND w.tag_id = ?`
		args = append(args, f.TagID)
	}
	if f.ThreadKind != "" {
		// 会话类型旁挂表：旧会话无行时按 chat 处理（COALESCE）。
		query += ` AND COALESCE(c.thread_kind, '` + ThreadKindChat + `') = ?`
		args = append(args, f.ThreadKind)
	}
	if cursor != "" {
		var n int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_agent_threads WHERE id=? AND project_id=?`, cursor, projectID).Scan(&n); err != nil {
			return nil, "", err
		}
		if n == 0 {
			return nil, "", ErrAgentThreadCursor
		}
		const at = `(SELECT updated_at FROM sys_agent_threads WHERE id=? AND project_id=?)`
		query += ` AND (t.updated_at < ` + at + ` OR (t.updated_at = ` + at + ` AND t.id < ?))`
		args = append(args, cursor, projectID, cursor, projectID, cursor)
	}
	query += ` ORDER BY t.updated_at DESC, t.id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := sqlDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := make([]AgentThread, 0)
	for rows.Next() {
		var t AgentThread
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.AgentID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, "", err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[limit-1].ID
	}
	return out, next, nil
}

// AgentThreadPreview 只返回当前会话最新消息的前 80 个字符。
func (s *Store) AgentThreadPreview(ctx context.Context, projectID, threadID string) (string, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return "", connErr
	}

	var text string
	err := sqlDB.QueryRowContext(ctx, `SELECT content FROM sys_agent_messages WHERE project_id=? AND thread_id=? ORDER BY created_at DESC, id DESC LIMIT 1`, projectID, threadID).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strutil.TruncateRunes(text, 80), nil
}

// ArchiveAgentThread 软删线程。
func (s *Store) ArchiveAgentThread(ctx context.Context, projectID, id string) error {
	now := time.Now().UTC()
	return s.execAffectOne(ctx,
		`UPDATE sys_agent_threads SET archived_at=?, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL`,
		now, now, id, projectID)
}

// AppendAgentMessage 写入一条消息并刷新线程 updated_at / 标题。
func (s *Store) AppendAgentMessage(ctx context.Context, msg AgentMessage) (AgentMessage, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentMessage{}, connErr
	}

	now := time.Now().UTC()
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	if msg.MentionsJSON == "" {
		msg.MentionsJSON = "[]"
	}
	if msg.ToolCallsJSON == "" {
		msg.ToolCallsJSON = "[]"
	}
	msg.CreatedAt = now
	_, err := sqlDB.ExecContext(ctx,
		`INSERT INTO sys_agent_messages(id, thread_id, project_id, role, content, agent_id, mentions_json, tool_calls_json, run_id, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ThreadID, msg.ProjectID, msg.Role, msg.Content, msg.AgentID, msg.MentionsJSON, msg.ToolCallsJSON, msg.RunID, now)
	if err != nil {
		return AgentMessage{}, err
	}
	title := strings.TrimSpace(msg.Content)
	title = strutil.TruncateRunes(title, 30)
	if msg.Role == "user" && title != "" {
		_, _ = sqlDB.ExecContext(ctx,
			`UPDATE sys_agent_threads SET updated_at = ?, title = CASE WHEN title IN ('New thread', '云 Agent', '云助手') THEN ? ELSE title END WHERE id = ?`,
			now, title, msg.ThreadID)
	} else {
		_, _ = sqlDB.ExecContext(ctx, `UPDATE sys_agent_threads SET updated_at = ? WHERE id = ?`, now, msg.ThreadID)
	}
	s.notifyWrite(ctx)
	return msg, nil
}

// ListAgentMessages 按时间顺序列出线程消息。
func (s *Store) ListAgentMessages(ctx context.Context, projectID, threadID string, limit int) ([]AgentMessage, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, connErr
	}

	limit = pagelimit.Normalize(limit, 200, 500)
	start := time.Now()
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT recent.id, recent.thread_id, recent.project_id, recent.role, recent.content, recent.agent_id,
		 recent.mentions_json, recent.tool_calls_json, recent.run_id, recent.created_at,
		 COALESCE(r.status, ''), COALESCE(r.error_code, '')
		 FROM (SELECT * FROM sys_agent_messages WHERE project_id = ? AND thread_id = ?
		       ORDER BY created_at DESC, id DESC LIMIT ?) AS recent
		 LEFT JOIN sys_agent_runs r ON r.id = recent.run_id AND r.project_id = recent.project_id
		 ORDER BY recent.created_at ASC, recent.id ASC`, projectID, threadID, int64(limit))
	s.observeReadPart("list_agent_messages", "query", time.Since(start), err)
	if err != nil {
		s.observeRead("list_agent_messages", time.Since(start), 0, err)
		return nil, err
	}
	defer rows.Close()
	scanStart := time.Now()
	out := make([]AgentMessage, 0)
	for rows.Next() {
		var m AgentMessage
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.ProjectID, &m.Role, &m.Content, &m.AgentID, &m.MentionsJSON, &m.ToolCallsJSON, &m.RunID, &m.CreatedAt, &m.RunStatus, &m.ErrorCode); err != nil {
			s.observeReadPart("list_agent_messages", "scan", time.Since(scanStart), err)
			s.observeRead("list_agent_messages", time.Since(start), len(out), err)
			return nil, err
		}
		out = append(out, m)
	}
	rerr := rows.Err()
	s.observeReadPart("list_agent_messages", "scan", time.Since(scanStart), rerr)
	s.observeRead("list_agent_messages", time.Since(start), len(out), rerr)
	return out, rerr
}
