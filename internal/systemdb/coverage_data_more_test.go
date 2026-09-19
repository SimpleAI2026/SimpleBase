package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func assertUnavailable(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestSettingsCRUDAndUnavailable(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if _, err := nilStore.GetGlobalSetting(ctx, "k"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ListGlobalSettings(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.PutGlobalSetting(ctx, "k", "{}"))
	if _, err := nilStore.ListProjectSettings(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.PutProjectSetting(ctx, "p", "k", "{}"))

	s := newTestStore(t)
	missing, err := s.GetGlobalSetting(ctx, "theme")
	if err != nil || missing.Key != "theme" || missing.ValueJSON != "" {
		t.Fatalf("missing: %+v err=%v", missing, err)
	}
	if err := s.PutGlobalSetting(ctx, "theme", `{"dark":true}`); err != nil {
		t.Fatal(err)
	}
	if err := s.PutGlobalSetting(ctx, "theme", `{"dark":false}`); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGlobalSetting(ctx, "theme")
	if err != nil || got.ValueJSON != `{"dark":false}` {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	list, err := s.ListGlobalSettings(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v err=%v", list, err)
	}
	if err := s.PutProjectSetting(ctx, "p1", "k", `{"a":1}`); err != nil {
		t.Fatal(err)
	}
	if err := s.PutProjectSetting(ctx, "p1", "k", `{"a":2}`); err != nil {
		t.Fatal(err)
	}
	pl, err := s.ListProjectSettings(ctx, "p1")
	if err != nil || len(pl) != 1 || pl[0].ValueJSON != `{"a":2}` {
		t.Fatalf("project settings: %+v err=%v", pl, err)
	}
}

func TestLogsMetricsAndRetention(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if _, err := nilStore.QueryLogs(ctx, LogQuery{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.LogLevelStats(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetRetention(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.PutRetention(ctx, "p", 7))
	if _, err := nilStore.MetricsSummary(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.MetricsTrend(ctx, "p", 7); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}

	s := newTestStore(t)
	if err := s.FlushLogs(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.FlushMetrics(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	s.RecordLog(LogEvent{ProjectID: "p1", Message: "hello world", RequestID: "r1"})
	s.RecordLog(LogEvent{ID: uuid.NewString(), ProjectID: "p1", Level: "error", Logger: "api", Message: "boom", OccurredAt: now})
	s.RecordLog(LogEvent{Level: "warn", Logger: "sys", Message: "global"})
	if err := s.FlushLogs(ctx); err != nil {
		t.Fatal(err)
	}

	all, err := s.QueryLogs(ctx, LogQuery{ProjectID: catalog.ReservedSystemProjectID, Limit: 0})
	if err != nil || len(all) != 3 {
		t.Fatalf("admin logs n=%d err=%v", len(all), err)
	}
	errs, err := s.QueryLogs(ctx, LogQuery{ProjectID: "p1", Level: "error", Limit: 10})
	if err != nil || len(errs) != 1 || errs[0].Level != "error" {
		t.Fatalf("level filter: %+v err=%v", errs, err)
	}
	q, err := s.QueryLogs(ctx, LogQuery{ProjectID: "p1", Q: "hello", From: now.Add(-time.Hour), To: now.Add(time.Hour), Limit: 600})
	if err != nil || len(q) != 1 {
		t.Fatalf("text+time: %+v err=%v", q, err)
	}
	stats, err := s.LogLevelStats(ctx, "p1")
	if err != nil || len(stats) == 0 {
		t.Fatalf("stats: %+v err=%v", stats, err)
	}

	ret, err := s.GetRetention(ctx, "p1")
	if err != nil || ret.KeepDays != 14 || ret.Scope != "global" {
		t.Fatalf("default retention: %+v err=%v", ret, err)
	}
	if err := s.PutRetention(ctx, "p1", 0); err != nil {
		t.Fatal(err)
	}
	ret, err = s.GetRetention(ctx, "p1")
	if err != nil || ret.KeepDays != 14 || ret.Scope != "project" {
		t.Fatalf("put default days: %+v err=%v", ret, err)
	}
	if err := s.PutRetention(ctx, "p1", 30); err != nil {
		t.Fatal(err)
	}
	ret, err = s.GetRetention(ctx, "p1")
	if err != nil || ret.KeepDays != 30 {
		t.Fatalf("updated retention: %+v", ret)
	}

	s.RecordMetric(MetricSample{ProjectID: "p1", Name: "http_requests", Value: 10})
	s.RecordMetric(MetricSample{ID: uuid.NewString(), ProjectID: "p1", Name: "http_errors", Value: 2, OccurredAt: now})
	s.RecordMetric(MetricSample{ProjectID: "p1", Name: "http_latency_ms", Value: 15})
	s.RecordMetric(MetricSample{ProjectID: "p2", Name: "http_requests", Value: 3})
	if err := s.FlushMetrics(ctx); err != nil {
		t.Fatal(err)
	}
	sum, err := s.MetricsSummary(ctx, "p1")
	if err != nil || sum.TotalRequests != 10 || sum.ErrorRate != 0.2 || sum.AvgLatencyMS == 0 {
		t.Fatalf("project summary: %+v err=%v", sum, err)
	}
	adminSum, err := s.MetricsSummary(ctx, catalog.ReservedSystemProjectID)
	if err != nil || adminSum.TotalRequests < 13 {
		t.Fatalf("admin summary: %+v err=%v", adminSum, err)
	}
	trend, err := s.MetricsTrend(ctx, "p1", 0)
	if err != nil {
		t.Fatalf("trend: %v", err)
	}
	_ = trend
	adminTrend, err := s.MetricsTrend(ctx, catalog.ReservedSystemProjectID, 91)
	if err != nil {
		t.Fatalf("admin trend: %v", err)
	}
	_ = adminTrend

	// auto-flush path (>=64)
	for i := 0; i < 64; i++ {
		s.RecordLog(LogEvent{ProjectID: "p1", Message: "bulk"})
	}
	for i := 0; i < 64; i++ {
		s.RecordMetric(MetricSample{ProjectID: "p1", Name: "http_requests", Value: 1})
	}
}

func TestLLMSessionsAndSettings(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if _, err := nilStore.CreateLLMSession(ctx, LLMSession{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ListLLMSessions(ctx, "p", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetLLMSession(ctx, "p", "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.ArchiveLLMSession(ctx, "p", "id"))
	if _, err := nilStore.AppendLLMMessage(ctx, LLMMessage{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ListLLMMessages(ctx, "p", "s", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetLLMSettings(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.PutLLMSettings(ctx, LLMSettings{}))

	s := newTestStore(t)
	sess, err := s.CreateLLMSession(ctx, LLMSession{ProjectID: "p1"})
	if err != nil || sess.Title != "New chat" || sess.ID == "" {
		t.Fatalf("create: %+v err=%v", sess, err)
	}
	fixedID := uuid.NewString()
	if _, err := s.CreateLLMSession(ctx, LLMSession{ID: fixedID, ProjectID: "p1", Title: "named"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListLLMSessions(ctx, "p1", 0)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: n=%d err=%v", len(list), err)
	}
	got, err := s.GetLLMSession(ctx, "p1", sess.ID)
	if err != nil || got.ID != sess.ID {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if _, err := s.GetLLMSession(ctx, "p1", uuid.NewString()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing: %v", err)
	}

	long := "abcdefghijklmnopqrstuvwxyz0123456789XXXXX" // >40
	if _, err := s.AppendLLMMessage(ctx, LLMMessage{SessionID: sess.ID, ProjectID: "p1", Role: "user", Content: long}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendLLMMessage(ctx, LLMMessage{ID: uuid.NewString(), SessionID: sess.ID, ProjectID: "p1", Role: "assistant", Content: "ok"}); err != nil {
		t.Fatal(err)
	}
	msgs, err := s.ListLLMMessages(ctx, "p1", sess.ID, 0)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("msgs: n=%d err=%v", len(msgs), err)
	}
	msgs, err = s.ListLLMMessages(ctx, "p1", sess.ID, 600)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("clamped msgs: n=%d err=%v", len(msgs), err)
	}
	updated, err := s.GetLLMSession(ctx, "p1", sess.ID)
	if err != nil || updated.Title == "New chat" || len(updated.Title) > 40 {
		t.Fatalf("title should truncate: %+v", updated)
	}
	if err := s.ArchiveLLMSession(ctx, "p1", sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetLLMSession(ctx, "p1", sess.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("archived: %v", err)
	}

	st, err := s.GetLLMSettings(ctx, "p1")
	if err != nil || st.Temperature != 0.7 || st.MaxTokens != 1024 {
		t.Fatalf("default settings: %+v err=%v", st, err)
	}
	if err := s.PutLLMSettings(ctx, LLMSettings{ProjectID: "p1", DefaultProvider: "openai", DefaultModel: "m", Temperature: 0.1, MaxTokens: 8}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutLLMSettings(ctx, LLMSettings{ProjectID: "p1", DefaultProvider: "openai", DefaultModel: "m2", Temperature: 0.2, MaxTokens: 16}); err != nil {
		t.Fatal(err)
	}
	st, err = s.GetLLMSettings(ctx, "p1")
	if err != nil || st.DefaultModel != "m2" || st.MaxTokens != 16 {
		t.Fatalf("updated settings: %+v err=%v", st, err)
	}
}

func TestS3IndexRefreshAndCRUD(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	assertUnavailable(t, nilStore.UpsertS3Object(ctx, "p", "k", 1, "", "", time.Time{}))
	assertUnavailable(t, nilStore.SoftDeleteS3Object(ctx, "p", "k"))
	if _, err := nilStore.ListS3Objects(ctx, "p", "", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetS3Object(ctx, "p", "k"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, _, err := nilStore.RefreshS3Index(ctx, "p", nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}

	s := newTestStore(t)
	pid := "proj-s3"
	if err := s.UpsertS3Object(ctx, pid, "keep.txt", 1, "e1", "text/plain", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertS3Object(ctx, pid, "keep.txt", 2, "e2", "text/plain", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertS3Object(ctx, pid, "gone.txt", 3, "", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertS3Object(ctx, pid, "pref/a.txt", 4, "", "", time.Now()); err != nil {
		t.Fatal(err)
	}

	pref, err := s.ListS3Objects(ctx, pid, "pref/", 0)
	if err != nil || len(pref) != 1 || pref[0].Key != "pref/a.txt" {
		t.Fatalf("prefix list: %+v err=%v", pref, err)
	}
	got, err := s.GetS3Object(ctx, pid, "keep.txt")
	if err != nil || got.Size != 2 || got.ETag != "e2" {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if _, err := s.GetS3Object(ctx, pid, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing: %v", err)
	}
	if err := s.SoftDeleteS3Object(ctx, pid, "pref/a.txt"); err != nil {
		t.Fatal(err)
	}

	listed := []objectstore.FileObject{
		{Key: pid + "/keep.txt", Size: 9, LastModified: time.Now()},
		{Key: "new.txt", Size: 5, LastModified: time.Now()},
	}
	ins, rem, err := s.RefreshS3Index(ctx, pid, listed)
	if err != nil {
		t.Fatal(err)
	}
	if ins != 1 || rem != 1 {
		t.Fatalf("refresh ins=%d rem=%d want 1/1", ins, rem)
	}
	if _, err := s.GetS3Object(ctx, pid, "gone.txt"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("gone should be removed")
	}
	keep, err := s.GetS3Object(ctx, pid, "keep.txt")
	if err != nil || keep.Size != 9 {
		t.Fatalf("updated keep: %+v err=%v", keep, err)
	}
	if _, err := s.GetS3Object(ctx, pid, "new.txt"); err != nil {
		t.Fatal(err)
	}
}
