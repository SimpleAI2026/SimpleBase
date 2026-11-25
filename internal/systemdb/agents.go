// 文件说明：agents.go — internal/systemdb 中 AgentRunQueued、AgentRunRunning 等符号的实现
//
// 用法
//
//	CloudAgent、CreateCloudAgent、ListCloudAgents（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] godatabase/bench, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/agents.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/ducklake, godatabase/limit, internal/utils/strutil
//	[同包] async_flush.go, bootstrap.go, cron_jobs.go, function_triggers.go 等 22 个
//
// 主要符号
//
//	type CloudAgent struct {…}
//	func (s *Store) CreateCloudAgent(ctx context.Context, a CloudAgent) (CloudAgent, error)
//	func (s *Store) ListCloudAgents(ctx context.Context, projectID string) (out []CloudAgent, err error)
//	func (s *Store) GetCloudAgent(ctx context.Context, projectID, id string) (CloudAgent, error)
//	func (s *Store) UpdateCloudAgent(ctx context.Context, a CloudAgent) (CloudAgent, error)
//	func (s *Store) ArchiveCloudAgent(ctx context.Context, projectID, id string) error
//	func (s *Store) CountCloudAgents(ctx context.Context, projectID string) (int, error)
//	type rowScanner interface {…}
//	... 另有 3 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
)

const (
	AgentRunQueued    = "queued"
	AgentRunRunning   = "running"
	AgentRunCompleted = "completed"
	AgentRunFailed    = "failed"
	AgentRunCanceled  = "canceled"

	// cloudAgentColumns 是 sys_cloud_agents 未归档行的读取列清单，
	// 与 scanCloudAgent 的扫描顺序一一对应（planv6.0 §7 A3）。
	cloudAgentColumns = `id, project_id, name, module, description, system_prompt, tool_ids, model_override, team_enabled, created_at, updated_at, builtin_key`
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
	// 写能力不再按助手配置：所有助手都可写，破坏性命令仍由人工确认拦截
	// （planv6.7 v2 §3.4）。v48 的 writable 列保留在库里但不再读写。
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateCloudAgent 写入一个 agent。
func (s *Store) CreateCloudAgent(ctx context.Context, a CloudAgent) (CloudAgent, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return CloudAgent{}, connErr
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
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO sys_cloud_agents(id, project_id, name, module, description, system_prompt, tool_ids, model_override, team_enabled, created_at, updated_at, archived_at, builtin_key)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
		a.ID, a.ProjectID, a.Name, a.Module, a.Description, a.SystemPrompt, toolJSON, a.ModelOverride, team, now, now, a.BuiltinKey)
	if err != nil {
		return CloudAgent{}, err
	}
	s.notifyWrite(ctx)
	return a, nil
}

// ListCloudAgents 列出未归档 agent。
func (s *Store) ListCloudAgents(ctx context.Context, projectID string) (out []CloudAgent, err error) {
	// 先判库可用再登记读观测：库不可用时不产生观测日志。
	if _, err := s.conn(); err != nil {
		return nil, err
	}
	started := time.Now()
	defer func() { s.observeRead("list_cloud_agents", time.Since(started), len(out), err) }()
	return queryList(ctx, s, scanCloudAgent,
		`SELECT `+cloudAgentColumns+`
		 FROM sys_cloud_agents WHERE project_id = ? AND archived_at IS NULL
		 ORDER BY CASE WHEN builtin_key = 'general' THEN 0 ELSE 1 END, created_at ASC`, projectID)
}

// GetCloudAgent 按 ID 取未归档 agent。
func (s *Store) GetCloudAgent(ctx context.Context, projectID, id string) (CloudAgent, error) {
	return queryOne(ctx, s, scanCloudAgent,
		`SELECT `+cloudAgentColumns+`
		 FROM sys_cloud_agents WHERE id = ? AND project_id = ? AND archived_at IS NULL`,
		id, projectID)
}

// UpdateCloudAgent 更新可变字段。
func (s *Store) UpdateCloudAgent(ctx context.Context, a CloudAgent) (CloudAgent, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return CloudAgent{}, connErr
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
	res, err := sqlDB.ExecContext(ctx,
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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	release, err := s.seedLocks.Lock(ctx, projectID)
	if err != nil {
		return err
	}
	defer release()
	now := time.Now().UTC()
	// 软删是幂等的：冲突表示提交失败，整段事务重放；已归档时第二次得到 ErrNoRows，不再重试。
	err = ducklake.RetryOnConflict(ctx, func() error {
		tx, err := sqlDB.BeginTx(ctx, nil)
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
	return s.countRows(ctx,
		`SELECT COUNT(*) FROM sys_cloud_agents WHERE project_id = ? AND archived_at IS NULL`, projectID)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCloudAgent(sc rowScanner) (CloudAgent, error) {
	var a CloudAgent
	var toolJSON string
	var team int64
	if err := sc.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Module, &a.Description, &a.SystemPrompt, &toolJSON, &a.ModelOverride, &team, &a.CreatedAt, &a.UpdatedAt, &a.BuiltinKey); err != nil {
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
