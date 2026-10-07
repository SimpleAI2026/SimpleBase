package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestSandboxRowCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := "proj-1"

	r, err := s.CreateSandbox(ctx, SandboxRow{ProjectID: pid, Name: "a", CloudName: "sbx-a", Source: "api",
		Image: "python:3.12-slim", CPUs: 1, MemoryMiB: 256, Network: "none", IdleTimeoutS: 300, MaxDurationS: 1800,
		Status: "pending", CreatedBy: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSandbox(ctx, SandboxRow{ProjectID: pid, Name: "agent-x", CloudName: "sb-x", Source: "agent",
		ThreadID: "t1", Image: "i", CPUs: 1, MemoryMiB: 256, Network: "none", Status: "pending"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSandbox(ctx, pid, r.ID)
	if err != nil || got.Name != "a" || got.CPUs != 1 || got.ThreadID != "" || !got.StartedAt.IsZero() {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := s.GetSandbox(ctx, "other", r.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross project must be ErrNoRows, got %v", err)
	}
	if byName, err := s.GetSandboxByName(ctx, pid, "a"); err != nil || byName.ID != r.ID {
		t.Fatalf("by name: %+v %v", byName, err)
	}
	if byThread, err := s.GetSandboxByThread(ctx, pid, "t1"); err != nil || byThread.Source != "agent" {
		t.Fatalf("by thread: %+v %v", byThread, err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	got.Status, got.StartedAt, got.ExpiresAt, got.LastActiveAt = "running", now, now.Add(-time.Minute), now
	if err := s.UpdateSandbox(ctx, got); err != nil {
		t.Fatal(err)
	}
	list, next, err := s.ListSandboxes(ctx, pid, "running", "", "", 0)
	if err != nil || next != "" || len(list) != 1 || list[0].ID != r.ID || list[0].StartedAt.IsZero() {
		t.Fatalf("list running: %+v %q %v", list, next, err)
	}
	if n, _ := s.CountSandboxes(ctx, pid); n != 2 {
		t.Fatalf("count = %d", n)
	}
	exp, err := s.ListExpiredSandboxes(ctx, now, 10)
	if err != nil || len(exp) != 1 {
		t.Fatalf("expired: %+v %v", exp, err)
	}

	got.LastError = "cloud removal failed"
	if err := s.UpdateSandbox(ctx, got); err != nil {
		t.Fatal(err)
	}
	if failed, err := s.ListFailedSandboxRemovals(ctx, 10); err != nil || len(failed) != 1 || failed[0].ID != r.ID {
		t.Fatalf("failed removals: %+v %v", failed, err)
	}
	if err := s.SoftDeleteSandbox(ctx, pid, r.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSandbox(ctx, pid, r.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted row must be hidden, got %v", err)
	}
	if err := s.SoftDeleteSandbox(ctx, pid, r.ID, ""); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("double delete must be ErrNoRows, got %v", err)
	}
	if n, err := s.PurgeDeletedSandboxes(ctx, time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("purge = %d %v", n, err)
	}
}

func TestSandboxListCursorPagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	// 5 行，其中两行创建时间相同，验证 id 兜底排序不丢不重。
	stamps := []time.Duration{0, time.Second, time.Second, 2 * time.Second, 3 * time.Second}
	want := map[string]bool{}
	for i, d := range stamps {
		source := "api"
		if i == 4 {
			source = "agent"
		}
		r, err := s.CreateSandbox(ctx, SandboxRow{ProjectID: "p", Name: "n" + string(rune('a'+i)), CloudName: "c",
			Source: source, Image: "i", CPUs: 1, MemoryMiB: 256, Network: "none", Status: "pending", CreatedAt: base.Add(d)})
		if err != nil {
			t.Fatal(err)
		}
		want[r.ID] = true
	}
	other, err := s.CreateSandbox(ctx, SandboxRow{ProjectID: "other", Name: "x", CloudName: "c", Source: "api",
		Image: "i", CPUs: 1, MemoryMiB: 256, Network: "none", Status: "pending", CreatedAt: base})
	if err != nil {
		t.Fatal(err)
	}

	seen, cursor, pages := map[string]bool{}, "", 0
	var last time.Time
	for {
		rows, next, err := s.ListSandboxes(ctx, "p", "", "", cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, r := range rows {
			if seen[r.ID] {
				t.Fatalf("duplicate row %s", r.ID)
			}
			if !last.IsZero() && r.CreatedAt.After(last) {
				t.Fatalf("not in created_at DESC order")
			}
			seen[r.ID], last = true, r.CreatedAt
		}
		if next == "" {
			break
		}
		if len(rows) != 2 || next != rows[1].ID {
			t.Fatalf("page %d: n=%d next=%q", pages, len(rows), next)
		}
		cursor = next
	}
	if pages != 3 || len(seen) != len(want) {
		t.Fatalf("pages=%d seen=%d want=%d", pages, len(seen), len(want))
	}
	for id := range want {
		if !seen[id] {
			t.Fatalf("missing row %s", id)
		}
	}

	// 恰好一页时不返回游标。
	if rows, next, err := s.ListSandboxes(ctx, "p", "", "", "", 5); err != nil || len(rows) != 5 || next != "" {
		t.Fatalf("exact page: n=%d next=%q err=%v", len(rows), next, err)
	}
	// 过滤条件与游标叠加。
	first, next, err := s.ListSandboxes(ctx, "p", "", "api", "", 3)
	if err != nil || len(first) != 3 || next == "" {
		t.Fatalf("filtered page1: n=%d next=%q err=%v", len(first), next, err)
	}
	rest, next, err := s.ListSandboxes(ctx, "p", "", "api", next, 3)
	if err != nil || len(rest) != 1 || next != "" || rest[0].Source != "api" {
		t.Fatalf("filtered page2: %+v next=%q err=%v", rest, next, err)
	}
	// 游标行被软删后仍可继续翻页。
	if err := s.SoftDeleteSandbox(ctx, "p", first[2].ID, ""); err != nil {
		t.Fatal(err)
	}
	if rows, _, err := s.ListSandboxes(ctx, "p", "", "api", first[2].ID, 3); err != nil || len(rows) != 1 || rows[0].ID != rest[0].ID {
		t.Fatalf("cursor on soft-deleted row: %+v %v", rows, err)
	}
	// 未知游标、其它项目的游标都被拒绝。
	for _, bad := range []string{"missing", other.ID} {
		if _, _, err := s.ListSandboxes(ctx, "p", "", "", bad, 2); !errors.Is(err, ErrSandboxCursor) {
			t.Fatalf("cursor %q: %v", bad, err)
		}
	}
}
