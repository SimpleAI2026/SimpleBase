// 文件说明：agents.go — internal/systemdb 中 AgentRunQueued、AgentRunRunning 等符号的实现
//
// 用法
//
//	(s *Store) CreateCloudAgent(ctx context.Context, a CloudAgent) (CloudAgent, error)  // CreateCloudAgent 写入一个 agent
//	(s *Store) ListCloudAgents(ctx context.Context, projectID string) (out ...  // ListCloudAgents 列出未归档 agent
//	同包调用：rowScanner → agent_schedules.go, cron_jobs.go 等；AgentRun → agent_schedules.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/agents.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/ducklake, internal/utils/limit
//	[同包] agent_schedules.go, async_flush.go, bootstrap.go, cron_jobs.go, function_triggers.go 等 22 个
//	[关键引用] rowScanner ← agent_schedules.go, cron_jobs.go, function_triggers.go
//
// 主要符号
//
//	const AgentRunQueued
//	const AgentRunRunning
//	const AgentRunCompleted
//	const AgentRunFailed
//	const AgentRunCanceled
//	type CloudAgent struct {…}
//	... 另有 37 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	pagelimit "github.com/linkxzhou/SimpleBase/internal/utils/limit"
)

const (
	AgentRunQueued    = "queued"
	AgentRunRunning   = "running"
	AgentRunCompleted = "completed"
	AgentRunFailed    = "failed"
	AgentRunCanceled  = "canceled"

	// cloudAgentColumns 是 sys_cloud_agents 未归档行的读取列清单，
	// 与 scanCloudAgent 的扫描顺序一一对应（planv6.0 §7 A3）。
	cloudAgentColumns = `id, project_id, name, module, description, system_prompt, tool_ids, model_override, team_enabled, created_at, updated_at, builtin_key, writable`
	// agentRunColumns 是 sys_agent_runs 的读取列清单，与 scanAgentRun 的顺序一一对应。
	agentRunColumns = `id, thread_id, project_id, agent_id, status, error, started_at, finished_at, created_at, duration_ms, prompt_tokens, completion_tokens, reasoning_tokens, tool_calls, error_code, COALESCE(initiator_id, '')`
)

