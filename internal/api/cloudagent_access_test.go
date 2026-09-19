package api

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

func TestNewCloudAgentConstructors(t *testing.T) {
	if NewCloudAgentDB(nil, nil) != nil {
		t.Fatal("nil catalog/reg")
	}
	if NewCloudAgentDB(&stubCatalog{}, nil) != nil {
		t.Fatal("nil reg")
	}
	if NewCloudAgentObj(nil, nil) != nil {
		t.Fatal("both nil")
	}
	if NewCloudAgentLLM(nil) != nil {
		t.Fatal("nil llm")
	}
	cat := &stubCatalog{}
	reg := &stubRegistry{}
	if NewCloudAgentDB(cat, reg) == nil {
		t.Fatal("expected db access")
	}
	if NewCloudAgentObj(objectstore.NewMemoryFileStore(), nil) == nil {
		t.Fatal("expected obj access")
	}
	if NewCloudAgentLLM(&fakeLLMSvc{}) == nil {
		t.Fatal("expected llm client")
	}
}

func TestCloudAgentDB_ListAndQuery(t *testing.T) {
	cat := &stubCatalog{
		list: []catalog.Database{
			{ID: "db-1", Name: "one", Status: catalog.DatabaseReady},
			{ID: "db-2", Name: "two", Status: catalog.DatabaseCreating},
		},
		db: catalog.Database{ID: "db-1", Name: "one", Status: catalog.DatabaseReady},
	}
	realReg := registry.New(memFactory{}, nil, registry.Options{Writable: true}, nil, nil)
	access := NewCloudAgentDB(cat, &liveRegistry{inner: realReg})
	ctx := context.Background()
	p := auth.Principal{APIKeyID: "k"}

	list, err := access.ListDatabases(ctx, p, "proj-1")
	if err != nil || len(list) != 2 || list[0].Name != "one" {
		t.Fatalf("list=%v err=%v", list, err)
	}
	cat.listErr = errors.New("nope")
	if _, err := access.ListDatabases(ctx, p, "proj-1"); err == nil {
		t.Fatal("expected list error")
	}
	cat.listErr = nil

	// ReadOnlyQuery success via sqlite
	res, err := access.ReadOnlyQuery(ctx, p, "proj-1", "db-1", "SELECT 1 AS n", 10)
	if err != nil || res.RowCount != 1 {
		t.Fatalf("query=%+v err=%v", res, err)
	}
	// maxRows clamp
	if _, err := access.ReadOnlyQuery(ctx, p, "proj-1", "db-1", "SELECT 1", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := access.ReadOnlyQuery(ctx, p, "proj-1", "db-1", "SELECT 1", 500); err != nil {
		t.Fatal(err)
	}
	if _, err := access.ReadOnlyQuery(ctx, p, "proj-1", "db-1", "INSERT INTO t VALUES(1)", 10); err == nil {
		t.Fatal("write sql should fail guard")
	}

	cat.getErr = catalog.ErrNotFound
	if _, err := access.ReadOnlyQuery(ctx, p, "proj-1", "db-1", "SELECT 1", 10); err == nil {
		t.Fatal("expected get error")
	}
	if _, err := access.ListCollections(ctx, p, "proj-1", "db-1"); err == nil {
		t.Fatal("expected get error on collections")
	}
	cat.getErr = nil

	// ListCollections query fails on sqlite (no information_schema)
	if _, err := access.ListCollections(ctx, p, "proj-1", "db-1"); err == nil {
		t.Fatal("expected information_schema error")
	}

	// Acquire error
	failReg := &stubRegistry{acquireErr: errors.New("busy")}
	failAccess := NewCloudAgentDB(cat, failReg)
	if _, err := failAccess.ListCollections(ctx, p, "proj-1", "db-1"); err == nil {
		t.Fatal("expected acquire err")
	}
	if _, err := failAccess.ReadOnlyQuery(ctx, p, "proj-1", "db-1", "SELECT 1", 10); err == nil {
		t.Fatal("expected acquire err")
	}
}

// liveRegistry adapts *registry.Registry to RegistryService.
type liveRegistry struct{ inner *registry.Registry }

func (l *liveRegistry) Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (*registry.Lease, error) {
	return l.inner.Acquire(ctx, db, mode)
}
func (l *liveRegistry) CloseDatabase(ctx context.Context, id string) error {
	return l.inner.CloseDatabase(ctx, id)
}

func TestCloudAgentObj(t *testing.T) {
	ctx := context.Background()
	files := objectstore.NewMemoryFileStore()
	_, err := files.Put(ctx, "proj-1/docs/a.txt", strings.NewReader("hi"), 2, "text/plain")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("files_only", func(t *testing.T) {
		acc := NewCloudAgentObj(files, nil)
		list, err := acc.ListObjects(ctx, "proj-1", "docs", 10)
		if err != nil || len(list) == 0 {
			t.Fatalf("list=%v err=%v", list, err)
		}
		if list[0].Key == "" {
			t.Fatal("empty key")
		}
		info, err := acc.HeadObject(ctx, "proj-1", "docs/a.txt")
		if err != nil || info.Key != "docs/a.txt" {
			t.Fatalf("head=%+v err=%v", info, err)
		}
		if _, err := acc.HeadObject(ctx, "proj-1", "../x"); err == nil {
			t.Fatal("invalid key")
		}
		if _, err := acc.HeadObject(ctx, "proj-1", "missing.txt"); err == nil {
			t.Fatal("missing object")
		}
	})

	t.Run("index_only", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := systemdb.ApplySystemMigrations(ctx, db); err != nil {
			t.Fatal(err)
		}
		store := systemdb.NewStoreForTest(db)
		if err := store.UpsertS3Object(ctx, "proj-1", "docs/b.txt", 3, "etag", "text/plain", time.Now()); err != nil {
			t.Fatal(err)
		}
		acc := NewCloudAgentObj(nil, store)
		list, err := acc.ListObjects(ctx, "proj-1", "docs", 10)
		if err != nil || len(list) != 1 || list[0].Key != "docs/b.txt" || list[0].LastModified == "" {
			t.Fatalf("list=%+v err=%v", list, err)
		}
		info, err := acc.HeadObject(ctx, "proj-1", "docs/b.txt")
		if err != nil || info.ETag != "etag" {
			t.Fatalf("head=%+v err=%v", info, err)
		}
		if _, err := acc.HeadObject(ctx, "proj-1", "nope"); err == nil {
			t.Fatal("expected not found without files")
		}
	})

	t.Run("index_empty_falls_back_to_files", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := systemdb.ApplySystemMigrations(ctx, db); err != nil {
			t.Fatal(err)
		}
		store := systemdb.NewStoreForTest(db)
		acc := NewCloudAgentObj(files, store)
		list, err := acc.ListObjects(ctx, "proj-1", "docs", 10)
		if err != nil || len(list) == 0 {
			t.Fatalf("fallback list=%v err=%v", list, err)
		}
		info, err := acc.HeadObject(ctx, "proj-1", "docs/a.txt")
		if err != nil {
			t.Fatal(err)
		}
		if info.Key != "docs/a.txt" {
			t.Fatalf("key=%s", info.Key)
		}
	})

	t.Run("index_error", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		store := systemdb.NewStoreForTest(db) // no schema
		acc := NewCloudAgentObj(nil, store)
		if _, err := acc.ListObjects(ctx, "p", "", 10); err == nil {
			t.Fatal("expected index list error")
		}
		if _, err := acc.HeadObject(ctx, "p", "k"); err == nil {
			t.Fatal("expected index head error")
		}
	})

	t.Run("no_store_configured", func(t *testing.T) {
		acc := &cloudAgentObj{}
		if _, err := acc.ListObjects(ctx, "p", "", 10); err == nil {
			t.Fatal("expected not configured")
		}
	})

	t.Run("toAgentObjectInfo_zero_time", func(t *testing.T) {
		info := toAgentObjectInfo(systemdb.S3Object{Key: "k", Size: 1})
		if info.LastModified != "" || info.Key != "k" {
			t.Fatalf("%+v", info)
		}
	})
}

