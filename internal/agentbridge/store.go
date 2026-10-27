// 文件说明：store.go — internal/agentbridge 中 LogAccess、NewLogAccess 等符号的实现
//
// 用法
//
//	NewLogAccess(store *systemdb.Store) *LogAccess
//	(l *LogAccess) SearchLogs(ctx context.Context, projectID, level, q string, limit ...  // SearchLogs 查询项目日志
//	(l *LogAccess) LevelStats(ctx context.Context, projectID string) ...  // LevelStats 按 level 统计
//	同包调用：LogAccess → principal.go；SettingsAccess → principal.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/store.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, internal/systemdb
//	[同包] cli.go, command.go, cron_agent.go, delegation.go, errors.go 等 6 个
//	[关键引用] LogAccess ← principal.go
//
// 主要符号
//
//	type LogAccess struct {…}
//	NewLogAccess(store *systemdb.Store) *LogAccess
//	(l *LogAccess) SearchLogs(ctx context.Context, projectID, level, q string, limit ...
//	(l *LogAccess) LevelStats(ctx context.Context, projectID string) ...
//	type SettingsAccess struct {…}
//	NewSettingsAccess(store *systemdb.Store) *SettingsAccess
//	... 另有 1 个顶层符号

package agentbridge

import (
	"context"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// LogAccess 把 systemdb 的日志查询适配成 gocloudagent.LogAccess。
// 保持原有只读语义与 admin 项目不过滤 project 的行为（由 Store 内部处理）。
type LogAccess struct{ store *systemdb.Store }

// NewLogAccess 包装 systemdb 日志查询；store 为 nil 时返回 nil（调用方按
// 「日志未配置」处理，与迁移前一致）。
func NewLogAccess(store *systemdb.Store) *LogAccess {
	if store == nil {
		return nil
	}
	return &LogAccess{store: store}
}

// SearchLogs 查询项目日志。
func (l *LogAccess) SearchLogs(ctx context.Context, projectID, level, q string, limit int) ([]gocloudagent.LogEvent, error) {
	rows, err := l.store.QueryLogs(ctx, systemdb.LogQuery{ProjectID: projectID, Level: level, Q: q, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]gocloudagent.LogEvent, 0, len(rows))
	for _, e := range rows {
		out = append(out, gocloudagent.LogEvent{
			ID:         e.ID,
			ProjectID:  e.ProjectID,
			Level:      e.Level,
			Logger:     e.Logger,
			Message:    e.Message,
			FieldsJSON: e.FieldsJSON,
			RequestID:  e.RequestID,
			OccurredAt: e.OccurredAt,
		})
	}
	return out, nil
}

// LevelStats 按 level 统计。
func (l *LogAccess) LevelStats(ctx context.Context, projectID string) ([]gocloudagent.LogLevelCount, error) {
	stats, err := l.store.LogLevelStats(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]gocloudagent.LogLevelCount, 0, len(stats))
	for _, s := range stats {
		out = append(out, gocloudagent.LogLevelCount{Level: s.Level, Count: s.Count})
	}
	return out, nil
}

// SettingsAccess 把 sys_llm_settings 适配成 gocloudagent.SettingsAccess。
// 只转出默认模型，API Key / BaseURL 不出宿主。
type SettingsAccess struct{ store *systemdb.Store }

// NewSettingsAccess 包装项目 LLM 设置；store 为 nil 时返回 nil。
func NewSettingsAccess(store *systemdb.Store) *SettingsAccess {
	if store == nil {
		return nil
	}
	return &SettingsAccess{store: store}
}

// LLMSettings 返回项目默认模型。
func (s *SettingsAccess) LLMSettings(ctx context.Context, projectID string) (gocloudagent.LLMSettings, error) {
	st, err := s.store.GetLLMSettings(ctx, projectID)
	if err != nil {
		return gocloudagent.LLMSettings{}, err
	}
	return gocloudagent.LLMSettings{ProjectID: st.ProjectID, DefaultModel: st.DefaultModel}, nil
}
