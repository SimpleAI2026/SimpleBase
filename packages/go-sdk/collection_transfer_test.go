package gosdk

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectionTransferStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/import"):
			if r.Header.Get("Content-Type") != "application/x-ndjson" || r.URL.Query().Get("dry_run") != "1" {
				t.Errorf("unexpected import request: %#v", r)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(ImportResult{DryRun: true, Scanned: 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/export"):
			if r.URL.Query().Get("format") != "csv" || r.URL.Query().Get("fields") != "id,title" {
				t.Errorf("unexpected export query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte("id,title\n1,one\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(Options{URL: server.URL, Token: "token", ProjectID: "project"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Database("db").ImportCollection(context.Background(), "tasks", "ndjson", true, strings.NewReader("{\"title\":\"one\"}\n"))
	if err != nil || !result.DryRun || result.Scanned != 1 {
		t.Fatalf("import result=%#v err=%v", result, err)
	}
	var output bytes.Buffer
	err = client.Database("db").ExportCollection(context.Background(), "tasks", ExportQuery{Format: "csv", Fields: []string{"id", "title"}}, &output)
	if err != nil || output.String() != "id,title\n1,one\n" {
		t.Fatalf("export=%q err=%v", output.String(), err)
	}
}
