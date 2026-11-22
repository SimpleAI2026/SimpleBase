// 文件说明：agent_tags.go — planv6.7 AGC-NAV/AGC-ROUTE 的工作台标签、会话元数据与路由记录。
//
// 三张表：
//   - sys_agent_workspace_tags：标签本体与配置（预置 alerts/stats/scheduled 的覆盖行 + 自定义标签；general 不写覆盖行）
//   - sys_agent_thread_workspace：会话的工作台元数据（只在建会话事务内插入）
//   - sys_agent_run_routes：每个 run 的路由结论（刷新后恢复回答原因）
//
// DuckLake 无 PK/UNIQUE 约束，唯一性全部由应用层保证：
//   - (project_id, id) 的标签行至多一行；同项目未归档 name 不重复（大小写不敏感）
//   - (project_id, thread_id) 的元数据行至多一行（仅建会话事务内插入这一条写入路径）
//   - (project_id, run_id) 的路由行至多一行（InsertAgentRunRoute 先查后插）

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
)

const (
	// AgentTagGeneral 是包含项目全部助手的默认标签，ID 固定且同时是 builtin_key。
	AgentTagGeneral = "general"
	// AgentTagAlerts / AgentTagStats 是预置标签，ID 固定且同时是 builtin_key。
	AgentTagAlerts = "alerts"
	AgentTagStats  = "stats"
	// AgentTagScheduled 是「定时」内置群：群定时任务的默认落点（v2 §11）。
	AgentTagScheduled = "scheduled"

	// DispatchAuto 表示一轮一助手的共享会话（auto 会话按 planv6.7 §3 路由）。
	DispatchAuto = "auto"
	// DispatchCollaborative 表示协作群会话（planv6.7 v2）：一轮可 @ 多位助手，
	// 由群路由提示词决定分工，必要时交给当前人。与 auto 并存，互不改变。
	DispatchCollaborative = "collaborative"

	// 会话来源（sys_agent_thread_workspace.source；旧会话无行视为 legacy）。
	ThreadSourceWorkspace = "workspace" // 新工作台创建
	ThreadSourceStarter   = "starter"   // 首页一句话建应用

	// 路由结论（sys_agent_run_routes.route_kind）。
	RouteAuto     = "auto"     // 路由器选中
	RouteMention  = "mention"  // 用户 @ 指定
	RouteFallback = "fallback" // none/超时/越界/失败后的兜底
	RouteRetry    = "retry"    // 失败重试固定原助手

	// 标签数量上限（AGC-NAV §2.1：每项目最多 50 个未归档自定义标签）。
	agentTagCustomLimit = 50
)

var (
	ErrAgentTagNotFound  = errors.New("systemdb: agent tag not found")
	ErrAgentTagConflict  = errors.New("systemdb: agent tag version conflict")
	ErrAgentTagDuplicate = errors.New("systemdb: agent tag name already exists")
	ErrAgentTagBuiltin   = errors.New("systemdb: builtin agent tag cannot be archived")
	ErrAgentTagLimit     = errors.New("systemdb: agent tag limit exceeded")
)

// AgentTagPrompt 是标签里的快捷提示词（点击填入输入框，不是系统指令）。
type AgentTagPrompt struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// HumanPolicy 是群的人工参与策略（v2 §6.4）：是否允许路由把任务交给当前人。
// 空串等价 HumanPolicyAllowed，兼容旧配置。
type HumanPolicy = string

const (
	// HumanPolicyAllowed 允许路由请求 @当前人（默认）。
	HumanPolicyAllowed = ""
	// HumanPolicyDisabled 群内不产生人工任务；需要人时按 routing_failed 结束。
	HumanPolicyDisabled = "disabled"
)

