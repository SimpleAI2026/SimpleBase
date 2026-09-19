package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func invokeTool(t *testing.T, tools []tool.BaseTool, idx int, ctx context.Context, args string) (string, error) {
	t.Helper()
	inv, ok := tools[idx].(tool.InvokableTool)
	if !ok {
		t.Fatal("not invokable")
	}
	return inv.InvokableRun(ctx, args)
}

func TestBuildToolsAllAndErrorPaths(t *testing.T) {
	db := &errDB{dbs: []DatabaseInfo{{ID: "d1", Name: "n", Status: "ready"}}}
	obj := &errObj{list: []ObjectInfo{{Key: "k", Size: 1, ETag: "e", ContentType: "text/plain"}}, head: ObjectInfo{Key: "k", Size: 2}}
	logs := &errLogs{
		events: []systemdb.LogEvent{{Level: "info", Logger: "l", Message: "hello", OccurredAt: time.Unix(0, 0).UTC()}},
		stats:  []systemdb.LogLevelCount{{Level: "info", Count: 4}},
	}
	ids := []string{
		ToolListDatabases, ToolListCollections, ToolReadonlySQL,
		ToolListObjects, ToolHeadObject, ToolSearchLogs, ToolLogLevelStats, "unknown",
	}
	tools, err := buildTools(ids, toolDeps{DB: db, Obj: obj, Logs: logs})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 7 {
		t.Fatalf("tools=%d", len(tools))
	}
	ctx := withRunContext(context.Background(), RunContext{ProjectID: "p1"})

	got, err := invokeTool(t, tools, 0, ctx, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	var list []DatabaseInfo
	if err := json.Unmarshal([]byte(got), &list); err != nil || len(list) != 1 {
		t.Fatalf("list_databases %s err=%v", got, err)
	}

	got, err = invokeTool(t, tools, 1, ctx, `{"database_id":"d1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(got)) {
		t.Fatal(got)
	}

	db.dbs = nil // unused
	got, err = invokeTool(t, tools, 2, ctx, `{"database_id":"d1","sql":"SELECT 1"}`)
	if err != nil {
		t.Fatal(err)
	}

	got, err = invokeTool(t, tools, 3, ctx, `{"prefix":"a/"}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err = invokeTool(t, tools, 4, ctx, `{"key":"k"}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err = invokeTool(t, tools, 5, ctx, `{"level":"info","q":"he","limit":0}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err = invokeTool(t, tools, 6, ctx, `{}`)
	if err != nil {
		t.Fatal(err)
	}

	// missing run context
	if _, err := invokeTool(t, tools, 0, context.Background(), `{}`); err == nil {
		t.Fatal("missing context")
	}
	if _, err := invokeTool(t, tools, 1, context.Background(), `{"database_id":"d1"}`); err == nil {
		t.Fatal("missing context")
	}
	if _, err := invokeTool(t, tools, 2, context.Background(), `{"database_id":"d1","sql":"SELECT 1"}`); err == nil {
		t.Fatal("missing context")
	}
	if _, err := invokeTool(t, tools, 3, context.Background(), `{}`); err == nil {
		t.Fatal("missing context")
	}
	if _, err := invokeTool(t, tools, 4, context.Background(), `{"key":"k"}`); err == nil {
		t.Fatal("missing context")
	}
	if _, err := invokeTool(t, tools, 5, context.Background(), `{}`); err == nil {
		t.Fatal("missing context")
	}
	if _, err := invokeTool(t, tools, 6, context.Background(), `{}`); err == nil {
		t.Fatal("missing context")
	}

	// validation
	if _, err := invokeTool(t, tools, 1, ctx, `{"database_id":""}`); err == nil {
		t.Fatal("collections require id")
	}
	if _, err := invokeTool(t, tools, 2, ctx, `{"database_id":"","sql":""}`); err == nil {
		t.Fatal("sql required")
	}
	if _, err := invokeTool(t, tools, 4, ctx, `{"key":"  "}`); err == nil {
		t.Fatal("key required")
	}

	// dep errors
	db.err = errors.New("db")
	obj.err = errors.New("obj")
	logs.err = errors.New("logs")
	if _, err := invokeTool(t, tools, 0, ctx, `{}`); err == nil {
		t.Fatal("db list err")
	}
	if _, err := invokeTool(t, tools, 1, ctx, `{"database_id":"d1"}`); err == nil {
		t.Fatal("coll err")
	}
	if _, err := invokeTool(t, tools, 2, ctx, `{"database_id":"d1","sql":"SELECT 1"}`); err == nil {
		t.Fatal("sql err")
	}
	if _, err := invokeTool(t, tools, 3, ctx, `{}`); err == nil {
		t.Fatal("obj list err")
	}
	if _, err := invokeTool(t, tools, 4, ctx, `{"key":"k"}`); err == nil {
		t.Fatal("head err")
	}
	if _, err := invokeTool(t, tools, 5, ctx, `{"limit":2}`); err == nil {
		t.Fatal("search err")
	}
	if _, err := invokeTool(t, tools, 6, ctx, `{}`); err == nil {
		t.Fatal("stats err")
	}

	// nil deps
	nilTools, err := buildTools(ids, toolDeps{})
	if err != nil {
		t.Fatal(err)
	}
	for i, args := range []string{`{}`, `{"database_id":"d1"}`, `{"database_id":"d1","sql":"SELECT 1"}`, `{}`, `{"key":"k"}`, `{}`, `{}`} {
		if _, err := invokeTool(t, nilTools, i, ctx, args); err == nil {
			t.Fatalf("nil dep tool %d", i)
		}
	}

	if _, err := marshalToolJSON(make(chan int)); err == nil {
		t.Fatal("marshal chan")
	}
	s, err := marshalToolJSON(map[string]string{"ok": "1"})
	if err != nil || s == "" {
		t.Fatal(err)
	}
	if _, err := requireRun(context.Background()); err == nil {
		t.Fatal("require run")
	}
	if _, err := requireRun(withRunContext(context.Background(), RunContext{})); err == nil {
		t.Fatal("empty project")
	}
}
