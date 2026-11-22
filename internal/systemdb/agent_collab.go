// 文件说明：agent_collab.go — 协作群的轮次（turn）、任务（task）、事件与消息身份。
//
// 用法
//
//	CreateAgentTurn(ctx, AgentTurn) (AgentTurn, error)          创建轮次（幂等键先查后插）
//	GetAgentTurn(ctx, projectID, turnID) (AgentTurn, error)
//	ClaimAgentTurn(ctx, projectID, threadID, turnID) error      用 active_turn_id 做 CAS 抢占
//	ReleaseAgentTurn(ctx, projectID, threadID, turnID) error
//	CreateAgentTask(ctx, AgentTask) (AgentTask, error)
//	ListAgentTasks(ctx, projectID, turnID) ([]AgentTask, error)
//	UpdateAgentTask(ctx, AgentTask, expectedVersion) (AgentTask, error)
//	AppendAgentTurnEvent / ListAgentTurnEvents / LastAgentTurnSeq
//	AppendAgentMessageMeta / ListAgentMessageMeta
//
// DuckLake 没有 PK/UNIQUE 约束，逻辑唯一性全部由应用层加锁 + 先查后插保证：
//   - (project_id, id) 的轮次至多一行；同会话同 idempotency_key 至多一行
//   - (project_id, thread_id) 的协作元数据至多一行
//   - (project_id, message_id) 的消息身份至多一行
//   - (project_id, turn_id, seq) 的事件序号由 LastAgentTurnSeq 递增分配
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
	// ThreadKind 是会话类型（v2 §5.3）：影响图标、默认路由提示词与定时落点。
	ThreadKindChat       = "chat"
	ThreadKindAlert      = "alert"
	ThreadKindStatistics = "statistics"
	ThreadKindScheduled  = "scheduled"

	// TurnStatus 是轮次状态机（v2 §6.1）。
	TurnStatusRouting      = "routing"       // 正在决定分工
	TurnStatusRunning      = "running"       // 任务执行中
	TurnStatusWaitingHuman = "waiting_human" // 等人处理（不占模型）
	TurnStatusCompleted    = "completed"
	TurnStatusFailed       = "failed"
	TurnStatusCanceled     = "canceled"
	// TurnStatusInterrupted 表示进程重启后收敛的残留轮次，不自动重跑。
	TurnStatusInterrupted = "interrupted"

	// TaskStatus 是任务状态机（v2 §6.2）。
	TaskStatusPending      = "pending"
	TaskStatusRunning      = "running"
	TaskStatusWaitingHuman = "waiting_human"
	TaskStatusCompleted    = "completed"
	TaskStatusFailed       = "failed"
	TaskStatusBlocked      = "blocked" // 上游失败导致无法执行
	TaskStatusCanceled     = "canceled"
	TaskStatusInterrupted  = "interrupted"

	// TaskTargetAgent / TaskTargetHuman 是任务的两种执行者。
	TaskTargetAgent = "agent"
	TaskTargetHuman = "human"

	// TaskReplyContinue 表示人工回复后继续推进；TaskReplyUnable 表示无法处理。
	TaskReplyContinue = "continue"
	TaskReplyUnable   = "unable"

	// MessageKindUser / Route / Agent / Human / Summary 是消息身份类别。
	MessageKindUser    = "user"
	MessageKindRoute   = "route"
	MessageKindAgent   = "agent"
	MessageKindHuman   = "human"
	MessageKindSummary = "summary"

	// TurnInitiatorUser / Cron / Event 是轮次发起者类别。
	TurnInitiatorUser  = "user"
	TurnInitiatorCron  = "cron"
	TurnInitiatorEvent = "event"
)

