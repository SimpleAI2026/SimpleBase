// session.go 实现 OAuth2 Password Grant 的会话签发与 JWT 校验（planv3.0 §4.3）。
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// 会话错误。
var (
	ErrInvalidRefreshToken = errors.New("auth: invalid refresh token")
	ErrSessionRevoked      = errors.New("auth: session revoked")
)

// Session 是 refresh token 会话（sys_user_sessions）。
type Session struct {
	ID               string
	UserID           string
	RefreshTokenHash string
	AccessJTI        string
	UserAgent        string
	IP               string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	RevokedAt        *time.Time
}

// SessionRepository 抽象 sys_user_sessions。
type SessionRepository interface {
	Create(ctx context.Context, s Session) error
	GetByRefreshHash(ctx context.Context, hash string) (Session, error)
	GetByID(ctx context.Context, id string) (Session, error)
	Update(ctx context.Context, s Session) error
	RevokeByUser(ctx context.Context, userID string, at time.Time) error
	RevokeByIDs(ctx context.Context, ids []string, at time.Time) error
}

// TokenPair 是登录/刷新响应中的令牌。
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	SessionID    string
	AccessJTI    string
}

// SessionConfig 会话 TTL。
type SessionConfig struct {
	AccessTTL  time.Duration // 默认 2h
	RefreshTTL time.Duration // 默认 168h
}

// SessionService 签发/校验 access JWT 与 refresh token。
type SessionService struct {
	users   *UserService
	repo    SessionRepository
	secret  string
	cfg     SessionConfig
	now     func() time.Time
}

// NewSessionService 构造 SessionService。secret 与 API Key HMAC 盐共用。
func NewSessionService(users *UserService, repo SessionRepository, secret string, cfg SessionConfig) *SessionService {
	if cfg.AccessTTL <= 0 {
		cfg.AccessTTL = 2 * time.Hour
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = 7 * 24 * time.Hour
	}
	return &SessionService{
		users:  users,
		repo:   repo,
		secret: secret,
		cfg:    cfg,
		now:    time.Now,
	}
}

// WithClock 注入时钟（测试）。
func (s *SessionService) WithClock(now func() time.Time) *SessionService {
	s.now = now
	return s
}

// HashRefresh 计算 refresh token 摘要（绝不存原文）。
func HashRefresh(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "rt_" + hex.EncodeToString(b), nil
}

// Login 校验口令并签发 TokenPair。must_change_password 时返回 ErrMustChangePasswd
// （handler 可单独回 423 + user，不发正式 token）。
func (s *SessionService) Login(ctx context.Context, username, password, userAgent, ip string) (User, TokenPair, error) {
	u, err := s.users.VerifyCredentials(ctx, username, password)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	if u.MustChangePassword {
		return u, TokenPair{}, ErrMustChangePasswd
	}
	pair, err := s.issue(ctx, u, userAgent, ip)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	_ = s.users.MarkLogin(ctx, u.ID)
	return u, pair, nil
}

// IssueAfterPasswordChange 用于强制改密后补发 token。
func (s *SessionService) IssueAfterPasswordChange(ctx context.Context, u User, userAgent, ip string) (TokenPair, error) {
	return s.issue(ctx, u, userAgent, ip)
}

func (s *SessionService) issue(ctx context.Context, u User, userAgent, ip string) (TokenPair, error) {
	sid := newID()
	jti := newID()
	now := s.now().UTC()
	rt, err := newRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}
	sess := Session{
		ID:               sid,
		UserID:           u.ID,
		RefreshTokenHash: HashRefresh(rt),
		AccessJTI:        jti,
		UserAgent:        userAgent,
		IP:               ip,
		ExpiresAt:        now.Add(s.cfg.RefreshTTL),
		CreatedAt:        now,
	}
	if err := s.repo.Create(ctx, sess); err != nil {
		return TokenPair{}, err
	}
	access, err := s.signAccess(u, sid, jti, now)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:  access,
		RefreshToken: rt,
		ExpiresIn:    int64(s.cfg.AccessTTL.Seconds()),
		SessionID:    sid,
		AccessJTI:    jti,
	}, nil
}

