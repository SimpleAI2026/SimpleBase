package collection

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/uglyer/go-sqlite3"
)

func TestRepositoryPersistsLogicalView(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE sys_collections (id VARCHAR, project_id VARCHAR, name VARCHAR, schema_mode VARCHAR, view_query VARCHAR, status VARCHAR, created_at TIMESTAMP, updated_at TIMESTAMP);
	CREATE TABLE sys_collection_fields (id VARCHAR, project_id VARCHAR, collection VARCHAR, name VARCHAR, type VARCHAR, required BIGINT, unique_flag BIGINT, default_json VARCHAR, options_json VARCHAR, hidden BIGINT, readonly BIGINT, order_index BIGINT, created_at TIMESTAMP, updated_at TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(db, nil)
	input := Collection{ProjectID: "p1", Name: "active_tasks", SchemaMode: "view", ViewQuery: "SELECT id, data, created_at, updated_at FROM tasks", Fields: []Field{{Name: "title", Type: "text"}}}
	if err := repository.Put(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.Get(context.Background(), "p1", "active_tasks")
	if err != nil {
		t.Fatal(err)
	}
	if stored.SchemaMode != "view" || stored.ViewQuery != input.ViewQuery || len(stored.Fields) != 1 {
		t.Fatalf("stored view=%#v", stored)
	}
	items, err := repository.List(context.Background(), "p1")
	if err != nil || len(items) != 1 || items[0].SchemaMode != "view" {
		t.Fatalf("list=%#v err=%v", items, err)
	}
}

func TestValidateViewSchema(t *testing.T) {
	for _, schema := range []Collection{
		{Name: "v", SchemaMode: "view"},
		{Name: "v", SchemaMode: "json", ViewQuery: "SELECT 1"},
		{Name: "v", SchemaMode: "other"},
	} {
		if err := ValidateSchema(schema); err == nil {
			t.Fatalf("accepted invalid schema %#v", schema)
		}
	}
}
