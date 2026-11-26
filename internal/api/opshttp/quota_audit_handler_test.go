// 文件说明：quota_audit_handler_test.go — internal/api 的单元测试（TestQuotaHandler、TestAuditHandler 等）
//
// 用法
//
//	go test ./internal/api -run 'TestQuotaHandler|TestAuditHandler'
//	同包调用：fakeUsage → agent_skills_router_e2e_test.go, agent_threads_test.go；fakeAudit → agent_skills_router_e2e_test.go, audit_helper_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/quota_audit_handler_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] fakeUsage ← agent_skills_router_e2e_test.go, agent_threads_test.go；fakeAudit ← agent_skills_router_e2e_test.go, audit_helper_test.go
//
// 主要符号
//
//	type fakeUsage struct{…}
//	(f *fakeUsage) CheckQuota(_ context.Context, _ string, kind string) error
//	type fakeAudit struct{…}
//	(f *fakeAudit) Record(_ context.Context, e AuditEvent) error
//	(f *fakeAudit) ListOperations(context.Context, string, string, int) ([]catalog.Operation, error)
//	setupPlan79Router(t *testing.T, usage UsageService, audit AuditService, withProject bool) *echo.Echo
//	TestQuotaHandler(t *testing.T)
//	TestAuditHandler(t *testing.T)

package opshttp

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

type fakeAudit struct {
	ops    []catalog.Operation
	err    error
	last   httpkit.AuditEvent
	recErr error
}

func (f *fakeAudit) Record(_ context.Context, e httpkit.AuditEvent) error {
	f.last = e
	return f.recErr
}
func (f *fakeAudit) ListOperations(context.Context, string, string, int) ([]catalog.Operation, error) {
	return f.ops, f.err
}

func setupPlan79Router(t *testing.T, usage httpkit.UsageService, audit httpkit.AuditService, withProject bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := httpkit.WithPrincipal(c.Request().Context(), auth.Principal{APIKeyID: "k"})
			if withProject {
				ctx = httpkit.WithProject(ctx, httpkit.ProjectContext{ID: "proj-1", TenantID: "t"})
			}
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	if usage != nil {
		qh := &QuotaHandler{svc: usage}
		e.GET("/quota", qh.GetQuota)
	}
	if audit != nil {
		ah := &AuditHandler{svc: audit}
		e.GET("/audit", ah.ListOperations)
	}
	return e
}

func TestQuotaHandler(t *testing.T) {
	t.Run("missing_project", func(t *testing.T) {
		e := setupPlan79Router(t, &fakeUsage{}, nil, false)
		rec := doRequest(e, http.MethodGet, "/quota", nil)
		if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), `"invalid_request"`) {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})
	t.Run("both_ok", func(t *testing.T) {
		e := setupPlan79Router(t, &fakeUsage{}, nil, true)
		rec := doRequest(e, http.MethodGet, "/quota", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
		if !contains(rec.Body.String(), `"llm_allowed":true`) || !contains(rec.Body.String(), `"database_allowed":true`) {
			t.Fatalf("body=%s", rec.Body.String())
		}
	})
	t.Run("quota_denied", func(t *testing.T) {
		e := setupPlan79Router(t, &fakeUsage{errLLM: errors.New("over"), errDB: errors.New("over")}, nil, true)
		rec := doRequest(e, http.MethodGet, "/quota", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		if !contains(rec.Body.String(), `"llm_allowed":false`) || !contains(rec.Body.String(), `"database_allowed":false`) {
			t.Fatalf("body=%s", rec.Body.String())
		}
	})
}

func TestAuditHandler(t *testing.T) {
	t.Run("missing_project", func(t *testing.T) {
		e := setupPlan79Router(t, nil, &fakeAudit{}, false)
		rec := doRequest(e, http.MethodGet, "/audit", nil)
		if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), `"invalid_request"`) {
			t.Fatalf("status=%d", rec.Code)
		}
	})
	t.Run("list_ok_default_limit", func(t *testing.T) {
		a := &fakeAudit{ops: []catalog.Operation{{ID: "op-1", Kind: "create"}}}
		e := setupPlan79Router(t, nil, a, true)
		rec := doRequest(e, http.MethodGet, "/audit", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
		if !contains(rec.Body.String(), "op-1") {
			t.Fatalf("body=%s", rec.Body.String())
		}
	})
	t.Run("limit_and_invalid", func(t *testing.T) {
		e := setupPlan79Router(t, nil, &fakeAudit{}, true)
		rec := doRequest(e, http.MethodGet, "/audit?limit=3", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		if !contains(rec.Body.String(), `"limit":3`) {
			t.Fatalf("body=%s", rec.Body.String())
		}
		rec = doRequest(e, http.MethodGet, "/audit?limit=nope", nil)
		if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"limit":50`) {
			t.Fatalf("invalid limit should fall back: %s", rec.Body.String())
		}
		rec = doRequest(e, http.MethodGet, "/audit?limit=0", nil)
		if !contains(rec.Body.String(), `"limit":50`) {
			t.Fatalf("zero limit: %s", rec.Body.String())
		}
	})
	t.Run("list_error", func(t *testing.T) {
		e := setupPlan79Router(t, nil, &fakeAudit{err: catalog.ErrNotFound}, true)
		rec := doRequest(e, http.MethodGet, "/audit", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
	})
}
