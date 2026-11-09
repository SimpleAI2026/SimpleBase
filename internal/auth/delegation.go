// 文件说明：delegation.go — internal/auth 中 DelegationAudience、DelegationType 等符号的实现
//
// 用法
//
//	NewDelegationRegistry() *DelegationRegistry
//	(r *DelegationRegistry) Remember(runID, jti string, exp time.Time)  // Remember 记下一次签发，供 RevokeRun 找到 jti
//	(r *DelegationRegistry) RevokeRun(runID string)  // RevokeRun 作废该 run 签发过的全部 jti
//	同包调用：DelegationType → middleware.go；DelegationRegistry → middleware.go；Revoked() → middleware.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/delegation.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] api_key_repository.go, cache.go, id.go, jwt.go, middleware.go 等 15 个
//	[关键引用] DelegationType ← middleware.go
//
// 主要符号
//
//	const DelegationAudience
//	const DelegationType
//	type DelegationRegistry struct {…}
//	NewDelegationRegistry() *DelegationRegistry
//	(r *DelegationRegistry) Remember(runID, jti string, exp time.Time)
//	(r *DelegationRegistry) RevokeRun(runID string)
//	... 另有 8 个顶层符号

package auth

import (
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

const (
	DelegationAudience = "simplebase-cli"
	DelegationType     = "agent_delegation"
)

// DelegationRegistry 记录本进程签发的委托 jti，run 结束时作废。
type DelegationRegistry struct {
	mu      sync.Mutex
	revoked map[string]time.Time
	byRun   map[string][]delegationToken
}

// delegationToken 记录一次签发的 jti 与其过期时间，作废后的保留时间按 exp 计算。
type delegationToken struct {
	jti string
	exp time.Time
}

func NewDelegationRegistry() *DelegationRegistry {
	return &DelegationRegistry{revoked: map[string]time.Time{}, byRun: map[string][]delegationToken{}}
}

// Remember 记下一次签发，供 RevokeRun 找到 jti；exp 为该令牌的过期时间，
// 作废记录的保留时间据此计算，不再使用固定时长。
func (r *DelegationRegistry) Remember(runID, jti string, exp time.Time) {
	if r == nil || jti == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byRun[runID] = append(r.byRun[runID], delegationToken{jti: jti, exp: exp})
}

// RevokeRun 作废该 run 签发过的全部 jti。每条作废记录保留到 exp 加一分钟
// 时钟偏移余量，过期后由 Revoked 惰性清理。
func (r *DelegationRegistry) RevokeRun(runID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, tok := range r.byRun[runID] {
		retain := tok.exp.Add(time.Minute)
		if tok.exp.IsZero() {
			retain = time.Now().Add(31 * time.Minute)
		}
		if old, ok := r.revoked[tok.jti]; !ok || retain.After(old) {
			r.revoked[tok.jti] = retain
		}
	}
	delete(r.byRun, runID)
}

// Revoked 报告 jti 是否已作废。
func (r *DelegationRegistry) Revoked(jti string, now time.Time) bool {
	if r == nil || jti == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	exp, ok := r.revoked[jti]
	if !ok {
		return false
	}
	if !exp.IsZero() && now.After(exp) {
		delete(r.revoked, jti)
		return false
	}
	return true
}

// HMACDelegationIssuer 用进程内 HMAC 密钥签发委托令牌。
type HMACDelegationIssuer struct {
	Secret string
	Reg    *DelegationRegistry
}

func (d HMACDelegationIssuer) Issue(p Principal, projectID, runID string, ttl time.Duration) (string, error) {
	now := time.Now()
	tok, err := IssueDelegation(d.Secret, p, projectID, runID, ttl, now)
	if err != nil {
		return "", err
	}
	if claims, err := VerifyJWT(d.Secret, tok, now); err == nil {
		d.Reg.Remember(runID, claims.JWTID, time.Unix(claims.ExpiresAt, 0))
	}
	return tok, nil
}

func (d HMACDelegationIssuer) RevokeRun(runID string) {
	if d.Reg != nil {
		d.Reg.RevokeRun(runID)
	}
}

// IssueDelegation 签发短时委托 JWT。权限位是调用方的子集，项目被钉死。
func IssueDelegation(secret string, p Principal, projectID, runID string, ttl time.Duration, now time.Time) (string, error) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if ttl > 30*time.Minute {
		ttl = 30 * time.Minute
	}
	subject := p.UserID
	if subject == "" {
		if p.APIKeyID != "" {
			subject = "apikey:" + p.APIKeyID
		} else {
			subject = "agent"
		}
	}
	perms := make([]string, 0, len(p.Permissions))
	for perm := range p.Permissions {
		perms = append(perms, string(perm))
	}
	claims := JWTClaims{
		Issuer:    jwtIssuer,
		Subject:   subject,
		Username:  p.Username,
		Role:      string(p.Role),
		JWTID:     runID + ":" + now.UTC().Format("20060102150405.000000000"),
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		Audience:  DelegationAudience,
		Type:      DelegationType,
		ProjectID: projectID,
		TenantID:  p.TenantID,
		Perms:     perms,
		APIKeyID:  p.APIKeyID,
		UserID:    p.UserID,
		RunID:     runID,
	}
	return SignJWT(secret, claims)
}

// PrincipalFromDelegation 把委托 claims 还原成 Principal。权限只来自令牌，不升权。
func PrincipalFromDelegation(c JWTClaims) (Principal, error) {
	if c.Type != DelegationType || c.Audience != DelegationAudience || c.ProjectID == "" {
		return Principal{}, ErrInvalidToken
	}
	perms := make(map[Permission]struct{}, len(c.Perms))
	for _, p := range c.Perms {
		perms[Permission(p)] = struct{}{}
	}
	return Principal{
		APIKeyID:    c.APIKeyID,
		UserID:      c.UserID,
		Username:    c.Username,
		Role:        Role(c.Role),
		AccessJTI:   c.JWTID,
		TenantID:    c.TenantID,
		ProjectIDs:  map[string]struct{}{c.ProjectID: {}},
		Permissions: perms,
	}, nil
}

// DelegationMark 返回路由级 delegable 标记中间件（planv6.0 D1）。
// 挂在可委托路由上：置 Echo context 标记，RequireDelegable 据此放行
// Principal.Delegated 主体；未挂标记的路由对委托令牌默认 403。
func DelegationMark() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set(DelegableContextKey, true)
			return next(c)
		}
	}
}
