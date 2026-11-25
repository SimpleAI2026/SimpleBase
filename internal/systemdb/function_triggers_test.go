// 文件说明：function_triggers_test.go — internal/systemdb 的单元测试（TestFunctionTriggerRepository 等）
//
// 用法
//
//	go test ./internal/systemdb -run 'TestFunctionTriggerRepository'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app 等 8 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/function_triggers_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_approvals.go, agent_approvals_test.go, agent_collab.go, agent_collab_cron.go, agent_collab_test.go 等 54 个
//
// 主要符号
//
//	TestFunctionTriggerRepository(t *testing.T)

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