// CloudAgent 是 sys_cloud_agents 一行。
type CloudAgent struct {
	ID            string
	ProjectID     string
	Name          string
	Module        string
	Description   string
	SystemPrompt  string
	ToolIDs       []string
	ModelOverride string
	BuiltinKey    string
	TeamEnabled   bool
	// Writable 是该助手是否允许执行写操作（planv6.0 v2 §3.2，迁移 v48）。
	// 默认 false = 只读：不可写助手经 CLI 只能执行只读命令。
	Writable  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AgentThread 是 sys_agent_threads 一行。
type AgentThread struct {
	ID        string
	ProjectID string
	Title     string
	// AgentID 是会话归属的助手（planv6.0 §3.7 修订：每个助手一份会话）。
	// 空串表示未绑定，读取时按「默认助手」处理，兼容 v47 之前的历史数据。
	AgentID   string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
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

// AgentRun 是 sys_agent_runs 一行。
type AgentRun struct {
	ID               string
	ThreadID         string
	ProjectID        string
	AgentID          string
	Status           string
	Error            string
	StartedAt        time.Time
	FinishedAt       time.Time
	CreatedAt        time.Time
	DurationMS       int64
	PromptTokens     int
	CompletionTokens int
	ReasoningTokens  int
	ToolCalls        int
	ErrorCode        string
	RetryOfRunID     string
	// InitiatorID 是发起人标识（planv6.0 D5）：登录态为 UserID，
	// API Key 为 "apikey:<id>"，历史数据为空串（兼容分支）。
	InitiatorID string
}

// CreateCloudAgent 写入一个 agent。
func (s *Store) CreateCloudAgent(ctx context.Context, a CloudAgent) (CloudAgent, error) {
	if s == nil || s.db == nil {
		return CloudAgent{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	a.CreatedAt = now
	a.UpdatedAt = now
	toolJSON, err := marshalStringSlice(a.ToolIDs)
	if err != nil {
		return CloudAgent{}, err
	}
	team := int64(0)
	if a.TeamEnabled {
		team = 1
	}
	writable := int64(0)
	if a.Writable {
		writable = 1
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sys_cloud_agents(id, project_id, name, module, description, system_prompt, tool_ids, model_override, team_enabled, created_at, updated_at, archived_at, builtin_key, writable)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		a.ID, a.ProjectID, a.Name, a.Module, a.Description, a.SystemPrompt, toolJSON, a.ModelOverride, team, now, now, a.BuiltinKey, writable)
	if err != nil {
		return CloudAgent{}, err
	}
	s.notifyWrite(ctx)
	return a, nil
}

// ListCloudAgents 列出未归档 agent。
func (s *Store) ListCloudAgents(ctx context.Context, projectID string) (out []CloudAgent, err error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	started := time.Now()
	defer func() { s.observeRead("list_cloud_agents", time.Since(started), len(out), err) }()
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+cloudAgentColumns+`
		 FROM sys_cloud_agents WHERE project_id = ? AND archived_at IS NULL
		 ORDER BY CASE WHEN builtin_key = 'general' THEN 0 ELSE 1 END, created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out = make([]CloudAgent, 0)
	for rows.Next() {
		a, err := scanCloudAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetCloudAgent 按 ID 取未归档 agent。
func (s *Store) GetCloudAgent(ctx context.Context, projectID, id string) (CloudAgent, error) {
	if s == nil || s.db == nil {
		return CloudAgent{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+cloudAgentColumns+`
		 FROM sys_cloud_agents WHERE id = ? AND project_id = ? AND archived_at IS NULL`,
		id, projectID)
	a, err := scanCloudAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudAgent{}, sql.ErrNoRows
	}
	return a, err
}

// UpdateCloudAgent 更新可变字段。
func (s *Store) UpdateCloudAgent(ctx context.Context, a CloudAgent) (CloudAgent, error) {
	if s == nil || s.db == nil {
		return CloudAgent{}, ErrUnavailable
	}
	now := time.Now().UTC()
	a.UpdatedAt = now
	toolJSON, err := marshalStringSlice(a.ToolIDs)
	if err != nil {
		return CloudAgent{}, err
	}
	team := int64(0)
	if a.TeamEnabled {
		team = 1
	}
	writable := int64(0)
	if a.Writable {
		writable = 1
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_cloud_agents SET name=?, module=?, description=?, system_prompt=?, tool_ids=?, model_override=?, team_enabled=?, writable=?, updated_at=?
		 WHERE id=? AND project_id=? AND archived_at IS NULL`,
		a.Name, a.Module, a.Description, a.SystemPrompt, toolJSON, a.ModelOverride, team, writable, now, a.ID, a.ProjectID)
	if err != nil {
		return CloudAgent{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return CloudAgent{}, sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return a, nil
}

// ArchiveCloudAgent 软删。
func (s *Store) ArchiveCloudAgent(ctx context.Context, projectID, id string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	release, err := s.lockAgentSeed(ctx, projectID)
	if err != nil {
		return err
	}
	defer release()
	now := time.Now().UTC()
	// 软删是幂等的：冲突表示提交失败，整段事务重放；已归档时第二次得到 ErrNoRows，不再重试。
	err = ducklake.RetryOnConflict(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		res, err := tx.ExecContext(ctx,
			`UPDATE sys_cloud_agents SET archived_at=?, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL`,
			now, now, id, projectID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return sql.ErrNoRows
		}
		// 内置 Agent 删除后写 dismissed，播种不再补回（DuckLake 无 PK 约束，存在性由应用层判断）。
		var key string
		if err := tx.QueryRowContext(ctx, `SELECT builtin_key FROM sys_cloud_agents WHERE id=? AND project_id=?`, id, projectID).Scan(&key); err != nil {
			return err
		}
		if key != "" {
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_cloud_agent_seed_dismissed WHERE project_id=? AND builtin_key=?`, projectID, key).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				if _, err = tx.ExecContext(ctx, `INSERT INTO sys_cloud_agent_seed_dismissed(project_id, builtin_key) VALUES(?, ?)`, projectID, key); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	})
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// CountCloudAgents 返回未归档数量（含 0）。
func (s *Store) CountCloudAgents(ctx context.Context, projectID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_cloud_agents WHERE project_id = ? AND archived_at IS NULL`, projectID).Scan(&n)
	return int(n), err
}

// CreateAgentThread 创建会话线程。
func (s *Store) CreateAgentThread(ctx context.Context, t AgentThread) (AgentThread, error) {
	if s == nil || s.db == nil {
		return AgentThread{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Title == "" {
		t.Title = "New thread"
	}
	t.CreatedAt = now
	t.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_threads(id, project_id, title, agent_id, created_by, created_at, updated_at, archived_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, NULL)`,
		t.ID, t.ProjectID, t.Title, t.AgentID, t.CreatedBy, now, now)
	if err != nil {
		return AgentThread{}, err
	}
	s.notifyWrite(ctx)
	return t, nil
}

// ListAgentThreads 列出未归档线程。agentID 非空时只返回该助手的会话；
// 空串表示不过滤（保留给内部调用与既有测试）。
func (s *Store) ListAgentThreads(ctx context.Context, projectID, agentID string, limit int) ([]AgentThread, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
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
	rows, err := s.db.QueryContext(ctx, query, args...)
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
	if s == nil || s.db == nil {
		return AgentThread{}, ErrUnavailable
	}
	var t AgentThread
	err := s.db.QueryRowContext(ctx,
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
	if s == nil || s.db == nil {
		return AgentThread{}, ErrUnavailable
	}
	res, err := s.db.ExecContext(ctx, `UPDATE sys_agent_threads SET title=?, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL`, title, time.Now().UTC(), id, projectID)
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

// PageAgentThreads 按更新时间/id 倒序 keyset 分页。
// agentID 非空时只返回该助手的会话；空串不过滤。
func (s *Store) PageAgentThreads(ctx context.Context, projectID, agentID, cursor string, limit int) ([]AgentThread, string, error) {
	if s == nil || s.db == nil {
		return nil, "", ErrUnavailable
	}
	limit = pagelimit.Normalize(limit, 50, 200)
	query := `SELECT id, project_id, title, COALESCE(agent_id, ''), created_by, created_at, updated_at FROM sys_agent_threads WHERE project_id=? AND archived_at IS NULL`
	args := []any{projectID}
	if agentID != "" {
		query += ` AND COALESCE(agent_id, '') = ?`
		args = append(args, agentID)
	}
	if cursor != "" {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_agent_threads WHERE id=? AND project_id=?`, cursor, projectID).Scan(&n); err != nil {
			return nil, "", err
		}
		if n == 0 {
			return nil, "", ErrAgentThreadCursor
		}
		const at = `(SELECT updated_at FROM sys_agent_threads WHERE id=? AND project_id=?)`
		query += ` AND (updated_at < ` + at + ` OR (updated_at = ` + at + ` AND id < ?))`
		args = append(args, cursor, projectID, cursor, projectID, cursor)
	}
	query += ` ORDER BY updated_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
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
	if s == nil || s.db == nil {
		return "", ErrUnavailable
	}
	var text string
	err := s.db.QueryRowContext(ctx, `SELECT content FROM sys_agent_messages WHERE project_id=? AND thread_id=? ORDER BY created_at DESC, id DESC LIMIT 1`, projectID, threadID).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	runes := []rune(text)
	if len(runes) > 80 {
		text = string(runes[:80])
	}
	return text, nil
}

// ArchiveAgentThread 软删线程。
func (s *Store) ArchiveAgentThread(ctx context.Context, projectID, id string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_threads SET archived_at=?, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL`,
		now, now, id, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return nil
}

// AppendAgentMessage 写入一条消息并刷新线程 updated_at / 标题。
func (s *Store) AppendAgentMessage(ctx context.Context, msg AgentMessage) (AgentMessage, error) {
	if s == nil || s.db == nil {
		return AgentMessage{}, ErrUnavailable
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
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_messages(id, thread_id, project_id, role, content, agent_id, mentions_json, tool_calls_json, run_id, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ThreadID, msg.ProjectID, msg.Role, msg.Content, msg.AgentID, msg.MentionsJSON, msg.ToolCallsJSON, msg.RunID, now)
	if err != nil {
		return AgentMessage{}, err
	}
	title := strings.TrimSpace(msg.Content)
	if runes := []rune(title); len(runes) > 30 {
		title = string(runes[:30])
	}
	if msg.Role == "user" && title != "" {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE sys_agent_threads SET updated_at = ?, title = CASE WHEN title IN ('New thread', '云 Agent', '云助手') THEN ? ELSE title END WHERE id = ?`,
			now, title, msg.ThreadID)
	} else {
		_, _ = s.db.ExecContext(ctx, `UPDATE sys_agent_threads SET updated_at = ? WHERE id = ?`, now, msg.ThreadID)
	}
	s.notifyWrite(ctx)
	return msg, nil
}

// ListAgentMessages 按时间顺序列出线程消息。
func (s *Store) ListAgentMessages(ctx context.Context, projectID, threadID string, limit int) ([]AgentMessage, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	limit = pagelimit.Normalize(limit, 200, 500)
	start := time.Now()
	rows, err := s.db.QueryContext(ctx,
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

// CreateAgentRun 写入 queued/running 记录。
func (s *Store) CreateAgentRun(ctx context.Context, r AgentRun) (AgentRun, error) {
	if s == nil || s.db == nil {
		return AgentRun{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.Status == "" {
		r.Status = AgentRunQueued
	}
	r.CreatedAt = now
	var started any
	if !r.StartedAt.IsZero() {
		started = r.StartedAt.UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_runs(id, thread_id, project_id, agent_id, status, error, started_at, finished_at, created_at, initiator_id)
		 VALUES(?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		r.ID, r.ThreadID, r.ProjectID, r.AgentID, r.Status, r.Error, started, now, r.InitiatorID)
	if err != nil {
		return AgentRun{}, err
	}
	s.notifyWrite(ctx)
	return r, nil
}

// GetAgentRun 按 ID 取 run（必须属于 project）。
func (s *Store) GetAgentRun(ctx context.Context, projectID, id string) (AgentRun, error) {
	if s == nil || s.db == nil {
		return AgentRun{}, ErrUnavailable
	}
	var r AgentRun
	var started, finished sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT `+agentRunColumns+`
		 FROM sys_agent_runs WHERE id = ? AND project_id = ?`,
		id, projectID).Scan(&r.ID, &r.ThreadID, &r.ProjectID, &r.AgentID, &r.Status, &r.Error, &started, &finished, &r.CreatedAt, &r.DurationMS, &r.PromptTokens, &r.CompletionTokens, &r.ReasoningTokens, &r.ToolCalls, &r.ErrorCode, &r.InitiatorID)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRun{}, sql.ErrNoRows
	}
	if err != nil {
		return AgentRun{}, err
	}
	if started.Valid {
		r.StartedAt = started.Time
	}
	if finished.Valid {
		r.FinishedAt = finished.Time
	}
	return r, nil
}

// ListAgentRuns 列出项目内某个会话最近的运行记录。
func (s *Store) ListAgentRuns(ctx context.Context, projectID, threadID string, limit int) ([]AgentRun, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	limit = pagelimit.Normalize(limit, 20, 100)
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentRunColumns+` FROM sys_agent_runs WHERE project_id=? AND thread_id=? ORDER BY created_at DESC, id DESC LIMIT ?`, projectID, threadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentRun, 0)
	for rows.Next() {
		var r AgentRun
		var start, finish sql.NullTime
		if err := rows.Scan(&r.ID, &r.ThreadID, &r.ProjectID, &r.AgentID, &r.Status, &r.Error, &start, &finish, &r.CreatedAt, &r.DurationMS, &r.PromptTokens, &r.CompletionTokens, &r.ReasoningTokens, &r.ToolCalls, &r.ErrorCode, &r.InitiatorID); err != nil {
			return nil, err
		}
		if start.Valid {
			r.StartedAt = start.Time
		}
		if finish.Valid {
			r.FinishedAt = finish.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListActiveAgentRuns 列出项目内状态仍为 running/queued 的 run（含定时任务触发的）。
// 进程重启后可能残留未收尾的 running 行，调用方需再与运行时内存态核对。
func (s *Store) ListActiveAgentRuns(ctx context.Context, projectID string) ([]AgentRun, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentRunColumns+` FROM sys_agent_runs WHERE project_id=? AND status IN (?, ?) ORDER BY created_at DESC, id DESC LIMIT 200`, projectID, AgentRunRunning, AgentRunQueued)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentRun, 0)
	for rows.Next() {
		var r AgentRun
		var start, finish sql.NullTime
		if err := rows.Scan(&r.ID, &r.ThreadID, &r.ProjectID, &r.AgentID, &r.Status, &r.Error, &start, &finish, &r.CreatedAt, &r.DurationMS, &r.PromptTokens, &r.CompletionTokens, &r.ReasoningTokens, &r.ToolCalls, &r.ErrorCode, &r.InitiatorID); err != nil {
			return nil, err
		}
		if start.Valid {
			r.StartedAt = start.Time
		}
		if finish.Valid {
			r.FinishedAt = finish.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateAgentRunMetrics 写入运行指标与安全错误码，不存上游原始错误。
func (s *Store) UpdateAgentRunMetrics(ctx context.Context, projectID, id string, r AgentRun) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	_, err := s.db.ExecContext(ctx, `UPDATE sys_agent_runs SET duration_ms=?, prompt_tokens=?, completion_tokens=?, reasoning_tokens=?, tool_calls=?, error_code=? WHERE project_id=? AND id=?`, r.DurationMS, r.PromptTokens, r.CompletionTokens, r.ReasoningTokens, r.ToolCalls, r.ErrorCode, projectID, id)
	if err == nil {
		s.notifyWrite(ctx)
	}
	return err
}

// UpdateAgentRunStatus 更新 run 状态与错误。
// LIFE-04：状态只允许单向迁移——已处于终态（completed/failed/canceled）的
// run 不再接受任何状态写入（同状态重复写入幂等跳过），避免 CancelRun 与
// finishAgentRun 交错时最终状态取决于谁后写。
func (s *Store) UpdateAgentRunStatus(ctx context.Context, projectID, id, status, errMsg string, started, finished *time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	var startAny, finAny any
	if started != nil && !started.IsZero() {
		startAny = started.UTC()
	}
	if finished != nil && !finished.IsZero() {
		finAny = finished.UTC()
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_runs SET status=?, error=?,
		 started_at = CASE WHEN ? IS NOT NULL THEN ? ELSE started_at END,
		 finished_at = CASE WHEN ? IS NOT NULL THEN ? ELSE finished_at END
		 WHERE id=? AND project_id=? AND (status NOT IN ('completed','failed','canceled') OR status=?)`,
		status, errMsg, startAny, startAny, finAny, finAny, id, projectID, status)
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	_, _ = res.RowsAffected() // 条件不命中（终态保持）不算错误：单向迁移语义
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCloudAgent(sc rowScanner) (CloudAgent, error) {
	var a CloudAgent
	var toolJSON string
	var team int64
	var writable int64
	if err := sc.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Module, &a.Description, &a.SystemPrompt, &toolJSON, &a.ModelOverride, &team, &a.CreatedAt, &a.UpdatedAt, &a.BuiltinKey, &writable); err != nil {
		return CloudAgent{}, err
	}
	ids, err := unmarshalStringSlice(toolJSON)
	if err != nil {
		return CloudAgent{}, err
	}
	a.ToolIDs = ids
	a.TeamEnabled = team != 0
	a.Writable = writable != 0
	return a, nil
}

func marshalStringSlice(ids []string) (string, error) {
	if ids == nil {
		ids = []string{}
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalStringSlice(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return []string{}, nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(s), &ids); err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

// defaultCloudAgentSpecs 是新项目与存量项目的内置助手。
var defaultCloudAgentSpecs = []CloudAgent{
	{
		Name:        "通用助手",
		Module:      "general",
		BuiltinKey:  "general",
		Description: "跨数据库、对象存储与日志的项目助手",
		ToolIDs:     []string{"list_databases", "list_collections", "readonly_sql", "list_objects", "head_object", "search_logs", "log_level_stats"},
	},
	{
		Name:        "Database",
		BuiltinKey:  "database",
		Module:      "database",
		Description: "Inspect project databases and run readonly SQL",
		ToolIDs:     []string{"list_databases", "list_collections", "readonly_sql"},
	},
	{
		Name:        "键值助手",
		BuiltinKey:  "kv",
		Module:      "kv",
		Description: "管理项目键值缓存与统计指标（需启用 CLI 技能）",
		ToolIDs:     []string{},
	},
	{
		Name:        "云函数助手",
		BuiltinKey:  "gofunction",
		Module:      "gofunction",
		Description: "管理项目云函数与聚合 API（需启用 CLI 技能）",
		ToolIDs:     []string{},
	},
	{
		Name:        "定时任务助手",
		BuiltinKey:  "cron",
		Module:      "cron",
		Description: "管理定时统计任务（需启用 CLI 技能）",
		ToolIDs:     []string{},
	},
	{
		Name:        "S3",
		BuiltinKey:  "s3",
		Module:      "s3",
		Description: "List and inspect project object storage",
		ToolIDs:     []string{"list_objects", "head_object"},
	},
	{
		Name:        "Logs",
		BuiltinKey:  "logs",
		Module:      "logs",
		Description: "Search project logs and summarize levels",
		ToolIDs:     []string{"search_logs", "log_level_stats"},
	},
	{
		Name:        "沙盒助手",
		BuiltinKey:  "sandbox",
		Module:      "sandbox",
		Description: "在项目隔离沙盒中调试脚本与数据口径",
		ToolIDs:     []string{"sandbox_exec", "sandbox_shell", "sandbox_read_file", "sandbox_write_file"},
	},
}

// SeedDefaultCloudAgents 为新旧项目幂等补齐未被删除的内置助手。
func (s *Store) SeedDefaultCloudAgents(ctx context.Context, projectID string) error {
	return s.SeedDefaultCloudAgentsWithSandbox(ctx, projectID, false)
}

func (s *Store) SeedDefaultCloudAgentsWithSandbox(ctx context.Context, projectID string, sandboxAvailable bool) error {
	return s.SeedDefaultCloudAgentsWithCapabilities(ctx, projectID, sandboxAvailable, false)
}

// SeedDefaultCloudAgentsWithCapabilities 仅为已启用的能力播种 CLI 专用助手。
func (s *Store) SeedDefaultCloudAgentsWithCapabilities(ctx context.Context, projectID string, sandboxAvailable, skillsCLI bool) (err error) {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	// 单写进程内同项目播种必须串行；每个等待者均可被自己的 context 取消。
	release, err := s.lockAgentSeed(ctx, projectID)
	if err != nil {
		return err
	}
	defer release()
	started := time.Now()
	defer func() { s.observeRead("seed_default_agents", time.Since(started), 0, err) }()
	// 一次读取各表的 key，避免每个内置助手都对远端小文件做两次点查。
	dismissed, err := s.agentSeedKeys(ctx, `SELECT builtin_key FROM sys_cloud_agent_seed_dismissed WHERE project_id=?`, projectID)
	if err != nil {
		return err
	}
	existing, err := s.agentSeedKeys(ctx, `SELECT builtin_key FROM sys_cloud_agents WHERE project_id=? AND builtin_key<>''`, projectID)
	if err != nil {
		return err
	}
	for _, spec := range defaultCloudAgentSpecs {
		if (!skillsCLI && (spec.Module == "kv" || spec.Module == "gofunction" || spec.Module == "cron")) || (!sandboxAvailable && spec.Module == "sandbox") {
			continue
		}
		if dismissed[spec.BuiltinKey] || existing[spec.BuiltinKey] {
			continue
		}
		res, err := s.db.ExecContext(ctx, `UPDATE sys_cloud_agents SET builtin_key=? WHERE project_id=? AND module=? AND name=? AND builtin_key='' AND archived_at IS NULL`, spec.BuiltinKey, projectID, spec.Module, spec.Name)
		if err != nil {
			return err
		}
		matched, _ := res.RowsAffected()
		if matched > 0 {
			s.notifyWrite(ctx)
			continue
		}
		if spec.BuiltinKey == "general" && sandboxAvailable {
			spec.ToolIDs = append(append([]string(nil), spec.ToolIDs...), "sandbox_exec", "sandbox_shell", "sandbox_read_file", "sandbox_write_file")
		}
		spec.ProjectID = projectID
		if _, err := s.CreateCloudAgent(ctx, spec); err != nil {
			return err
		}
	}
	return nil
}

// lockAgentSeed 仅串行同一项目，其他项目互不等待。调用方必须执行 release。
func (s *Store) lockAgentSeed(ctx context.Context, projectID string) (func(), error) {
	for {
		s.seedMu.Lock()
		if s.seedFlights == nil {
			s.seedFlights = make(map[string]chan struct{})
		}
		if done := s.seedFlights[projectID]; done != nil {
			s.seedMu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if err := ctx.Err(); err != nil {
			s.seedMu.Unlock()
			return nil, err
		}
		done := make(chan struct{})
		s.seedFlights[projectID] = done
		s.seedMu.Unlock()
		return func() {
			s.seedMu.Lock()
			delete(s.seedFlights, projectID)
			close(done)
			s.seedMu.Unlock()
		}, nil
	}
}

// agentSeedKeys 只查询当前项目，扫描完成后立即释放连接供播种写入使用。
func (s *Store) agentSeedKeys(ctx context.Context, query, projectID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make(map[string]bool, len(defaultCloudAgentSpecs))
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys[key] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}
