// 文件说明：server_test.go — gocloudagent/mcp 的单元测试（TestConnectInMemory、TestConnectInMemorySurvivesCancelledConnectContext 等）
//
// 用法
//
//	go test ./gocloudagent/mcp -run 'TestConnectInMemory|TestConnectInMemorySurvivesCancelledConnectContext|TestNewHTTPHandlerRoundTrip|TestNewHTTPHandlerStatusCodes'
//	同包调用：testSpec() → pagination_test.go；assertTools() → pagination_test.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/tools, internal/agentbridge 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/mcp/server_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] adapter.go, adapter_test.go, client.go, http_transport.go, http_transport_test.go 等 10 个
//	[关键引用] testSpec ← pagination_test.go；assertTools ← pagination_test.go
//
// 主要符号
//
//	const objectSchema
//	testSpec() *ServerSpec
//	assertTools(t *testing.T, tools []ToolInfo)
//	assertCalls(t *testing.T, sess Session)
//	TestConnectInMemory(t *testing.T)
//	TestConnectInMemorySurvivesCancelledConnectContext(t *testing.T)
//	TestNewHTTPHandlerRoundTrip(t *testing.T)
//	TestNewHTTPHandlerStatusCodes(t *testing.T)
//	... 另有 3 个顶层符号

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const objectSchema = `{"type":"object","properties":{"q":{"type":"string"}}}`

func testSpec() *ServerSpec {
	return &ServerSpec{
		Name:         "unit",
		Version:      "9",
		Instructions: "hello",
		Tools: []ServerTool{
			{
				Name: "echo", Description: "echo args", InputSchema: json.RawMessage(objectSchema), ReadOnly: true,
				Handler: func(_ context.Context, args []byte) (CallResult, error) {
					return CallResult{Text: "got:" + string(args)}, nil
				},
			},
			{
				Name: "drop", Description: "destroy", InputSchema: json.RawMessage(objectSchema), Destructive: true,
				Handler: func(context.Context, []byte) (CallResult, error) {
					return CallResult{Text: "dropped"}, nil
				},
			},
			{
				Name: "write", Description: "write", InputSchema: json.RawMessage(objectSchema),
				Handler: func(context.Context, []byte) (CallResult, error) {
					return CallResult{Text: `{"status":403}`, IsError: true}, nil
				},
			},
			{
				Name: "boom", Description: "fails", InputSchema: json.RawMessage(objectSchema),
				Handler: func(context.Context, []byte) (CallResult, error) {
					return CallResult{}, errors.New("handler failed")
				},
			},
		},
	}
}

