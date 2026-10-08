package systemdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// LLMProviderCred 是 sys_llm_provider_creds 一行：项目级厂商凭证。
// CredentialsJSON 存各字段原文（含 api_key）；仅服务端读取，API 返回时脱敏。
type LLMProviderCred struct {
	ProjectID       string
	Provider        string
	CredentialsJSON string
	DefaultModel    string
	Enabled         bool
	UpdatedAt       time.Time
}

// decodedProviderCred 是解析后的凭证（不外发）。
type decodedProviderCred struct {
	Provider     string            `json:"provider"`
	DefaultModel string            `json:"defaultModel"`
	Credentials  map[string]string `json:"credentials"`
	UpdatedAt    string            `json:"updatedAt"`
}

// MaskedProviderCred 是 API 返回的脱敏视图：secret 字段仅保留掩码。
type MaskedProviderCred struct {
	Provider     string            `json:"provider"`
	DefaultModel string            `json:"default_model"`
	Enabled      bool              `json:"enabled"`
	Credentials  map[string]string `json:"credentials"` // secret 字段为掩码；非 secret 为原文
	HasAPIKey    bool              `json:"has_api_key"`
	UpdatedAt    string            `json:"updated_at"`
}

// secretCredKeys 是需要脱敏的凭证字段。
var secretCredKeys = map[string]bool{
	"api_key": true,
}

// MaskSecretValue 掩码展示：仅保留首尾，避免完整 key 出现在 API 响应。
func MaskSecretValue(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 8 {
		return "••••••••"
	}
	return v[:3] + "..." + v[len(v)-4:]
}

// ListLLMProviderCreds 列出项目全部厂商凭证（原文；仅服务端内部使用）。
func (s *Store) ListLLMProviderCreds(ctx context.Context, projectID string) ([]LLMProviderCred, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id, provider, credentials_json, default_model, enabled, updated_at
		 FROM sys_llm_provider_creds WHERE project_id = ? ORDER BY provider`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LLMProviderCred, 0)
	for rows.Next() {
		var row LLMProviderCred
		var enabled int64
		if err := rows.Scan(&row.ProjectID, &row.Provider, &row.CredentialsJSON,
			&row.DefaultModel, &enabled, &row.UpdatedAt); err != nil {
			return nil, err
		}
		row.Enabled = enabled != 0
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetLLMProviderCred 读取单个厂商凭证；无记录返回 ErrNotFound 语义（零值 + nil）。
func (s *Store) GetLLMProviderCred(ctx context.Context, projectID, provider string) (LLMProviderCred, error) {
	if s == nil || s.db == nil {
		return LLMProviderCred{}, ErrUnavailable
	}
	var row LLMProviderCred
	var enabled int64
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id, provider, credentials_json, default_model, enabled, updated_at
		 FROM sys_llm_provider_creds WHERE project_id = ? AND provider = ?`, projectID, provider).
		Scan(&row.ProjectID, &row.Provider, &row.CredentialsJSON, &row.DefaultModel, &enabled, &row.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return LLMProviderCred{ProjectID: projectID, Provider: provider}, nil
	}
	if err != nil {
		return LLMProviderCred{}, err
	}
	row.Enabled = enabled != 0
	return row, nil
}

// UpsertLLMProviderCred 全量写入项目厂商凭证（credentials 为字段→值映射；
// secret 字段值为空字符串时保留既有值，便于“留空则不修改”）。
func (s *Store) UpsertLLMProviderCred(ctx context.Context, projectID, provider, defaultModel string, credentials map[string]string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	prev, err := s.GetLLMProviderCred(ctx, projectID, provider)
	if err != nil {
		return err
	}
	merged := map[string]string{}
	if prev.CredentialsJSON != "" {
		var old map[string]string
		if json.Unmarshal([]byte(prev.CredentialsJSON), &old) == nil {
			for k, v := range old {
				merged[k] = v
			}
		}
	}
	for k, v := range credentials {
		v = strings.TrimSpace(v)
		if v == "" && secretCredKeys[k] {
			continue // secret 留空：保留旧值
		}
		if v == "" {
			delete(merged, k)
			continue
		}
		merged[k] = v
	}
	payload, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var existing string
	err = s.db.QueryRowContext(ctx,
		`SELECT provider FROM sys_llm_provider_creds WHERE project_id = ? AND provider = ?`,
		projectID, provider).Scan(&existing)
	if err == nil {
		_, err = s.db.ExecContext(ctx,
			`UPDATE sys_llm_provider_creds SET credentials_json=?, default_model=?, enabled=1, updated_at=?
			 WHERE project_id=? AND provider=?`,
			string(payload), defaultModel, now, projectID, provider)
	} else if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO sys_llm_provider_creds(project_id, provider, credentials_json, default_model, enabled, updated_at)
			 VALUES(?, ?, ?, ?, 1, ?)`,
			projectID, provider, string(payload), defaultModel, now)
	}
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// DeleteLLMProviderCred 删除项目厂商凭证。
func (s *Store) DeleteLLMProviderCred(ctx context.Context, projectID, provider string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM sys_llm_provider_creds WHERE project_id = ? AND provider = ?`,
		projectID, provider); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// MaskedLLMProviderCreds 把凭证列表转换为脱敏视图（供 API 返回）。
func MaskedLLMProviderCreds(rows []LLMProviderCred) []MaskedProviderCred {
	out := make([]MaskedProviderCred, 0, len(rows))
	for _, r := range rows {
		creds := map[string]string{}
		if r.CredentialsJSON != "" {
			var m map[string]string
			if json.Unmarshal([]byte(r.CredentialsJSON), &m) == nil {
				for k, v := range m {
					if secretCredKeys[k] {
						creds[k] = MaskSecretValue(v)
					} else {
						creds[k] = v
					}
				}
			}
		}
		out = append(out, MaskedProviderCred{
			Provider:     r.Provider,
			DefaultModel: r.DefaultModel,
			Enabled:      r.Enabled,
			Credentials:  creds,
			HasAPIKey:    creds["api_key"] != "",
			UpdatedAt:    r.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}
