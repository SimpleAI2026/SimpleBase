package gosdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMigrationStatusAndPlan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects/p/migrations/status":
			_, _ = w.Write([]byte(`{"format_version":1,"ready":true,"expected":1,"present":1,"missing":[],"tables":[]}`))
		case "/v1/projects/p/migrations/plan":
			_, _ = w.Write([]byte(`{"format_version":1,"pending":[],"statements":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(Options{URL: server.URL, Token: "t", ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.MigrationStatus(context.Background())
	if err != nil || !status.Ready {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	plan, err := client.MigrationPlan(context.Background())
	if err != nil || len(plan.Statements) != 0 {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
}
