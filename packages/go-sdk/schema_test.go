package gosdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestListTableRowsDecodesColumnObjects(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/v1/projects/project%2Fone/databases/db%2Fone/schema/tables/items/rows" {
			t.Errorf("unexpected %s", r.URL.EscapedPath())
		}
		if got := r.URL.Query().Get("limit"); got != "10" {
			t.Errorf("limit = %q", got)
		}
		io.WriteString(w, `{"table":"items","columns":[`+
			`{"name":"id","type":"BIGINT","nullable":false},`+
			`{"name":"token","type":"VARCHAR","nullable":true,"sensitive":true}],`+
			`"rows":[[1,"***"]],"limit":10,"offset":0,"total":1}`)
	})
	got, err := c.ListTableRows(context.Background(), "db/one", "items", 10)
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	if got.Table != "items" || got.Total != 1 || got.Limit != 10 || len(got.Rows) != 1 {
		t.Fatalf("rows: %+v", got)
	}
	if len(got.Columns) != 2 || got.Columns[0].Name != "id" || got.Columns[0].Type != "BIGINT" {
		t.Fatalf("columns: %+v", got.Columns)
	}
	if got.Columns[0].Nullable == nil || *got.Columns[0].Nullable {
		t.Fatalf("id nullable: %+v", got.Columns[0])
	}
	if !got.Columns[1].Sensitive {
		t.Fatalf("sensitive flag lost: %+v", got.Columns[1])
	}
	if _, err := c.ListTableRows(context.Background(), "", "items", 10); err == nil {
		t.Fatal("empty database id accepted")
	}
}

func TestSQLDatabaseAndSchema(t *testing.T) {
	nullable := false
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/v1/projects/project%2Fone/databases":
			var body CreateDatabaseInput
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Name != "shop" || body.DataModel != "sql" || body.InitSQL != "CREATE TABLE t (id INTEGER)" {
				t.Fatalf("create body: %+v", body)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":"db","data_model":"sql"}`)
		case "/v1/projects/project%2Fone/databases/db%2Fone/schema":
			if r.Method != http.MethodGet {
				t.Fatalf("method %s", r.Method)
			}
			io.WriteString(w, `{"tables":[{"name":"t","columns":[{"name":"id","type":"INTEGER","nullable":false}]}]}`)
		case "/v1/projects/project%2Fone/databases/db%2Fone/schema/tables":
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"name":"t","columns":[{"name":"id","type":"INTEGER","nullable":false}]}`)
		case "/v1/projects/project%2Fone/databases/db%2Fone/schema/columns":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["table"] != "t" || body["name"] != "email" || body["type"] != "VARCHAR" || body["nullable"] != false {
				t.Fatalf("column body: %+v", body)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"name":"email","type":"VARCHAR","nullable":false}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.EscapedPath())
		}
	})

	if _, err := c.CreateDatabaseWith(context.Background(), CreateDatabaseInput{}); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err := c.DatabaseSchema(context.Background(), ""); err == nil {
		t.Fatal("empty database id accepted")
	}
	if _, err := c.CreateTable(context.Background(), "", SchemaTable{Name: "t"}); err == nil {
		t.Fatal("empty database id accepted for create table")
	}
	if _, err := c.AddColumn(context.Background(), "", "t", SchemaColumn{Name: "id", Type: "INTEGER"}); err == nil {
		t.Fatal("empty database id accepted for add column")
	}

	db, err := c.CreateDatabaseWith(context.Background(), CreateDatabaseInput{
		Name: "shop", DataModel: "sql", InitSQL: "CREATE TABLE t (id INTEGER)",
	})
	if err != nil || db.DataModel != "sql" {
		t.Fatalf("create: %+v %v", db, err)
	}
	schema, err := c.DatabaseSchema(context.Background(), "db/one")
	if err != nil || len(schema.Tables) != 1 || schema.Tables[0].Columns[0].Name != "id" {
		t.Fatalf("schema: %+v %v", schema, err)
	}
	created, err := c.CreateTable(context.Background(), "db/one", SchemaTable{
		Name: "t", Columns: []SchemaColumn{{Name: "id", Type: "INTEGER", Nullable: &nullable}},
	})
	if err != nil || created.Name != "t" {
		t.Fatalf("create table: %+v %v", created, err)
	}
	col, err := c.AddColumn(context.Background(), "db/one", "t", SchemaColumn{Name: "email", Type: "VARCHAR", Nullable: &nullable})
	if err != nil || col.Name != "email" {
		t.Fatalf("add column: %+v %v", col, err)
	}
}

func TestCreateProjectSendsEngineOnlyWhenSet(t *testing.T) {
	var bodies []map[string]string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/projects" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		bodies = append(bodies, body)
		io.WriteString(w, `{"id":"p1","name":"n","engine":"sqlite"}`)
	})
	if _, err := c.CreateProject(context.Background(), "n", "sqlite"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateProject(context.Background(), "n", ""); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 {
		t.Fatalf("requests = %d", len(bodies))
	}
	if bodies[0]["engine"] != "sqlite" || bodies[0]["name"] != "n" {
		t.Fatalf("first body %v", bodies[0])
	}
	if _, ok := bodies[1]["engine"]; ok {
		t.Fatalf("empty engine must be omitted: %v", bodies[1])
	}
}
