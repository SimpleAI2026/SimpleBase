// 文件说明：sandbox_logs_handler_test.go — internal/api 的单元测试（TestLogsPlain、TestLogsFollowIsSSE 等）
//
// 用法
//
//	go test ./internal/api -run 'TestLogsPlain|TestLogsFollowIsSSE|TestLogsUnsupported|TestHostProxy'
//	同包调用：rowFor() → e2b_envd_watch_test.go, sandbox_snapshot_handler_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sandbox_logs_handler_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] rowFor ← e2b_envd_watch_test.go, sandbox_snapshot_handler_test.go
//
// 主要符号
//
//	rowFor(t *testing.T, sb *fakeRouterSandbox) string
//	TestLogsPlain(t *testing.T)
//	TestLogsFollowIsSSE(t *testing.T)
//	TestLogsUnsupported(t *testing.T)
//	TestHostProxy(t *testing.T)
//	TestHostProxyBadPort(t *testing.T)
//	TestHostProxyUnsupported(t *testing.T)

package api

// sandbox_logs_handler_test.go：logs 与 host 端点的形状。
//
//   1. logs 默认 text/plain，按记录顺序输出；
//   2. follow=1 时是 SSE，且先回放历史；
//   3. 后端不具备能力时返回 501，而不是空列表；
//   4. host 把请求代理进沙盒端口，端口非法返回 400；
//   5. host 需要写权限。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

// rowFor 建一个沙盒行并返回它的 id。
func rowFor(t *testing.T, sb *fakeRouterSandbox) string {
	t.Helper()
	row, err := sb.Create(context.Background(), "proj-1", sandbox.CreateInput{Name: "logs"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	return row.ID
}

// 默认 text/plain：两条记录按顺序写出。
func TestLogsPlain(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)

	rec := call(e, http.MethodGet, "/e2b/sandboxes/"+id+"/logs", key, "", map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "hello") || !strings.Contains(body, "warn") {
		t.Fatalf("body = %q", body)
	}
	if strings.Index(body, "hello") > strings.Index(body, "warn") {
		t.Fatalf("records out of order: %q", body)
	}
}

// follow=1：SSE，先回放历史。
func TestLogsFollowIsSSE(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/e2b/sandboxes/"+id+"/logs?follow=1", nil).WithContext(ctx)
	req.Header.Set("X-API-Key", key)
	req.Header.Set("E2b-Sandbox-Id", id)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.ServeHTTP(rec, req)
	}()
	// 历史在 handler 入口同步写出；follow 随后一直轮询。等历史落盘后取消并等 handler 退出，
	// 再读响应体（httptest.ResponseRecorder 不是并发安全的，边写边读会被 -race 判为数据竞争）。
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("follow stream did not stop after cancel")
	}

	if ct := rec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "data: hello") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

// 后端没有 logs 能力：501 + unsupported_feature。
func TestLogsUnsupported(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	sb.noLogs = true
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)

	rec := call(e, http.MethodGet, "/e2b/sandboxes/"+id+"/logs", key, "", map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported_feature") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

// host 代理：请求被送到沙盒端口，响应回来。
func TestHostProxy(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)

	rec := call(e, http.MethodPost, "/e2b/sandboxes/"+id+"/host/8080", key, "ping",
		map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "proxied:8080") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

// 端口非法：400。
func TestHostProxyBadPort(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	for _, port := range []string{"0", "65536", "abc"} {
		rec := call(e, http.MethodPost, "/e2b/sandboxes/"+id+"/host/"+port, key, "",
			map[string]string{"E2b-Sandbox-Id": id})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("port %q: status = %d", port, rec.Code)
		}
	}
}

// 后端没有端口能力：501。
func TestHostProxyUnsupported(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	sb.noPorts = true
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	rec := call(e, http.MethodPost, "/e2b/sandboxes/"+id+"/host/8080", key, "",
		map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}
