package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestOpenAPIMatchesRealRouter(t *testing.T) {
	for _, test := range []struct {
		name string
		full bool
	}{
		{name: "minimal"},
		{name: "full", full: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var e *echo.Echo
			if test.full {
				deps, _ := fullRouterDeps(t)
				e = NewRouter(deps)
			} else {
				e = NewRouter(testDeps(t, &fakeHealth{}))
			}
			metadata := routeMetadata(e.Routes())
			document := buildOpenAPI(e.Routes())
			operations := 0
			for path, methods := range document.Paths {
				operations += len(methods)
				for method, operation := range methods {
					if operation.OperationID == "" || !operation.Tryable && method == "get" && !strings.Contains(path, "export") && !strings.Contains(path, "realtime") && !strings.Contains(path, "content") && !strings.Contains(path, "download") {
						t.Fatalf("unexpected non-tryable route: %s %s", method, path)
					}
				}
			}
			if operations != len(metadata) {
				t.Fatalf("operations=%d metadata=%d", operations, len(metadata))
			}
			for _, item := range metadata {
				if document.Paths[item.Path][strings.ToLower(item.Method)].OperationID != item.OperationID {
					t.Fatalf("missing operation %s %s", item.Method, item.Path)
				}
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/openapi.json", nil)
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || payload["openapi"] != "3.0.3" {
				t.Fatalf("document=%v err=%v", payload, err)
			}
		})
	}
}
