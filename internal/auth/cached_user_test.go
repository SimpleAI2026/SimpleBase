// cached_user_test.go 验证 planv5.0 §4 P1.3：
// CachedUser 与 Principal 缓存同源同新鲜度——命中复用、
// 禁用/变更即时失效、TTL 过期后回查。
package auth

import (
	"context"
	"testing"
	"time"
)

func TestCachedUserHitAfterPrincipalLoad(t *testing.T) {
	ctx := context.Background()
	repo := newMemUserRepo()
	users := NewUserService(repo, "tenant-1")
	sessions := NewSessionService(users, newMemSessionRepo(), "secret", SessionConfig{})

	u, err := users.Create(ctx, CreateUserInput{Username: "cu-user", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}

	// 未加载前：缓存未命中。
	if _, hit := sessions.CachedUser(u.ID); hit {
		t.Fatal("CachedUser should miss before first PrincipalFromClaims")
	}
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"}); err != nil {
		t.Fatal(err)
	}
	got, hit := sessions.CachedUser(u.ID)
	if !hit {
		t.Fatal("CachedUser should hit after principal load")
	}
	if got.ID != u.ID || got.Username != u.Username || got.Status != UserStatusActive {
		t.Fatalf("cached user mismatch: %+v", got)
	}
}

func TestCachedUserInvalidatedOnChange(t *testing.T) {
	ctx := context.Background()
	repo := newMemUserRepo()
	users := NewUserService(repo, "tenant-1")
	sessions := NewSessionService(users, newMemSessionRepo(), "secret", SessionConfig{})

	u, err := users.Create(ctx, CreateUserInput{Username: "cu-dis", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"}); err != nil {
		t.Fatal(err)
	}
	if _, hit := sessions.CachedUser(u.ID); !hit {
		t.Fatal("expected cache hit before invalidation")
	}

	// Disable 走 OnChange → InvalidateUser：缓存即时清除。
	if _, err := users.Disable(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, hit := sessions.CachedUser(u.ID); hit {
		t.Fatal("CachedUser must be invalidated immediately on user change")
	}

	// 禁用用户不再能通过认证，也不会再进入缓存。
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"}); err == nil {
		t.Fatal("disabled user must be rejected")
	}
	if _, hit := sessions.CachedUser(u.ID); hit {
		t.Fatal("disabled user must not be cached")
	}
}

func TestCachedUserExpiresWithTTL(t *testing.T) {
	ctx := context.Background()
	repo := newMemUserRepo()
	users := NewUserService(repo, "tenant-1")

	now := time.Now()
	clock := now
	sessions := NewSessionService(users, newMemSessionRepo(), "secret", SessionConfig{}).WithClock(func() time.Time { return clock })

	u, err := users.Create(ctx, CreateUserInput{Username: "cu-ttl", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"}); err != nil {
		t.Fatal(err)
	}
	if _, hit := sessions.CachedUser(u.ID); !hit {
		t.Fatal("expected hit within TTL")
	}

	// TTL 过期后：缓存不再命中（调用方回查仓储）。
	clock = now.Add(principalCacheTTL + time.Second)
	if _, hit := sessions.CachedUser(u.ID); hit {
		t.Fatal("CachedUser must miss after TTL expiry")
	}
}

func TestCachedUserReflectsUpdate(t *testing.T) {
	ctx := context.Background()
	repo := newMemUserRepo()
	users := NewUserService(repo, "tenant-1")
	sessions := NewSessionService(users, newMemSessionRepo(), "secret", SessionConfig{})

	u, err := users.Create(ctx, CreateUserInput{Username: "cu-upd", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"}); err != nil {
		t.Fatal(err)
	}

	// 改 display_name：OnChange 失效缓存，重新加载后取到新值。
	newName := "Renamed"
	if _, err := users.Update(ctx, u.ID, UpdateUserInput{DisplayName: &newName}); err != nil {
		t.Fatal(err)
	}
	if _, hit := sessions.CachedUser(u.ID); hit {
		t.Fatal("cache must be invalidated on update")
	}
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j2"}); err != nil {
		t.Fatal(err)
	}
	got, hit := sessions.CachedUser(u.ID)
	if !hit || got.DisplayName != newName {
		t.Fatalf("reload should reflect update, got %+v hit=%v", got, hit)
	}
}

// MarkLogin 更新 last_login_at 后缓存必须失效（TTL 延长后 /auth/me
// 展示的登录时间依赖即时失效而非 TTL 兜底）。
func TestCachedUserInvalidatedOnMarkLogin(t *testing.T) {
	ctx := context.Background()
	repo := newMemUserRepo()
	users := NewUserService(repo, "tenant-1")
	sessions := NewSessionService(users, newMemSessionRepo(), "secret", SessionConfig{})

	u, err := users.Create(ctx, CreateUserInput{Username: "cu-login", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"}); err != nil {
		t.Fatal(err)
	}
	if _, hit := sessions.CachedUser(u.ID); !hit {
		t.Fatal("expected hit before MarkLogin")
	}
	if err := users.MarkLogin(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, hit := sessions.CachedUser(u.ID); hit {
		t.Fatal("cache must be invalidated after MarkLogin")
	}
	// 重新加载后能取到新的 last_login_at。
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j2"}); err != nil {
		t.Fatal(err)
	}
	got, hit := sessions.CachedUser(u.ID)
	if !hit || got.LastLoginAt == nil {
		t.Fatalf("reload should expose last_login_at, got %+v hit=%v", got, hit)
	}
}
