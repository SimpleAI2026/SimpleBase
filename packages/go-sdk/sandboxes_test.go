package gosdk

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSandboxAPIProtocol(t *testing.T) {
	seen := map[string]bool{}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing auth")
		}
		if !strings.HasPrefix(r.URL.EscapedPath(), "/v1/projects/project%2Fone/sandboxes") {
			t.Errorf("escaped path: %s", r.URL.EscapedPath())
		}
		if r.URL.Path == "/v1/projects/project/one/sandboxes/run" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["command"] != "echo ok" || body["image"] != "alpine:3.20" {
				t.Errorf("run body: %+v", body)
			}
			_, _ = w.Write([]byte(`{"exit_code":0,"stdout":"ok\n"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/files/content") {
			if r.URL.Query().Get("path") != "/workspace/a b.txt" {
				t.Errorf("query: %s", r.URL.RawQuery)
			}
			seen["file"] = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPost {
			seen["create"] = true
			_, _ = w.Write([]byte(`{"id":"abc","status":"pending"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	ctx := context.Background()
	sb, err := c.CreateSandbox(ctx, CreateSandboxInput{Name: "check"})
	if err != nil || sb.ID != "abc" {
		t.Fatalf("create: %+v %v", sb, err)
	}
	if err = c.WriteSandboxFile(ctx, "abc", "/workspace/a b.txt", "hello"); err != nil {
		t.Fatal(err)
	}
	result, err := c.RunSandbox(ctx, SandboxRunInput{SandboxExecInput: SandboxExecInput{Command: "echo ok"}, Image: "alpine:3.20"})
	if err != nil || result.Stdout != "ok\n" {
		t.Fatalf("run: %+v %v", result, err)
	}
	if !seen["create"] || !seen["file"] {
		t.Fatalf("calls: %+v", seen)
	}
}

func TestSandboxListPagination(t *testing.T) {
	var queries []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"sandboxes":[{"id":"a"}],"next_cursor":"a"}`))
			return
		}
		_, _ = w.Write([]byte(`{"sandboxes":[{"id":"b"}]}`))
	})
	ctx := context.Background()
	page, err := c.ListSandboxesPage(ctx, SandboxListOptions{Status: "running", Source: "api", Limit: 1})
	if err != nil || len(page.Sandboxes) != 1 || page.NextCursor != "a" {
		t.Fatalf("page1: %+v %v", page, err)
	}
	page, err = c.ListSandboxesPage(ctx, SandboxListOptions{Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(page.Sandboxes) != 1 || page.Sandboxes[0].ID != "b" || page.NextCursor != "" {
		t.Fatalf("page2: %+v %v", page, err)
	}
	list, err := c.ListSandboxes(ctx, "", "", 0)
	if err != nil || len(list) != 1 || list[0].ID != "a" {
		t.Fatalf("list: %+v %v", list, err)
	}
	want := []string{"limit=1&source=api&status=running", "cursor=a&limit=1", ""}
	for i, q := range want {
		if queries[i] != q {
			t.Fatalf("query %d = %q, want %q", i, queries[i], q)
		}
	}
}