func (s *SessionService) signAccess(u User, sid, jti string, now time.Time) (string, error) {
	return SignJWT(s.secret, JWTClaims{
		Issuer:    jwtIssuer,
		Subject:   u.ID,
		Username:  u.Username,
		Role:      string(u.Role),
		SessionID: sid,
		JWTID:     jti,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(s.cfg.AccessTTL).Unix(),
	})
}

// Refresh 轮转 refresh token：旧的作废，签发新的一对。
// 使用已撤销 refresh 重放 → 吊销该用户全部会话。
func (s *SessionService) Refresh(ctx context.Context, refreshToken, userAgent, ip string) (User, TokenPair, error) {
	if refreshToken == "" {
		return User{}, TokenPair{}, ErrInvalidRefreshToken
	}
	hash := HashRefresh(refreshToken)
	sess, err := s.repo.GetByRefreshHash(ctx, hash)
	if err != nil {
		return User{}, TokenPair{}, ErrInvalidRefreshToken
	}
	now := s.now().UTC()
	if sess.RevokedAt != nil {
		// 重放检测：吊销该用户全部会话。
		_ = s.repo.RevokeByUser(ctx, sess.UserID, now)
		return User{}, TokenPair{}, ErrSessionRevoked
	}
	if now.After(sess.ExpiresAt) {
		return User{}, TokenPair{}, ErrInvalidRefreshToken
	}
	u, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return User{}, TokenPair{}, ErrInvalidRefreshToken
	}
	if u.Status != UserStatusActive {
		return User{}, TokenPair{}, ErrUserDisabled
	}

	// 轮转：旧会话作废，发新会话。
	rev := now
	sess.RevokedAt = &rev
	if err := s.repo.Update(ctx, sess); err != nil {
		return User{}, TokenPair{}, err
	}
	pair, err := s.issue(ctx, u, userAgent, ip)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	return u, pair, nil
}

// Logout 吊销当前会话；refreshToken 为空时按 sessionID 吊销。
func (s *SessionService) Logout(ctx context.Context, sessionID, refreshToken string) error {
	now := s.now().UTC()
	if refreshToken != "" {
		sess, err := s.repo.GetByRefreshHash(ctx, HashRefresh(refreshToken))
		if err != nil {
			return nil
		}
		sess.RevokedAt = &now
		return s.repo.Update(ctx, sess)
	}
	if sessionID == "" {
		return nil
	}
	sess, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return nil
	}
	sess.RevokedAt = &now
	return s.repo.Update(ctx, sess)
}

// RevokeAllForUser 吊销用户全部会话（禁用/删用户/改密）。
func (s *SessionService) RevokeAllForUser(ctx context.Context, userID string) error {
	return s.repo.RevokeByUser(ctx, userID, s.now().UTC())
}

// VerifyAccess 校验 access JWT（签名+exp），不查库。
// 用户 status 校验由 PrincipalFromClaims 负责（每次请求查 sys_users）。
func (s *SessionService) VerifyAccess(token string) (JWTClaims, error) {
	return VerifyJWT(s.secret, token, s.now().UTC())
}

// PrincipalFromClaims 载入用户并展开 Principal；disabled/缺失返回错误。
func (s *SessionService) PrincipalFromClaims(ctx context.Context, claims JWTClaims) (Principal, error) {
	u, err := s.users.GetByID(ctx, claims.Subject)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	if u.Status != UserStatusActive {
		return Principal{}, ErrUserDisabled
	}
	p, err := s.users.PrincipalFromUser(ctx, u, claims.SessionID)
	if err != nil {
		return Principal{}, err
	}
	p.AccessJTI = claims.JWTID
	return p, nil
}

// TouchSession 更新会话当前 access jti（审计用，可选）。
func (s *SessionService) TouchSession(ctx context.Context, sessionID, jti string) error {
	if sessionID == "" {
		return nil
	}
	sess, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	sess.AccessJTI = jti
	return s.repo.Update(ctx, sess)
}
