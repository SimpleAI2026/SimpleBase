// 文件说明：agent_collab_cron.go — 群定时绑定与触发去重（planv6.7 v2 §11）。
//
// 用法
//
//	UpsertAgentGroupCron(ctx, AgentGroupCron) (AgentGroupCron, error)
//	GetAgentGroupCron(ctx, projectID, jobID) (AgentGroupCron, error)
//	ArchiveAgentGroupCron(ctx, projectID, jobID) error
//	ClaimCronFire(ctx, projectID, jobID, fireKey) (bool, error)   ← 返回 false 表示本次触发已处理
//
// 不改 sys_cron_jobs 的列，绑定关系单独一张表，避免动既有 cron 表结构。
// 唯一性由应用层保证：(project_id, job_id) 至多一条未归档绑定；
// (project_id, job_id, fire_key) 至多一条触发记录。
package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	// GroupCronReuseThread 每次触发复用同一会话（thread_id 非空）。
	GroupCronReuseThread = "reuse_thread"
	// GroupCronNewThread 每次触发新建一条会话（会话类型 = ThreadKind）。
	GroupCronNewThreadPerFire = "new_thread_per_fire"
)

var (
	// ErrGroupCronNotFound 表示群定时绑定不存在。
	ErrGroupCronNotFound = errors.New("systemdb: agent group cron not found")
)

// AgentGroupCron 是 sys_agent_group_cron 一行：cron 任务 → 群 的绑定。
type AgentGroupCron struct {
	ProjectID    string
	JobID        string
	TagID        string
	ThreadPolicy string
	ThreadKind   string
	ThreadID     string
	// OwnerUserID 是人工任务的默认接收人；空串表示没有负责人
	// （此时需要人的任务按 human_assignee_required 失败）。
	OwnerUserID string
	Prompt      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const agentGroupCronColumns = `project_id, job_id, tag_id, thread_policy, thread_kind,
	thread_id, owner_user_id, prompt, created_at, updated_at`

func scanAgentGroupCron(sc rowScanner) (AgentGroupCron, error) {
	var c AgentGroupCron
	if err := sc.Scan(&c.ProjectID, &c.JobID, &c.TagID, &c.ThreadPolicy, &c.ThreadKind,
		&c.ThreadID, &c.OwnerUserID, &c.Prompt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return AgentGroupCron{}, err
	}
	return c, nil
}

// UpsertAgentGroupCron 新建或更新绑定（同 project + job 只保留一行）。
func (s *Store) UpsertAgentGroupCron(ctx context.Context, c AgentGroupCron) (AgentGroupCron, error) {
	if s == nil || s.db == nil {
		return AgentGroupCron{}, ErrUnavailable
	}
	if c.ThreadPolicy == "" {
		c.ThreadPolicy = GroupCronReuseThread
	}
	if c.ThreadKind == "" {
		c.ThreadKind = ThreadKindScheduled
	}
	now := time.Now().UTC()
	c.UpdatedAt = now
	existing, err := s.GetAgentGroupCron(ctx, c.ProjectID, c.JobID)
	if err != nil {
		return AgentGroupCron{}, err
	}
	if existing.JobID == "" {
		c.CreatedAt = now
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO sys_agent_group_cron(project_id, job_id, tag_id, thread_policy, thread_kind,
				thread_id, owner_user_id, prompt, created_at, updated_at, archived_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
			c.ProjectID, c.JobID, c.TagID, c.ThreadPolicy, c.ThreadKind,
			c.ThreadID, c.OwnerUserID, c.Prompt, now, now); err != nil {
			return AgentGroupCron{}, err
		}
		s.notifyWrite(ctx)
		return c, nil
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_group_cron SET tag_id=?, thread_policy=?, thread_kind=?, thread_id=?,
			owner_user_id=?, prompt=?, updated_at=? WHERE project_id=? AND job_id=? AND archived_at IS NULL`,
		c.TagID, c.ThreadPolicy, c.ThreadKind, c.ThreadID, c.OwnerUserID, c.Prompt,
		now, c.ProjectID, c.JobID); err != nil {
		return AgentGroupCron{}, err
	}
	s.notifyWrite(ctx)
	return c, nil
}

// GetAgentGroupCron 读取绑定；无行返回零值 + nil（表示普通 agent 定时任务）。
func (s *Store) GetAgentGroupCron(ctx context.Context, projectID, jobID string) (AgentGroupCron, error) {
	if s == nil || s.db == nil {
		return AgentGroupCron{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentGroupCronColumns+` FROM sys_agent_group_cron
		 WHERE project_id=? AND job_id=? AND archived_at IS NULL ORDER BY created_at, rowid LIMIT 1`,
		projectID, jobID)
	c, err := scanAgentGroupCron(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentGroupCron{}, nil
	}
	return c, err
}

// ListAgentGroupCrons 列出项目内全部未归档群定时绑定。
func (s *Store) ListAgentGroupCrons(ctx context.Context, projectID string) ([]AgentGroupCron, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentGroupCronColumns+` FROM sys_agent_group_cron
		 WHERE project_id=? AND archived_at IS NULL ORDER BY created_at, job_id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentGroupCron, 0, 8)
	for rows.Next() {
		c, err := scanAgentGroupCron(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ArchiveAgentGroupCronsByTag 归档某个群的全部定时绑定（群归档时暂停调度）。
func (s *Store) ArchiveAgentGroupCronsByTag(ctx context.Context, projectID, tagID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_group_cron SET archived_at=?, updated_at=? WHERE project_id=? AND tag_id=? AND archived_at IS NULL`,
		time.Now().UTC(), time.Now().UTC(), projectID, tagID); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ArchiveAgentGroupCron 归档单条绑定。
func (s *Store) ArchiveAgentGroupCron(ctx context.Context, projectID, jobID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_group_cron SET archived_at=?, updated_at=? WHERE project_id=? AND job_id=? AND archived_at IS NULL`,
		time.Now().UTC(), time.Now().UTC(), projectID, jobID); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ClaimCronFire 抢占一次触发；已存在同 fire_key 时返回 false（本次跳过）。
// 返回 true 表示由本次调用负责推进，调用方此后应写入 turn_id / thread_id。
func (s *Store) ClaimCronFire(ctx context.Context, projectID, jobID, fireKey string) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_agent_cron_fires WHERE project_id=? AND job_id=? AND fire_key=?`,
		projectID, jobID, fireKey).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_cron_fires(project_id, job_id, fire_key, turn_id, thread_id, created_at)
		 VALUES(?, ?, ?, '', '', ?)`,
		projectID, jobID, fireKey, time.Now().UTC()); err != nil {
		return false, err
	}
	s.notifyWrite(ctx)
	return true, nil
}

// CompleteCronFire 回填触发产生的轮次与会话。
func (s *Store) CompleteCronFire(ctx context.Context, projectID, jobID, fireKey, turnID, threadID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_cron_fires SET turn_id=?, thread_id=? WHERE project_id=? AND job_id=? AND fire_key=?`,
		turnID, threadID, projectID, jobID, fireKey); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}
