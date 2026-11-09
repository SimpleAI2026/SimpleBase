package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestFunctionTriggerRepository(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	created, err := store.CreateFunctionTrigger(ctx, FunctionTrigger{ProjectID: "p1", Kind: "record_change", Target: "tasks", Events: []string{"create", "update"}, FuncFile: "handler", FuncExport: "Handle", Filter: `status="open"`, Enabled: true})
	if err != nil || created.ID == "" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	items, err := store.ListFunctionTriggers(ctx, "p1")
	if err != nil || len(items) != 1 || len(items[0].Events) != 2 || !items[0].Enabled {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	created.Enabled = false
	created.Target = "other"
	if _, err := store.UpdateFunctionTrigger(ctx, created); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetFunctionTrigger(ctx, "p1", created.ID)
	if err != nil || stored.Enabled || stored.Target != "other" {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	if err := store.ArchiveFunctionTrigger(ctx, "p1", created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetFunctionTrigger(ctx, "p1", created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("archived get err=%v", err)
	}
}
