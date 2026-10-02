package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/uglyer/go-sqlite3"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// newAPIKeyTestDB 构造带 sys_api_keys / sys_projects 的内存库。
func newAPIKeyTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE sys_projects (id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE sys_api_keys (
			id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, key_hash VARCHAR NOT NULL, permissions VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL, revoked_at TIMESTAMP)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO sys_projects(id, tenant_id, name, created_at) VALUES(?, ?, ?, ?)`,
		"proj-1", "tenant-1", "p", now); err != nil {
		t.Fatal(err)
	}
	return db
}

// setupAPIKeyRouter 构造挂了 api-keys 路由的测试 Echo；principal 决定注入身份。
func setupAPIKeyRouter(t *testing.T, principal auth.Principal) (*echo.Echo, *sql.DB, *auth.Service) {
	t.Helper()
	db := newAPIKeyTestDB(t)
	svc := auth.NewService(auth.NewSQLAPIKeyRepository(db), "test-secret")
	e := echo.New()
	e.HideBanner = true
	h := NewAPIKeysHandler(svc, db)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithPrincipal(c.Request().Context(), principal)
			ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	p := e.Group("/v1/projects/:projectID")
	p.POST("/api-keys", h.Create)
	p.GET("/api-keys", h.List)
	p.DELETE("/api-keys/:keyID", h.Delete)
	return e, db, svc
}

func superPrincipal() auth.Principal {
	return auth.Principal{
		UserID:      "u-1",
		Role:        auth.RoleSuperAdmin,
		TenantID:    "tenant-1",
		ProjectIDs:  map[string]struct{}{"proj-1": {}},
		Permissions: auth.PermissionsForRole(auth.RoleSuperAdmin),
	}
}

func adminRolePrincipal() auth.Principal {
	p := superPrincipal()
	p.Role = auth.RoleAdmin
	p.Permissions = auth.PermissionsForRole(auth.RoleAdmin)
	return p
}

func TestAPIKeys_CreateAndAuthenticate(t *testing.T) {
	e, db, svc := setupAPIKeyRouter(t, superPrincipal())
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys", map[string]any{"permissions": []string{"database:read", "database:write"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ID      string   `json:"id"`
		Secret  string   `json:"secret"`
		Perms   []string `json:"permissions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.Secret, "sb_live_") || resp.ID == "" {
		t.Fatalf("secret=%q id=%q", resp.Secret, resp.ID)
	}
	if len(resp.Perms) != 2 {
		t.Fatalf("perms=%v", resp.Perms)
	}
	// 新 Key 能通过真实认证链路。
	p, err := svc.Authenticate(context.Background(), resp.Secret)
	if err != nil || p.APIKeyID != resp.ID || !p.CanAccessProject("proj-1") {
		t.Fatalf("auth=%+v err=%v", p, err)
	}
	if !p.HasPermission(auth.DatabaseWrite) || p.HasPermission(auth.DatabaseAdmin) {
		t.Fatalf("unexpected perms")
	}
	_ = db
}

func TestAPIKeys_CreateDefaultFullPermissions(t *testing.T) {
	e, _, _ := setupAPIKeyRouter(t, superPrincipal())
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys", map[string]any{})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Permissions []string `json:"permissions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Permissions) != 5 {
		t.Fatalf("default perms=%v", resp.Permissions)
	}
}

func TestAPIKeys_CreateInvalidPermission(t *testing.T) {
	e, _, _ := setupAPIKeyRouter(t, superPrincipal())
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys", map[string]any{"permissions": []string{"root:all"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAPIKeys_AdminRoleForbidden(t *testing.T) {
	e, _, _ := setupAPIKeyRouter(t, adminRolePrincipal())
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys", map[string]any{})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestAPIKeys_ListNoSecret(t *testing.T) {
	e, _, _ := setupAPIKeyRouter(t, superPrincipal())
	if rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys", map[string]any{}); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/api-keys", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "sb_live_") {
		t.Fatalf("list leaked secret: %s", rec.Body.String())
	}
	var resp struct {
		Keys []struct {
			ID string `json:"id"`
		} `json:"keys"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Keys) != 1 || resp.Keys[0].ID == "" {
		t.Fatalf("keys=%+v", resp.Keys)
	}
}

func TestAPIKeys_RevokeInvalidatesAuth(t *testing.T) {
	e, db, svc := setupAPIKeyRouter(t, superPrincipal())
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys", map[string]any{})
	var created struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if _, err := svc.Authenticate(context.Background(), created.Secret); err != nil {
		t.Fatalf("pre-revoke: %v", err)
	}
	del := doRequest(e, http.MethodDelete, "/v1/projects/proj-1/api-keys/"+created.ID, nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", del.Code, del.Body.String())
	}
	if _, err := svc.Authenticate(context.Background(), created.Secret); err == nil {
		t.Fatal("revoked key still authenticates")
	}
	_ = db
}

func TestAPIKeys_CrossProjectDenied(t *testing.T) {
	p := superPrincipal()
	p.ProjectIDs = map[string]struct{}{"other": {}}
	e, _, _ := setupAPIKeyRouter(t, p)
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys", map[string]any{})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestGenerateAPIKeySecret_Format(t *testing.T) {
	s1, err := generateAPIKeySecret()
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := generateAPIKeySecret()
	if !strings.HasPrefix(s1, "sb_live_") || len(s1) != len("sb_live_")+32 {
		t.Fatalf("s1=%q", s1)
	}
	if s1 == s2 {
		t.Fatal("secrets must differ")
	}
}
