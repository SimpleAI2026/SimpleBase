package audit

import (
	"context"
	"testing"
	"time"
)

func TestRecordDefaultsDetailAndListLimits(t *testing.T) {
	repo := newTestRepoForAudit(t)
	svc := NewService(repo, nil)
	ctx := context.Background()

	if err := svc.Record(ctx, Event{
		ProjectID:  "proj-1",
		DatabaseID: "db-1",
		Kind:       "config_change",
		Status:     "ok",
		Detail:     "rotated",
		RequestID:  "r1",
	}); err != nil {
		t.Fatal(err)
	}
	ops, err := svc.ListOperations(ctx, Query{ProjectID: "proj-1", DatabaseID: "db-1", Kind: "config_change", Since: time.Now().Add(-time.Hour), Limit: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 {
		t.Fatalf("ops=%d", len(ops))
	}
	if ops[0].PrincipalID != "system" {
		t.Fatalf("default principal=%q", ops[0].PrincipalID)
	}
	if ops[0].Status != "ok: rotated" {
		t.Fatalf("status=%q", ops[0].Status)
	}

	ops, err = svc.ListOperations(ctx, Query{Limit: 201})
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 {
		t.Fatalf("limit clamp ops=%d", len(ops))
	}
}

func TestRedactLengths(t *testing.T) {
	if Redact("") != "" || Redact("a") != "*" || Redact("abcd") != "****" {
		t.Fatal("short redact")
	}
	if Redact("abcde") != "ab*de" {
		t.Fatalf("%q", Redact("abcde"))
	}
}
