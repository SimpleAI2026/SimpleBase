package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

// fakeRepo 是 Repository 的内存假实现。
type fakeRepo struct {
	records map[string]APIKeyRecord // key = keyHash
	findErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{records: make(map[string]APIKeyRecord)}
}

func (r *fakeRepo) FindByHash(ctx context.Context, keyHash string) (APIKeyRecord, error) {
	if r.findErr != nil {
		return APIKeyRecord{}, r.findErr
	}
	rec, ok := r.records[keyHash]
	if !ok {
		return APIKeyRecord{}, errors.New("not found")
	}
	return rec, nil
}

func TestAuthenticate_Success(t *testing.T) {
	svc := NewService(newFakeRepo(), "test-secret")
	rawKey := "sb_live_abc123"
	hash := svc.HashKey(rawKey)
	repo := newFakeRepo()
	repo.records[hash] = APIKeyRecord{
		ID:          "key-1",
		TenantID:    "tenant-1",
		ProjectIDs:  []string{"proj-1"},
		Permissions: []Permission{DatabaseRead, DatabaseAdmin},
		KeyHash:     hash,
	}
	svc.repo = repo

	p, err := svc.Authenticate(context.Background(), rawKey)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if p.APIKeyID != "key-1" {
		t.Errorf("expected APIKeyID key-1, got %s", p.APIKeyID)
	}
	if p.TenantID != "tenant-1" {
		t.Errorf("expected TenantID tenant-1, got %s", p.TenantID)
	}
	if !p.CanAccessProject("proj-1") {
		t.Error("expected CanAccessProject proj-1")
	}
	if !p.HasPermission(DatabaseAdmin) {
		t.Error("expected DatabaseAdmin permission")
	}
	if p.HasPermission(LLMInvoke) {
		t.Error("should not have LLMInvoke")
	}
}

func TestAuthenticate_EmptyKey(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	_, err := svc.Authenticate(context.Background(), "")
	if !errors.Is(err, ErrMissingCredentials) {
		t.Errorf("expected ErrMissingCredentials, got %v", err)
	}
}

func TestAuthenticate_InvalidKey(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	_, err := svc.Authenticate(context.Background(), "nonexistent-key")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthenticate_RevokedKey(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	rawKey := "sb_live_revoked"
	hash := svc.HashKey(rawKey)
	now := time.Now()
	repo := newFakeRepo()
	repo.records[hash] = APIKeyRecord{
		ID:        "key-revoked",
		TenantID:  "tenant-1",
		KeyHash:   hash,
		RevokedAt: &now,
	}
	svc.repo = repo

	_, err := svc.Authenticate(context.Background(), rawKey)
	if !errors.Is(err, ErrKeyRevoked) {
		t.Errorf("expected ErrKeyRevoked, got %v", err)
	}
}

func TestHashKey_Deterministic(t *testing.T) {
	svc := NewService(newFakeRepo(), "fixed-secret")
	h1 := svc.HashKey("mykey")
	h2 := svc.HashKey("mykey")
	if h1 != h2 {
		t.Error("hash should be deterministic")
	}
	// 不同 secret 产生不同 hash
	svc2 := NewService(newFakeRepo(), "different-secret")
	h3 := svc2.HashKey("mykey")
	if h1 == h3 {
		t.Error("different secrets should produce different hashes")
	}
}

func TestHashKey_HexEncoded(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	h := svc.HashKey("key")
	if len(h) != 64 { // SHA-256 = 32 bytes = 64 hex chars
		t.Errorf("expected 64 hex chars, got %d", len(h))
	}
}

func TestAuthorize_Allowed(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	p := Principal{
		TenantID:   "t1",
		ProjectIDs: map[string]struct{}{"p1": {}},
		Permissions: map[Permission]struct{}{
			DatabaseAdmin: {},
		},
	}
	if err := svc.Authorize(p, "p1", DatabaseAdmin); err != nil {
		t.Errorf("expected allowed, got %v", err)
	}
}

func TestAuthorize_WrongProject(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	p := Principal{
		ProjectIDs: map[string]struct{}{"p1": {}},
		Permissions: map[Permission]struct{}{
			DatabaseAdmin: {},
		},
	}
	if err := svc.Authorize(p, "p2", DatabaseAdmin); !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestAuthorize_MissingPermission(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	p := Principal{
		ProjectIDs: map[string]struct{}{"p1": {}},
		Permissions: map[Permission]struct{}{
			DatabaseRead: {},
		},
	}
	if err := svc.Authorize(p, "p1", DatabaseAdmin); !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestExtractBearerToken_Valid(t *testing.T) {
	token, err := ExtractBearerToken("Bearer sb_live_abc123")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if token != "sb_live_abc123" {
		t.Errorf("expected sb_live_abc123, got %s", token)
	}
}

func TestExtractBearerToken_MissingPrefix(t *testing.T) {
	_, err := ExtractBearerToken("sb_live_abc123")
	if !errors.Is(err, ErrMissingCredentials) {
		t.Errorf("expected ErrMissingCredentials, got %v", err)
	}
}

func TestExtractBearerToken_Empty(t *testing.T) {
	_, err := ExtractBearerToken("")
	if !errors.Is(err, ErrMissingCredentials) {
		t.Errorf("expected ErrMissingCredentials, got %v", err)
	}
}

func TestExtractBearerToken_BearerOnly(t *testing.T) {
	_, err := ExtractBearerToken("Bearer ")
	if !errors.Is(err, ErrMissingCredentials) {
		t.Errorf("expected ErrMissingCredentials, got %v", err)
	}
}

// APIKeyMiddleware 集成测试
func TestAPIKeyMiddleware_ValidToken(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	rawKey := "sb_live_test"
	hash := svc.HashKey(rawKey)
	repo := newFakeRepo()
	repo.records[hash] = APIKeyRecord{
		ID:          "key-1",
		TenantID:    "t1",
		ProjectIDs:  []string{"p1"},
		Permissions: []Permission{DatabaseRead},
		KeyHash:     hash,
	}
	svc.repo = repo

	e := echo.New()
	called := false
	e.Use(APIKeyMiddleware(svc, func(ctx context.Context, p Principal) context.Context {
		return context.WithValue(ctx, principalKey{}, p)
	}))
	e.GET("/test", func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Error("handler not called")
	}
}

func TestAPIKeyMiddleware_MissingHeader(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	e := echo.New()
	e.Use(APIKeyMiddleware(svc, func(ctx context.Context, p Principal) context.Context {
		return ctx
	}))
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAPIKeyMiddleware_InvalidToken(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	e := echo.New()
	e.Use(APIKeyMiddleware(svc, func(ctx context.Context, p Principal) context.Context {
		return ctx
	}))
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer bogus")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// Require 中间件测试
func TestRequire_HasPermission(t *testing.T) {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := Principal{Permissions: map[Permission]struct{}{DatabaseAdmin: {}}}
			ctx := context.WithValue(c.Request().Context(), principalKey{}, p)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	called := false
	e.GET("/admin", func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	}, Require(DatabaseAdmin, func(ctx context.Context) (Principal, bool) {
		p, ok := ctx.Value(principalKey{}).(Principal)
		return p, ok
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestRequire_MissingPermission(t *testing.T) {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := Principal{Permissions: map[Permission]struct{}{DatabaseRead: {}}}
			ctx := context.WithValue(c.Request().Context(), principalKey{}, p)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.GET("/admin", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, Require(DatabaseAdmin, func(ctx context.Context) (Principal, bool) {
		p, ok := ctx.Value(principalKey{}).(Principal)
		return p, ok
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

// principalKey 是测试用的 context key
type principalKey struct{}
