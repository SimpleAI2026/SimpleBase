package agentbridge

import (
	"context"

	"github.com/linkxzhou/SimpleBase/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// LogAccess 把 systemdb 的日志查询适配成 cloudagent.LogAccess。
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
func (l *LogAccess) SearchLogs(ctx context.Context, projectID, level, q string, limit int) ([]cloudagent.LogEvent, error) {
	rows, err := l.store.QueryLogs(ctx, systemdb.LogQuery{ProjectID: projectID, Level: level, Q: q, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]cloudagent.LogEvent, 0, len(rows))
	for _, e := range rows {
		out = append(out, cloudagent.LogEvent{
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
func (l *LogAccess) LevelStats(ctx context.Context, projectID string) ([]cloudagent.LogLevelCount, error) {
	stats, err := l.store.LogLevelStats(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]cloudagent.LogLevelCount, 0, len(stats))
	for _, s := range stats {
		out = append(out, cloudagent.LogLevelCount{Level: s.Level, Count: s.Count})
	}
	return out, nil
}

// SettingsAccess 把 sys_llm_settings 适配成 cloudagent.SettingsAccess。
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
func (s *SettingsAccess) LLMSettings(ctx context.Context, projectID string) (cloudagent.LLMSettings, error) {
	st, err := s.store.GetLLMSettings(ctx, projectID)
	if err != nil {
		return cloudagent.LLMSettings{}, err
	}
	return cloudagent.LLMSettings{ProjectID: st.ProjectID, DefaultModel: st.DefaultModel}, nil
}