var (
	// ErrTurnBusy 表示该会话已有活动轮次（同一时刻只允许一个）。
	ErrTurnBusy = errors.New("systemdb: agent turn already active")
	// ErrTurnNotFound 表示轮次不存在或不属于该项目。
	ErrTurnNotFound = errors.New("systemdb: agent turn not found")
	// ErrTurnConflict 表示轮次/任务版本冲突（并发推进）。
	ErrTurnConflict = errors.New("systemdb: agent turn version conflict")
	// ErrTaskNotFound 表示任务不存在或不属于该项目。
	ErrTaskNotFound = errors.New("systemdb: agent task not found")
	// ErrCollaborationUnavailable 表示协作表尚未建好（未升级）。
	ErrCollaborationUnavailable = errors.New("systemdb: collaboration schema is not ready")
)

// AgentThreadCollab 是 sys_agent_thread_collab 一行：会话级协作元数据。
type AgentThreadCollab struct {
	ProjectID       string
	ThreadID        string
	TagID           string
	ThreadKind      string
	ProtocolVersion int64
	ActiveTurnID    string
	Version         int64
}

// AgentTurn 是 sys_agent_turns 一行。
type AgentTurn struct {
	ID               string
	ProjectID        string
	ThreadID         string
	TagID            string
	InitiatorKind    string
	InitiatorID      string
	IdempotencyKey   string
	RequestHash      string
	Status           string
	RouteJSON        string
	ConfigVersion    int64
	ConfigSnapshotJS string
	ErrorCode        string
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AgentTask 是 sys_agent_tasks 一行。
type AgentTask struct {
	ID           string
	ProjectID    string
	TurnID       string
	ThreadID     string
	Seq          int64
	TargetKind   string
	TargetID     string
	Instruction  string
	DependsJSON  string
	Status       string
	Attempt      int64
	RunID        string
	AssigneeID   string
	ReplyText    string
	ReplyOutcome string
	RepliedBy    string
	ErrorCode    string
	Version      int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AgentTaskRun 是 sys_agent_task_runs 一行：任务的一次执行尝试。
type AgentTaskRun struct {
	ProjectID string
	TaskID    string
	Attempt   int64
	RunID     string
	Status    string
	CreatedAt time.Time
}

// AgentTurnEvent 是 sys_agent_turn_events 一行。
type AgentTurnEvent struct {
	ProjectID   string
	TurnID      string
	Seq         int64
	Kind        string
	EntityID    string
	PayloadJSON string
	CreatedAt   time.Time
}

// AgentMessageMeta 是 sys_agent_message_meta 一行：消息身份旁挂。
type AgentMessageMeta struct {
	ProjectID   string
	MessageID   string
	TurnID      string
	TaskID      string
	SpeakerKind string
	SpeakerID   string
	Kind        string
	CreatedAt   time.Time
}

const agentTurnColumns = `id, project_id, thread_id, tag_id, initiator_kind, initiator_id,
	idempotency_key, request_hash, status, route_json, config_version, config_snapshot_json,
	error_code, version, created_at, updated_at`

const agentTaskColumns = `id, project_id, turn_id, thread_id, seq, target_kind, target_id,
	instruction, depends_json, status, attempt, run_id, assignee_id, reply_text,
	reply_outcome, replied_by, error_code, version, created_at, updated_at`

func scanAgentTurn(sc rowScanner) (AgentTurn, error) {
	var t AgentTurn
	var snapshot string
	if err := sc.Scan(&t.ID, &t.ProjectID, &t.ThreadID, &t.TagID, &t.InitiatorKind, &t.InitiatorID,
		&t.IdempotencyKey, &t.RequestHash, &t.Status, &t.RouteJSON, &t.ConfigVersion, &snapshot,
		&t.ErrorCode, &t.Version, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return AgentTurn{}, err
	}
	t.ConfigSnapshotJS = snapshot
	return t, nil
}

func scanAgentTask(sc rowScanner) (AgentTask, error) {
	var t AgentTask
	if err := sc.Scan(&t.ID, &t.ProjectID, &t.TurnID, &t.ThreadID, &t.Seq, &t.TargetKind, &t.TargetID,
		&t.Instruction, &t.DependsJSON, &t.Status, &t.Attempt, &t.RunID, &t.AssigneeID, &t.ReplyText,
		&t.ReplyOutcome, &t.RepliedBy, &t.ErrorCode, &t.Version, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return AgentTask{}, err
	}
	return t, nil
}

// collabSchemaReady 探测协作表是否就绪（未升级环境返回明文错误而非 SQL 错误）。
func (s *Store) collabSchemaReady(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx, `SELECT 1 FROM sys_agent_turns LIMIT 0`); err != nil {
		if missingSystemTable(err) {
			return ErrCollaborationUnavailable
		}
		return err
	}
	return nil
}

// CollabSchemaReady 导出协作表就绪探测，供能力接口与写入口前置校验。
func (s *Store) CollabSchemaReady(ctx context.Context) error {
	return s.collabSchemaReady(ctx)
}

// CreateAgentThreadCollab 写入会话协作元数据（与建会话同事务）。
func (s *Store) CreateAgentThreadCollab(ctx context.Context, c AgentThreadCollab) (AgentThreadCollab, error) {
	if s == nil || s.db == nil {
		return AgentThreadCollab{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if c.ThreadKind == "" {
		c.ThreadKind = ThreadKindChat
	}
	if c.ProtocolVersion == 0 {
		c.ProtocolVersion = 2
	}
	c.Version = 1
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_thread_collab(project_id, thread_id, tag_id, thread_kind, protocol_version,
			active_turn_id, version, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, '', ?, ?, ?)`,
		c.ProjectID, c.ThreadID, c.TagID, c.ThreadKind, c.ProtocolVersion, c.Version, now, now); err != nil {
		return AgentThreadCollab{}, err
	}
	s.notifyWrite(ctx)
	return c, nil
}

// GetAgentThreadCollab 读取会话协作元数据；无行返回零值 + nil（旧会话）。
func (s *Store) GetAgentThreadCollab(ctx context.Context, projectID, threadID string) (AgentThreadCollab, error) {
	if s == nil || s.db == nil {
		return AgentThreadCollab{}, ErrUnavailable
	}
	var c AgentThreadCollab
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id, thread_id, tag_id, thread_kind, protocol_version, active_turn_id, version
		 FROM sys_agent_thread_collab WHERE project_id=? AND thread_id=? ORDER BY created_at, rowid LIMIT 1`,
		projectID, threadID).Scan(&c.ProjectID, &c.ThreadID, &c.TagID, &c.ThreadKind,
		&c.ProtocolVersion, &c.ActiveTurnID, &c.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentThreadCollab{}, nil
	}
	return c, err
}

// ListAgentThreadCollabs 批量读取协作元数据（列表 DTO 补 thread_kind，避免 N+1）。
func (s *Store) ListAgentThreadCollabs(ctx context.Context, projectID string, threadIDs []string) (map[string]AgentThreadCollab, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	out := make(map[string]AgentThreadCollab, len(threadIDs))
	if len(threadIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(threadIDs))
	args := make([]any, 0, len(threadIDs)+1)
	args = append(args, projectID)
	for i, id := range threadIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id, thread_id, tag_id, thread_kind, protocol_version, active_turn_id, version
		 FROM sys_agent_thread_collab WHERE project_id=? AND thread_id IN (`+strings.Join(placeholders, ", ")+`)`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c AgentThreadCollab
		if err := rows.Scan(&c.ProjectID, &c.ThreadID, &c.TagID, &c.ThreadKind,
			&c.ProtocolVersion, &c.ActiveTurnID, &c.Version); err != nil {
			return nil, err
		}
		if _, dup := out[c.ThreadID]; !dup {
			out[c.ThreadID] = c
		}
	}
	return out, rows.Err()
}

// ClaimAgentTurn 用 active_turn_id 做 CAS 抢占会话的活动轮次。
// 已占用且不是自己时返回 ErrTurnBusy（调用方转 409 turn_busy）。
func (s *Store) ClaimAgentTurn(ctx context.Context, projectID, threadID, turnID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_thread_collab SET active_turn_id=?, version=version+1, updated_at=?
		 WHERE project_id=? AND thread_id=? AND (active_turn_id='' OR active_turn_id=?)`,
		turnID, time.Now().UTC(), projectID, threadID, turnID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c, err := s.GetAgentThreadCollab(ctx, projectID, threadID)
		if err != nil {
			return err
		}
		if c.ThreadID == "" {
			// 旧会话没有协作行：按不可抢占处理，交由调用方建行后重试。
			return ErrTurnBusy
		}
		if c.ActiveTurnID != "" && c.ActiveTurnID != turnID {
			return ErrTurnBusy
		}
		return ErrTurnBusy
	}
	s.notifyWrite(ctx)
	return nil
}

// ReleaseAgentTurn 释放活动轮次占用（只有持有者能释放，幂等）。
func (s *Store) ReleaseAgentTurn(ctx context.Context, projectID, threadID, turnID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_thread_collab SET active_turn_id='', version=version+1, updated_at=?
		 WHERE project_id=? AND thread_id=? AND active_turn_id=?`,
		time.Now().UTC(), projectID, threadID, turnID); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// CreateAgentTurn 创建轮次。幂等键非空且已存在时直接返回既有轮次（不新建）。
func (s *Store) CreateAgentTurn(ctx context.Context, t AgentTurn) (AgentTurn, bool, error) {
	if s == nil || s.db == nil {
		return AgentTurn{}, false, ErrUnavailable
	}
	if err := s.collabSchemaReady(ctx); err != nil {
		return AgentTurn{}, false, err
	}
	if t.IdempotencyKey != "" {
		existing, err := s.findAgentTurnByKey(ctx, t.ProjectID, t.ThreadID, t.IdempotencyKey)
		if err != nil {
			return AgentTurn{}, false, err
		}
		if existing.ID != "" {
			return existing, true, nil
		}
	}
	now := time.Now().UTC()
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Status == "" {
		t.Status = TurnStatusRouting
	}
	if t.RouteJSON == "" {
		t.RouteJSON = "{}"
	}
	if t.ConfigSnapshotJS == "" {
		t.ConfigSnapshotJS = "{}"
	}
	t.Version = 1
	t.CreatedAt = now
	t.UpdatedAt = now
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_turns(id, project_id, thread_id, tag_id, initiator_kind, initiator_id,
			idempotency_key, request_hash, status, route_json, config_version, config_snapshot_json,
			error_code, version, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 1, ?, ?)`,
		t.ID, t.ProjectID, t.ThreadID, t.TagID, t.InitiatorKind, t.InitiatorID,
		t.IdempotencyKey, t.RequestHash, t.Status, t.RouteJSON, t.ConfigVersion,
		t.ConfigSnapshotJS, now, now); err != nil {
		return AgentTurn{}, false, err
	}
	s.notifyWrite(ctx)
	return t, false, nil
}

