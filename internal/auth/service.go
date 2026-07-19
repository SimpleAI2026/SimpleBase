// service.go 实现 auth.Service：解析 API key、构造 Principal、鉴权判断。
//
// API key 原文只在接收请求时短暂出现于内存；持久化只保存带 server secret 的
// HMAC 摘要，绝不可逆。日志/错误/审计不得回显 key 原文。
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 认证错误。api 层据此映射 401/403。
var (
	ErrMissingCredentials = errors.New("auth: missing or malformed authorization header")
	ErrInvalidCredentials = errors.New("auth: invalid api key")
	ErrKeyRevoked         = errors.New("auth: api key revoked")
	ErrForbidden          = errors.New("auth: permission denied")
)

// APIKeyRecord 是持久化的 API key 元数据。KeyHash 是 HMAC-SHA256(secret, rawKey) 的
// 十六进制编码，绝不存储原文。
type APIKeyRecord struct {
	ID          string
	TenantID    string
	ProjectIDs  []string
	Permissions []Permission
	KeyHash     string
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

// Repository 抽象 API key 的查找方式，由 Plan 5 的 api_key_repository 实现
// （基于 catalog 系统数据库）。
type Repository interface {
	// FindByHash 按 key hash 查找记录；未找到返回 ErrInvalidCredentials。
	FindByHash(ctx context.Context, keyHash string) (APIKeyRecord, error)
}

// Service 是认证与授权服务。
type Service struct {
	repo   Repository
	secret string
}

// NewService 构造 Service。secret 对应 config.Auth.APIKeyHashSecret，
// 用于计算 HMAC，绝不能为空（由 config.Validate 保证）。
func NewService(repo Repository, secret string) *Service {
	return &Service{repo: repo, secret: secret}
}

// HashKey 计算 API key 的 HMAC-SHA256 摘要，用于存储与比对。
// 调用方（创建 key 的管理流程）与 Authenticate 共用此函数，确保摘要一致。
func (s *Service) HashKey(rawKey string) string {
	mac := hmac.New(sha256.New, []byte(s.secret))
	mac.Write([]byte(rawKey))
	return hex.EncodeToString(mac.Sum(nil))
}

// Authenticate 解析并校验 rawKey，返回对应的 Principal。
// 拒绝：空 key、找不到记录、已撤销 key。
func (s *Service) Authenticate(ctx context.Context, rawKey string) (Principal, error) {
	if rawKey == "" {
		return Principal{}, ErrMissingCredentials
	}
	if s.repo == nil {
		return Principal{}, fmt.Errorf("auth: repository not configured")
	}
	hash := s.HashKey(rawKey)
	rec, err := s.repo.FindByHash(ctx, hash)
	if err != nil {
		return Principal{}, ErrInvalidCredentials
	}
	// 常量时间比对，进一步降低时序侧信道风险（repo 查找本身已按 hash 索引）。
	if subtle.ConstantTimeCompare([]byte(rec.KeyHash), []byte(hash)) != 1 {
		return Principal{}, ErrInvalidCredentials
	}
	if rec.RevokedAt != nil {
		return Principal{}, ErrKeyRevoked
	}

	pids := make(map[string]struct{}, len(rec.ProjectIDs))
	for _, p := range rec.ProjectIDs {
		pids[p] = struct{}{}
	}
	perms := make(map[Permission]struct{}, len(rec.Permissions))
	for _, p := range rec.Permissions {
		perms[p] = struct{}{}
	}
	return Principal{
		APIKeyID:    rec.ID,
		TenantID:    rec.TenantID,
		ProjectIDs:  pids,
		Permissions: perms,
	}, nil
}

// Authorize 校验 principal 是否可对 projectID 执行 permission。
// 未授权返回 ErrForbidden；跨 project 访问同样映射为 ErrForbidden，
// 避免向调用方泄露 project 是否存在。
func (s *Service) Authorize(p Principal, projectID string, permission Permission) error {
	if !p.CanAccessProject(projectID) {
		return ErrForbidden
	}
	if !p.HasPermission(permission) {
		return ErrForbidden
	}
	return nil
}

// ExtractBearerToken 从标准 "Authorization: Bearer <token>" header 中提取 token。
// 格式错误或缺失返回 ErrMissingCredentials。
func ExtractBearerToken(headerValue string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(headerValue, prefix) {
		return "", ErrMissingCredentials
	}
	token := strings.TrimSpace(strings.TrimPrefix(headerValue, prefix))
	if token == "" {
		return "", ErrMissingCredentials
	}
	return token, nil
}
