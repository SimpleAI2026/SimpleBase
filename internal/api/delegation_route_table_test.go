package api

// delegation_route_table_test.go：planv6.0 §3.1 委托白名单路由表驱动测试。
// 可委托集合的唯一事实来源是 router 挂载时的 delegable 标记；
// 本测试遍历真实 router 的全部路由，用委托令牌逐一探测，断言
// 「实际可委托集合」恰好等于计划 §3.1 的表。新增路由默认不可委托，
// 漏标 delegable 导致助手功能失效时，此测试会把差异列出来。

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// expectedDelegableRoutes 是 planv6.0 §3.1 表的可委托路由模板全集
// （method + 路由模板，:参数为占位符；相对 /v1/projects/:projectID）。
var expectedDelegableRoutes = map[string]bool{
	// database skill
	"POST /v1/projects/:projectID/databases":                                                          true,
	"GET /v1/projects/:projectID/databases":                                                           true,
	"GET /v1/projects/:projectID/databases/:databaseID":                                               true,
	"DELETE /v1/projects/:projectID/databases/:databaseID":                                            true,
	"POST /v1/projects/:projectID/databases/:databaseID/query":                                        true,
	"POST /v1/projects/:projectID/databases/:databaseID/execute":                                      true,
	"POST /v1/projects/:projectID/databases/:databaseID/batch":                                        true,
	"GET /v1/projects/:projectID/databases/:databaseID/schema":                                        true,
	"GET /v1/projects/:projectID/databases/:databaseID/schema/tables/:table/rows":                     true,
	"POST /v1/projects/:projectID/databases/:databaseID/schema/tables":                                true,
	"POST /v1/projects/:projectID/databases/:databaseID/schema/columns":                               true,
	"GET /v1/projects/:projectID/databases/:databaseID/data/collections":                              true,
	"POST /v1/projects/:projectID/databases/:databaseID/data/collections":                             true,
	"GET /v1/projects/:projectID/databases/:databaseID/data/collections/:collection":                  true,
	"POST /v1/projects/:projectID/databases/:databaseID/data/collections/:collection/import":          true,
	"DELETE /v1/projects/:projectID/databases/:databaseID/data/collections/:collection":               true,
	"POST /v1/projects/:projectID/databases/:databaseID/data/collections/:collection/documents":       true,
	"POST /v1/projects/:projectID/databases/:databaseID/data/collections/:collection/documents/batch": true,
	"GET /v1/projects/:projectID/databases/:databaseID/data/collections/:collection/documents/:id":    true,
	"PUT /v1/projects/:projectID/databases/:databaseID/data/collections/:collection/documents/:id":    true,
	"PATCH /v1/projects/:projectID/databases/:databaseID/data/collections/:collection/documents/:id":  true,
	"DELETE /v1/projects/:projectID/databases/:databaseID/data/collections/:collection/documents/:id": true,
	// kv skill
	"POST /v1/projects/:projectID/kv": true,
	// s3 skill
	"GET /v1/projects/:projectID/s3/objects":         true,
	"GET /v1/projects/:projectID/s3/objects/content": true,
	"POST /v1/projects/:projectID/s3/link":           true,
	"POST /v1/projects/:projectID/s3/objects":        true,
	"DELETE /v1/projects/:projectID/s3/objects":      true,
	"GET /v1/projects/:projectID/s3/presign":         true,
	// gofunction skill
	"GET /v1/projects/:projectID/gofunctions":                               true,
	"POST /v1/projects/:projectID/gofunctions":                              true,
	"GET /v1/projects/:projectID/gofunctions/:name":                         true,
	"PATCH /v1/projects/:projectID/gofunctions/:name":                       true,
	"DELETE /v1/projects/:projectID/gofunctions/:name":                      true,
	"GET /v1/projects/:projectID/gofunctions/:name/versions":                true,
	"POST /v1/projects/:projectID/gofunctions/:name/versions":               true,
	"GET /v1/projects/:projectID/gofunctions/:name/versions/:ver":           true,
	"POST /v1/projects/:projectID/gofunctions/:name/versions/:ver/activate": true,
	"POST /v1/projects/:projectID/gofunctions/:name/versions/:ver/test":     true,
	"POST /go/:projectID/:name/:functionName":                               true,
	// cron skill
	"GET /v1/projects/:projectID/cron-jobs":                 true,
	"POST /v1/projects/:projectID/cron-jobs":                true,
	"GET /v1/projects/:projectID/cron-jobs/:jobID":          true,
	"PATCH /v1/projects/:projectID/cron-jobs/:jobID":        true,
	"DELETE /v1/projects/:projectID/cron-jobs/:jobID":       true,
	"GET /v1/projects/:projectID/cron-jobs/:jobID/runs":     true,
	"POST /v1/projects/:projectID/cron-jobs/:jobID/trigger": true,
	// sandbox skill
	"GET /v1/projects/:projectID/sandboxes":               true,
	"POST /v1/projects/:projectID/sandboxes":              true,
	"POST /v1/projects/:projectID/sandboxes/run":          true,
	"GET /v1/projects/:projectID/sandboxes/:sandboxID":    true,
	"PATCH /v1/projects/:projectID/sandboxes/:sandboxID":  true,
	"DELETE /v1/projects/:projectID/sandboxes/:sandboxID": true,
	// logs skill（只读；retention 不可委托）
	"GET /v1/projects/:projectID/logs":        true,
	"GET /v1/projects/:projectID/logs/stats":  true,
	"GET /v1/projects/:projectID/logs/export": true,
	// project skill（只保留 quota/audit）
	"GET /v1/projects/:projectID/quota": true,
	"GET /v1/projects/:projectID/audit": true,
}

