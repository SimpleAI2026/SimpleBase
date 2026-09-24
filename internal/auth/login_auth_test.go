package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// memUserRepo 是 UserRepository 内存假实现。
type memUserRepo struct {
	mu       sync.Mutex
	byID     map[string]User
	byName   map[string]string // username → id
	owners   map[string]string // project → user
	byUser   map[string][]string
	allProjs []string
}

func newMemUserRepo() *memUserRepo {
	return &memUserRepo{
		byID:   map[string]User{},
		byName: map[string]string{},
		owners: map[string]string{},
		byUser: map[string][]string{},
	}
}

func (m *memUserRepo) Create(_ context.Context, u User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byName[u.Username]; ok {
		return ErrUsernameTaken
	}
	m.byID[u.ID] = u
	m.byName[u.Username] = u.ID
	return nil
}

func (m *memUserRepo) GetByID(_ context.Context, id string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return u, nil
}

func (m *memUserRepo) GetByUsername(_ context.Context, username string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byName[username]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return m.byID[id], nil
}

func (m *memUserRepo) List(_ context.Context, limit int, _ string) ([]User, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []User
	for _, u := range m.byID {
		out = append(out, u)
		if len(out) >= limit {
			break
		}
	}
	return out, "", nil
}

func (m *memUserRepo) Update(_ context.Context, u User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[u.ID]; !ok {
		return ErrUserNotFound
	}
	m.byID[u.ID] = u
	return nil
}

func (m *memUserRepo) CountByUsername(_ context.Context, username string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byName[username]; ok {
		return 1, nil
	}
	return 0, nil
}

func (m *memUserRepo) SetProjectOwner(_ context.Context, projectID, userID string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.owners[projectID] = userID
	m.byUser[userID] = append(m.byUser[userID], projectID)
	return nil
}

func (m *memUserRepo) GetProjectOwner(_ context.Context, projectID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.owners[projectID]; ok {
		return id, nil
	}
	return "", ErrUserNotFound
}

func (m *memUserRepo) ListProjectIDsByUser(_ context.Context, userID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.byUser[userID]...), nil
}

func (m *memUserRepo) ListAllProjectIDs(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.allProjs) > 0 {
		return append([]string(nil), m.allProjs...), nil
	}
	var out []string
	for p := range m.owners {
		out = append(out, p)
	}
	return out, nil
}

// memSessionRepo 是 SessionRepository 内存假实现。
type memSessionRepo struct {
	mu       sync.Mutex
	byID     map[string]Session
	byHash   map[string]string
	byUser   map[string][]string
	revoked  int
}

func newMemSessionRepo() *memSessionRepo {
	return &memSessionRepo{byID: map[string]Session{}, byHash: map[string]string{}, byUser: map[string][]string{}}
}

func (m *memSessionRepo) Create(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[s.ID] = s
	m.byHash[s.RefreshTokenHash] = s.ID
	m.byUser[s.UserID] = append(m.byUser[s.UserID], s.ID)
	return nil
}

func (m *memSessionRepo) GetByRefreshHash(_ context.Context, hash string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byHash[hash]
	if !ok {
		return Session{}, ErrInvalidRefreshToken
	}
	return m.byID[id], nil
}

func (m *memSessionRepo) GetByID(_ context.Context, id string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[id]
	if !ok {
		return Session{}, ErrInvalidRefreshToken
	}
	return s, nil
}

func (m *memSessionRepo) Update(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[s.ID] = s
	return nil
}

func (m *memSessionRepo) RevokeByUser(_ context.Context, userID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range m.byUser[userID] {
		s := m.byID[id]
		if s.RevokedAt == nil {
			s.RevokedAt = &at
			m.byID[id] = s
			m.revoked++
		}
	}
	return nil
}

func (m *memSessionRepo) RevokeByIDs(_ context.Context, ids []string, at time.Time) error {
	for _, id := range ids {
		s, err := m.GetByID(context.Background(), id)
		if err != nil {
			continue
		}
		s.RevokedAt = &at
		_ = m.Update(context.Background(), s)
	}
	return nil
}

func TestPasswordHashVerify(t *testing.T) {
	h, err := HashPassword("simplebase2026")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("simplebase2026", h) {
		t.Error("expected verify ok")
	}
	if VerifyPassword("wrong", h) {
		t.Error("expected verify fail")
	}
	if VerifyPassword("x", "not-a-hash") {
		t.Error("expected bad format fail")
	}
}

func TestJWTRoundTrip(t *testing.T) {
	now := time.Unix(1760000000, 0)
	tok, err := SignJWT("secret", JWTClaims{
		Issuer: jwtIssuer, Subject: "u1", Username: "alice", Role: "user",
		SessionID: "s1", JWTID: "j1", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !LooksLikeJWT(tok) {
		t.Error("expected LooksLikeJWT")
	}
	if LooksLikeJWT("sb_live_key") {
		t.Error("api key should not look like jwt")
	}
	claims, err := VerifyJWT("secret", tok, now)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "u1" || claims.Role != "user" {
		t.Errorf("claims %+v", claims)
	}
	// 过期
	if _, err := VerifyJWT("secret", tok, now.Add(2*time.Hour)); !errors.Is(err, ErrExpiredToken) {
		t.Errorf("expected expired, got %v", err)
	}
	// 错误密钥
	if _, err := VerifyJWT("other", tok, now); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected invalid, got %v", err)
	}
	// alg=none 拒绝
	none := b64.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." +
		b64.EncodeToString([]byte(`{"iss":"simplebase","sub":"u1","exp":9999999999}`)) + "."
	if _, err := VerifyJWT("secret", none, now); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected reject alg=none, got %v", err)
	}
}

