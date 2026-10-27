// 文件说明：llm.go — internal/systemdb 中 LLMSession、LLMMessage 等符号的实现
//
// 用法
//
//	MaskSecretValue(v string) string  // MaskSecretValue 掩码展示：仅保留首尾，避免完整 key 出现在 API 响应
//	(s *Store) CreateLLMSession(ctx context.Context, sess LLMSession) (LLMSession ...  // CreateLLMSession 创建会话
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/llm.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/mask
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//
// 主要符号
//
//	type LLMSession struct {…}
//	type LLMMessage struct {…}
//	type LLMSettings struct {…}
//	MaskSecretValue(v string) string
//	(s *Store) CreateLLMSession(ctx context.Context, sess LLMSession) (LLMSession ...
//	(s *Store) AppendLLMMessage(ctx context.Context, msg LLMMessage) (LLMMessage, error)
//	... 另有 2 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/utils/mask"
)

// LLMSession 是 sys_llm_sessions 一行。
type LLMSession struct {
	ID        string
	ProjectID string
	Title     string
	Provider  string
	Model     string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// LLMMessage 是 sys_llm_messages 一行。
type LLMMessage struct {
	ID          string
	SessionID   string
	ProjectID   string
	Role        string
	Content     string
	TokenInput  int64
	TokenOutput int64
	RequestID   string
	CreatedAt   time.Time
}

// LLMSettings 是项目级模型配置：连接信息（API Key / Base URL）+ 默认对话参数。
// APIKey 为原文，仅服务端内部使用，对外必须经 MaskSecretValue 脱敏，禁止记日志。
type LLMSettings struct {
	ProjectID       string
	DefaultProvider string
	DefaultModel    string
	APIKey          string
	BaseURL         string
	Temperature     float64
	MaxTokens       int
	UpdatedAt       time.Time
}

// MaskSecretValue 掩码展示：仅保留首尾，避免完整 key 出现在 API 响应。
func MaskSecretValue(v string) string {
	return mask.Secret(v)
}

// CreateLLMSession 创建会话。
func (s *Store) CreateLLMSession(ctx context.Context, sess LLMSession) (LLMSession, error) {
	if s == nil || s.db == nil {
		return LLMSession{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if sess.ID == "" {
		sess.ID = uuid.NewString()
	}
	if sess.Title == "" {
		sess.Title = "New chat"
	}
	sess.CreatedAt = now
	sess.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_llm_sessions(id, project_id, title, provider, model, created_by, created_at, updated_at, archived_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		sess.ID, sess.ProjectID, sess.Title, sess.Provider, sess.Model, sess.CreatedBy, now, now)
	if err != nil {
		return LLMSession{}, err
	}
	s.notifyWrite(ctx)
	return sess, nil
}

// AppendLLMMessage 写入一条消息并刷新会话 updated_at。
func (s *Store) AppendLLMMessage(ctx context.Context, msg LLMMessage) (LLMMessage, error) {
	if s == nil || s.db == nil {
		return LLMMessage{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	msg.CreatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_llm_messages(id, session_id, project_id, role, content, token_input, token_output, request_id, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, msg.ProjectID, msg.Role, msg.Content, msg.TokenInput, msg.TokenOutput, msg.RequestID, now)
	if err != nil {
		return LLMMessage{}, err
	}
	title := strings.TrimSpace(msg.Content)
	if len(title) > 40 {
		title = title[:40]
	}
	if msg.Role == "user" && title != "" {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE sys_llm_sessions SET updated_at = ?, title = CASE WHEN title = 'New chat' THEN ? ELSE title END WHERE id = ?`,
			now, title, msg.SessionID)
	} else {
		_, _ = s.db.ExecContext(ctx, `UPDATE sys_llm_sessions SET updated_at = ? WHERE id = ?`, now, msg.SessionID)
	}
	s.notifyWrite(ctx)
	return msg, nil
}

// GetLLMSettings 返回项目默认；无记录时返回零值。
func (s *Store) GetLLMSettings(ctx context.Context, projectID string) (LLMSettings, error) {
	if s == nil || s.db == nil {
		return LLMSettings{}, ErrUnavailable
	}
	var st LLMSettings
	var maxTok int64
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id, default_provider, default_model, COALESCE(api_key, ''), COALESCE(base_url, ''),
		        temperature, max_tokens, updated_at
		 FROM sys_llm_settings WHERE project_id = ?`, projectID).Scan(
		&st.ProjectID, &st.DefaultProvider, &st.DefaultModel, &st.APIKey, &st.BaseURL,
		&st.Temperature, &maxTok, &st.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return LLMSettings{ProjectID: projectID, Temperature: 0.7, MaxTokens: 1024}, nil
	}
	if err != nil {
		return LLMSettings{}, err
	}
	st.MaxTokens = int(maxTok)
	return st, nil
}

// PutLLMSettings 全量更新项目模型配置。
// APIKey 为空表示沿用库中既有值（前端只拿到脱敏串，不回传原文）。
func (s *Store) PutLLMSettings(ctx context.Context, st LLMSettings) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	st.UpdatedAt = now
	var existingKey string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(api_key, '') FROM sys_llm_settings WHERE project_id = ?`, st.ProjectID).Scan(&existingKey)
	if err == nil {
		if st.APIKey == "" {
			st.APIKey = existingKey
		}
		_, err = s.db.ExecContext(ctx,
			`UPDATE sys_llm_settings SET default_provider=?, default_model=?, api_key=?, base_url=?, temperature=?, max_tokens=?, updated_at=? WHERE project_id=?`,
			st.DefaultProvider, st.DefaultModel, st.APIKey, st.BaseURL, st.Temperature, int64(st.MaxTokens), now, st.ProjectID)
	} else if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO sys_llm_settings(project_id, default_provider, default_model, api_key, base_url, temperature, max_tokens, updated_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			st.ProjectID, st.DefaultProvider, st.DefaultModel, st.APIKey, st.BaseURL, st.Temperature, int64(st.MaxTokens), now)
	}
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}
