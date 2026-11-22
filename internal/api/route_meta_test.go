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

func TestParamSourceFor(t *testing.T) {
	for _, test := range []struct {
		param, path string
		kind        string
		dependsOn   []string
	}{
		{"projectID", "/v1/projects/{projectID}/databases", "project", nil},
		{"databaseID", "/v1/projects/{projectID}/databases/{databaseID}/schema", "databases", nil},
		{"collection", "/v1/projects/{projectID}/databases/{databaseID}/data/collections/{collection}", "collections", []string{"databaseID"}},
		{"table", "/v1/projects/{projectID}/databases/{databaseID}/schema/tables/{table}/rows", "tables", []string{"databaseID"}},
		{"agentID", "/v1/projects/{projectID}/agents/{agentID}", "agents", nil},
		{"threadID", "/v1/projects/{projectID}/agent-threads/{threadID}", "agent-threads", nil},
		{"jobID", "/v1/projects/{projectID}/cron-jobs/{jobID}", "cron-jobs", nil},
		{"name", "/v1/projects/{projectID}/gofunctions/{name}", "gofunctions", nil},
		{"name", "/go/{projectID}/{name}/{functionName}", "gofunctions", nil},
		{"id", "/v1/projects/{projectID}/function-triggers/{id}", "function-triggers", nil},
		{"id", "/v1/projects/{projectID}/backups/{id}", "backups", nil},
		{"serverID", "/v1/projects/{projectID}/mcp/servers/{serverID}", "mcp-servers", nil},
		{"sandboxID", "/v1/projects/{projectID}/sandboxes/{sandboxID}", "sandboxes", nil},
		{"keyID", "/v1/projects/{projectID}/api-keys/{keyID}", "api-keys", nil},
	} {
		got := paramSourceFor(test.param, test.path)
		if got == nil || got.Kind != test.kind {
			t.Fatalf("%s %s: got %+v, want kind %s", test.param, test.path, got, test.kind)
		}
		if strings.Join(got.DependsOn, ",") != strings.Join(test.dependsOn, ",") {
			t.Fatalf("%s %s: depends_on=%v want %v", test.param, test.path, got.DependsOn, test.dependsOn)
		}
	}
}

func TestParamSourceForUnannotated(t *testing.T) {
	for _, test := range []struct{ param, path string }{
		{"runID", "/v1/projects/{projectID}/agent-threads/{threadID}/runs/{runID}"},
		{"callID", "/v1/projects/{projectID}/agent-threads/{threadID}/calls/{callID}"},
		{"planID", "/v1/projects/{projectID}/agent-threads/{threadID}/plans/{planID}"},
		{"ver", "/v1/projects/{projectID}/gofunctions/{name}/versions/{ver}"},
		{"port", "/v1/projects/{projectID}/sandboxes/{sandboxID}/host/{port}"},
		{"id", "/v1/projects/{projectID}/databases/{databaseID}/data/collections/{collection}/documents/{id}"},
		{"functionName", "/go/{projectID}/{name}/{functionName}"},
		{"collection", "/v1/projects/{projectID}/other/{collection}"},
	} {
		if got := paramSourceFor(test.param, test.path); got != nil {
			t.Fatalf("%s %s: want nil, got %+v", test.param, test.path, got)
		}
	}
}

func TestParamSourceFor_ReturnsIndependentCopy(t *testing.T) {
	first := paramSourceFor("collection", "/data/collections/{collection}")
	first.DependsOn[0] = "tampered"
	second := paramSourceFor("collection", "/data/collections/{collection}")
	if second.DependsOn[0] != "databaseID" {
		t.Fatalf("rule table was mutated: %v", second.DependsOn)
	}
}

// 对账：所有 projectID 路径参数都必须带 kind=project，防止新路由漏标。
func TestOpenAPIProjectParamsAreAnnotated(t *testing.T) {
	deps, _ := fullRouterDeps(t)
	document := buildOpenAPI(NewRouter(deps).Routes())
	found := 0
	for path, methods := range document.Paths {
		for method, op := range methods {
			for _, param := range op.Parameters {
				if param.Name != "projectID" {
					continue
				}
				found++
				if param.Source == nil || param.Source.Kind != "project" {
					t.Fatalf("%s %s: projectID missing x-sb-source (got %+v)", method, path, param.Source)
				}
			}
		}
	}
	if found == 0 {
		t.Fatal("no projectID parameters found; reconciliation is vacuous")
	}
}

func TestOpenAPISourceSerialization(t *testing.T) {
	op := operationParameter{Name: "collection", In: "path", Required: true, Schema: map[string]any{"type": "string"}, Source: paramSourceFor("collection", "/data/collections/{collection}")}
	raw, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"x-sb-source":{"kind":"collections","depends_on":["databaseID"]}`) {
		t.Fatalf("unexpected json: %s", raw)
	}
	raw, _ = json.Marshal(operationParameter{Name: "runID", In: "path", Schema: map[string]any{}})
	if strings.Contains(string(raw), "x-sb-source") {
		t.Fatalf("unannotated param must omit x-sb-source: %s", raw)
	}
}