// findAgentTurnByKey 按幂等键查已有轮次；无行返回零值 + nil。
func (s *Store) findAgentTurnByKey(ctx context.Context, projectID, threadID, key string) (AgentTurn, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentTurnColumns+` FROM sys_agent_turns
		 WHERE project_id=? AND thread_id=? AND idempotency_key=? ORDER BY created_at, rowid LIMIT 1`,
		projectID, threadID, key)
	t, err := scanAgentTurn(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentTurn{}, nil
	}
	return t, err
}

// GetAgentTurn 按 ID 取轮次；无行返回 ErrTurnNotFound。
func (s *Store) GetAgentTurn(ctx context.Context, projectID, turnID string) (AgentTurn, error) {
	if s == nil || s.db == nil {
		return AgentTurn{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentTurnColumns+` FROM sys_agent_turns WHERE project_id=? AND id=?`,
		projectID, turnID)
	t, err := scanAgentTurn(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentTurn{}, ErrTurnNotFound
	}
	return t, err
}

// ListAgentTurns 列出会话的轮次（按创建时间倒序，keyset 分页）。
func (s *Store) ListAgentTurns(ctx context.Context, projectID, threadID string, limit int) ([]AgentTurn, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentTurnColumns+` FROM sys_agent_turns
		 WHERE project_id=? AND thread_id=? ORDER BY created_at DESC, id DESC LIMIT ?`,
		projectID, threadID, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentTurn, 0, limit)
	for rows.Next() {
		t, err := scanAgentTurn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		if len(out) > limit {
			break
		}
	}
	return out, rows.Err()
}