// AgentTagConfig 是 config_json 的唯一形状；不得存密钥。
type AgentTagConfig struct {
	AgentIDs []string         `json:"agent_ids"`
	Prompts  []AgentTagPrompt `json:"prompts"`
	// RoutingPrompt 是群路由提示词（planv6.7 v2）：只用来决定「交给谁、做什么」，
	// 不授予工具权限，也不是助手的系统提示词。旧配置没有该键，读出为空串。
	RoutingPrompt string `json:"routing_prompt,omitempty"`
	// FallbackAgentID 是路由失败/超时/越界时唯一可用的兜底助手（必须是本群成员）。
	// 为空表示没有兜底：此时请当前人澄清，仍无人可用则轮次以 routing_failed 结束。
	FallbackAgentID string `json:"fallback_agent_id,omitempty"`
	// HumanPolicy 控制是否允许人工任务；空串等价允许。
	HumanPolicy HumanPolicy `json:"human_policy,omitempty"`
	// SchemaVersion 是配置形状版本，便于后续演进；0/1 视为旧配置。
	SchemaVersion int `json:"schema_version,omitempty"`
}

// AgentTag 是 sys_agent_workspace_tags 一行。
type AgentTag struct {
	ID         string
	ProjectID  string
	BuiltinKey string // 非空表示预置标签（alerts/stats）
	Name       string
	Config     AgentTagConfig
	Version    int64
	CreatedBy  string
	// Persisted 为 false 表示内置默认、尚未写覆盖行（api 层合并用）。
	Persisted bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AgentThreadWorkspace 是 sys_agent_thread_workspace 一行；旧会话没有此行。
type AgentThreadWorkspace struct {
	ProjectID    string
	ThreadID     string
	TagID        string
	DispatchMode string
	Source       string
}

// AgentRunRoute 是 sys_agent_run_routes 一行。
type AgentRunRoute struct {
	ProjectID string
	RunID     string
	ThreadID  string
	AgentID   string
	RouteKind string
	RouterMS  int64
}

func marshalAgentTagConfig(cfg AgentTagConfig) (string, error) {
	if cfg.AgentIDs == nil {
		cfg.AgentIDs = []string{}
	}
	if cfg.Prompts == nil {
		cfg.Prompts = []AgentTagPrompt{}
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalAgentTagConfig(raw string) (AgentTagConfig, error) {
	var cfg AgentTagConfig
	if strings.TrimSpace(raw) == "" {
		return AgentTagConfig{AgentIDs: []string{}, Prompts: []AgentTagPrompt{}}, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return AgentTagConfig{}, err
	}
	if cfg.AgentIDs == nil {
		cfg.AgentIDs = []string{}
	}
	if cfg.Prompts == nil {
		cfg.Prompts = []AgentTagPrompt{}
	}
	return cfg, nil
}

func scanAgentTag(sc rowScanner) (AgentTag, error) {
	var t AgentTag
	var cfgJSON, builtinKey string
	var archived sql.NullTime
	if err := sc.Scan(&t.ID, &t.ProjectID, &builtinKey, &t.Name, &cfgJSON, &t.Version, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt, &archived); err != nil {
		return AgentTag{}, err
	}
	t.BuiltinKey = builtinKey
	t.Persisted = true
	cfg, err := unmarshalAgentTagConfig(cfgJSON)
	if err != nil {
		return AgentTag{}, err
	}
	t.Config = cfg
	return t, nil
}

const agentTagColumns = `id, project_id, builtin_key, name, config_json, version, created_by, created_at, updated_at, archived_at`

// ListAgentTagRows 只读未归档标签行（含预置覆盖行），ORDER BY created_at, id。
// 内置默认的合并在 api 层完成（需要 builtin_key → 本项目助手 ID 解析）。
func (s *Store) ListAgentTagRows(ctx context.Context, projectID string) ([]AgentTag, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentTagColumns+` FROM sys_agent_workspace_tags WHERE project_id=? AND archived_at IS NULL ORDER BY created_at, id`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentTag, 0)
	for rows.Next() {
		t, err := scanAgentTag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetAgentTagRow 按 ID 取未归档标签行；无行返回 sql.ErrNoRows。
// 预置标签无覆盖行时由 api 层回落默认（AGC-NAV §2.2）。
func (s *Store) GetAgentTagRow(ctx context.Context, projectID, id string) (AgentTag, error) {
	if s == nil || s.db == nil {
		return AgentTag{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentTagColumns+` FROM sys_agent_workspace_tags WHERE id=? AND project_id=? AND archived_at IS NULL`,
		id, projectID)
	t, err := scanAgentTag(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentTag{}, sql.ErrNoRows
	}
	return t, err
}

// agentTagNameTaken 查同项目未归档且不同 ID 的重名标签（lower 比对）。
func (s *Store) agentTagNameTaken(ctx context.Context, projectID, id, name string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_agent_workspace_tags WHERE project_id=? AND archived_at IS NULL AND lower(name)=lower(?) AND id<>?`,
		projectID, name, id).Scan(&n)
	return n > 0, err
}

// CreateAgentTag 创建自定义标签：锁内校验 50 上限与重名，生成 UUID 插入，version=1。
func (s *Store) CreateAgentTag(ctx context.Context, t AgentTag) (AgentTag, error) {
	if s == nil || s.db == nil {
		return AgentTag{}, ErrUnavailable
	}
	cfgJSON, err := marshalAgentTagConfig(t.Config)
	if err != nil {
		return AgentTag{}, err
	}
	release, err := s.lockAgentTag(ctx, t.ProjectID)
	if err != nil {
		return AgentTag{}, err
	}
	defer release()
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_agent_workspace_tags WHERE project_id=? AND archived_at IS NULL AND builtin_key=''`,
		t.ProjectID).Scan(&count); err != nil {
		return AgentTag{}, err
	}
	if count >= agentTagCustomLimit {
		return AgentTag{}, ErrAgentTagLimit
	}
	taken, err := s.agentTagNameTaken(ctx, t.ProjectID, "", t.Name)
	if err != nil {
		return AgentTag{}, err
	}
	if taken {
		return AgentTag{}, ErrAgentTagDuplicate
	}
	now := time.Now().UTC()
	t.ID = uuid.NewString()
	t.BuiltinKey = ""
	t.Version = 1
	t.Persisted = true
	t.CreatedAt = now
	t.UpdatedAt = now
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_workspace_tags(id, project_id, builtin_key, name, config_json, version, created_by, created_at, updated_at, archived_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		t.ID, t.ProjectID, t.BuiltinKey, t.Name, cfgJSON, t.Version, t.CreatedBy, now, now); err != nil {
		return AgentTag{}, err
	}
	s.notifyWrite(ctx)
	return t, nil
}

// IsBuiltinAgentTag 报告 key 是否是预置群（general/alerts/stats/scheduled）。
// 预置群不可归档，名称固定，配置走 UpsertBuiltinAgentTag 的 CAS 覆盖行。
func IsBuiltinAgentTag(key string) bool {
	return key == AgentTagGeneral || key == AgentTagAlerts || key == AgentTagStats || key == AgentTagScheduled
}

// IsReservedAgentTagID 报告 id 是否是保留的内置群 ID（不可用于自定义群）。
func IsReservedAgentTagID(id string) bool {
	return IsBuiltinAgentTag(id)
}

// UpsertBuiltinAgentTag 保存预置标签的首个覆盖行或推进版本：
// 无行且 expectedVersion==0 → 插入 version=1；有行且版本相等 → version+1；
// 否则 ErrAgentTagConflict。名称固定为 key 对应的显示名。
func (s *Store) UpsertBuiltinAgentTag(ctx context.Context, projectID, key, name string, cfg AgentTagConfig, expectedVersion int64) (AgentTag, error) {
	if s == nil || s.db == nil {
		return AgentTag{}, ErrUnavailable
	}
	if !IsBuiltinAgentTag(key) {
		return AgentTag{}, ErrAgentTagNotFound
	}
	cfgJSON, err := marshalAgentTagConfig(cfg)
	if err != nil {
		return AgentTag{}, err
	}
	release, err := s.lockAgentTag(ctx, projectID)
	if err != nil {
		return AgentTag{}, err
	}
	defer release()
	var version int64
	err = s.db.QueryRowContext(ctx,
		`SELECT version FROM sys_agent_workspace_tags WHERE id=? AND project_id=? AND archived_at IS NULL`,
		key, projectID).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		if expectedVersion != 0 {
			return AgentTag{}, ErrAgentTagConflict
		}
		now := time.Now().UTC()
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO sys_agent_workspace_tags(id, project_id, builtin_key, name, config_json, version, created_by, created_at, updated_at, archived_at)
			 VALUES(?, ?, ?, ?, ?, 1, '', ?, ?, NULL)`,
			key, projectID, key, name, cfgJSON, now, now); err != nil {
			return AgentTag{}, err
		}
		s.notifyWrite(ctx)
		return s.GetAgentTagRow(ctx, projectID, key)
	}
	if err != nil {
		return AgentTag{}, err
	}
	if version != expectedVersion {
		return AgentTag{}, ErrAgentTagConflict
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_workspace_tags SET name=?, config_json=?, version=version+1, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL`,
		name, cfgJSON, time.Now().UTC(), key, projectID)
	if err != nil {
		return AgentTag{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return AgentTag{}, ErrAgentTagNotFound
	}
	s.notifyWrite(ctx)
	return s.GetAgentTagRow(ctx, projectID, key)
}

// UpdateAgentTag 更新自定义标签；RowsAffected==0 时区分 NotFound 与 Conflict。
func (s *Store) UpdateAgentTag(ctx context.Context, projectID, id, name string, cfg AgentTagConfig, expectedVersion int64) (AgentTag, error) {
	if s == nil || s.db == nil {
		return AgentTag{}, ErrUnavailable
	}
	cfgJSON, err := marshalAgentTagConfig(cfg)
	if err != nil {
		return AgentTag{}, err
	}
	release, err := s.lockAgentTag(ctx, projectID)
	if err != nil {
		return AgentTag{}, err
	}
	defer release()
	if name != "" {
		taken, err := s.agentTagNameTaken(ctx, projectID, id, name)
		if err != nil {
			return AgentTag{}, err
		}
		if taken {
			return AgentTag{}, ErrAgentTagDuplicate
		}
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_workspace_tags SET name=?, config_json=?, version=version+1, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL AND version=?`,
		name, cfgJSON, time.Now().UTC(), id, projectID, expectedVersion)
	if err != nil {
		return AgentTag{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err := s.GetAgentTagRow(ctx, projectID, id)
		if errors.Is(err, sql.ErrNoRows) {
			return AgentTag{}, ErrAgentTagNotFound
		}
		if err != nil {
			return AgentTag{}, err
		}
		return AgentTag{}, ErrAgentTagConflict
	}
	s.notifyWrite(ctx)
	return s.GetAgentTagRow(ctx, projectID, id)
}

// ArchiveAgentTag 归档自定义群。v2 §9.2：**保留**会话的 tag_id 归属，
// 历史仍可在群下查看，只是该群不再接受新发送与新调度（候选范围不扩大）。
// 预置标签返回 ErrAgentTagBuiltin。
func (s *Store) ArchiveAgentTag(ctx context.Context, projectID, id string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if IsBuiltinAgentTag(id) {
		return ErrAgentTagBuiltin
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_workspace_tags SET archived_at=?, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL`,
		time.Now().UTC(), time.Now().UTC(), id, projectID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.GetAgentTagRow(ctx, projectID, id); errors.Is(err, sql.ErrNoRows) {
			return ErrAgentTagNotFound
		}
		return ErrAgentTagNotFound
	}
	s.notifyWrite(ctx)
	return nil
}

// CreateAgentThreadWithWorkspace 在同一事务内插入会话与工作台元数据；
// ws 为零值时不写元数据行（等价于原 CreateAgentThread 语义）。
// 元数据行只在建会话事务内插入，这是唯一写入路径（AGC-NAV §2.1）。
func (s *Store) CreateAgentThreadWithWorkspace(ctx context.Context, t AgentThread, ws AgentThreadWorkspace) (AgentThread, error) {
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
	writeMeta := ws.TagID != "" || ws.DispatchMode != "" || ws.Source != ""
	err := ducklake.RetryOnConflict(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sys_agent_threads(id, project_id, title, agent_id, created_by, created_at, updated_at, archived_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, NULL)`,
			t.ID, t.ProjectID, t.Title, t.AgentID, t.CreatedBy, now, now); err != nil {
			return err
		}
		if writeMeta {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO sys_agent_thread_workspace(project_id, thread_id, tag_id, dispatch_mode, source, created_at)
				 VALUES(?, ?, ?, ?, ?, ?)`,
				t.ProjectID, t.ID, ws.TagID, ws.DispatchMode, ws.Source, now); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
	if err != nil {
		return AgentThread{}, err
	}
	s.notifyWrite(ctx)
	return t, nil
}

// GetAgentThreadWorkspace 读取会话工作台元数据；旧会话无行返回零值 + nil。
// DuckLake 无唯一约束，遇多行取最早一行（正常不应发生，见 §9 风险）。
func (s *Store) GetAgentThreadWorkspace(ctx context.Context, projectID, threadID string) (AgentThreadWorkspace, error) {
	if s == nil || s.db == nil {
		return AgentThreadWorkspace{}, ErrUnavailable
	}
	var ws AgentThreadWorkspace
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id, thread_id, tag_id, dispatch_mode, source FROM sys_agent_thread_workspace
		 WHERE project_id=? AND thread_id=? ORDER BY created_at, rowid LIMIT 1`,
		projectID, threadID).Scan(&ws.ProjectID, &ws.ThreadID, &ws.TagID, &ws.DispatchMode, &ws.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentThreadWorkspace{}, nil
	}
	return ws, err
}

// ListAgentThreadWorkspaces 批量读取会话元数据（列表 DTO 补字段，避免 N+1）。
// threadIDs 为空返回空 map。
func (s *Store) ListAgentThreadWorkspaces(ctx context.Context, projectID string, threadIDs []string) (map[string]AgentThreadWorkspace, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	out := make(map[string]AgentThreadWorkspace, len(threadIDs))
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
	query := `SELECT project_id, thread_id, tag_id, dispatch_mode, source FROM sys_agent_thread_workspace
		 WHERE project_id=? AND thread_id IN (` + strings.Join(placeholders, ", ") + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ws AgentThreadWorkspace
		if err := rows.Scan(&ws.ProjectID, &ws.ThreadID, &ws.TagID, &ws.DispatchMode, &ws.Source); err != nil {
			return nil, err
		}
		if _, dup := out[ws.ThreadID]; !dup {
			out[ws.ThreadID] = ws
		}
	}
	return out, rows.Err()
}

// UpdateAgentRunAgent 只允许把 run 的空 agent_id 回填一次（planv6.7 §3.3）。
func (s *Store) UpdateAgentRunAgent(ctx context.Context, projectID, runID, agentID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_runs SET agent_id=? WHERE id=? AND project_id=? AND agent_id=''`,
		agentID, runID, projectID)
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	_, _ = res.RowsAffected() // 已回填过不算错误：幂等语义
	return nil
}

// InsertAgentRunRoute 写入路由结论；同 run 已有行则跳过（先查后插）。
func (s *Store) InsertAgentRunRoute(ctx context.Context, r AgentRunRoute) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_agent_run_routes WHERE project_id=? AND run_id=?`,
		r.ProjectID, r.RunID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_run_routes(project_id, run_id, thread_id, agent_id, route_kind, router_ms, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		r.ProjectID, r.RunID, r.ThreadID, r.AgentID, r.RouteKind, r.RouterMS, time.Now().UTC()); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ListAgentRunRoutes 返回 runID → 路由结论（供 ListMessages 补 route_kind）。
func (s *Store) ListAgentRunRoutes(ctx context.Context, projectID, threadID string) (map[string]AgentRunRoute, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id, run_id, thread_id, agent_id, route_kind, router_ms FROM sys_agent_run_routes
		 WHERE project_id=? AND thread_id=?`,
		projectID, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]AgentRunRoute)
	for rows.Next() {
		var r AgentRunRoute
		if err := rows.Scan(&r.ProjectID, &r.RunID, &r.ThreadID, &r.AgentID, &r.RouteKind, &r.RouterMS); err != nil {
			return nil, err
		}
		out[r.RunID] = r
	}
	return out, rows.Err()
}

// lockAgentTag 仅串行同一项目的标签写入（与 lockAgentSeed 同构）。
// 调用方必须执行 release。
func (s *Store) lockAgentTag(ctx context.Context, projectID string) (func(), error) {
	for {
		s.tagMu.Lock()
		if s.tagFlights == nil {
			s.tagFlights = make(map[string]chan struct{})
		}
		if done := s.tagFlights[projectID]; done != nil {
			s.tagMu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if err := ctx.Err(); err != nil {
			s.tagMu.Unlock()
			return nil, err
		}
		done := make(chan struct{})
		s.tagFlights[projectID] = done
		s.tagMu.Unlock()
		return func() {
			s.tagMu.Lock()
			delete(s.tagFlights, projectID)
			close(done)
			s.tagMu.Unlock()
		}, nil
	}
}