func TestRolePermissions(t *testing.T) {
	super := PermissionsForRole(RoleSuperAdmin)
	if _, ok := super[UserAdmin]; !ok {
		t.Error("super should have user:admin")
	}
	admin := PermissionsForRole(RoleAdmin)
	if _, ok := admin[DatabaseWrite]; ok {
		t.Error("admin must not have write")
	}
	if _, ok := admin[DatabaseRead]; !ok {
		t.Error("admin should have read")
	}
	user := PermissionsForRole(RoleUser)
	if _, ok := user[UserAdmin]; ok {
		t.Error("user must not have user:admin")
	}
}

func TestUserServiceLoginAndPrincipal(t *testing.T) {
	repo := newMemUserRepo()
	users := NewUserService(repo, "tenant-1")
	ctx := context.Background()

	boot, created, err := users.EnsureBootstrapUser(ctx)
	if err != nil || !created {
		t.Fatalf("bootstrap: created=%v err=%v", created, err)
	}
	if boot.Username != BootstrapUsername || boot.Role != RoleSuperAdmin {
		t.Fatalf("bootstrap user %+v", boot)
	}
	// 幂等
	_, created2, err := users.EnsureBootstrapUser(ctx)
	if err != nil || created2 {
		t.Fatalf("bootstrap twice: created=%v err=%v", created2, err)
	}

	u, err := users.VerifyCredentials(ctx, BootstrapUsername, BootstrapPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.VerifyCredentials(ctx, BootstrapUsername, "bad"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("bad pass: %v", err)
	}

	// Create 拒绝 super
	if _, err := users.Create(ctx, CreateUserInput{Username: "x", Password: "12345678", Role: RoleSuperAdmin}); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("create super: %v", err)
	}
	// Create user
	alice, err := users.Create(ctx, CreateUserInput{Username: "Alice", Password: "12345678", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	if alice.Username != "alice" {
		t.Errorf("username should normalize, got %s", alice.Username)
	}
	// 项目归属
	if err := users.AssignProjectOwner(ctx, "proj-1", alice.ID); err != nil {
		t.Fatal(err)
	}
	p, err := users.PrincipalFromUser(ctx, alice, "sid")
	if err != nil {
		t.Fatal(err)
	}
	if p.CanAccessProject("proj-1") {
		// ok
	} else {
		t.Error("alice should access proj-1")
	}
	if p.CanAccessProject(AdminProjectID) {
		t.Error("user must not access admin project")
	}
	if p.HasPermission(UserAdmin) {
		t.Error("user must not have user:admin")
	}

	// super 展开含 admin 项目
	sp, err := users.PrincipalFromUser(ctx, u, "sid")
	if err != nil {
		t.Fatal(err)
	}
	if !sp.CanAccessProject(AdminProjectID) {
		t.Error("super should access admin project")
	}
	if !sp.HasPermission(UserAdmin) {
		t.Error("super should have user:admin")
	}
}

func TestSessionLoginRefreshRevoke(t *testing.T) {
	repo := newMemUserRepo()
	users := NewUserService(repo, "tenant-1")
	srepo := newMemSessionRepo()
	sessions := NewSessionService(users, srepo, "secret", SessionConfig{
		AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour,
	})
	ctx := context.Background()
	_, _, _ = users.EnsureBootstrapUser(ctx)

	u, pair, err := sessions.Login(ctx, BootstrapUsername, BootstrapPassword, "ua", "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("empty tokens")
	}
	claims, err := sessions.VerifyAccess(pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != u.ID {
		t.Errorf("sub %s want %s", claims.Subject, u.ID)
	}
	// refresh 轮转
	_, pair2, err := sessions.Refresh(ctx, pair.RefreshToken, "ua", "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if pair2.RefreshToken == pair.RefreshToken {
		t.Error("refresh should rotate")
	}
	// 旧 refresh 重放 → session revoked + 全部吊销
	if _, _, err := sessions.Refresh(ctx, pair.RefreshToken, "ua", "1.1.1.1"); !errors.Is(err, ErrSessionRevoked) {
		t.Errorf("replay: %v", err)
	}
	// 全部吊销后新 refresh 也失效
	if _, _, err := sessions.Refresh(ctx, pair2.RefreshToken, "ua", "1.1.1.1"); err == nil {
		t.Error("expected revoked session fail")
	}
}

func TestMustChangePassword(t *testing.T) {
	repo := newMemUserRepo()
	users := NewUserService(repo, "t")
	sessions := NewSessionService(users, newMemSessionRepo(), "secret", SessionConfig{})
	ctx := context.Background()
	flag := true
	u, err := users.Create(ctx, CreateUserInput{Username: "bob", Password: "12345678", Role: RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.Update(ctx, u.ID, UpdateUserInput{MustChangePassword: &flag}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sessions.Login(ctx, "bob", "12345678", "", ""); !errors.Is(err, ErrMustChangePasswd) {
		t.Errorf("expected must change, got %v", err)
	}
	// 改密后可登录
	if err := users.ChangePassword(ctx, u.ID, "12345678", "abcdefgh"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sessions.Login(ctx, "bob", "abcdefgh", "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapUserProtected(t *testing.T) {
	repo := newMemUserRepo()
	users := NewUserService(repo, "t")
	ctx := context.Background()
	boot, _, err := users.EnsureBootstrapUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st := UserStatusDisabled
	if _, err := users.Update(ctx, boot.ID, UpdateUserInput{Status: &st}); !errors.Is(err, ErrUserProtected) {
		t.Errorf("disable super: %v", err)
	}
	r := RoleUser
	if _, err := users.Update(ctx, boot.ID, UpdateUserInput{Role: &r}); !errors.Is(err, ErrUserProtected) {
		t.Errorf("demote super: %v", err)
	}
}
