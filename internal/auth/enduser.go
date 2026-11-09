// 文件说明：enduser.go — 终端用户体系（production-readiness-plan §8.2.6）
//
// 用法
//
//	NewEndUserService(repo EndUserRepository, secret string) *EndUserService
//	(s *EndUserService) Signup(ctx, projectID, SignupInput) (EndUser, string, error)
//	(s *EndUserService) Login(ctx, projectID, username, password) (EndUser, string, error)
//	auth.VerifyEndUserToken(secret, token, wantProjectID) (EndUserClaims, error)
//
// 终端用户与管理员/平台会话完全隔离：独立表 sys_end_users、独立 JWT 域
// （HS256 secret 相同但 audience=enduser，验签侧强制 audience 匹配）、
// 独立路由 /v1/projects/:projectID/end-users/*。
package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

// 终端用户领域错误。
var (
	ErrEndUserExists       = errors.New("auth: end user already exists")
	ErrEndUserNotFound     = errors.New("auth: end user not found")
	ErrEndUserDisabled     = errors.New("auth: end user disabled")
	ErrEndUserBadCredentials = errors.New("auth: invalid end user credentials")
	ErrEndUserTokenInvalid = errors.New("auth: invalid end user token")
)

// EndUser 是 sys_end_users 一行。
type EndUser struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	ExternalID   string    `json:"external_id"` // 项目内唯一标识（用户名/手机号/邮箱等）
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name"`
	Email        string    `json:"email,omitempty"`
	Status       string    `json:"status"` // active | disabled
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// EndUserSignupInput 是注册入参（§8.2.6：username+password 模式）。
type EndUserSignupInput struct {
	ExternalID  string
	Password    string
	DisplayName string
	Email       string
}

// EndUserRepository 是终端用户仓储接口（systemdb 提供 SQL 实现）。
type EndUserRepository interface {
	Create(ctx context.Context, u EndUser) error
	GetByExternalID(ctx context.Context, projectID, externalID string) (EndUser, error)
	UpdatePassword(ctx context.Context, id string, hash string, updatedAt time.Time) error
	Count(ctx context.Context, projectID string) (int64, error)
}

// EndUserService 是终端用户领域服务。
type EndUserService struct {
	repo   EndUserRepository
	secret string // 项目级终端用户 JWT 密钥（与平台密钥同源；aud 隔离）
	now    func() time.Time
}

// EndUserTokenTTL 是终端用户 JWT 有效期（§8.2.6：短寿命、无 refresh）。
const EndUserTokenTTL = 24 * time.Hour

// NewEndUserService 构造终端用户服务。secret 用于签发 audience=enduser 的 JWT。
func NewEndUserService(repo EndUserRepository, secret string) *EndUserService {
	return &EndUserService{repo: repo, secret: secret, now: time.Now}
}

// WithClock 注入时钟（测试）。
func (s *EndUserService) WithClock(now func() time.Time) *EndUserService {
	s.now = now
	return s
}

// Signup 注册终端用户（用户名+口令；§8.2.6 表格 signup 模式）。
// 返回创建的用户与已签发的终端用户 JWT。
func (s *EndUserService) Signup(ctx context.Context, projectID string, in EndUserSignupInput) (EndUser, string, error) {
	in.ExternalID = strings.TrimSpace(in.ExternalID)
	if in.ExternalID == "" || len(in.Password) < MinPasswordLength {
		return EndUser{}, "", ErrEndUserBadCredentials
	}
	if _, hit := weakPasswords[strings.ToLower(in.Password)]; hit {
		return EndUser{}, "", ErrWeakPassword
	}
	if _, err := s.repo.GetByExternalID(ctx, projectID, in.ExternalID); err == nil {
		return EndUser{}, "", ErrEndUserExists
	} else if !errors.Is(err, ErrEndUserNotFound) {
		return EndUser{}, "", err
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return EndUser{}, "", err
	}
	now := s.now().UTC()
	u := EndUser{
		ID:           newID(),
		ProjectID:    projectID,
		ExternalID:   in.ExternalID,
		PasswordHash: hash,
		DisplayName:  strings.TrimSpace(in.DisplayName),
		Email:        strings.TrimSpace(in.Email),
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return EndUser{}, "", err
	}
	token, err := s.issueToken(u)
	if err != nil {
		return EndUser{}, "", err
	}
	return u, token, nil
}

// Login 终端用户登录：验口令 → 签发 audience=enduser 的 JWT。
func (s *EndUserService) Login(ctx context.Context, projectID, externalID, password string) (EndUser, string, error) {
	externalID = strings.TrimSpace(externalID)
	u, err := s.repo.GetByExternalID(ctx, projectID, externalID)
	if err != nil {
		if errors.Is(err, ErrEndUserNotFound) {
			// 仍做一次假校验，消除时序侧信道。
			_ = VerifyPassword(password, "$argon2id$v=19$m=65536,t=3,p=1$c2FsdA$hashhashhashhashhashhashhash")
			return EndUser{}, "", ErrEndUserBadCredentials
		}
		return EndUser{}, "", err
	}
	if u.Status != "active" {
		return EndUser{}, "", ErrEndUserDisabled
	}
	if !VerifyPassword(password, u.PasswordHash) {
		return EndUser{}, "", ErrEndUserBadCredentials
	}
	token, err := s.issueToken(u)
	if err != nil {
		return EndUser{}, "", err
	}
	return u, token, nil
}

// endUserAudience 是终端用户 JWT 的 audience 标记（§8.2.6：验签侧强制匹配，
// 防止终端用户 token 被拿去平台会话域使用——平台中间件对 aud=enduser 直接拒绝）。
const endUserAudience = "enduser"

func (s *EndUserService) issueToken(u EndUser) (string, error) {
	now := s.now()
	claims := JWTClaims{
		Issuer:    jwtIssuer,
		Subject:   u.ID,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(EndUserTokenTTL).Unix(),
		Audience:  endUserAudience,
		Type:      "enduser",
		ProjectID: u.ProjectID,
	}
	return SignJWT(s.secret, claims)
}

// VerifyEndUserToken 验证终端用户 JWT：aud/type 必须为 enduser、未过期、项目匹配。
func VerifyEndUserToken(secret, token, wantProjectID string) (JWTClaims, error) {
	claims, err := VerifyJWT(secret, token, time.Now())
	if err != nil {
		return claims, ErrEndUserTokenInvalid
	}
	if claims.Audience != endUserAudience || claims.Type != "enduser" {
		return claims, ErrEndUserTokenInvalid
	}
	if wantProjectID != "" && claims.ProjectID != wantProjectID {
		return claims, ErrEndUserTokenInvalid
	}
	return claims, nil
}
