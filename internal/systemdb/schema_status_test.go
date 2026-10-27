package systemdb

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/uglyer/go-sqlite3"
)

func TestSchemaStatusAndPlanAreReadOnly(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	status, err := Status(ctx, db)
	if err != nil || status.Ready || status.Present != 0 || len(status.Missing) != SystemTableCount() {
		t.Fatalf("empty status=%#v err=%v", status, err)
	}
	plan, err := Plan(ctx, db)
	if err != nil || len(plan.Statements) != SystemTableCount() {
		t.Fatalf("empty plan=%#v err=%v", plan, err)
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("read-only status created tables=%d err=%v", tables, err)
	}
	if err := EnsureSystemSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	status, err = Status(ctx, db)
	if err != nil || !status.Ready || status.Present != SystemTableCount() || len(status.Missing) != 0 {
		t.Fatalf("ready status=%#v err=%v", status, err)
	}
	plan, err = Plan(ctx, db)
	if err != nil || len(plan.Pending) != 0 || len(plan.Statements) != 0 {
		t.Fatalf("ready plan=%#v err=%v", plan, err)
	}
}
