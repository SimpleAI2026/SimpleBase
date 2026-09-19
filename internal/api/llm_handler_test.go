package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

type fakeLLMSvc struct {
	chatErr   error
	streamErr error
	listErr   error
	resp      LLMResponse
	reader    LLMStreamReader
	names     []string
}

func (f *fakeLLMSvc) Chat(context.Context, string, LLMRequest) (LLMResponse, error) {
	if f.chatErr != nil {
		return LLMResponse{}, f.chatErr
	}
	if f.resp.Content == "" {
		return LLMResponse{Content: "ok", Model: "m", Provider: "p", FinishReason: "stop", Usage: LLMTokenUsage{PromptTokens: 2, CompletionTokens: 3}}, nil
	}
	return f.resp, nil
}
func (f *fakeLLMSvc) Stream(context.Context, string, LLMRequest) (LLMStreamReader, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	if f.reader != nil {
		return f.reader, nil
	}
	return &seqStream{chunks: []*LLMStreamChunk{{Type: "delta", Content: "hel"}, {Type: "delta", Content: "lo", FinishReason: "stop"}}}, nil
}
func (f *fakeLLMSvc) ListProviders(context.Context, string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.names, nil
}

type seqStream struct {
	chunks []*LLMStreamChunk
	i      int
}

func (s *seqStream) Next() (*LLMStreamChunk, error) {
	if s.i >= len(s.chunks) {
		return nil, io.EOF
	}
	c := s.chunks[s.i]
	s.i++
	return c, nil
}
func (s *seqStream) Close() error { return nil }

func setupLLMRouter(t *testing.T, h *LLMHandler, withProject, withPrincipal bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			if withPrincipal {
				ctx = WithPrincipal(ctx, auth.Principal{APIKeyID: "key-llm"})
			}
			if withProject {
				ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "t"})
			}
			ctx = WithRequestID(ctx, "rid-llm")
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.POST("/chat", h.Chat)
	e.POST("/stream", h.Stream)
	e.GET("/providers", h.ListProviders)
	return e
}

func TestLLMHandler_Chat(t *testing.T) {
	t.Run("missing_project", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, false, true)
		rec := doRequest(e, http.MethodPost, "/chat", llmChatRequest{Messages: []llmMessageDTO{{Role: "user", Content: "hi"}}})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})
	t.Run("bad_json", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, true, true)
		req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader("{"))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Fatalf("expected bind error, got %d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("empty_messages", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, true, true)
		rec := doRequest(e, http.MethodPost, "/chat", llmChatRequest{})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("quota_denied", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}, usage: &fakeUsage{errLLM: errors.New("quota")}}, true, true)
		rec := doRequest(e, http.MethodPost, "/chat", llmChatRequest{Messages: []llmMessageDTO{{Role: "user", Content: "hi"}}})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("svc_error", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{chatErr: errors.New("llm down")}}, true, true)
		rec := doRequest(e, http.MethodPost, "/chat", llmChatRequest{Messages: []llmMessageDTO{{Role: "user", Content: "hi"}}})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})
	t.Run("success_audit_persist", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := systemdb.ApplySystemMigrations(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		store := systemdb.NewStoreForTest(db)
		aud := &fakeAudit{}
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}, usage: &fakeUsage{}, audit: aud, store: store}, true, true)
		rec := doRequest(e, http.MethodPost, "/chat", llmChatRequest{
			Messages: []llmMessageDTO{{Role: "user", Content: "hello world"}},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
		var out llmChatResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Content != "ok" {
			t.Fatalf("content=%q", out.Content)
		}
		if aud.last.Kind != "llm_chat" || aud.last.Status != "ok" || aud.last.PrincipalID != "key-llm" {
			t.Fatalf("audit=%+v", aud.last)
		}
		sessions, err := store.ListLLMSessions(context.Background(), "proj-1", 10)
		if err != nil || len(sessions) != 1 {
			t.Fatalf("sessions=%v err=%v", sessions, err)
		}
	})
	t.Run("persist_existing_session", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := systemdb.ApplySystemMigrations(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		store := systemdb.NewStoreForTest(db)
		sess, err := store.CreateLLMSession(context.Background(), systemdb.LLMSession{ProjectID: "proj-1", Title: "old"})
		if err != nil {
			t.Fatal(err)
		}
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}, store: store}, true, false)
		rec := doRequest(e, http.MethodPost, "/chat", llmChatRequest{
			SessionID: sess.ID,
			Messages:  []llmMessageDTO{{Role: "user", Content: "again"}},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("persist_create_session_error", func(t *testing.T) {
		// store with no schema → CreateLLMSession fails, persistChat returns
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		store := systemdb.NewStoreForTest(db)
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}, store: store}, true, true)
		rec := doRequest(e, http.MethodPost, "/chat", llmChatRequest{Messages: []llmMessageDTO{{Role: "user", Content: "x"}}})
		if rec.Code != http.StatusOK {
			t.Fatalf("chat should still succeed: %d %s", rec.Code, rec.Body.String())
		}
	})
}

func TestLLMHandler_Stream(t *testing.T) {
	t.Run("missing_project", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, false, true)
		rec := doRequest(e, http.MethodPost, "/stream", llmChatRequest{Messages: []llmMessageDTO{{Role: "user", Content: "hi"}}})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})
	t.Run("empty_messages", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, true, true)
		rec := doRequest(e, http.MethodPost, "/stream", llmChatRequest{})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})
	t.Run("quota", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}, usage: &fakeUsage{errLLM: errors.New("q")}}, true, true)
		rec := doRequest(e, http.MethodPost, "/stream", llmChatRequest{Messages: []llmMessageDTO{{Role: "user", Content: "hi"}}})
		if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "data:") {
			t.Fatal("quota should fail before stream")
		}
	})
	t.Run("svc_error", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{streamErr: errors.New("no stream")}}, true, true)
		rec := doRequest(e, http.MethodPost, "/stream", llmChatRequest{Messages: []llmMessageDTO{{Role: "user", Content: "hi"}}})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})
	t.Run("success_sse", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, true, true)
		rec := doRequest(e, http.MethodPost, "/stream", llmChatRequest{Messages: []llmMessageDTO{{Role: "user", Content: "hi"}}})
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "data:") || !strings.Contains(body, `"type":"end"`) {
			t.Fatalf("sse body=%s", body)
		}
		if rec.Header().Get(echo.HeaderContentType) != "text/event-stream" {
			t.Fatalf("ct=%s", rec.Header().Get(echo.HeaderContentType))
		}
	})
	t.Run("bad_json", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, true, true)
		req := httptest.NewRequest(http.MethodPost, "/stream", strings.NewReader("not-json"))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "data:") {
			t.Fatal("bad json should not stream")
		}
	})
}

func TestLLMHandler_ListProviders(t *testing.T) {
	t.Run("missing_project", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, false, true)
		rec := doRequest(e, http.MethodGet, "/providers", nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})
	t.Run("error", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{listErr: errors.New("x")}}, true, true)
		rec := doRequest(e, http.MethodGet, "/providers", nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})
	t.Run("ok", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{names: []string{"openai", "anthropic"}}}, true, true)
		rec := doRequest(e, http.MethodGet, "/providers", nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "anthropic") {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
	})
}

func TestLLMHandler_recordAuditNil(t *testing.T) {
	h := &LLMHandler{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h.recordAudit(c, "p", "k", "ok") // no panic when audit is nil
	h.persistChat(c, "p", llmChatRequest{}, LLMResponse{}) // store nil
}
