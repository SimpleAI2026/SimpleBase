// 文件说明：realtime_scope_test.go — 策略闸门对实时订阅的集合范围约束（自 internal/api/policy_gate_test.go 随实现迁入）

package api

import (
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/api/datahttp"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

func TestPolicyGateRealtimeDefersCollectionScope(t *testing.T) {
	for _, test := range []struct {
		collection string
		status     int
	}{{"orders", http.StatusOK}, {"other", http.StatusForbidden}} {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/v1/projects/p1/realtime?collections="+test.collection, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/projects/:projectID/realtime")
		c.SetRequest(req.WithContext(WithPrincipal(req.Context(), auth.Principal{Scope: &auth.KeyPolicy{
			Resources: []string{"collection:orders"}, Actions: []string{"read"},
		}})))
		handler := policyGate(func(c echo.Context) error {
			if err := (&datahttp.RealtimeHandler{}).CheckScope(c, datahttp.SplitRealtimeNames(c.QueryParam("collections"), true)); err != nil {
				return WriteError(c, err)
			}
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		if rec.Code != test.status {
			t.Fatalf("collection %s: status=%d want %d", test.collection, rec.Code, test.status)
		}
	}
}
