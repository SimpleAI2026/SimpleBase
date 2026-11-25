// 文件说明：agent_seed.go — 云助手默认预置（defaultCloudAgentSpecs）与按项目幂等播种
//
// 用法
//
//	SeedDefaultCloudAgents、SeedDefaultCloudAgentsWithSandbox、SeedDefaultCloudAgentsWithCapabilities（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] godatabase/bench, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/agent_seed.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/ducklake, godatabase/limit, internal/utils/strutil
//	[同包] async_flush.go, bootstrap.go, cron_jobs.go, function_triggers.go 等 22 个
//
// 主要符号
//
//	func (s *Store) SeedDefaultCloudAgents(ctx context.Context, projectID string) error
//	func (s *Store) SeedDefaultCloudAgentsWithSandbox(ctx context.Context, projectID string, sandboxAvailable bool
//	func (s *Store) SeedDefaultCloudAgentsWithCapabilities(ctx context.Context, projectID string, sandboxAvailable
//	func (s *Store) agentSeedKeys(ctx context.Context, query, projectID string) (map[string]bool, error)

package systemdb

import (
	"context"
	"time"
)

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
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	// 单写进程内同项目播种必须串行；每个等待者均可被自己的 context 取消。
	release, err := s.seedLocks.Lock(ctx, projectID)
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
		res, err := sqlDB.ExecContext(ctx, `UPDATE sys_cloud_agents SET builtin_key=? WHERE project_id=? AND module=? AND name=? AND builtin_key='' AND archived_at IS NULL`, spec.BuiltinKey, projectID, spec.Module, spec.Name)
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
