// 文件说明：pagination_test.go — gocloudagent/mcp 的单元测试（TestListToolsStopsAtPageBudget、TestListToolsWithinBudgetStillWorks 等）
//
// 用法
//
//	go test ./gocloudagent/mcp -run 'TestListToolsStopsAtPageBudget|TestListToolsWithinBudgetStillWorks'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/tools, internal/agentbridge 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/mcp/pagination_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] adapter.go, adapter_test.go, client.go, http_transport.go, http_transport_test.go 等 10 个
//
// 主要符号
//
//	endlessPagesServer(t *testing.T, pages *int32, emptyPages bool) *httptest.Server
//	TestListToolsStopsAtPageBudget(t *testing.T)
//	TestListToolsWithinBudgetStillWorks(t *testing.T)

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// endlessPagesServer 模拟恶意 MCP Server：tools/list 永远带 nextCursor，
// 且每页要么为空、要么全是无效工具（空名字），因此「累计有效工具数」永远到不了上限。
func endlessPagesServer(t *testing.T, pages *int32, emptyPages bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		write := func(result any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		}
		switch req.Method {
		case "initialize":
			write(map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "evil", "version": "1"},
			})
		case "tools/list":
			atomic.AddInt32(pages, 1)
			tools := []map[string]any{{"name": "", "inputSchema": map[string]any{"type": "object"}}}
			if emptyPages {
				tools = []map[string]any{}
			}
			write(map[string]any{"tools": tools, "nextCursor": "again"})
		case "notifications/initialized", "notifications/cancelled":
			w.WriteHeader(http.StatusAccepted)
		default:
			// server/discover 等未实现的方法：按 JSON-RPC 返回 method not found，
			// 客户端据此回退到 initialize。
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// BUG-15：恶意 Server 无限分页时，ListTools 必须在页数预算内结束并返回错误，
// 不能无限请求直到调用方 ctx 超时。
func TestListToolsStopsAtPageBudget(t *testing.T) {
	for _, empty := range []bool{false, true} {
		var pages int32
		srv := endlessPagesServer(t, &pages, empty)
		sess, err := ConnectHTTP(context.Background(), "evil", HTTPOptions{Endpoint: srv.URL, HTTPClient: srv.Client()})
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		start := time.Now()
		_, err = sess.ListTools(ctx)
		cancel()
		_ = sess.Close()
		if err == nil || !errors.Is(err, ErrTooManyPages) {
			t.Fatalf("empty=%v: err=%v, want ErrTooManyPages", empty, err)
		}
		if got := atomic.LoadInt32(&pages); got > int32(MaxListPages)+1 {
			t.Fatalf("empty=%v: requested %d pages, budget is %d", empty, got, MaxListPages)
		}
		if time.Since(start) > 8*time.Second {
			t.Fatalf("empty=%v: took %v, must stop by budget not by ctx timeout", empty, time.Since(start))
		}
	}
}

// 正常 Server（有限页、有效工具）不受影响。
func TestListToolsWithinBudgetStillWorks(t *testing.T) {
	sess, err := ConnectInMemory(context.Background(), "ok", testSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	tools, err := sess.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertTools(t, tools)
}
