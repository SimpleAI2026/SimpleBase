package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

func TestSandboxDomainErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{sandbox.ErrUnavailable, 503, "sandbox_unavailable"},
		{sandbox.ErrNotFound, 404, "sandbox_not_found"},
		{sandbox.ErrFileNotFound, 404, "sandbox_file_not_found"},
		{sandbox.ErrNameConflict, 409, "sandbox_name_conflict"},
		{sandbox.ErrLimitExceeded, 429, "sandbox_limit_exceeded"},
		{sandbox.ErrInvalidSpec, 400, "sandbox_invalid_spec"},
		{sandbox.ErrInvalidPath, 400, "sandbox_invalid_path"},
		{sandbox.ErrFileTooLarge, 413, "sandbox_file_too_large"},
		{sandbox.ErrBusy, 409, "sandbox_busy"},
		{sandbox.ErrGone, 410, "sandbox_gone"},
		{sandbox.ErrBackend, 502, "sandbox_backend_error"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			got := Error(tc.err, "r")
			if got == nil || got.HTTPStatus != tc.status || got.Body.Error.Code != tc.code {
				t.Fatalf("error: %+v", got)
			}
		})
	}
}

func sandboxHandlerContext(t *testing.T, projectID string, writable bool) (*echo.Echo, echo.Context, *SandboxHandler) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/"+projectID+"/sandboxes", strings.NewReader(`{"name":"demo"}`))
	c := e.NewContext(req, httptest.NewRecorder())
	ctx := WithProject(context.Background(), ProjectContext{ID: projectID, TenantID: "tenant"})
	c.SetRequest(req.WithContext(ctx))
	return e, c, NewSandboxHandler(nil, writable, nil, nil, nil)
}

func TestSandboxHandlerDisabledAndWriteGuards(t *testing.T) {
	_, c, h := sandboxHandlerContext(t, "p", true)
	if err := h.Capabilities(c); err != nil {
		t.Fatal(err)
	}
	if rec := c.Response().Writer.(*httptest.ResponseRecorder); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"available":false`) {
		t.Fatalf("capabilities: %d %s", rec.Code, rec.Body.String())
	}
	_, c, h = sandboxHandlerContext(t, "p", false)
	if err := h.Create(c); err != nil {
		t.Fatal(err)
	}
	if rec := c.Response().Writer.(*httptest.ResponseRecorder); rec.Code != 503 {
		t.Fatalf("readonly create: %d %s", rec.Code, rec.Body.String())
	}
	_, c, h = sandboxHandlerContext(t, catalog.ReservedSystemProjectID, true)
	if err := h.Create(c); err != nil {
		t.Fatal(err)
	}
	if rec := c.Response().Writer.(*httptest.ResponseRecorder); rec.Code != 403 {
		t.Fatalf("system create: %d %s", rec.Code, rec.Body.String())
	}
}
