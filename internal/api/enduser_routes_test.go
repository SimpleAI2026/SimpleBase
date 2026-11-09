// 文件说明：enduser_routes_test.go — 终端用户路由与密钥域隔离测试（§8.2.6）
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

// testEndUserRepo 复用 auth 包测试的内存仓储语义。
type testEndUserRepo struct {
	mu    sync.Mutex
	items map[string]auth.EndUser
}

func (m *testEndUserRepo) Create(_ context.Context, u auth.EndUser) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := u.ProjectID + "/" + u.ExternalID
	if _, ok := m.items[k]; ok {
		return auth.ErrEndUserExists
	}
	m.items[k] = u
	return nil
}

func (m *testEndUserRepo) GetByExternalID(_ context.Context, projectID, externalID string) (auth.EndUser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.items[projectID+"/"+externalID]
	if !ok {
		return auth.EndUser{}, auth.ErrEndUserNotFound
	}
	return u, nil
}

func (m *testEndUserRepo) UpdatePassword(_ context.Context, id string, hash string, at time.Time) error { return nil }
func (m *testEndUserRepo) Count(_ context.Context, projectID string) (int64, error)                     { return 0, nil }

func newEndUserRouter(t *testing.T) *echo.Echo {
	t.Helper()
	deps := Dependencies{
		Config: config.Config{Auth: config.AuthConfig{SessionSecret: "test-secret"}},
	}
	deps.EndUsers = auth.NewEndUserService(&testEndUserRepo{items: map[string]auth.EndUser{}}, "test-secret")
	e := echo.New()
	e.POST("/v1/projects/:projectID/end-users/signup", func(c echo.Context) error {
		h := NewEndUsersHandler(deps.EndUsers)
		return h.Signup(c)
	})
	e.POST("/v1/projects/:projectID/end-users/login", func(c echo.Context) error {
		h := NewEndUsersHandler(deps.EndUsers)
		return h.Login(c)
	})
	e.GET("/v1/projects/:projectID/end-users/me", func(c echo.Context) error {
		h := NewEndUsersHandler(deps.EndUsers)
		return h.Me(c)
	}, endUserAuth("test-secret"))
	return e
}

func TestEndUserRoutesSignupLoginMe(t *testing.T) {
	e := newEndUserRouter(t)

	// signup。
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/p1/end-users/signup", strings.NewReader(
		`{"username":"alice","password":"passw0rd-long","display_name":"Alice"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup: %d body=%s", rec.Code, rec.Body.String())
	}

	// login。
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/p1/end-users/login", strings.NewReader(
		`{"username":"alice","password":"passw0rd-long"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d body=%s", rec.Code, rec.Body.String())
	}
	token := extractJSONStringField(rec.Body.String(), "token")

	// me（带 token）。
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/p1/end-users/me", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: %d body=%s", rec.Code, rec.Body.String())
	}

	// me 不带 token → 401。
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/projects/p1/end-users/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me without token: %d", rec.Code)
	}

	// 平台 token（aud=enduser）不能走平台会话域：直接验证守卫语义——
	// 用同一 secret 签的平台形态 token（无 aud）被 endUserAuth 拒绝。
	platformToken, err := auth.SignJWT("test-secret", auth.JWTClaims{
		Subject: "u1", IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/p1/end-users/me", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+platformToken)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("platform token must be rejected by endUserAuth: %d", rec.Code)
	}
}

func TestEndUserRoutesDuplicateAndWrongPassword(t *testing.T) {
	e := newEndUserRouter(t)
	body := strings.NewReader(`{"username":"bob","password":"passw0rd-long"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/p1/end-users/signup", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first signup: %d", rec.Code)
	}
	// 重复注册 → 409。
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/p1/end-users/signup",
		strings.NewReader(`{"username":"bob","password":"passw0rd-long"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate signup: %d", rec.Code)
	}
	// 错口令 → 401。
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/p1/end-users/login",
		strings.NewReader(`{"username":"bob","password":"wrong"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", rec.Code)
	}
}

// extractJSONStringField 从 {"token":"..."} 里抠字段（避免引入 json 依赖样板）。
func extractJSONStringField(body, field string) string {
	for _, part := range strings.Split(body, ",") {
		if strings.Contains(part, "\""+field+"\"") {
			idx := strings.Index(part, ":")
			if idx < 0 {
				continue
			}
			v := strings.TrimSpace(part[idx+1:])
			return strings.Trim(v, `"}`)
		}
	}
	return ""
}
