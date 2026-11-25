// 文件说明：record_test.go — internal/audit 的单元测试（TestRecordDefaultsDetailAndListLimits 等）
//
// 用法
//
//	go test ./internal/audit -run 'TestRecordDefaultsDetailAndListLimits'
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/audit/record_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] service.go, service_test.go
//
// 主要符号
//
//	TestRecordDefaultsDetailAndListLimits(t *testing.T)

package audit

import (
	"context"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
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
	if ops[0].PrincipalID != auth.ActorSystem {
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
