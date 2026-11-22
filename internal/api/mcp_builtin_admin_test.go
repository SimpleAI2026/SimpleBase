package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func mcpLegacyServer(projectID, name string) systemdb.MCPServer {
	return systemdb.MCPServer{
		ProjectID: projectID, Name: name, Transport: systemdb.MCPTransportHTTP,
		URL: "https://legacy.example.com/mcp", TimeoutS: 10, Enabled: true,
	}
}

func errCode(body map[string]any) string {
	inner, _ := body["error"].(map[string]any)
	code, _ := inner["code"].(string)
	return code
}

func mcpAdminDo(t *testing.T, e *echo.Echo, method, path, key, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if key != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec, out
}

func TestMCPServers_ListIncludesBuiltin(t *testing.T) {
	e, _, key := builtinMCPRouter(t, true)
	rec, body := mcpAdminDo(t, e, http.MethodGet, "/v1/projects/proj-1/mcp/servers", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	list, _ := body["builtin"].([]any)
	if len(list) != 1 {
		t.Fatalf("builtin=%v", body["builtin"])
	}
	first := list[0].(map[string]any)
	if first["name"] != "simplebase" || first["builtin"] != true || first["enabled"] != true {
		t.Fatalf("first=%v", first)
	}
	if url, _ := first["url"].(string); !strings.HasSuffix(url, "/v1/projects/proj-1/mcp") {
		t.Fatalf("url=%v", first["url"])
	}
	if tools, _ := first["tools"].([]any); len(tools) != 22 {
		t.Fatalf("tools=%d", len(tools))
	}
	if _, ok := body["servers"].([]any); !ok {
		t.Fatalf("servers must stay in response: %v", body)
	}
}

func TestMCPServers_ListBuiltinEmptyWhenInstanceSwitchOff(t *testing.T) {
	e, _, key := builtinMCPRouter(t, false)
	rec, body := mcpAdminDo(t, e, http.MethodGet, "/v1/projects/proj-1/mcp/servers", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	list, ok := body["builtin"].([]any)
	if !ok || len(list) != 0 {
		t.Fatalf("builtin must be [] when disabled: %v", body["builtin"])
	}
}

func TestMCPServers_ReservedNameRejected(t *testing.T) {
	e, _, key := builtinMCPRouter(t, true)
	for _, name := range []string{"simplebase", "SimpleBase", "sb-admin", "SB-x"} {
		rec, body := mcpAdminDo(t, e, http.MethodPost, "/v1/projects/proj-1/mcp/servers", key,
			`{"name":"`+name+`","url":"https://mcp.example.com/mcp"}`)
		if rec.Code != http.StatusBadRequest || errCode(body) != "mcp_name_reserved" {
			t.Fatalf("name=%s status=%d body=%s", name, rec.Code, rec.Body.String())
		}
	}
	rec, _ := mcpAdminDo(t, e, http.MethodPost, "/v1/projects/proj-1/mcp/servers", key,
		`{"name":"github","url":"https://mcp.example.com/mcp"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("normal name status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMCPServers_UpdateCannotRenameToReserved(t *testing.T) {
	e, _, key := builtinMCPRouter(t, true)
	rec, body := mcpAdminDo(t, e, http.MethodPost, "/v1/projects/proj-1/mcp/servers", key,
		`{"name":"github","url":"https://mcp.example.com/mcp"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	id, _ := body["id"].(string)
	rec, body = mcpAdminDo(t, e, http.MethodPatch, "/v1/projects/proj-1/mcp/servers/"+id, key, `{"name":"sb-evil"}`)
	if rec.Code != http.StatusBadRequest || errCode(body) != "mcp_name_reserved" {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMCPServers_LegacyReservedNameFlagged(t *testing.T) {
	store := openAgentStore(t)
	e := mcpServerEcho(store)
	// 绕过 handler 直接写库，模拟升级前已存在的同名配置。
	if _, err := store.CreateMCPServer(t.Context(), mcpLegacyServer("p1", "simplebase")); err != nil {
		t.Fatal(err)
	}
	rec, body := doJSON(t, e, http.MethodGet, "/mcp/servers", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	servers, _ := body["servers"].([]any)
	if len(servers) != 1 || servers[0].(map[string]any)["name_conflict"] != true {
		t.Fatalf("servers=%v", servers)
	}
}

func TestBuiltinMCP_StateToggleControlsEndpointAndAudits(t *testing.T) {
	e, deps, key := builtinMCPRouter(t, true)
	audit := deps.Audit.(*fakeAudit)

	rec, body := mcpAdminDo(t, e, http.MethodPut, "/v1/projects/proj-1/mcp/builtin/simplebase/state", key, `{"enabled":false}`)
	if rec.Code != http.StatusOK || body["enabled"] != false {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if audit.last.Kind != "mcp.builtin.disable" || audit.last.ProjectID != "proj-1" {
		t.Fatalf("audit=%+v", audit.last)
	}
	rec = mcpPost(t, e, "/v1/projects/proj-1/mcp", key, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled endpoint status=%d", rec.Code)
	}
	_, listBody := mcpAdminDo(t, e, http.MethodGet, "/v1/projects/proj-1/mcp/servers", key, "")
	first := listBody["builtin"].([]any)[0].(map[string]any)
	if first["enabled"] != false {
		t.Fatalf("list must reflect disabled: %v", first)
	}

	rec, _ = mcpAdminDo(t, e, http.MethodPut, "/v1/projects/proj-1/mcp/builtin/simplebase/state", key, `{"enabled":true}`)
	if rec.Code != http.StatusOK || audit.last.Kind != "mcp.builtin.enable" {
		t.Fatalf("status=%d audit=%+v", rec.Code, audit.last)
	}
}

func TestBuiltinMCP_StateValidation(t *testing.T) {
	e, _, key := builtinMCPRouter(t, true)
	rec, _ := mcpAdminDo(t, e, http.MethodPut, "/v1/projects/proj-1/mcp/builtin/simplebase/state", key, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled status=%d", rec.Code)
	}
	rec, _ = mcpAdminDo(t, e, http.MethodPut, "/v1/projects/proj-1/mcp/builtin/nope/state", key, `{"enabled":true}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown name status=%d", rec.Code)
	}
	rec, _ = mcpAdminDo(t, e, http.MethodPut, "/v1/projects/proj-1/mcp/builtin/simplebase/state", "", `{"enabled":true}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", rec.Code)
	}
}

func TestBuiltinMCP_StateRejectedWhenInstanceReadOnly(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Config.Agent.MCP.Builtin = true
	deps.Config.Instance.Writable = false
	e := NewRouter(deps)
	rec, _ := mcpAdminDo(t, e, http.MethodPut, "/v1/projects/proj-1/mcp/builtin/simplebase/state", key, `{"enabled":false}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuiltinMCP_TestEndpointListsToolsWithoutExternalMCPEnabled(t *testing.T) {
	e, deps, key := builtinMCPRouter(t, true)
	if deps.Config.Agent.MCP.Enabled {
		t.Fatal("precondition: agent.mcp.enabled must be false")
	}
	rec, body := mcpAdminDo(t, e, http.MethodPost, "/v1/projects/proj-1/mcp/builtin/simplebase/test", key, "")
	if rec.Code != http.StatusOK || body["ok"] != true {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if tools, _ := body["tools"].([]any); len(tools) != 22 {
		t.Fatalf("tools=%d", len(tools))
	}
	rec, _ = mcpAdminDo(t, e, http.MethodPost, "/v1/projects/proj-1/mcp/builtin/nope/test", key, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown name status=%d", rec.Code)
	}
}
