package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

type fakeUsage struct {
	errLLM error
	errDB  error
	calls  []string
}

func (f *fakeUsage) CheckQuota(_ context.Context, _ string, kind string) error {
	f.calls = append(f.calls, kind)
	if kind == "llm" {
		return f.errLLM
	}
	return f.errDB
}

type fakeAudit struct {
	ops    []catalog.Operation
	err    error
	last   AuditEvent
	recErr error
}

func (f *fakeAudit) Record(_ context.Context, e AuditEvent) error {
	f.last = e
	return f.recErr
}
func (f *fakeAudit) ListOperations(context.Context, string, string, int) ([]catalog.Operation, error) {
	return f.ops, f.err
}

func setupPlan79Router(t *testing.T, usage UsageService, audit AuditService, withProject bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithPrincipal(c.Request().Context(), auth.Principal{APIKeyID: "k"})
			if withProject {
				ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "t"})
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
		if rec.Code != http.StatusInternalServerError {
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
		if rec.Code != http.StatusInternalServerError {
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
