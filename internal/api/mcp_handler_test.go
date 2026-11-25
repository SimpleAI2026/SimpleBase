// 文件说明：mcp_handler_test.go — internal/api 的单元测试（TestMCPServerHandlerCreateNeverEchoesCredentials、TestMCPServerHandlerRejectsInvalidNameAndOverLimit 等）
//
// 用法
//
//	go test ./internal/api -run 'TestMCPServerHandlerCreateNeverEchoesCredentials|TestMCPServerHandlerRejectsInvalidNameAndOverLimit|TestMCPServerHandlerUpdateKeepsHeadersWhenOmitted|TestMCPServerHandlerDeleteAndTest'
//	同包调用：mcpServerEcho() → mcp_builtin_admin_test.go；doJSON() → mcp_builtin_admin_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/mcp_handler_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] mcpServerEcho ← mcp_builtin_admin_test.go；doJSON ← mcp_builtin_admin_test.go
//
// 主要符号
//
//	mcpServerEcho(store *systemdb.Store) *echo.Echo
//	doJSON(t *testing.T, e *echo.Echo, method, path, body string) (*httptest.ResponseRecorder, map[string]any)
//	TestMCPServerHandlerCreateNeverEchoesCredentials(t *testing.T)
//	TestMCPServerHandlerRejectsInvalidNameAndOverLimit(t *testing.T)
//	TestMCPServerHandlerUpdateKeepsHeadersWhenOmitted(t *testing.T)
//	TestMCPServerHandlerDeleteAndTest(t *testing.T)

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// mcpServerEcho 挂一个只含 MCP 配置路由的 echo，用于 handler 级测试。
func mcpServerEcho(store *systemdb.Store) *echo.Echo {
	h := &mcpServerHandler{store: store, limit: 2}
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithProject(c.Request().Context(), ProjectContext{ID: "p1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.GET("/mcp/servers", h.List)
	e.POST("/mcp/servers", h.Create)
	e.PATCH("/mcp/servers/:serverID", h.Update)
	e.DELETE("/mcp/servers/:serverID", h.Delete)
	e.POST("/mcp/servers/:serverID/test", h.Test)
	return e
}

func doJSON(t *testing.T, e *echo.Echo, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec, out
}

func TestMCPServerHandlerCreateNeverEchoesCredentials(t *testing.T) {
	store := openAgentStore(t)
	e := mcpServerEcho(store)

	rec, body := doJSON(t, e, http.MethodPost, "/mcp/servers",
		`{"name":"github","url":"https://mcp.example.com/mcp","headers":{"Authorization":"Bearer top-secret"},"tools_policy":{"create_issue":{"destructive":true}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	// 响应只回 header 名字与 has_credential，绝不回原文。
	raw := rec.Body.String()
	if strings.Contains(raw, "top-secret") {
		t.Fatalf("credential leaked in response: %s", raw)
	}
	if body["has_credential"] != true {
		t.Fatalf("has_credential=%v", body["has_credential"])
	}
	names, _ := body["header_names"].([]any)
	if len(names) != 1 || names[0] != "Authorization" {
		t.Fatalf("header_names=%v", names)
	}

	// 列表同样不泄漏。
	rec, body = doJSON(t, e, http.MethodGet, "/mcp/servers", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "top-secret") {
		t.Fatalf("list leaked: %s", rec.Body.String())
	}
	servers, _ := body["servers"].([]any)
	if len(servers) != 1 {
		t.Fatalf("servers=%v", servers)
	}
}

func TestMCPServerHandlerRejectsInvalidNameAndOverLimit(t *testing.T) {
	store := openAgentStore(t)
	e := mcpServerEcho(store)

	if rec, _ := doJSON(t, e, http.MethodPost, "/mcp/servers", `{"name":"bad name!","url":"https://x/mcp"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid name status=%d", rec.Code)
	}
	if rec, _ := doJSON(t, e, http.MethodPost, "/mcp/servers", `{"name":"ok","url":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing url status=%d", rec.Code)
	}
	// limit=2：第三个别被接受。
	for _, n := range []string{"a", "b"} {
		if rec, _ := doJSON(t, e, http.MethodPost, "/mcp/servers", `{"name":"`+n+`","url":"https://x/mcp"}`); rec.Code != http.StatusOK {
			t.Fatalf("create %s status=%d", n, rec.Code)
		}
	}
	if rec, _ := doJSON(t, e, http.MethodPost, "/mcp/servers", `{"name":"c","url":"https://x/mcp"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("over limit status=%d", rec.Code)
	}
}

func TestMCPServerHandlerUpdateKeepsHeadersWhenOmitted(t *testing.T) {
	store := openAgentStore(t)
	e := mcpServerEcho(store)

	_, body := doJSON(t, e, http.MethodPost, "/mcp/servers",
		`{"name":"s","url":"https://x/mcp","headers":{"Authorization":"Bearer keep-me"}}`)
	id, _ := body["id"].(string)

	// headers 省略：沿用既有值（前端只有名字，不回传原文）。
	rec, updated := doJSON(t, e, http.MethodPatch, "/mcp/servers/"+id, `{"timeout_s":30}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}
	if updated["timeout_s"] != float64(30) || updated["has_credential"] != true {
		t.Fatalf("updated=%v", updated)
	}
	srv, err := store.GetMCPServer(context.Background(), "p1", id)
	if err != nil {
		t.Fatal(err)
	}
	if systemdb.ParseMCPHeaders(srv.HeadersJSON)["Authorization"] != "Bearer keep-me" {
		t.Fatalf("headers overwritten: %s", srv.HeadersJSON)
	}

	// 显式传空对象表示清空。
	if rec, updated = doJSON(t, e, http.MethodPatch, "/mcp/servers/"+id, `{"headers":{}}`); rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d", rec.Code)
	}
	if updated["has_credential"] != false {
		t.Fatalf("has_credential=%v after clearing", updated["has_credential"])
	}
}

func TestMCPServerHandlerDeleteAndTest(t *testing.T) {
	store := openAgentStore(t)
	e := mcpServerEcho(store)

	_, body := doJSON(t, e, http.MethodPost, "/mcp/servers", `{"name":"s","url":"https://x/mcp"}`)
	id, _ := body["id"].(string)

	// testConnect 为 nil（MCP 未启用）→ 503。
	if rec, _ := doJSON(t, e, http.MethodPost, "/mcp/servers/"+id+"/test", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("test status=%d", rec.Code)
	}
	h := &mcpServerHandler{store: store, testConnect: func(context.Context, systemdb.MCPServer) ([]string, error) {
		return []string{"read", "write"}, nil
	}}
	e2 := echo.New()
	e2.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.SetRequest(c.Request().WithContext(WithProject(c.Request().Context(), ProjectContext{ID: "p1"})))
			return next(c)
		}
	})
	e2.POST("/mcp/servers/:serverID/test", h.Test)
	if rec, out := doJSON(t, e2, http.MethodPost, "/mcp/servers/"+id+"/test", ""); rec.Code != http.StatusOK || out["ok"] != true {
		t.Fatalf("test status=%d out=%v", rec.Code, out)
	}

	if rec, _ := doJSON(t, e, http.MethodDelete, "/mcp/servers/"+id, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete status=%d", rec.Code)
	}
	// 再删一次 404。
	if rec, _ := doJSON(t, e, http.MethodDelete, "/mcp/servers/"+id, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete status=%d", rec.Code)
	}
}
