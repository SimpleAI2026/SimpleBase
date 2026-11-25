// 文件说明：client_conn_reuse_test.go — gocloudagent/cli 的单元测试（TestDefaultClientReusesConnectionsAcrossRequests、TestInjectedHTTPClientIsUsedAsIs 等）
//
// 用法
//
//	go test ./gocloudagent/cli -run 'TestDefaultClientReusesConnectionsAcrossRequests|TestInjectedHTTPClientIsUsedAsIs|TestDefaultClientIsRaceFreeOnFirstUse'
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/client_conn_reuse_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_loop.go, agent_loop_test.go, budget_test.go, client.go, client_http_status_test.go 等 39 个
//
// 主要符号
//
//	countingServer(t *testing.T, conns *int32) *httptest.Server
//	TestDefaultClientReusesConnectionsAcrossRequests(t *testing.T)
//	TestInjectedHTTPClientIsUsedAsIs(t *testing.T)
//	TestDefaultClientIsRaceFreeOnFirstUse(t *testing.T)

package cli

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// countingServer 统计新建的 TCP 连接数（而不是请求数）。
func countingServer(t *testing.T, conns *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			atomic.AddInt32(conns, 1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// 同一个 HTTPClient 的连续请求应复用连接（默认客户端）。
func TestDefaultClientReusesConnectionsAcrossRequests(t *testing.T) {
	var conns int32
	srv := countingServer(t, &conns)
	c := &HTTPClient{Endpoint: srv.URL + "/v1", Token: "t"}
	t.Cleanup(c.CloseIdleConnections)
	for i := 0; i < 5; i++ {
		if _, err := c.Chat(context.Background(), "", agentcore.ChatRequest{Model: "m"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(&conns); got != 1 {
		t.Fatalf("opened %d TCP connections for 5 sequential requests, want 1 (keep-alive reuse)", got)
	}
}

// 调用方注入的 HTTP 客户端优先，且不会被本包替换或关闭。
func TestInjectedHTTPClientIsUsedAsIs(t *testing.T) {
	var conns int32
	srv := countingServer(t, &conns)
	custom := &http.Client{Transport: &http.Transport{}}
	c := &HTTPClient{Endpoint: srv.URL + "/v1", Token: "t", HTTP: custom}
	if c.httpClient() != custom {
		t.Fatal("injected client must be returned unchanged")
	}
	if _, err := c.Chat(context.Background(), "", agentcore.ChatRequest{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	c.CloseIdleConnections() // 不得 panic，也不得关闭调用方的 Transport
}

// 零值与并发：多个 goroutine 同时首次使用不得产生数据竞争（-race 下验证）。
func TestDefaultClientIsRaceFreeOnFirstUse(t *testing.T) {
	var conns int32
	srv := countingServer(t, &conns)
	c := &HTTPClient{Endpoint: srv.URL + "/v1", Token: "t"}
	t.Cleanup(c.CloseIdleConnections)
	done := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			_, err := c.Chat(context.Background(), "", agentcore.ChatRequest{Model: "m"})
			done <- err
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
