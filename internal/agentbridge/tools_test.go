// 文件说明：tools_test.go — internal/agentbridge 的单元测试（TestBuildToolsDropsUnknownAndRunsList、TestReadonlySQLToolRejectsWrite 等）
//
// 用法
//
//	go test ./internal/agentbridge -run 'TestBuildToolsDropsUnknownAndRunsList|TestReadonlySQLToolRejectsWrite'
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/tools_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//
// 主要符号
//
//	type fakeDB struct{…}
//	(f *fakeDB) ListDatabases(context.Context, auth.Principal, string) ([]DatabaseInfo, error)
//	(f *fakeDB) ListCollections(context.Context, auth.Principal, string, string) ([]string, error)
//	(f *fakeDB) ReadOnlyQuery(_ context.Context, _ auth.Principal, _, _, sqlText string, _ int) (SQLResult, error)
//	TestBuildToolsDropsUnknownAndRunsList(t *testing.T)
//	TestReadonlySQLToolRejectsWrite(t *testing.T)

package agentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/linkxzhou/SimpleBase/internal/auth"
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
	// 真实只读校验在宿主的 sqlguard（internal/agentbridge 有对账测试）。
	// 这里只做最小替身：非 SELECT/WITH 视为写查询，验证工具把错误回给模型。
	head := strings.ToUpper(strings.TrimSpace(sqlText))
	if !strings.HasPrefix(head, "SELECT") && !strings.HasPrefix(head, "WITH") {
		return SQLResult{}, errors.New("readonly only")
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
	if err == nil || !strings.Contains(err.Error(), "readonly only") {
		t.Fatalf("expected readonly error, got %v", err)
	}
}
