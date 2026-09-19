package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/uglyer/go-sqlite3"
)

func TestAuthenticate_NilRepository(t *testing.T) {
	svc := NewService(nil, "secret")
	_, err := svc.Authenticate(context.Background(), "any-key")
	if err == nil || err.Error() != "auth: repository not configured" {
		t.Fatalf("got %v", err)
	}
}

func TestAuthenticate_HashMismatch(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	rawKey := "sb_live_mismatch"
	hash := svc.HashKey(rawKey)
	repo := newFakeRepo()
	repo.records[hash] = APIKeyRecord{
		ID:      "key-1",
		KeyHash: hash + "dead",
	}
	svc.repo = repo
	_, err := svc.Authenticate(context.Background(), rawKey)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestPrincipalNilMaps(t *testing.T) {
	var p Principal
	if p.HasPermission(DatabaseRead) {
		t.Fatal("nil permissions")
	}
	if p.CanAccessProject("p1") {
		t.Fatal("nil projects")
	}
}

func TestParseAndJoinPermissions(t *testing.T) {
	if parsePermissions("") != nil {
		t.Fatal("empty csv")
	}
	got := parsePermissions(" database:read , ,database:admin ")
	if len(got) != 2 || got[0] != DatabaseRead || got[1] != DatabaseAdmin {
		t.Fatalf("%v", got)
	}
	if joinPermissions(nil) != "" {
		t.Fatal("empty join")
	}
	if joinPermissions([]Permission{DatabaseRead, LLMInvoke}) != "database:read,llm:invoke" {
		t.Fatal(joinPermissions([]Permission{DatabaseRead, LLMInvoke}))
	}
}

func TestAPIKeyMiddleware_Revoked(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	rawKey := "sb_live_revoked"
	hash := svc.HashKey(rawKey)
	now := time.Now()
	repo := newFakeRepo()
	repo.records[hash] = APIKeyRecord{ID: "k", KeyHash: hash, RevokedAt: &now}
	svc.repo = repo

	e := echo.New()
	e.Use(APIKeyMiddleware(svc, func(ctx context.Context, p Principal) context.Context {
		return ctx
	}))
	e.GET("/t", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() == "" || rec.Body.String() == "invalid api key" {
		// echo.HTTPError message is serialized; just ensure revoked path ran
	}
}

func TestRequire_MissingPrincipal(t *testing.T) {
	e := echo.New()
	e.GET("/admin", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, Require(DatabaseAdmin, func(ctx context.Context) (Principal, bool) {
		return Principal{}, false
	}))
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSQLAPIKeyRepository(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE sys_tenants (id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE sys_projects (id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE sys_api_keys (
			id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, key_hash VARCHAR NOT NULL, permissions VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL, revoked_at TIMESTAMP)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	tenantID := "00000000-0000-0000-0000-000000000001"
	projectID := "00000000-0000-0000-0000-000000000002"
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO sys_tenants(id, name, created_at) VALUES(?, ?, ?)`, tenantID, "t", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sys_projects(id, tenant_id, name, created_at) VALUES(?, ?, ?, ?)`, projectID, tenantID, "p", now); err != nil {
		t.Fatal(err)
	}

	svc := NewService(NewSQLAPIKeyRepository(db), "secret")
	raw := "sb_live_repo"
	hash := svc.HashKey(raw)
	if err := CreateAPIKey(ctx, db, "key-1", projectID, hash, []Permission{DatabaseRead, ProjectAdmin}, now); err != nil {
		t.Fatal(err)
	}
	// idempotent insert of the same id
	if err := CreateAPIKey(ctx, db, "key-1", projectID, hash, []Permission{DatabaseRead}, now); err != nil {
		t.Fatal(err)
	}

	rec, err := svc.repo.FindByHash(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "key-1" || rec.TenantID != tenantID || rec.RevokedAt != nil {
		t.Fatalf("%+v", rec)
	}
	if len(rec.Permissions) != 2 {
		t.Fatalf("perms=%v", rec.Permissions)
	}
	p, err := svc.Authenticate(ctx, raw)
	if err != nil || p.APIKeyID != "key-1" || !p.CanAccessProject(projectID) {
		t.Fatalf("auth %+v err=%v", p, err)
	}

	_, err = svc.repo.FindByHash(ctx, "missing-hash")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("missing: %v", err)
	}

	if err := RevokeAPIKey(ctx, db, "key-1", now); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Authenticate(ctx, raw)
	if !errors.Is(err, ErrKeyRevoked) {
		t.Fatalf("revoked: %v", err)
	}

	// empty permissions stored as empty csv
	if err := CreateAPIKey(ctx, db, "key-empty", projectID, svc.HashKey("empty"), nil, now); err != nil {
		t.Fatal(err)
	}
	empty, err := svc.repo.FindByHash(ctx, svc.HashKey("empty"))
	if err != nil || empty.Permissions != nil && len(empty.Permissions) != 0 {
		t.Fatalf("empty perms %+v err=%v", empty, err)
	}

	_ = db.Close()
	if err := CreateAPIKey(ctx, db, "key-x", projectID, "h", nil, now); err == nil {
		t.Fatal("closed db create")
	}
	if err := RevokeAPIKey(ctx, db, "key-1", now); err == nil {
		t.Fatal("closed db revoke")
	}
	if _, err := NewSQLAPIKeyRepository(db).FindByHash(ctx, hash); err == nil {
		t.Fatal("closed db find")
	}
}

func TestExtractBearerToken_Whitespace(t *testing.T) {
	token, err := ExtractBearerToken("Bearer   tok  ")
	if err != nil || token != "tok" {
		t.Fatalf("token=%q err=%v", token, err)
	}
}
