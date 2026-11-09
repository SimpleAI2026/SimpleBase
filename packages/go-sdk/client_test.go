package gosdk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	c, err := NewClient(Options{URL: s.URL, APIKey: "secret", ProjectID: "project/one", DatabaseID: "db/one"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNewClientValidation(t *testing.T) {
	for _, endpoint := range []string{"", "file:///tmp/a", "http://example.com/path", "http://user@example.com", "http://example.com?x=1"} {
		if _, err := NewClient(Options{URL: endpoint, APIKey: "a", ProjectID: "p"}); err == nil {
			t.Errorf("accepted URL %q", endpoint)
		}
	}
	if _, err := NewClient(Options{URL: "http://localhost", ProjectID: "p"}); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestDatabaseAndSQLProtocol(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Accept") != "application/json" {
			t.Error("missing auth/accept")
		}
		if r.URL.EscapedPath() != "/v1/projects/project%2Fone/databases/db%2Fone/query" {
			t.Errorf("path: %s", r.URL.EscapedPath())
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing JSON content type")
		}
		var payload struct {
			SQL     string `json:"sql"`
			Args    []any  `json:"args"`
			MaxRows int    `json:"max_rows"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.SQL != "SELECT ?" || len(payload.Args) != 1 || payload.MaxRows != 5 {
			t.Errorf("payload: %+v", payload)
		}
		io.WriteString(w, `{"columns":["value"],"rows":[[42]],"row_count":1}`)
	})
	result, err := c.Query(context.Background(), "SELECT ?", []any{42}, 5)
	if err != nil || result.RowCount != 1 {
		t.Fatalf("query: %+v, %v", result, err)
	}
}

func TestDatabaseAndDocumentOperations(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects/project/one/databases":
			if r.Method == http.MethodGet {
				if r.URL.Query().Get("limit") != "25" || r.URL.Query().Get("cursor") != "next cursor" {
					t.Error("pagination")
				}
				io.WriteString(w, `{"databases":[]}`)
			} else {
				w.WriteHeader(http.StatusCreated)
				io.WriteString(w, `{"id":"db"}`)
			}
		case "/v1/projects/project/one/databases/db/one/data/collections":
			w.WriteHeader(http.StatusCreated)
		case "/v1/projects/project/one/databases/db/one/data/collections/Books/documents/doc/1":
			if r.URL.EscapedPath() != "/v1/projects/project%2Fone/databases/db%2Fone/data/collections/Books/documents/doc%2F1" {
				t.Error("document ID was not encoded")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected path %s (%s)", r.URL.EscapedPath(), r.Method)
		}
	})
	if _, err := c.ListDatabases(context.Background(), 25, "next cursor"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateDatabase(context.Background(), "db"); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateCollection(context.Background(), "Books"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteDocument(context.Background(), "Books", "doc/1"); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateCollection(context.Background(), "_invalid"); err == nil {
		t.Fatal("invalid name accepted")
	}
	if _, err := c.Database("").Query(context.Background(), "SELECT 1", nil, 0); err == nil {
		t.Fatal("missing database ID accepted")
	}
}

func TestAPIErrorAndBatchFailure(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/batch") {
			io.WriteString(w, `{"results":[],"error":{"failed_index":0,"code":"invalid_sql","message":"rolled back"}}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"error":{"code":"forbidden","message":"denied","request_id":"rid-1"}}`)
	})
	_, err := c.GetDatabase(context.Background(), "bad")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 403 || apiErr.Code != "forbidden" || apiErr.RequestID != "rid-1" {
		t.Fatalf("error: %v", err)
	}
	batch, err := c.Batch(context.Background(), []SQLStatement{{SQL: "INSERT INTO t VALUES (?)", Args: []any{1}}}, true)
	if err != nil || batch.Error == nil || batch.Error.FailedIndex != 0 {
		t.Fatalf("batch: %+v %v", batch, err)
	}
}

func TestCancellationAndOversizedError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, strings.Repeat("sensitive", maxErrorBytes))
	})
	_, err := c.GetDatabase(context.Background(), "bad")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("unsafe error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.GetDatabase(ctx, "bad")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestDeleteAndDocumentCRUD(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/databases/to-delete"):
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"database_id":"to-delete","status":"deleted"}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/documents"):
			var doc Document
			if err := json.NewDecoder(r.Body).Decode(&doc); err != nil || doc["title"] != "hello" {
				t.Errorf("insert: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":"doc-1","title":"hello"}`)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/documents/doc-1"):
			io.WriteString(w, `{"id":"doc-1","title":"updated"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/data/collections/Notes"):
			io.WriteString(w, `{"rows":[{"id":"doc-1"}]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/data/collections"):
			io.WriteString(w, `{"collections":["Notes"]}`)
		default:
			t.Errorf("unexpected operation: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()
	deleted, err := c.DeleteDatabase(ctx, "to-delete")
	if err != nil || deleted.Status != "deleted" {
		t.Fatalf("delete: %+v %v", deleted, err)
	}
	if cols, err := c.ListCollections(ctx); err != nil || len(cols.Collections) != 1 {
		t.Fatalf("collections: %+v %v", cols, err)
	}
	if docs, err := c.ListDocuments(ctx, "Notes"); err != nil || len(docs.Rows) != 1 {
		t.Fatalf("documents: %+v %v", docs, err)
	}
	doc, err := c.InsertDocument(ctx, "Notes", Document{"title": "hello"})
	if err != nil || doc["id"] != "doc-1" {
		t.Fatalf("insert: %+v %v", doc, err)
	}
	doc, err = c.UpdateDocument(ctx, "Notes", "doc-1", Document{"title": "updated"})
	if err != nil || doc["title"] != "updated" {
		t.Fatalf("update: %+v %v", doc, err)
	}
}

func TestStorageQueryAndStreamingUpload(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("prefix") != "dir /" || r.URL.Query().Get("refresh") != "1" {
				t.Error("query:", r.URL.RawQuery)
			}
			io.WriteString(w, `[]`)
			return
		}
		if r.Header.Get("Content-Type") == "" || !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Error("content type")
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		key, _ := io.ReadAll(part)
		if part.FormName() != "key" || string(key) != "dir/file.txt" {
			t.Error("multipart key")
		}
		part, err = reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		contents, _ := io.ReadAll(part)
		if part.FormName() != "file" || part.FileName() != "file.txt" || part.Header.Get("Content-Type") != "text/plain" || string(contents) != "hello" {
			t.Error("multipart file")
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"key":"dir/file.txt","size":5}`)
	})
	if _, err := c.ListObjects(context.Background(), "dir /", true); err != nil {
		t.Fatal(err)
	}
	obj, err := c.UploadObject(context.Background(), "dir/file.txt", strings.NewReader("hello"), "", "text/plain")
	if err != nil || obj.Key != "dir/file.txt" {
		t.Fatalf("upload: %+v %v", obj, err)
	}
	if _, err := c.PresignObject(context.Background(), "../secret"); err == nil {
		t.Fatal("invalid key accepted")
	}
}

func TestStorageDeleteAndPresign(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "dir/a b.txt" {
			t.Error("object key was not encoded")
		}
		switch r.Method {
		case http.MethodGet:
			io.WriteString(w, `{"url":"https://download.example/object"}`)
		case http.MethodDelete:
			io.WriteString(w, `{"ok":true}`)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	link, err := c.PresignObject(context.Background(), "dir/a b.txt")
	if err != nil || link.URL == "" {
		t.Fatalf("presign: %+v %v", link, err)
	}
	deleted, err := c.DeleteObject(context.Background(), "dir/a b.txt")
	if err != nil || !deleted.OK {
		t.Fatalf("delete: %+v %v", deleted, err)
	}
}