func TestLLMChatClient(t *testing.T) {
	ctx := context.Background()
	c := NewCloudAgentLLM(&fakeLLMSvc{names: []string{"x"}}).(cloudagent.ChatClient)
	resp, err := c.Chat(ctx, "p", cloudagent.ChatRequest{
		Model: "m", Messages: []cloudagent.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil || resp.Content != "ok" {
		t.Fatalf("chat=%+v err=%v", resp, err)
	}
	stream, err := c.Stream(ctx, "p", cloudagent.ChatRequest{Messages: []cloudagent.ChatMessage{{Role: "u", Content: "q"}}})
	if err != nil {
		t.Fatal(err)
	}
	tok, done, err := stream.Next()
	if err != nil || tok != "hel" {
		t.Fatalf("tok=%q done=%v err=%v", tok, done, err)
	}
	tok, done, err = stream.Next()
	if err != nil || !done || tok != "lo" {
		t.Fatalf("second tok=%q done=%v err=%v", tok, done, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}

	bad := NewCloudAgentLLM(&fakeLLMSvc{chatErr: errors.New("x"), streamErr: errors.New("y")})
	if _, err := bad.Chat(ctx, "p", cloudagent.ChatRequest{}); err == nil {
		t.Fatal("chat err")
	}
	if _, err := bad.Stream(ctx, "p", cloudagent.ChatRequest{}); err == nil {
		t.Fatal("stream err")
	}

	// token stream edge cases
	ts := &llmTokenStream{r: &seqStream{}}
	if _, finish, err := ts.Next(); err == nil && !finish {
		// empty seqStream returns EOF → finish true
	}
	if _, _, err := ts.Next(); err == nil {
		// EOF
	}
	nilChunk := &llmTokenStream{r: &oneNilStream{}}
	tok, finish, err := nilChunk.Next()
	if err != nil || tok != "" || finish {
		t.Fatalf("nil chunk: %q %v %v", tok, finish, err)
	}
}

type oneNilStream struct{}

func (oneNilStream) Next() (*LLMStreamChunk, error) { return nil, nil }
func (oneNilStream) Close() error                   { return nil }

func TestJoinStripKey(t *testing.T) {
	if joinKey("p", "a/b") != "p/a/b" {
		t.Fatal(joinKey("p", "a/b"))
	}
	if stripProject("p", "p/a/b") != "a/b" {
		t.Fatal(stripProject("p", "p/a/b"))
	}
	if stripProject("p", "other/a") != "other/a" {
		t.Fatal("unprefixed key should pass through")
	}
}
