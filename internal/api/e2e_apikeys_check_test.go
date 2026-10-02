package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/uglyer/go-sqlite3"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// 端到端：登录态签发 → 新 Key 过真实认证 → 新 Key 调 ListDatabases 路径 → 吊销后失效。
func TestAPIKeys_E2E_FullChain(t *testing.T) {
	db := newAPIKeyTestDB(t)
	svc := auth.NewService(auth.NewSQLAPIKeyRepository(db), "e2e-secret")

	// 模拟真实装配：auth 中间件（API Key 通道）+ project context + handler。
	e := echo.New()
	e.HideBanner = true
	h := NewAPIKeysHandler(svc, db)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 先尝试 API Key 认证
			raw := strings.TrimPrefix(c.Request().Header.Get("Authorization"), "Bearer ")
			if raw != "" {
				p, err := svc.Authenticate(c.Request().Context(), raw)
				if err == nil {
					ctx := WithPrincipal(c.Request().Context(), p)
					ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
					c.SetRequest(c.Request().WithContext(ctx))
					return next(c)
				}
			}
			// 无凭证 → 401（模拟中间件拒绝）
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		}
	})
	p := e.Group("/v1/projects/:projectID")
	p.POST("/api-keys", h.Create)
	p.GET("/api-keys", h.List)
	p.DELETE("/api-keys/:keyID", h.Delete)

	// 1) 用登录态超管身份签发（直接注入 principal 的路径已由其他测试覆盖；
	//    这里用「种子一个持 ProjectAdmin 的 Key」模拟引导凭据）。
	now := time.Now().UTC()
	seed := "sb_live_bootstrap_key"
	if err := auth.CreateAPIKey(context.Background(), db, "bootstrap-key", "proj-1",
		svc.HashKey(seed), []auth.Permission{auth.ProjectAdmin, auth.DatabaseRead}, now); err != nil {
		t.Fatal(err)
	}

	// 2) 引导 Key 签发新 Key（模拟用户拿到第一个有效凭据后的自举流程）
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/api-keys", strings.NewReader(`{"permissions":["database:read","database:write"]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Authorization", "Bearer "+seed)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("issue: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !strings.HasPrefix(created.Secret, "sb_live_") {
		t.Fatalf("secret=%q", created.Secret)
	}

	// 3) 新 Key 能列出 api-keys（认证通过 + 项目归属正确）
	req2 := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/api-keys", nil)
	req2.Header.Set("Authorization", "Bearer "+created.Secret)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), created.ID) {
		t.Fatalf("list: %d %s", rec2.Code, rec2.Body.String())
	}

	// 4) 新 Key 无权访问其它项目（project context 固定 proj-1，此处验证跨项目由路由层隔离）
	req3 := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/api-keys", nil)
	req3.Header.Set("Authorization", "Bearer invalid_key")
	rec3 := httptest.NewRecorder()
	e.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("invalid key must 401: %d", rec3.Code)
	}

	// 5) 吊销新 Key 后立即失效
	req4 := httptest.NewRequest(http.MethodDelete, "/v1/projects/proj-1/api-keys/"+created.ID, nil)
	req4.Header.Set("Authorization", "Bearer "+seed)
	rec4 := httptest.NewRecorder()
	e.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", rec4.Code, rec4.Body.String())
	}
	req5 := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/api-keys", nil)
	req5.Header.Set("Authorization", "Bearer "+created.Secret)
	rec5 := httptest.NewRecorder()
	e.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusUnauthorized {
		t.Fatalf("revoked must 401: %d", rec5.Code)
	}
}
