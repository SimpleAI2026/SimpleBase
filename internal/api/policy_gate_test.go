package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

func TestPolicyGateResourcesAndExpiry(t *testing.T) {
	e := echo.New()
	called := false
	handler := policyGate(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	check := func(method, path string, policy *auth.KeyPolicy, status int) {
		req := httptest.NewRequest(method, path, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection")
		c.SetParamNames("projectID", "databaseID", "collection")
		c.SetParamValues("p1", "db1", "orders")
		c.SetRequest(req.WithContext(WithPrincipal(req.Context(), auth.Principal{Scope: policy})))
		called = false
		_ = handler(c)
		if rec.Code != status || called != (status == http.StatusOK) {
			t.Fatalf("%s %s: status=%d called=%v", method, path, rec.Code, called)
		}
	}
	readOrders := &auth.KeyPolicy{Resources: []string{"collection:orders"}, Actions: []string{"read"}}
	check(http.MethodGet, "/", readOrders, http.StatusOK)
	check(http.MethodPost, "/", readOrders, http.StatusForbidden)
	check(http.MethodGet, "/", &auth.KeyPolicy{Resources: []string{"collection:other"}, Actions: []string{"read"}}, http.StatusForbidden)
	expired := time.Now().Add(-time.Minute)
	check(http.MethodGet, "/", &auth.KeyPolicy{Resources: []string{"collection:*"}, Actions: []string{"read"}, ExpiresAt: &expired}, http.StatusForbidden)
}

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
			if err := (&RealtimeHandler{}).checkScope(c, splitRealtimeNames(c.QueryParam("collections"), true)); err != nil {
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

func TestScopeSubset(t *testing.T) {
	parent := &auth.KeyPolicy{Resources: []string{"collection:orders"}, Actions: []string{"read"}, HiddenFields: []string{"secret"}}
	if !scopeSubset(parent, parent) {
		t.Fatal("same scope rejected")
	}
	if scopeSubset(parent, &auth.KeyPolicy{Resources: []string{"collection:*"}, Actions: []string{"read"}}) {
		t.Fatal("wider resource accepted")
	}
	if scopeSubset(parent, &auth.KeyPolicy{Resources: []string{"collection:orders"}, Actions: []string{"read", "write"}}) {
		t.Fatal("wider action accepted")
	}
}