// probeDelegable 对每条真实路由发起委托令牌请求，判定是否被委托边界放行。
// 放行 = 未返回 403（委托拒绝发生在鉴权层；其余 4xx/5xx 属业务校验，不影响判定）。
func TestDelegation_RouteTable(t *testing.T) {
	env := newSkillRouterEnv(t)
	tok := env.delegationToken(t)

	var mismatches []string
	for _, r := range env.echo.Routes() {
		// 跳过非业务路由（metrics/pprof/静态资源）与 Echo 组前缀伪路由。
		if !strings.HasPrefix(r.Path, "/v1/") && !strings.HasPrefix(r.Path, "/go/") || r.Path == "/v1/openapi.json" {
			continue
		}
		// Echo 为路由组注册的 NotFound 伪路由，其 Method 字段即为 "echo_route_not_found"
		// （如兼容出口的 oa 组 /v1/projects/:projectID/v1）。
		if r.Name == "echo_route_not_found" || r.Method == "echo_route_not_found" ||
			strings.HasSuffix(r.Path, "/*") || strings.HasSuffix(r.Path, "/:projectID") {
			continue
		}
		path := materializeRoute(r.Path)
		req := httptest.NewRequest(r.Method, path, strings.NewReader("{}"))
		req.Header.Set(echoContentType, "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		env.echo.ServeHTTP(rec, req)
		key := r.Method + " " + r.Path
		allowed := rec.Code != http.StatusForbidden
		if allowed != expectedDelegableRoutes[key] {
			mismatches = append(mismatches, fmt.Sprintf("%s: want delegable=%v, got %d", key, expectedDelegableRoutes[key], rec.Code))
		}
	}
	if len(mismatches) > 0 {
		sort.Strings(mismatches)
		t.Fatalf("委托路由表与 planv6.0 §3.1 不一致（%d 处）：\n%s", len(mismatches), strings.Join(mismatches, "\n"))
	}
}

const echoContentType = "Content-Type"

// materializeRoute 把 :param 模板替换为测试值。
func materializeRoute(path string) string {
	out := path
	for _, kv := range [][2]string{
		{":projectID", "proj-1"},
		{":databaseID", "db-1"},
		{":collection", "c1"},
		{":id", "d1"},
		{":table", "t1"},
		{":name", "hello"},
		{":ver", "1"},
		{":jobID", "j1"},
		{":sandboxID", "s1"},
		{":functionName", "Hello"},
		{":userID", "u1"},
		{":keyID", "k1"},
		{":agentID", "a1"},
		{":threadID", "t1"},
		{":runID", "r1"},
		{":callID", "c1"},
		{":scheduleID", "s1"},
		{":version", "1"},
	} {
		out = strings.ReplaceAll(out, kv[0], kv[1])
	}
	return out
}

// delegationToken 为 env 签发 proj-1 的委托令牌（复用运行时同款 secret）。
func (env *skillRouterEnv) delegationToken(t *testing.T) string {
	t.Helper()
	p := auth.Principal{
		UserID: "u-1", Role: auth.RoleUser, TenantID: "tenant-1",
		ProjectIDs:  map[string]struct{}{"proj-1": {}},
		Permissions: auth.PermissionsForRole(auth.RoleUser),
	}
	tok, err := auth.IssueDelegation("test-secret", p, "proj-1", "run-table", time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}
