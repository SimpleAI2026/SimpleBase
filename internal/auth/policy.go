// 文件说明：policy.go — internal/auth 中 KeyPolicy、PolicyLoader 等符号的实现
//
// 用法
//
//	(s *Service) WithPolicyLoader(loader PolicyLoader) *Service
//	LoadKeyPolicy(ctx context.Context, db *sql.DB, keyID, projectID string) ...
//	同包调用：KeyPolicy → principal.go, service.go；PolicyLoader → service.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/policy.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] api_key_repository.go, cache.go, delegation.go, id.go, jwt.go 等 15 个
//	[关键引用] KeyPolicy ← principal.go, service.go
//
// 主要符号
//
//	type KeyPolicy struct {…}
//	type PolicyLoader func(context.Context, string, string) (*KeyPolicy, error)
//	(s *Service) WithPolicyLoader(loader PolicyLoader) *Service
//	LoadKeyPolicy(ctx context.Context, db *sql.DB, keyID, projectID string) ...
//	(p *KeyPolicy) Allows(resource, action string, now time.Time) bool
//	... 另有 1 个顶层符号

package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type KeyPolicy struct {
	Resources      []string
	Actions        []string
	HiddenFields   []string
	ReadonlyFields []string
	ExpiresAt      *time.Time
}

type PolicyLoader func(context.Context, string, string) (*KeyPolicy, error)

func (s *Service) WithPolicyLoader(loader PolicyLoader) *Service {
	s.policyLoader = loader
	return s
}

func LoadKeyPolicy(ctx context.Context, db *sql.DB, keyID, projectID string) (*KeyPolicy, error) {
	if db == nil {
		return nil, nil
	}
	row := db.QueryRowContext(ctx, `SELECT resources_json, actions_json, hidden_fields_json, readonly_fields_json, expires_at FROM sys_api_key_policies WHERE key_id = ? AND project_id = ?`, keyID, projectID)
	var resources, actions, hidden, readonly string
	var expires sql.NullTime
	if err := row.Scan(&resources, &actions, &hidden, &readonly, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "does not exist") || strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	policy := &KeyPolicy{}
	_ = json.Unmarshal([]byte(resources), &policy.Resources)
	_ = json.Unmarshal([]byte(actions), &policy.Actions)
	_ = json.Unmarshal([]byte(hidden), &policy.HiddenFields)
	_ = json.Unmarshal([]byte(readonly), &policy.ReadonlyFields)
	if expires.Valid {
		value := expires.Time
		policy.ExpiresAt = &value
	}
	return policy, nil
}

func (p *KeyPolicy) Allows(resource, action string, now time.Time) bool {
	if p == nil {
		return true
	}
	if p.ExpiresAt != nil && !now.Before(*p.ExpiresAt) {
		return false
	}
	return matchScope(p.Resources, resource, false) && matchScope(p.Actions, action, true)
}

func matchScope(patterns []string, value string, exactWild bool) bool {
	for _, pattern := range patterns {
		if pattern == "*" || pattern == value || exactWild && strings.HasSuffix(pattern, "*") && strings.HasPrefix(value, strings.TrimSuffix(pattern, "*")) {
			return true
		}
		if !exactWild && strings.HasSuffix(pattern, "/") && strings.HasPrefix(value, pattern) {
			return true
		}
		if !exactWild && strings.HasSuffix(pattern, ":*") && strings.HasPrefix(value, strings.TrimSuffix(pattern, "*")) {
			return true
		}
	}
	return false
}