// UpdateAgentTurnStatus 以版本 CAS 推进轮次状态；expectedVersion<=0 表示不校验版本。
// routeJSON / errCode 传空串表示保持原值（不会用空串覆盖已有内容）。
func (s *Store) UpdateAgentTurnStatus(ctx context.Context, projectID, turnID, status, routeJSON, errCode string, expectedVersion int64) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	sets := []string{"status=?", "version=version+1", "updated_at=?"}
	args := []any{status, time.Now().UTC()}
	if routeJSON != "" {
		sets = append(sets, "route_json=?")
		args = append(args, routeJSON)
	}
	if errCode != "" {
		sets = append(sets, "error_code=?")
		args = append(args, errCode)
	}
	args = append(args, projectID, turnID)
	query := `UPDATE sys_agent_turns SET ` + strings.Join(sets, ", ") + ` WHERE project_id=? AND id=?`
	if expectedVersion > 0 {
		query += ` AND version=?`
		args = append(args, expectedVersion)
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if expectedVersion > 0 {
			return ErrTurnConflict
		}
		return ErrTurnNotFound
	}
	s.notifyWrite(ctx)
	return nil
}

// ClearAgentTurnError 清空轮次的错误码（重试成功后调用）。
func (s *Store) ClearAgentTurnError(ctx context.Context, projectID, turnID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_turns SET error_code='', updated_at=? WHERE project_id=? AND id=?`,
		time.Now().UTC(), projectID, turnID); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ListRunningAgentTurns 列出仍在推进的轮次（进程启动时收敛为 interrupted）。
func (s *Store) ListRunningAgentTurns(ctx context.Context, cutoff time.Time) ([]AgentTurn, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if err := s.collabSchemaReady(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentTurnColumns+` FROM sys_agent_turns
		 WHERE status IN (?, ?) AND created_at < ? ORDER BY created_at LIMIT 500`,
		TurnStatusRouting, TurnStatusRunning, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentTurn, 0)
	for rows.Next() {
		t, err := scanAgentTurn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateAgentTask 写入任务；seq 由调用方按轮次内顺序分配。
func (s *Store) CreateAgentTask(ctx context.Context, t AgentTask) (AgentTask, error) {
	if s == nil || s.db == nil {
		return AgentTask{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Status == "" {
		t.Status = TaskStatusPending
	}
	if t.DependsJSON == "" {
		t.DependsJSON = "[]"
	}
	t.Version = 1
	t.CreatedAt = now
	t.UpdatedAt = now
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_tasks(id, project_id, turn_id, thread_id, seq, target_kind, target_id,
			instruction, depends_json, status, attempt, run_id, assignee_id, reply_text,
			reply_outcome, replied_by, error_code, version, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', ?, '', '', '', '', 1, ?, ?)`,
		t.ID, t.ProjectID, t.TurnID, t.ThreadID, t.Seq, t.TargetKind, t.TargetID,
		t.Instruction, t.DependsJSON, t.Status, t.AssigneeID, now, now); err != nil {
		return AgentTask{}, err
	}
	s.notifyWrite(ctx)
	return t, nil
}

