package llmgateway

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/voocel/litellm/providers"
	_ "github.com/uglyer/go-sqlite3"
)

func openaiCompatServer(t *testing.T, fail bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "provider down", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if stream, _ := body["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"1","object":"chat.completion","model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`)
	}))
}

func testProviders(baseURL string) ProjectProviders {
	return ProjectProviders{
		Default: "openai",
		Providers: []ProviderConfig{
			{Name: "openai", APIKey: "sk-test", BaseURL: baseURL, Model: "gpt-4o-mini"},
		},
	}
}

func TestClientKeyAndGetClientBranches(t *testing.T) {
	if clientKey("p", "openai") != "p|openai" {
		t.Fatal(clientKey("p", "openai"))
	}
	svc := NewService(&fakeResolver{err: errors.New("catalog down")}, nil, nil).(*service)
	if _, _, err := svc.getClient(context.Background(), "p", ""); err == nil {
		t.Fatal("resolve error")
	}
	if _, err := svc.Chat(context.Background(), "p", Request{}); err == nil {
		t.Fatal("chat resolve")
	}
	if _, err := svc.Stream(context.Background(), "p", Request{}); err == nil {
		t.Fatal("stream resolve")
	}

	svc = NewService(&fakeResolver{providers: ProjectProviders{
		Default:   "missing",
		Providers: []ProviderConfig{{Name: "openai", APIKey: "k"}},
	}}, nil, nil).(*service)
	if _, _, err := svc.getClient(context.Background(), "p", ""); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("not found: %v", err)
	}

	svc = NewService(&fakeResolver{providers: ProjectProviders{
		Providers: []ProviderConfig{{Name: "openai", APIKey: ""}},
	}}, nil, nil).(*service)
	if _, _, err := svc.getClient(context.Background(), "p", ""); err == nil {
		t.Fatal("empty api key should fail create")
	}

	svc = NewService(&fakeResolver{providers: ProjectProviders{
		Providers: []ProviderConfig{{Name: "not-a-real-provider", APIKey: "k"}},
	}}, nil, nil).(*service)
	if _, _, err := svc.getClient(context.Background(), "p", "not-a-real-provider"); err == nil {
		t.Fatal("unknown provider")
	}
}

func TestChatAndStreamWithLocalProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	srv := openaiCompatServer(t, false)
	defer srv.Close()

	rec := &countingRecorder{}
	var logBuf bytes.Buffer
	logger := observability.NewLogger("warn", "json", &logBuf)
	svc := NewService(&fakeResolver{providers: testProviders(srv.URL)}, rec, logger)
	maxTok := 16
	temp := 0.1
	out, err := svc.Chat(context.Background(), "proj-a", Request{
		Messages:    []providers.Message{{Role: "user", Content: "hi"}},
		MaxTokens:   &maxTok,
		Temperature: &temp,
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if out.Content != "hi" || out.Usage.TotalTokens != 4 {
		t.Fatalf("%+v", out)
	}
	if !rec.called {
		t.Fatal("recorder")
	}

	// cache hit + explicit model
	out, err = svc.Chat(context.Background(), "proj-a", Request{
		Model:    "gpt-4",
		Messages: []providers.Message{{Role: "user", Content: "again"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "hi" {
		t.Fatalf("%+v", out)
	}

	// recorder error with logger
	rec.err = errors.New("usage down")
	if _, err := svc.Chat(context.Background(), "proj-a", Request{
		Messages: []providers.Message{{Role: "user", Content: "x"}},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logBuf.String(), "record usage failed") {
		t.Fatalf("log=%s", logBuf.String())
	}

	// recorder error without logger
	svcNoLog := NewService(&fakeResolver{providers: testProviders(srv.URL)}, &countingRecorder{err: errors.New("x")}, nil)
	if _, err := svcNoLog.Chat(context.Background(), "proj-b", Request{
		Messages: []providers.Message{{Role: "user", Content: "x"}},
	}); err != nil {
		t.Fatal(err)
	}

	// stream with recorder wrapper
	rec.err = nil
	rec.called = false
	reader, err := svc.Stream(context.Background(), "proj-a", Request{
		Messages: []providers.Message{{Role: "user", Content: "stream"}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if _, ok := reader.(*usageRecordingReader); !ok {
		t.Fatalf("want usageRecordingReader, got %T", reader)
	}
	for {
		_, err := reader.Next()
		if err != nil {
			break
		}
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	// stream without recorder
	raw, err := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, nil).Stream(context.Background(), "proj-c", Request{
		Model:    "gpt-4",
		Messages: []providers.Message{{Role: "user", Content: "s"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := raw.(*usageRecordingReader); ok {
		t.Fatal("raw reader should not wrap")
	}
	_ = raw.Close()
}

func TestChatAndStreamProviderErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	srv := openaiCompatServer(t, true)
	defer srv.Close()
	svc := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, nil)
	if _, err := svc.Chat(context.Background(), "p", Request{
		Messages: []providers.Message{{Role: "user", Content: "x"}},
	}); err == nil {
		t.Fatal("chat should fail")
	}
	if _, err := svc.Stream(context.Background(), "p", Request{
		Messages: []providers.Message{{Role: "user", Content: "x"}},
	}); err == nil {
		t.Fatal("stream should fail")
	}
}

func TestUsageRecordingReader(t *testing.T) {
	if extractUsage(&providers.StreamChunk{}) != nil {
		t.Fatal("extractUsage is a stub")
	}

	inner := &memStream{chunks: []*providers.StreamChunk{
		{Type: "usage", Usage: &providers.Usage{TotalTokens: 9}},
		{Content: "a", FinishReason: "stop"},
	}}
	rec := &countingRecorder{}
	r := &usageRecordingReader{inner: inner, recorder: rec, project: "p", provider: "openai", model: "m", accum: Usage{TotalTokens: 0}}
	if _, err := r.Next(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Next(); err != nil {
		t.Fatal(err)
	}
	// TotalTokens still 0 because extractUsage returns nil; force accum then Close.
	r.accum = Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !rec.called {
		t.Fatal("close should record")
	}
	r.maybeRecord() // already recorded

	r2 := &usageRecordingReader{inner: &memStream{err: io.EOF}, recorder: rec, accum: Usage{TotalTokens: 5}}
	_, _ = r2.Next()
	if !r2.recorded {
		t.Fatal("error path records")
	}

	var logBuf bytes.Buffer
	r3 := &usageRecordingReader{
		inner:    &memStream{err: io.EOF},
		recorder: &countingRecorder{err: errors.New("no")},
		logger:   observability.NewLogger("warn", "json", &logBuf),
		accum:    Usage{TotalTokens: 2},
		project:  "p",
	}
	_, _ = r3.Next()
	if !strings.Contains(logBuf.String(), "record stream usage failed") {
		t.Fatalf("log=%s", logBuf.String())
	}

	r4 := &usageRecordingReader{inner: &memStream{err: io.EOF}, recorder: &countingRecorder{err: errors.New("no")}, accum: Usage{TotalTokens: 2}}
	_, _ = r4.Next()

	r5 := &usageRecordingReader{inner: &memStream{chunks: []*providers.StreamChunk{{FinishReason: "stop"}}}}
	_, _ = r5.Next()
	_ = r5.Close()
}

func TestCatalogResolver(t *testing.T) {
	r := NewCatalogResolver(nil, nil)
	if _, err := r.Resolve(context.Background(), "p"); err == nil {
		t.Fatal("nil catalog")
	}

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := systemdb.ApplySystemMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := catalog.NewSQLRepository(db)
	cat := catalog.NewService(repo, objectstore.KeyBuilder{}, nil, objectstore.DuckLakeStorage{}, nil)

	now := time.Now().UTC()
	if err := repo.UpsertProviderConfig(ctx, catalog.LLMProviderConfig{
		ID: "1", ProjectID: "p1", Provider: "openai", CredentialRef: "ref-ok",
		Enabled: true, DefaultModel: "gpt-4", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertProviderConfig(ctx, catalog.LLMProviderConfig{
		ID: "2", ProjectID: "p1", Provider: "anthropic", CredentialRef: "ref-bad",
		Enabled: true, DefaultModel: "claude", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertProviderConfig(ctx, catalog.LLMProviderConfig{
		ID: "3", ProjectID: "p1", Provider: "gemini", CredentialRef: "",
		Enabled: true, DefaultModel: "gem", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	creds := &memCreds{keys: map[string]string{"ref-ok": "sk-live"}, fail: map[string]error{"ref-bad": errors.New("denied")}}
	got, err := NewCatalogResolver(cat, creds).Resolve(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Providers) != 2 {
		t.Fatalf("skipped bad cred: %+v", got.Providers)
	}
	if got.Providers[0].APIKey != "sk-live" || got.Providers[1].APIKey != "" {
		t.Fatalf("%+v", got.Providers)
	}

	got, err = NewCatalogResolver(cat, nil).Resolve(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Providers[0].APIKey != "" {
		t.Fatal("nil credential keeps empty key")
	}

	_ = db.Close()
	if _, err := NewCatalogResolver(cat, nil).Resolve(ctx, "p1"); err == nil {
		t.Fatal("closed db")
	}
}

type countingRecorder struct {
	called bool
	err    error
}

func (c *countingRecorder) RecordLLM(context.Context, string, string, string, Usage) error {
	c.called = true
	return c.err
}

type memStream struct {
	chunks []*providers.StreamChunk
	err    error
	i      int
}

func (m *memStream) Next() (*providers.StreamChunk, error) {
	if m.i >= len(m.chunks) {
		if m.err != nil {
			return nil, m.err
		}
		return nil, io.EOF
	}
	c := m.chunks[m.i]
	m.i++
	return c, nil
}
func (m *memStream) Close() error { return nil }

type memCreds struct {
	keys map[string]string
	fail map[string]error
}

func (m *memCreds) Resolve(_ context.Context, ref string) (string, error) {
	if err := m.fail[ref]; err != nil {
		return "", err
	}
	return m.keys[ref], nil
}
