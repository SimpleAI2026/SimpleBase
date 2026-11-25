// 文件说明：access_test.go — internal/agentbridge 的单元测试（TestNewCloudAgentConstructors、TestCloudAgentDB_ListAndQuery 等）
//
// 用法
//
//	go test ./internal/agentbridge -run 'TestNewCloudAgentConstructors|TestCloudAgentDB_ListAndQuery|TestCloudAgentObj|TestLLMChatClient'
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/access_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, bridge_test.go, cli.go 等 33 个
//
// 主要符号
//
//	type stubCatalog struct{…}
//	(s *stubCatalog) GetDatabase(context.Context, auth.Principal, string, string) (catalog.Database, error)
//	(s *stubCatalog) ListDatabases(context.Context, auth.Principal, string, catalog.Page) ([]catalog.Database, str...
//	type stubOpener struct{…}
//	(s *stubOpener) Acquire(context.Context, catalog.Database, godatabase.AccessMode) (*registry.Lease, error)
//	type liveOpener struct{…}
//	(l *liveOpener) Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (*registry.Lease...
//	type memFactory struct{…}
//	... 另有 21 个顶层符号

package agentbridge

// access_test.go：宿主数据库 / 对象 / 模型访问适配的用例
// （自 internal/api/gocloudagent_access_test.go 随实现迁入，夹具改为本包内的最小假实现）。

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/voocel/litellm/providers"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

// stubCatalog 只实现 CatalogReader 需要的两个查询。
type stubCatalog struct {
	db      catalog.Database
	list    []catalog.Database
	getErr  error
	listErr error
}

func (s *stubCatalog) GetDatabase(context.Context, auth.Principal, string, string) (catalog.Database, error) {
	if s.getErr != nil {
		return catalog.Database{}, s.getErr
	}
	return s.db, nil
}

func (s *stubCatalog) ListDatabases(context.Context, auth.Principal, string, catalog.Page) ([]catalog.Database, string, error) {
	if s.listErr != nil {
		return nil, "", s.listErr
	}
	return s.list, "next", nil
}

// stubOpener 实现 DatabaseOpener；acquireErr 非空时模拟取句柄失败。
type stubOpener struct {
	acquireErr error
}

func (s *stubOpener) Acquire(context.Context, catalog.Database, godatabase.AccessMode) (*registry.Lease, error) {
	if s.acquireErr != nil {
		return nil, s.acquireErr
	}
	return nil, nil
}

// liveOpener 把 *registry.Registry 适配为 DatabaseOpener（同 dbbridge.Registry）。
type liveOpener struct{ inner *registry.Registry }

func (l *liveOpener) Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (*registry.Lease, error) {
	return l.inner.Acquire(ctx, dbbridge.SpecOf(db), mode)
}

type memFactory struct{}

func (memFactory) Open(_ context.Context, _ godatabase.Spec, _ godatabase.AccessMode) (*sql.DB, error) {
	return sql.Open("sqlite3", ":memory:")
}