// GetAgentTask 按 ID 取任务。
func (s *Store) GetAgentTask(ctx context.Context, projectID, taskID string) (AgentTask, error) {
	if s == nil || s.db == nil {
		return AgentTask{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentTaskColumns+` FROM sys_agent_tasks WHERE project_id=? AND id=?`, projectID, taskID)
	t, err := scanAgentTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentTask{}, ErrTaskNotFound
	}
	return t, err
}

// ListAgentTasks 按 seq 升序返回轮次内的任务。
func (s *Store) ListAgentTasks(ctx context.Context, projectID, turnID string) ([]AgentTask, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentTaskColumns+` FROM sys_agent_tasks
		 WHERE project_id=? AND turn_id=? ORDER BY seq, id`, projectID, turnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentTask, 0, 8)
	for rows.Next() {
		t, err := scanAgentTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListHumanAgentTasks 列出指派给某人的待办（assignee 为空表示不按人过滤）。
func (s *Store) ListHumanAgentTasks(ctx context.Context, projectID, assignee, status string, limit int) ([]AgentTask, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `SELECT ` + agentTaskColumns + ` FROM sys_agent_tasks
		WHERE project_id=? AND target_kind=?`
	args := []any{projectID, TaskTargetHuman}
	if assignee != "" {
		query += ` AND assignee_id=?`
		args = append(args, assignee)
	}
	if status != "" {
		query += ` AND status=?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentTask, 0, limit)
	for rows.Next() {
		t, err := scanAgentTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CountPendingHumanTasks 统计某人的待办数（侧栏角标）。
func (s *Store) CountPendingHumanTasks(ctx context.Context, projectID, assignee string) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	query := `SELECT COUNT(*) FROM sys_agent_tasks WHERE project_id=? AND target_kind=? AND status=?`
	args := []any{projectID, TaskTargetHuman, TaskStatusWaitingHuman}
	if assignee != "" {
		query += ` AND assignee_id=?`
		args = append(args, assignee)
	}
	var n int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

// UpdateAgentTask 以版本 CAS 更新任务；expectedVersion<=0 表示不校验版本。
// 只更新可写字段：状态、run、重试次数、回复、错误码。
func (s *Store) UpdateAgentTask(ctx context.Context, t AgentTask, expectedVersion int64) (AgentTask, error) {
	if s == nil || s.db == nil {
		return AgentTask{}, ErrUnavailable
	}
	query := `UPDATE sys_agent_tasks SET status=?, attempt=?, run_id=?, assignee_id=?, reply_text=?,
		reply_outcome=?, replied_by=?, error_code=?, version=version+1, updated_at=?
		WHERE project_id=? AND id=?`
	args := []any{t.Status, t.Attempt, t.RunID, t.AssigneeID, t.ReplyText,
		t.ReplyOutcome, t.RepliedBy, t.ErrorCode, time.Now().UTC(), t.ProjectID, t.ID}
	if expectedVersion > 0 {
		query += ` AND version=?`
		args = append(args, expectedVersion)
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return AgentTask{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if expectedVersion > 0 {
			return AgentTask{}, ErrTurnConflict
		}
		return AgentTask{}, ErrTaskNotFound
	}
	s.notifyWrite(ctx)
	return s.GetAgentTask(ctx, t.ProjectID, t.ID)
}

// AppendAgentTaskRun 记录任务的一次运行尝试（保留历史，不覆盖）。
func (s *Store) AppendAgentTaskRun(ctx context.Context, r AgentTaskRun) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_task_runs(project_id, task_id, attempt, run_id, status, created_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		r.ProjectID, r.TaskID, r.Attempt, r.RunID, r.Status, time.Now().UTC()); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// LastAgentTurnSeq 返回轮次事件的当前最大序号（0 表示还没有事件）。
func (s *Store) LastAgentTurnSeq(ctx context.Context, projectID, turnID string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	var seq sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(seq) FROM sys_agent_turn_events WHERE project_id=? AND turn_id=?`,
		projectID, turnID).Scan(&seq)
	if err != nil {
		return 0, err
	}
	return seq.Int64, nil
}

// AppendAgentTurnEvent 追加一条轮次事件，序号由调用方保证单调递增。
func (s *Store) AppendAgentTurnEvent(ctx context.Context, ev AgentTurnEvent) (AgentTurnEvent, error) {
	if s == nil || s.db == nil {
		return AgentTurnEvent{}, ErrUnavailable
	}
	if ev.PayloadJSON == "" {
		ev.PayloadJSON = "{}"
	}
	ev.CreatedAt = time.Now().UTC()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_turn_events(project_id, turn_id, seq, kind, entity_id, payload_json, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		ev.ProjectID, ev.TurnID, ev.Seq, ev.Kind, ev.EntityID, ev.PayloadJSON, ev.CreatedAt); err != nil {
		return AgentTurnEvent{}, err
	}
	s.notifyWrite(ctx)
	return ev, nil
}

// ListAgentTurnEvents 返回 seq > after 的事件（快照 + 续传）。
func (s *Store) ListAgentTurnEvents(ctx context.Context, projectID, turnID string, after int64, limit int) ([]AgentTurnEvent, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id, turn_id, seq, kind, entity_id, payload_json, created_at
		 FROM sys_agent_turn_events WHERE project_id=? AND turn_id=? AND seq>?
		 ORDER BY seq LIMIT ?`, projectID, turnID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentTurnEvent, 0, 16)
	for rows.Next() {
		var ev AgentTurnEvent
		if err := rows.Scan(&ev.ProjectID, &ev.TurnID, &ev.Seq, &ev.Kind, &ev.EntityID,
			&ev.PayloadJSON, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// AppendAgentMessageMeta 写入消息身份（幂等：同 message_id 已存在则跳过）。
func (s *Store) AppendAgentMessageMeta(ctx context.Context, m AgentMessageMeta) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_agent_message_meta WHERE project_id=? AND message_id=?`,
		m.ProjectID, m.MessageID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_message_meta(project_id, message_id, turn_id, task_id,
			speaker_kind, speaker_id, kind, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ProjectID, m.MessageID, m.TurnID, m.TaskID, m.SpeakerKind, m.SpeakerID, m.Kind,
		m.CreatedAt); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ListAgentMessageMeta 批量读取消息身份（thread 维度，按 message_id 建索引）。
func (s *Store) ListAgentMessageMeta(ctx context.Context, projectID string, messageIDs []string) (map[string]AgentMessageMeta, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	out := make(map[string]AgentMessageMeta, len(messageIDs))
	if len(messageIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(messageIDs))
	args := make([]any, 0, len(messageIDs)+1)
	args = append(args, projectID)
	for i, id := range messageIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id, message_id, turn_id, task_id, speaker_kind, speaker_id, kind, created_at
		 FROM sys_agent_message_meta WHERE project_id=? AND message_id IN (`+strings.Join(placeholders, ", ")+`)`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m AgentMessageMeta
		if err := rows.Scan(&m.ProjectID, &m.MessageID, &m.TurnID, &m.TaskID, &m.SpeakerKind,
			&m.SpeakerID, &m.Kind, &m.CreatedAt); err != nil {
			return nil, err
		}
		out[m.MessageID] = m
	}
	return out, rows.Err()
}

// ListAgentMessageMetaByTurn 返回轮次内全部消息身份（刷新后重建任务 ↔ 消息）。
func (s *Store) ListAgentMessageMetaByTurn(ctx context.Context, projectID, turnID string) ([]AgentMessageMeta, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id, message_id, turn_id, task_id, speaker_kind, speaker_id, kind, created_at
		 FROM sys_agent_message_meta WHERE project_id=? AND turn_id=? ORDER BY created_at, message_id`,
		projectID, turnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentMessageMeta, 0, 8)
	for rows.Next() {
		var m AgentMessageMeta
		if err := rows.Scan(&m.ProjectID, &m.MessageID, &m.TurnID, &m.TaskID, &m.SpeakerKind,
			&m.SpeakerID, &m.Kind, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarshalAgentTaskDepends 把依赖任务 ID 列表序列化成 depends_json。
func MarshalAgentTaskDepends(ids []string) string {
	if ids == nil {
		ids = []string{}
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// UnmarshalAgentTaskDepends 解析 depends_json；解析失败返回空列表（不阻断推进）。
func UnmarshalAgentTaskDepends(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