func assertTools(t *testing.T, tools []ToolInfo) {
	t.Helper()
	if len(tools) != 4 {
		t.Fatalf("tools = %d, want 4", len(tools))
	}
	byName := map[string]ToolInfo{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	if !byName["echo"].ReadOnlyHint || byName["echo"].DestructiveHint {
		t.Errorf("echo hints = %+v", byName["echo"])
	}
	if byName["drop"].ReadOnlyHint || !byName["drop"].DestructiveHint {
		t.Errorf("drop hints = %+v", byName["drop"])
	}
	if byName["write"].ReadOnlyHint || byName["write"].DestructiveHint {
		t.Errorf("write hints = %+v", byName["write"])
	}
	if byName["echo"].Description != "echo args" {
		t.Errorf("description = %q", byName["echo"].Description)
	}
}

func assertCalls(t *testing.T, sess Session) {
	t.Helper()
	ctx := context.Background()
	res, err := sess.CallTool(ctx, "echo", []byte(`{"q":"x"}`))
	if err != nil || res.IsError || res.Text != `got:{"q":"x"}` {
		t.Fatalf("echo = %+v, %v", res, err)
	}
	res, err = sess.CallTool(ctx, "write", nil)
	if err != nil || !res.IsError || res.Text != `{"status":403}` {
		t.Fatalf("write = %+v, %v", res, err)
	}
	res, err = sess.CallTool(ctx, "boom", nil)
	if err != nil || !res.IsError || !strings.Contains(res.Text, "handler failed") {
		t.Fatalf("boom = %+v, %v", res, err)
	}
}

func TestConnectInMemory(t *testing.T) {
	sess, err := ConnectInMemory(context.Background(), "unit", testSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if sess.ServerID() != "unit" {
		t.Errorf("ServerID = %q", sess.ServerID())
	}
	tools, err := sess.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertTools(t, tools)
	assertCalls(t, sess)
}

func TestConnectInMemorySurvivesCancelledConnectContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sess, err := ConnectInMemory(ctx, "unit", testSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	cancel()
	if _, err := sess.CallTool(context.Background(), "echo", nil); err != nil {
		t.Fatalf("session should outlive connect ctx: %v", err)
	}
}

func TestNewHTTPHandlerRoundTrip(t *testing.T) {
	ts := httptest.NewServer(NewHTTPHandler(func(*http.Request) *ServerSpec { return testSpec() }))
	defer ts.Close()
	sess, err := ConnectHTTP(context.Background(), "unit", HTTPOptions{Endpoint: ts.URL, HTTPClient: ts.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	tools, err := sess.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertTools(t, tools)
	assertCalls(t, sess)
}

func TestNewHTTPHandlerStatusCodes(t *testing.T) {
	var spec *ServerSpec
	h := NewHTTPHandler(func(*http.Request) *ServerSpec { return spec })

	do := func(method string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := do(http.MethodPost, `{}`); rec.Code != http.StatusNotFound {
		t.Errorf("nil spec = %d, want 404", rec.Code)
	}

	spec = &ServerSpec{Name: "bad", Tools: []ServerTool{{Name: "x", InputSchema: json.RawMessage(`{"type":"string"}`), Handler: testSpec().Tools[0].Handler}}}
	rec := do(http.MethodPost, `{}`)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `type \"object\"`) {
		t.Errorf("invalid spec = %d %s", rec.Code, rec.Body.String())
	}

	spec = testSpec()
	if rec := do(http.MethodGet, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d, want 405", rec.Code)
	}
	if rec := do(http.MethodDelete, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE = %d, want 405", rec.Code)
	}
}

func TestNewHTTPHandlerReturnsJSONNotSSE(t *testing.T) {
	h := NewHTTPHandler(func(*http.Request) *ServerSpec { return testSpec() })
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	if rec.Header().Get("Mcp-Session-Id") != "" {
		t.Errorf("stateless server must not set a session id")
	}
}

func TestNewHTTPHandlerRejectsOversizedBody(t *testing.T) {
	h := NewHTTPHandler(func(*http.Request) *ServerSpec { return testSpec() })
	big := bytes.Repeat([]byte("a"), MaxServerRequestBytes+1024)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"q":"` + string(big) + `"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), `"result"`) {
		t.Fatalf("oversized body was accepted: %d", rec.Code)
	}
}

func TestServerSpecValidation(t *testing.T) {
	ok := func(context.Context, []byte) (CallResult, error) { return CallResult{}, nil }
	tool := func(name, schema string) ServerTool {
		return ServerTool{Name: name, InputSchema: json.RawMessage(schema), Handler: ok}
	}
	tooMany := make([]ServerTool, MaxToolsPerServer+1)
	for i := range tooMany {
		tooMany[i] = tool("t"+string(rune('a'+i%26))+string(rune('a'+i/26)), objectSchema)
	}
	hugeSchema := `{"type":"object","description":"` + strings.Repeat("x", MaxSchemaBytes) + `"}`

	cases := map[string]*ServerSpec{
		"nil spec":          nil,
		"empty name":        {Tools: []ServerTool{tool("a", objectSchema)}},
		"too many tools":    {Name: "s", Tools: tooMany},
		"empty tool name":   {Name: "s", Tools: []ServerTool{tool(" ", objectSchema)}},
		"duplicate tool":    {Name: "s", Tools: []ServerTool{tool("a", objectSchema), tool("a", objectSchema)}},
		"nil handler":       {Name: "s", Tools: []ServerTool{{Name: "a", InputSchema: json.RawMessage(objectSchema)}}},
		"missing schema":    {Name: "s", Tools: []ServerTool{tool("a", "")}},
		"non-object schema": {Name: "s", Tools: []ServerTool{tool("a", `{"type":"array"}`)}},
		"no type":           {Name: "s", Tools: []ServerTool{tool("a", `{"properties":{}}`)}},
		"invalid json":      {Name: "s", Tools: []ServerTool{tool("a", `{bad`)}},
		"schema too large":  {Name: "s", Tools: []ServerTool{tool("a", hugeSchema)}},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ConnectInMemory(context.Background(), "x", spec); err == nil {
				t.Fatal("ConnectInMemory accepted invalid spec")
			}
		})
	}

	limit := make([]ServerTool, MaxToolsPerServer)
	for i := range limit {
		limit[i] = tool("t"+string(rune('a'+i%26))+string(rune('a'+i/26)), objectSchema)
	}
	sess, err := ConnectInMemory(context.Background(), "x", &ServerSpec{Name: "s", Tools: limit})
	if err != nil {
		t.Fatalf("exactly MaxToolsPerServer tools should be accepted: %v", err)
	}
	sess.Close()
}
