package cloudagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
)

type fakeDB struct {
	dbs []DatabaseInfo
	sql string
}

func (f *fakeDB) ListDatabases(context.Context, auth.Principal, string) ([]DatabaseInfo, error) {
	return f.dbs, nil
}
func (f *fakeDB) ListCollections(context.Context, auth.Principal, string, string) ([]string, error) {
	return []string{"users"}, nil
}
func (f *fakeDB) ReadOnlyQuery(_ context.Context, _ auth.Principal, _, _, sqlText string, _ int) (SQLResult, error) {
	f.sql = sqlText
	if err := sqlguard.Validate(sqlText, sqlguard.ReadOnly); err != nil {
		return SQLResult{}, err
	}
	return SQLResult{Columns: []string{"n"}, Rows: [][]any{{int64(1)}}, RowCount: 1}, nil
}

func TestBuildToolsDropsUnknownAndRunsList(t *testing.T) {
	tools, err := buildTools([]string{ToolListDatabases, "delete_object"}, toolDeps{
		DB: &fakeDB{dbs: []DatabaseInfo{{ID: "d1", Name: "default", Status: "ready"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(tools))
	}
	inv, ok := tools[0].(tool.InvokableTool)
	if !ok {
		t.Fatal("not invokable")
	}
	ctx := withRunContext(context.Background(), RunContext{ProjectID: "p1"})
	got, err := inv.InvokableRun(ctx, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	var list []DatabaseInfo
	if err := json.Unmarshal([]byte(got), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "default" {
		t.Fatalf("got %+v", list)
	}
}

func TestReadonlySQLToolRejectsWrite(t *testing.T) {
	db := &fakeDB{}
	tools, err := buildTools([]string{ToolReadonlySQL}, toolDeps{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	inv := tools[0].(tool.InvokableTool)
	ctx := withRunContext(context.Background(), RunContext{ProjectID: "p1"})
	_, err = inv.InvokableRun(ctx, `{"database_id":"d1","sql":"DELETE FROM users"}`)
	if err == nil {
		t.Fatal("expected write sql to fail")
	}
}
