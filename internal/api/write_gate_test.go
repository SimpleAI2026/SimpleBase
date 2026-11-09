package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// WR-01：路由表驱动测试。只读实例（Writable=false）下，所有写接口必须
// 在路由层被 writeGate 拦截返回 503，而不是落到 handler。
// 新增写路由时若忘记挂 writeGate，此测试会漏出（返回非 503）。
func TestWriteGate_AllWriteRoutesBlockedOnReadOnly(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Config.Instance.Writable = false // 只读实例
	e := NewRouter(deps)

	// 全部写路由清单（与 mountV1Routes 挂载保持同步）。
	writeRoutes := []struct{ method, path string }{
		{http.MethodPut, "/v1/auth/password"},
		{http.MethodPost, "/v1/users"},
		{http.MethodPatch, "/v1/users/u1"},
		{http.MethodDelete, "/v1/users/u1"},
		{http.MethodPost, "/v1/projects"},
		{http.MethodDelete, "/v1/projects/proj-1/databases/db1"},
		{http.MethodPost, "/v1/projects/proj-1/api-keys"},
		{http.MethodDelete, "/v1/projects/proj-1/api-keys/k1"},
		{http.MethodPost, "/v1/projects/proj-1/databases/db1/execute"},
		{http.MethodPost, "/v1/projects/proj-1/databases/db1/batch"},
		{http.MethodPost, "/v1/projects/proj-1/databases/db1/schema/tables"},
		{http.MethodPost, "/v1/projects/proj-1/databases/db1/schema/columns"},
		{http.MethodPost, "/v1/projects/proj-1/databases/db1/data/collections"},
		{http.MethodPost, "/v1/projects/proj-1/databases/db1/data/collections/c/import"},
		{http.MethodPost, "/v1/projects/proj-1/databases/db1/data/collections/c/documents"},
		{http.MethodPut, "/v1/projects/proj-1/databases/db1/data/collections/c/documents/d1"},
		{http.MethodDelete, "/v1/projects/proj-1/databases/db1/data/collections/c/documents/d1"},
		{http.MethodPatch, "/v1/projects/proj-1/databases/db1/data/collections/c/documents/d1"},
		{http.MethodPost, "/v1/projects/proj-1/databases/db1/data/collections/c/documents/batch"},
		{http.MethodDelete, "/v1/projects/proj-1/databases/db1/data/collections/c"},
		{http.MethodPut, "/v1/projects/proj-1/databases/db1/data/collections/c/meta"},
		{http.MethodDelete, "/v1/projects/proj-1/databases/db1/data/collections/c/meta"},
		{http.MethodPost, "/v1/projects/proj-1/s3/objects"},
		{http.MethodDelete, "/v1/projects/proj-1/s3/objects"},
		{http.MethodPut, "/v1/projects/proj-1/logs/retention"},
		{http.MethodPut, "/v1/projects/proj-1/settings"},
		{http.MethodPut, "/v1/settings"},
		{http.MethodPut, "/v1/projects/proj-1/llm/settings"},
		{http.MethodPost, "/v1/projects/proj-1/agents"},
		{http.MethodPatch, "/v1/projects/proj-1/agents/a1"},
		{http.MethodDelete, "/v1/projects/proj-1/agents/a1"},
		{http.MethodPost, "/v1/projects/proj-1/agent-threads"},
		{http.MethodPatch, "/v1/projects/proj-1/agent-threads/t1"},
		{http.MethodDelete, "/v1/projects/proj-1/agent-threads/t1"},
		{http.MethodPost, "/v1/projects/proj-1/agent-threads/t1/runs"},
		{http.MethodPost, "/v1/projects/proj-1/agent-runs/r1/cancel"},
		{http.MethodPost, "/v1/projects/proj-1/agent-runs/r1/confirmations/c1"},
		{http.MethodPost, "/v1/projects/proj-1/agent-schedules"},
		{http.MethodPatch, "/v1/projects/proj-1/agent-schedules/s1"},
		{http.MethodDelete, "/v1/projects/proj-1/agent-schedules/s1"},
		{http.MethodPost, "/v1/projects/proj-1/agent-schedules/s1/run"},
		{http.MethodPost, "/v1/projects/proj-1/gofunctions"},
		{http.MethodPatch, "/v1/projects/proj-1/gofunctions/f1"},
		{http.MethodDelete, "/v1/projects/proj-1/gofunctions/f1"},
		{http.MethodPost, "/v1/projects/proj-1/gofunctions/f1/versions"},
		{http.MethodPost, "/v1/projects/proj-1/gofunctions/f1/versions/1/activate"},
		{http.MethodPost, "/v1/projects/proj-1/gofunctions/f1/versions/1/test"},
		{http.MethodPost, "/v1/projects/proj-1/cron-jobs"},
		{http.MethodPatch, "/v1/projects/proj-1/cron-jobs/j1"},
		{http.MethodDelete, "/v1/projects/proj-1/cron-jobs/j1"},
		{http.MethodPost, "/v1/projects/proj-1/cron-jobs/j1/trigger"},
		{http.MethodPost, "/v1/projects/proj-1/function-triggers"},
		{http.MethodPatch, "/v1/projects/proj-1/function-triggers/t1"},
		{http.MethodDelete, "/v1/projects/proj-1/function-triggers/t1"},
		{http.MethodPost, "/v1/projects/proj-1/function-triggers/t1/invoke"},
		{http.MethodPost, "/v1/projects/proj-1/backups"},
		{http.MethodDelete, "/v1/projects/proj-1/backups/b1"},
		{http.MethodPost, "/v1/projects/proj-1/backups/b1/restore"},
	}

	var leaked []string
	for _, r := range writeRoutes {
		req := httptest.NewRequest(r.method, r.path, strings.NewReader("{}"))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		// 门禁先于 handler：预期 503 readonly_instance。
		// 404 = 路由未挂载（依赖缺失，只读实例无害）；其余非 503 即漏挂。
		if rec.Code == http.StatusNotFound {
			continue
		}
		if rec.Code != http.StatusServiceUnavailable {
			leaked = append(leaked, r.method+" "+r.path+" -> "+strconv.Itoa(rec.Code))
		}
	}
	if len(leaked) > 0 {
		t.Fatalf("write routes missing writeGate (WR-01):\n%s", strings.Join(leaked, "\n"))
	}
}

// WR-01 反向：Writable=true 时门禁放行（不能把正常实例挡掉）。
func TestWriteGate_PassesWhenWritable(t *testing.T) {
	deps, key := fullRouterDeps(t)
	e := NewRouter(deps) // Writable=true
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/api-keys", strings.NewReader("{}"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("writable instance must pass writeGate, body=%s", rec.Body.String())
	}
}
