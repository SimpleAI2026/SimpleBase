// 文件说明：jwt.go — HS256 JWT 的签发与校验（标准库实现，不引第三方）
//
// 用法
//
//	SignJWT(secret string, claims JWTClaims) (string, error)  // SignJWT 用 secret 签发 HS256 JWT
//	VerifyJWT(secret, token string, now time.Time) (JWTClaims, error)  // VerifyJWT 校验签名、alg、iss、exp，返回 claims
//	同包调用：JWTClaims → delegation.go, middleware.go 等；VerifyJWT() → delegation.go, middleware.go 等；ErrInvalidToken → delegation.go, session.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/jwt.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] api_key_repository.go, cache.go, delegation.go, id.go, middleware.go 等 15 个
//	[关键引用] JWTClaims ← delegation.go, middleware.go, session.go
//
// 主要符号
//
//	var ErrInvalidToken
//	var ErrExpiredToken
//	type JWTClaims struct {…}
//	SignJWT(secret string, claims JWTClaims) (string, error)
//	VerifyJWT(secret, token string, now time.Time) (JWTClaims, error)
//	LooksLikeJWT(raw string) bool
//	... 另有 2 个顶层符号

package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// JWT 相关错误。
var (
	ErrInvalidToken = errors.New("auth: invalid token")
	ErrExpiredToken = errors.New("auth: token expired")
)

// JWTClaims 是 access token 载荷（planv3.0 §4.3.6）。
// 委托令牌额外携带 aud/typ/project_id/perms，普通登录态这些字段为空。
type JWTClaims struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"`
	Username  string   `json:"username"`
	Role      string   `json:"role"`
	SessionID string   `json:"sid"`
	JWTID     string   `json:"jti"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	Audience  string   `json:"aud,omitempty"`
	Type      string   `json:"typ,omitempty"`
	ProjectID string   `json:"project_id,omitempty"`
	TenantID  string   `json:"tenant_id,omitempty"`
	Perms     []string `json:"perms,omitempty"`
	APIKeyID  string   `json:"api_key_id,omitempty"`
	UserID    string   `json:"uid,omitempty"`
	RunID     string   `json:"run_id,omitempty"`
}

const jwtIssuer = "simplebase"

var b64 = base64.RawURLEncoding

// SignJWT 用 secret 签发 HS256 JWT。
func SignJWT(secret string, claims JWTClaims) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("auth: jwt secret is empty")
	}
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	hj, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	pj, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := b64.EncodeToString(hj) + "." + b64.EncodeToString(pj)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unsigned))
	sig := b64.EncodeToString(mac.Sum(nil))
	return unsigned + "." + sig, nil
}

// VerifyJWT 校验签名、alg、iss、exp，返回 claims。
func VerifyJWT(secret, token string, now time.Time) (JWTClaims, error) {
	if secret == "" {
		return JWTClaims{}, fmt.Errorf("auth: jwt secret is empty")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return JWTClaims{}, ErrInvalidToken
	}
	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unsigned))
	want := mac.Sum(nil)
	got, err := b64.DecodeString(parts[2])
	if err != nil {
		return JWTClaims{}, ErrInvalidToken
	}
	if !hmac.Equal(want, got) {
		return JWTClaims{}, ErrInvalidToken
	}
	hj, err := b64.DecodeString(parts[0])
	if err != nil {
		return JWTClaims{}, ErrInvalidToken
	}
	var header map[string]string
	if err := json.Unmarshal(hj, &header); err != nil {
		return JWTClaims{}, ErrInvalidToken
	}
	// 拒绝 alg=none / 非 HS256。
	if header["alg"] != "HS256" {
		return JWTClaims{}, ErrInvalidToken
	}
	pj, err := b64.DecodeString(parts[1])
	if err != nil {
		return JWTClaims{}, ErrInvalidToken
	}
	var claims JWTClaims
	if err := json.Unmarshal(pj, &claims); err != nil {
		return JWTClaims{}, ErrInvalidToken
	}
	if claims.Issuer != jwtIssuer {
		return JWTClaims{}, ErrInvalidToken
	}
	if claims.ExpiresAt > 0 && now.Unix() >= claims.ExpiresAt {
		return JWTClaims{}, ErrExpiredToken
	}
	if claims.Subject == "" {
		return JWTClaims{}, ErrInvalidToken
	}
	return claims, nil
}

// LooksLikeJWT 判断 Bearer token 是否为 JWT 形态（三段 base64url）。
// API Key（如 sb_live_…）不带两个点，走 API Key 通道。
func LooksLikeJWT(raw string) bool {
	return strings.Count(raw, ".") == 2
}
