// 文件说明：agent_collab.go — 协作群的轮次（turn）、任务（task）、事件与消息身份。
//
// 用法
//
//	AgentThreadCollab、AgentTurn、CollabSchemaReady（包内使用，由同包 handler / 路由装配调用）
//
// 主要符号
//
//	type AgentThreadCollab struct {…}
//	type AgentTurn struct {…}
//	func scanAgentTurn(sc rowScanner) (AgentTurn, error)
//	func (s *Store) collabSchemaReady(ctx context.Context) error
//	func (s *Store) CollabSchemaReady(ctx context.Context) error
//	func (s *Store) CreateAgentThreadCollab(ctx context.Context, c AgentThreadCollab) (AgentThreadCollab, error)
//	func (s *Store) GetAgentThreadCollab(ctx context.Context, projectID, threadID string) (AgentThreadCollab, erro
//	func (s *Store) ListAgentThreadCollabs(ctx context.Context, projectID string, threadIDs []string) (map[string]
//	... 另有 9 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
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
	TurnStatusPlanning     = "planning"      // 栅栏决策中（planv6.8 §13：取代旧 routing）
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

// collabSchemaReady 探测协作表是否就绪（未升级环境返回明文错误而非 SQL 错误）。
func (s *Store) collabSchemaReady(ctx context.Context) error {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	if _, err := sqlDB.ExecContext(ctx, `SELECT 1 FROM sys_agent_turns LIMIT 0`); err != nil {
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentThreadCollab{}, connErr
	}

	now := time.Now().UTC()
	if c.ThreadKind == "" {
		c.ThreadKind = ThreadKindChat
	}
	if c.ProtocolVersion == 0 {
		c.ProtocolVersion = 2
	}
	c.Version = 1
	if _, err := sqlDB.ExecContext(ctx,
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentThreadCollab{}, connErr
	}

	var c AgentThreadCollab
	err := sqlDB.QueryRowContext(ctx,
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, connErr
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
	rows, err := sqlDB.QueryContext(ctx,
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	res, err := sqlDB.ExecContext(ctx,
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	if _, err := sqlDB.ExecContext(ctx,
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentTurn{}, false, connErr
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
		t.Status = TurnStatusPlanning
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
	if _, err := sqlDB.ExecContext(ctx,
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
	t, err := queryOne(ctx, s, scanAgentTurn,
		`SELECT `+agentTurnColumns+` FROM sys_agent_turns WHERE project_id=? AND id=?`,
		projectID, turnID)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentTurn{}, ErrTurnNotFound
	}
	return t, err
}

// ListAgentTurns 列出会话的轮次（按创建时间倒序，keyset 分页）。
func (s *Store) ListAgentTurns(ctx context.Context, projectID, threadID string, limit int) ([]AgentTurn, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, connErr
	}

	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := sqlDB.QueryContext(ctx,
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
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
	res, err := sqlDB.ExecContext(ctx, query, args...)
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	if _, err := sqlDB.ExecContext(ctx,
		`UPDATE sys_agent_turns SET error_code='', updated_at=? WHERE project_id=? AND id=?`,
		time.Now().UTC(), projectID, turnID); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ListRunningAgentTurns 列出仍在推进的轮次（进程启动时收敛为 interrupted）。
func (s *Store) ListRunningAgentTurns(ctx context.Context, cutoff time.Time) ([]AgentTurn, error) {
	if err := s.collabSchemaReady(ctx); err != nil {
		return nil, err
	}
	return queryList(ctx, s, scanAgentTurn,
		`SELECT `+agentTurnColumns+` FROM sys_agent_turns
		 WHERE status IN (?, ?) AND created_at < ? ORDER BY created_at LIMIT 500`,
		TurnStatusPlanning, TurnStatusRunning, cutoff)
}
