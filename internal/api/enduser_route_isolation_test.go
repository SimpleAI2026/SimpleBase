// 文件说明：enduser_route_isolation_test.go — 终端用户 token 不得进入集合行级规则路由。
//
// 用法
//
//	go test ./internal/api -run TestEndUserTokenRejectedOnCollectionAccessRoute
//
// 调用架构
//
//	[上游] 无（同包测试）
//	[本文件] internal/api/enduser_route_isolation_test.go
//	[下游] NewRouter 的全局鉴权中间件（auth.AuthMiddlewareWithDelegation）
//
// 主要符号
//
//	TestEndUserTokenRejectedOnCollectionAccessRoute

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

func TestEndUserTokenRejectedOnCollectionAccessRoute(t *testing.T) {
	env := newSkillRouterEnv(t)
	now := time.Now()
	token, err := auth.SignJWT("test-secret", auth.JWTClaims{
		Issuer:    "simplebase",
		Subject:   "eu-1",
		Audience:  "enduser",
		Type:      "enduser",
		ProjectID: "proj-1",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections/c1/access"},
		{http.MethodPut, "/v1/projects/proj-1/databases/db-1/data/collections/c1/access"},
		{http.MethodDelete, "/v1/projects/proj-1/databases/db-1/data/collections/c1/access"},
	}
	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"mode":"off"}`))
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		env.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: status=%d body=%s", route.method, route.path, rec.Code, rec.Body.String())
		}
	}
}
