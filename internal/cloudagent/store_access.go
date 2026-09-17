package cloudagent

import (
	"context"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type storeLogs struct{ store *systemdb.Store }

// NewLogAccess wraps systemdb log queries.
func NewLogAccess(store *systemdb.Store) LogAccess {
	if store == nil {
		return nil
	}
	return &storeLogs{store: store}
}

func (s *storeLogs) SearchLogs(ctx context.Context, projectID, level, q string, limit int) ([]systemdb.LogEvent, error) {
	return s.store.QueryLogs(ctx, systemdb.LogQuery{ProjectID: projectID, Level: level, Q: q, Limit: limit})
}

func (s *storeLogs) LevelStats(ctx context.Context, projectID string) ([]systemdb.LogLevelCount, error) {
	return s.store.LogLevelStats(ctx, projectID)
}

type storeSettings struct{ store *systemdb.Store }

// NewSettingsAccess wraps sys_llm_settings (no secrets).
func NewSettingsAccess(store *systemdb.Store) SettingsAccess {
	if store == nil {
		return nil
	}
	return &storeSettings{store: store}
}

func (s *storeSettings) LLMSettings(ctx context.Context, projectID string) (systemdb.LLMSettings, error) {
	return s.store.GetLLMSettings(ctx, projectID)
}