func TestNewCloudAgentConstructors(t *testing.T) {
	if NewCloudAgentDB(nil, nil) != nil {
		t.Fatal("nil catalog/opener")
	}
	if NewCloudAgentDB(&stubCatalog{}, nil) != nil {
		t.Fatal("nil opener")
	}
	if NewCloudAgentObj(nil, nil) != nil {
		t.Fatal("both nil")
	}
	if NewCloudAgentLLM(nil) != nil {
		t.Fatal("nil llm")
	}
	// typed-nil 进接口：必须按空处理，不能返回一个持有 nil 依赖的适配器。
	var nilStore *systemdb.Store
	if NewCloudAgentObj(nil, nilStore) != nil {
		t.Fatal("typed-nil index must be treated as empty")
	}
	cat := &stubCatalog{}
	if NewCloudAgentDB(cat, &stubOpener{}) == nil {
		t.Fatal("expected db access")
	}
	if NewCloudAgentObj(objectstore.NewMemoryFileStore(), nil) == nil {
		t.Fatal("expected obj access")
	}
	if NewCloudAgentLLM(&fakeGateway{}) == nil {
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
	access := NewCloudAgentDB(cat, &liveOpener{inner: realReg})
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
	// SQL 只读守卫：写语句必须在取句柄之前被拒绝。
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
	failAccess := NewCloudAgentDB(cat, &stubOpener{acquireErr: errors.New("busy")})
	if _, err := failAccess.ListCollections(ctx, p, "proj-1", "db-1"); err == nil {
		t.Fatal("expected acquire err")
	}
	if _, err := failAccess.ReadOnlyQuery(ctx, p, "proj-1", "db-1", "SELECT 1", 10); err == nil {
		t.Fatal("expected acquire err")
	}
}

func TestCloudAgentObj(t *testing.T) {
	ctx := context.Background()
	files := objectstore.NewMemoryFileStore()
	// 物理布局：{projectID}/files/{userKey}（joinKey 负责拼 files 段）。
	_, err := files.Put(ctx, "proj-1/files/docs/a.txt", strings.NewReader("hi"), 2, "text/plain")
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
		store := openIndexStore(t, true)
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
		acc := NewCloudAgentObj(files, openIndexStore(t, true))
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
		acc := NewCloudAgentObj(nil, openIndexStore(t, false)) // 未建 schema：查询必须报错而不是回退
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
	var c cloudagent.ChatClient = NewCloudAgentLLM(&fakeGateway{})
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

	bad := NewCloudAgentLLM(&fakeGateway{chatErr: errors.New("x"), streamErr: errors.New("y")})
	if _, err := bad.Chat(ctx, "p", cloudagent.ChatRequest{}); err == nil {
		t.Fatal("chat err")
	}
	if _, err := bad.Stream(ctx, "p", cloudagent.ChatRequest{}); err == nil {
		t.Fatal("stream err")
	}

	// token stream edge cases：空 seqStream 返回 EOF，finish 必须为 true。
	ts := &llmTokenStream{r: &seqStream{}}
	if _, finish, err := ts.Next(); err != io.EOF || !finish {
		t.Fatalf("empty stream: finish=%v err=%v", finish, err)
	}
	if _, finish, err := ts.Next(); err != io.EOF || !finish {
		t.Fatalf("empty stream again: finish=%v err=%v", finish, err)
	}
	nilChunk := &llmTokenStream{r: &oneNilStream{}}
	tok, finish, err := nilChunk.Next()
	if err != nil || tok != "" || finish {
		t.Fatalf("nil chunk: %q %v %v", tok, finish, err)
	}
}

// 请求 / 增量帧的双向映射：工具定义、工具调用增量与用量都要原样过网关形状。
func TestLLMChatClient_RequestAndDeltaMapping(t *testing.T) {
	gw := &fakeGateway{reader: &seqStream{chunks: []*providers.StreamChunk{
		{Type: "tool_call_delta", ToolCallDelta: &providers.ToolCallDelta{Index: 1, ID: "call-1", FunctionName: "lookup", ArgumentsDelta: `{"a"`}},
		{Type: "delta", Content: "x", Usage: &providers.Usage{PromptTokens: 4, CompletionTokens: 2, ReasoningTokens: 1}, FinishReason: "tool_calls"},
	}}}
	c := NewCloudAgentLLM(gw)
	_, err := c.Chat(context.Background(), "p", cloudagent.ChatRequest{
		Model:       "m",
		MaxTokens:   intPtr(9),
		Temperature: floatPtr(0.2),
		Messages: []cloudagent.ChatMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", ToolCalls: []cloudagent.ChatToolCall{{ID: "c0", Name: "f", Arguments: "{}"}}},
			{Role: "tool", ToolCallID: "c0", Content: "result"},
		},
		Tools: []agentcore.ToolSpec{{Name: "f", Description: "d", Parameters: []byte(`{"type":"object"}`)}, {Name: "bare"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := gw.lastReq
	if req.Model != "m" || req.MaxTokens == nil || *req.MaxTokens != 9 || req.Temperature == nil || *req.Temperature != 0.2 || req.ToolChoice != nil {
		t.Fatalf("request scalars = %+v", req)
	}
	if len(req.Messages) != 3 || req.Messages[1].ToolCalls[0].Function.Name != "f" || req.Messages[1].ToolCalls[0].Type != "function" || req.Messages[2].ToolCallID != "c0" {
		t.Fatalf("messages = %+v", req.Messages)
	}
	if len(req.Tools) != 2 || req.Tools[0].Function.Name != "f" || req.Tools[0].Type != "function" {
		t.Fatalf("tools = %+v", req.Tools)
	}
	if bare, ok := req.Tools[1].Function.Parameters.(map[string]any); !ok || bare["type"] != "object" {
		t.Fatalf("tool without parameters must get an empty object schema, got %#v", req.Tools[1].Function.Parameters)
	}

	stream, err := c.Stream(context.Background(), "p", cloudagent.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	delta, err := stream.(agentcore.DeltaStream).NextDelta()
	if err != nil || delta.ToolCall == nil || delta.ToolCall.Index != 1 || delta.ToolCall.ID != "call-1" || delta.ToolCall.Name != "lookup" || delta.ToolCall.ArgsDelta != `{"a"` || delta.Finish {
		t.Fatalf("tool delta = %+v err=%v", delta, err)
	}
	delta, err = stream.(agentcore.DeltaStream).NextDelta()
	if err != nil || delta.Content != "x" || delta.Usage == nil || delta.Usage.PromptTokens != 4 || delta.Usage.CompletionTokens != 2 || delta.Usage.ReasoningTokens != 1 || delta.Finish {
		t.Fatalf("usage delta = %+v err=%v", delta, err)
	}
	// 非流式的工具调用：网关的 providers.ToolCall → Agent 的扁平 ChatToolCall。
	gw.resp = llmgateway.Response{Content: "", ToolCalls: []providers.ToolCall{{ID: "tc1", Type: "function", Function: providers.FunctionCall{Name: "g", Arguments: "[]"}}}}
	resp, err := c.Chat(context.Background(), "p", cloudagent.ChatRequest{})
	if err != nil || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "g" || resp.ToolCalls[0].ID != "tc1" {
		t.Fatalf("chat tool calls = %+v err=%v", resp, err)
	}
}

func TestJoinStripKey(t *testing.T) {
	// 物理布局：{projectID}/files/{userKey}。
	if joinKey("p", "a/b") != "p/files/a/b" {
		t.Fatal(joinKey("p", "a/b"))
	}
	if joinKey("p", "") != "p/files" {
		t.Fatal(joinKey("p", ""))
	}
	if stripProject("p", "p/files/a/b") != "a/b" {
		t.Fatal(stripProject("p", "p/files/a/b"))
	}
	if stripProject("p", "p/files") != "" {
		t.Fatal(stripProject("p", "p/files"))
	}
	if stripProject("p", "other/a") != "other/a" {
		t.Fatal("unprefixed key should pass through")
	}
}

// ---- 夹具 ----

// openIndexStore 打开内存系统库；withSchema=false 时故意不建表，用来制造索引查询错误。
func openIndexStore(t *testing.T, withSchema bool) *systemdb.Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if withSchema {
		if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	return systemdb.NewStoreForTest(db)
}

// fakeGateway 是 llmgateway.Service 的最小假实现（网关形状：providers.*）。
type fakeGateway struct {
	chatErr   error
	streamErr error
	resp      llmgateway.Response
	reader    llmgateway.StreamReader
	lastReq   llmgateway.Request
}

func (f *fakeGateway) Chat(_ context.Context, _ string, req llmgateway.Request) (llmgateway.Response, error) {
	f.lastReq = req
	if f.chatErr != nil {
		return llmgateway.Response{}, f.chatErr
	}
	if f.resp.Content == "" && len(f.resp.ToolCalls) == 0 {
		return llmgateway.Response{Content: "ok", Model: "m", Provider: "p", FinishReason: "stop", Usage: llmgateway.Usage{PromptTokens: 2, CompletionTokens: 3}}, nil
	}
	return f.resp, nil
}

func (f *fakeGateway) Stream(_ context.Context, _ string, req llmgateway.Request) (llmgateway.StreamReader, error) {
	f.lastReq = req
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	if f.reader != nil {
		return f.reader, nil
	}
	return &seqStream{chunks: []*providers.StreamChunk{{Type: "delta", Content: "hel"}, {Type: "delta", Content: "lo", FinishReason: "stop"}}}, nil
}

func (f *fakeGateway) ListProviders(context.Context, string) ([]string, error) { return nil, nil }

func (f *fakeGateway) ProviderModels(context.Context, string) (map[string][]string, error) {
	return nil, nil
}

type seqStream struct {
	chunks []*providers.StreamChunk
	i      int
}

func (s *seqStream) Next() (*providers.StreamChunk, error) {
	if s.i >= len(s.chunks) {
		return nil, io.EOF
	}
	c := s.chunks[s.i]
	s.i++
	return c, nil
}
func (s *seqStream) Close() error { return nil }

type oneNilStream struct{}

func (oneNilStream) Next() (*providers.StreamChunk, error) { return nil, nil }
func (oneNilStream) Close() error                          { return nil }

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
