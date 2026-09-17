package systemdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	AgentRunQueued    = "queued"
	AgentRunRunning   = "running"
	AgentRunCompleted = "completed"
	AgentRunFailed    = "failed"
	AgentRunCanceled  = "canceled"
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
	TeamEnabled   bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// AgentThread 是 sys_agent_threads 一行。
type AgentThread struct {
	ID        string
	ProjectID string
	Title     string
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
	CreatedAt     time.Time
}

// AgentRun 是 sys_agent_runs 一行。
type AgentRun struct {
	ID         string
	ThreadID   string
	ProjectID  string
	AgentID    string
	Status     string
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
	CreatedAt  time.Time
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
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sys_cloud_agents(id, project_id, name, module, description, system_prompt, tool_ids, model_override, team_enabled, created_at, updated_at, archived_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		a.ID, a.ProjectID, a.Name, a.Module, a.Description, a.SystemPrompt, toolJSON, a.ModelOverride, team, now, now)
	if err != nil {
		return CloudAgent{}, err
	}
	s.notifyWrite(ctx)
	return a, nil
}

// ListCloudAgents 列出未归档 agent。
func (s *Store) ListCloudAgents(ctx context.Context, projectID string) ([]CloudAgent, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, module, description, system_prompt, tool_ids, model_override, team_enabled, created_at, updated_at
		 FROM sys_cloud_agents WHERE project_id = ? AND archived_at IS NULL
		 ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CloudAgent, 0)
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
		`SELECT id, project_id, name, module, description, system_prompt, tool_ids, model_override, team_enabled, created_at, updated_at
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
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_cloud_agents SET name=?, module=?, description=?, system_prompt=?, tool_ids=?, model_override=?, team_enabled=?, updated_at=?
		 WHERE id=? AND project_id=? AND archived_at IS NULL`,
		a.Name, a.Module, a.Description, a.SystemPrompt, toolJSON, a.ModelOverride, team, now, a.ID, a.ProjectID)
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
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_cloud_agents SET archived_at=?, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL`,
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
		`INSERT INTO sys_agent_threads(id, project_id, title, created_by, created_at, updated_at, archived_at)
		 VALUES(?, ?, ?, ?, ?, ?, NULL)`,
		t.ID, t.ProjectID, t.Title, t.CreatedBy, now, now)
	if err != nil {
		return AgentThread{}, err
	}
	s.notifyWrite(ctx)
	return t, nil
}

// ListAgentThreads 列出未归档线程。
func (s *Store) ListAgentThreads(ctx context.Context, projectID string, limit int) ([]AgentThread, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, title, created_by, created_at, updated_at
		 FROM sys_agent_threads WHERE project_id = ? AND archived_at IS NULL
		 ORDER BY updated_at DESC LIMIT ?`, projectID, int64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentThread, 0)
	for rows.Next() {
		var t AgentThread
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
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
		`SELECT id, project_id, title, created_by, created_at, updated_at
		 FROM sys_agent_threads WHERE id = ? AND project_id = ? AND archived_at IS NULL`,
		id, projectID).Scan(&t.ID, &t.ProjectID, &t.Title, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentThread{}, sql.ErrNoRows
	}
	return t, err
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
	if len(title) > 40 {
		title = title[:40]
	}
	if msg.Role == "user" && title != "" {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE sys_agent_threads SET updated_at = ?, title = CASE WHEN title = 'New thread' THEN ? ELSE title END WHERE id = ?`,
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
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, thread_id, project_id, role, content, agent_id, mentions_json, tool_calls_json, run_id, created_at
		 FROM sys_agent_messages WHERE project_id = ? AND thread_id = ?
		 ORDER BY created_at ASC LIMIT ?`, projectID, threadID, int64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentMessage, 0)
	for rows.Next() {
		var m AgentMessage
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.ProjectID, &m.Role, &m.Content, &m.AgentID, &m.MentionsJSON, &m.ToolCallsJSON, &m.RunID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
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
		`INSERT INTO sys_agent_runs(id, thread_id, project_id, agent_id, status, error, started_at, finished_at, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
		r.ID, r.ThreadID, r.ProjectID, r.AgentID, r.Status, r.Error, started, now)
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
		`SELECT id, thread_id, project_id, agent_id, status, error, started_at, finished_at, created_at
		 FROM sys_agent_runs WHERE id = ? AND project_id = ?`,
		id, projectID).Scan(&r.ID, &r.ThreadID, &r.ProjectID, &r.AgentID, &r.Status, &r.Error, &started, &finished, &r.CreatedAt)
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

// UpdateAgentRunStatus 更新 run 状态与错误。
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
	_, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_runs SET status=?, error=?,
		 started_at = CASE WHEN ? IS NOT NULL THEN ? ELSE started_at END,
		 finished_at = CASE WHEN ? IS NOT NULL THEN ? ELSE finished_at END
		 WHERE id=? AND project_id=?`,
		status, errMsg, startAny, startAny, finAny, finAny, id, projectID)
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCloudAgent(sc rowScanner) (CloudAgent, error) {
	var a CloudAgent
	var toolJSON string
	var team int64
	if err := sc.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Module, &a.Description, &a.SystemPrompt, &toolJSON, &a.ModelOverride, &team, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return CloudAgent{}, err
	}
	ids, err := unmarshalStringSlice(toolJSON)
	if err != nil {
		return CloudAgent{}, err
	}
	a.ToolIDs = ids
	a.TeamEnabled = team != 0
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

// defaultCloudAgentSpecs 是新项目 / DevMode 的三个模块 agent。
var defaultCloudAgentSpecs = []CloudAgent{
	{
		Name:        "Database",
		Module:      "database",
		Description: "Inspect project databases and run readonly SQL",
		ToolIDs:     []string{"list_databases", "list_collections", "readonly_sql"},
	},
	{
		Name:        "S3",
		Module:      "s3",
		Description: "List and inspect project object storage",
		ToolIDs:     []string{"list_objects", "head_object"},
	},
	{
		Name:        "Logs",
		Module:      "logs",
		Description: "Search project logs and summarize levels",
		ToolIDs:     []string{"search_logs", "log_level_stats"},
	},
}

// SeedDefaultCloudAgents 幂等写入 3 个默认 agent；已有任意未归档 agent 则跳过。
func (s *Store) SeedDefaultCloudAgents(ctx context.Context, projectID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	n, err := s.CountCloudAgents(ctx, projectID)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, spec := range defaultCloudAgentSpecs {
		spec.ProjectID = projectID
		if _, err := s.CreateCloudAgent(ctx, spec); err != nil {
			return err
		}
	}
	return nil
}
