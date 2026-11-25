// 文件说明：agent_events.go — 协作轮次事件流（AgentTurnEvent）与消息元数据（AgentMessageMeta）的持久化
//
// 用法
//
//	AgentTurnEvent、AgentMessageMeta、LastAgentTurnSeq（包内使用，由同包 handler / 路由装配调用）
//
// 主要符号
//
//	type AgentTurnEvent struct {…}
//	type AgentMessageMeta struct {…}
//	func (s *Store) LastAgentTurnSeq(ctx context.Context, projectID, turnID string) (int64, error)
//	func (s *Store) AppendAgentTurnEvent(ctx context.Context, ev AgentTurnEvent) (AgentTurnEvent, error)
//	func (s *Store) ListAgentTurnEvents(ctx context.Context, projectID, turnID string, after int64, limit int) ([]
//	func (s *Store) AppendAgentMessageMeta(ctx context.Context, m AgentMessageMeta) error
//	func (s *Store) ListAgentMessageMeta(ctx context.Context, projectID string, messageIDs []string) (map[string]A
//	func (s *Store) ListAgentMessageMetaByTurn(ctx context.Context, projectID, turnID string) ([]AgentMessageMeta,

package systemdb

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

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

// LastAgentTurnSeq 返回轮次事件的当前最大序号（0 表示还没有事件）。
func (s *Store) LastAgentTurnSeq(ctx context.Context, projectID, turnID string) (int64, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return 0, connErr
	}

	var seq sql.NullInt64
	err := sqlDB.QueryRowContext(ctx,
		`SELECT MAX(seq) FROM sys_agent_turn_events WHERE project_id=? AND turn_id=?`,
		projectID, turnID).Scan(&seq)
	if err != nil {
		return 0, err
	}
	return seq.Int64, nil
}

// AppendAgentTurnEvent 追加一条轮次事件，序号由调用方保证单调递增。
func (s *Store) AppendAgentTurnEvent(ctx context.Context, ev AgentTurnEvent) (AgentTurnEvent, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentTurnEvent{}, connErr
	}

	if ev.PayloadJSON == "" {
		ev.PayloadJSON = "{}"
	}
	ev.CreatedAt = time.Now().UTC()
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO sys_agent_turn_events(project_id, turn_id, seq, kind, entity_id, payload_json, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		ev.ProjectID, ev.TurnID, ev.Seq, ev.Kind, ev.EntityID, ev.PayloadJSON, ev.CreatedAt); err != nil {
		return AgentTurnEvent{}, err
	}
	s.notifyWrite(ctx)
	return ev, nil
}

// AppendAgentTurnEventAtomic 原子分配轮次内 seq 并追加事件，返回带 seq 的事件。
// 调用方无需也不应预先读取 LastAgentTurnSeq。
//
// seq 不能交给 SQL 分配：系统库是 DuckLake 表，INSERT … RETURNING 会直接报
// 「RETURNING clause not yet supported for insertion into DuckLake table」
// （SQLite 夹具却能通过，只有真实 DuckLake 用例能拦住）。这里按轮次加锁后
// 「读最大序号 → 写入」，单写实例下同一轮次内单调且不重复。
func (s *Store) AppendAgentTurnEventAtomic(ctx context.Context, ev AgentTurnEvent) (AgentTurnEvent, error) {
	if s == nil {
		return AgentTurnEvent{}, ErrUnavailable
	}
	key := ev.ProjectID + "/" + ev.TurnID
	release, err := s.turnEventLocks.Lock(ctx, key)
	if err != nil {
		return AgentTurnEvent{}, err
	}
	defer release()
	seq, err := s.LastAgentTurnSeq(ctx, ev.ProjectID, ev.TurnID)
	if err != nil {
		return AgentTurnEvent{}, err
	}
	ev.Seq = seq + 1
	return s.AppendAgentTurnEvent(ctx, ev)
}

// ListAgentTurnEvents 返回 seq > after 的事件（快照 + 续传）。
func (s *Store) ListAgentTurnEvents(ctx context.Context, projectID, turnID string, after int64, limit int) ([]AgentTurnEvent, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, connErr
	}

	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := sqlDB.QueryContext(ctx,
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	var n int
	if err := sqlDB.QueryRowContext(ctx,
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
	if _, err := sqlDB.ExecContext(ctx,
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, connErr
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
	rows, err := sqlDB.QueryContext(ctx,
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, connErr
	}

	rows, err := sqlDB.QueryContext(ctx,
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
