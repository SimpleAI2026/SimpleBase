// 文件说明：enduser_test.go — 终端用户体系测试（§8.2.6：注册/登录/守卫/密钥域隔离）
package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memEndUserRepo struct {
	mu    sync.Mutex
	items map[string]EndUser
}

func (m *memEndUserRepo) key(projectID, externalID string) string {
	return projectID + "/" + externalID
}

func (m *memEndUserRepo) Create(_ context.Context, u EndUser) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(u.ProjectID, u.ExternalID)
	if _, ok := m.items[k]; ok {
		return ErrEndUserExists
	}
	m.items[k] = u
	return nil
}

func (m *memEndUserRepo) GetByExternalID(_ context.Context, projectID, externalID string) (EndUser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.items[m.key(projectID, externalID)]
	if !ok {
		return EndUser{}, ErrEndUserNotFound
	}
	return u, nil
}

func (m *memEndUserRepo) UpdatePassword(_ context.Context, id string, hash string, updatedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, u := range m.items {
		if u.ID == id {
			u.PasswordHash = hash
			u.UpdatedAt = updatedAt
			m.items[k] = u
			return nil
		}
	}
	return ErrEndUserNotFound
}

func (m *memEndUserRepo) Count(_ context.Context, projectID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, u := range m.items {
		if u.ProjectID == projectID {
			n++
		}
	}
	return n, nil
}

func newEndUserService(t *testing.T) *EndUserService {
	t.Helper()
	return NewEndUserService(&memEndUserRepo{items: map[string]EndUser{}}, "enduser-secret")
}

func TestEndUserSignupLogin(t *testing.T) {
	svc := newEndUserService(t)
	ctx := context.Background()

	u, token, err := svc.Signup(ctx, "proj-1", EndUserSignupInput{
		ExternalID: "alice", Password: "passw0rd-long", DisplayName: "Alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.ProjectID != "proj-1" || u.ExternalID != "alice" {
		t.Fatalf("user %+v", u)
	}
	if token == "" {
		t.Fatal("token must be issued on signup")
	}

	// 重复注册拒绝。
	if _, _, err := svc.Signup(ctx, "proj-1", EndUserSignupInput{ExternalID: "alice", Password: "passw0rd-long"}); !errors.Is(err, ErrEndUserExists) {
		t.Fatalf("duplicate: %v", err)
	}
	// 弱口令拒绝。
	if _, _, err := svc.Signup(ctx, "proj-1", EndUserSignupInput{ExternalID: "bob", Password: "12345678"}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak: %v", err)
	}

	// 登录。
	_, token2, err := svc.Login(ctx, "proj-1", "alice", "passw0rd-long")
	if err != nil || token2 == "" {
		t.Fatalf("login: %v", err)
	}
	// 错口令。
	if _, _, err := svc.Login(ctx, "proj-1", "alice", "wrong"); !errors.Is(err, ErrEndUserBadCredentials) {
		t.Fatalf("bad login: %v", err)
	}
	// 同 external_id 不同项目互不影响。
	if _, _, err := svc.Login(ctx, "proj-2", "alice", "passw0rd-long"); !errors.Is(err, ErrEndUserBadCredentials) {
		t.Fatalf("cross-project: %v", err)
	}

	// token 验证：项目匹配。
	if _, err := VerifyEndUserToken("enduser-secret", token, "proj-1"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// 项目不匹配拒绝。
	if _, err := VerifyEndUserToken("enduser-secret", token, "proj-2"); !errors.Is(err, ErrEndUserTokenInvalid) {
		t.Fatalf("cross project token must be rejected: %v", err)
	}
	// 密钥不对拒绝。
	if _, err := VerifyEndUserToken("other-secret", token, "proj-1"); !errors.Is(err, ErrEndUserTokenInvalid) {
		t.Fatalf("wrong secret must be rejected")
	}
}

func TestEndUserTokenAudienceIsolation(t *testing.T) {
	svc := newEndUserService(t)
	ctx := context.Background()
	_, token, err := svc.Signup(ctx, "p", EndUserSignupInput{ExternalID: "u1", Password: "passw0rd-long"})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := VerifyJWT("enduser-secret", token, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// aud/type 必须带 enduser 标记（平台中间件据此拒绝）。
	if claims.Audience != "enduser" || claims.Type != "enduser" {
		t.Fatalf("claims %+v", claims)
	}
}

func TestEndUserTokenExpiry(t *testing.T) {
	svc := newEndUserService(t)
	// 用未来基准：VerifyJWT 用真实 time.Now()，注入时钟只影响签发侧的 iat/exp。
	base := time.Now().Add(-time.Hour)
	svc.WithClock(func() time.Time { return base })
	_, token, err := svc.Signup(context.Background(), "p", EndUserSignupInput{ExternalID: "exp", Password: "passw0rd-long"})
	if err != nil {
		t.Fatal(err)
	}
	// exp = base+24h，当前时刻在有效期内。
	if _, err := VerifyEndUserToken("enduser-secret", token, "p"); err != nil {
		t.Fatalf("fresh token should verify: %v", err)
	}
	// 签发时刻提前到「现在-25h」：exp 已过 → 拒绝。
	stale := time.Now().Add(-25 * time.Hour)
	svc.WithClock(func() time.Time { return stale })
	_, token2, err := svc.Signup(context.Background(), "p", EndUserSignupInput{ExternalID: "exp2", Password: "passw0rd-long"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyEndUserToken("enduser-secret", token2, "p"); !errors.Is(err, ErrEndUserTokenInvalid) {
		t.Fatalf("expired token must be rejected: %v", err)
	}
}
