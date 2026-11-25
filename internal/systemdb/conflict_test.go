// 文件说明：conflict_test.go — internal/systemdb 的单元测试（TestCreateDatabaseIdempotentConflict 等）
//
// 用法
//
//	go test ./internal/systemdb -run 'TestCreateDatabaseIdempotentConflict'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app 等 8 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/conflict_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_approvals.go, agent_approvals_test.go, agent_collab.go, agent_collab_cron.go, agent_collab_test.go 等 54 个
//
// 主要符号
//
//	timeNow() time.Time
//	TestCreateDatabaseIdempotentConflict(t *testing.T)

package systemdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func timeNow() time.Time { return time.Now().UTC() }

// TestCreateDatabaseIdempotentConflict 复现 perfbench 复跑场景（§7）：
// 同名库第二次创建必须映射为 ErrAlreadyExists（409），
// 不得冒出未映射错误（500）。覆盖缓存与失效逻辑加入后的契约。
func TestCreateDatabaseIdempotentConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := Seed(ctx, SeedInput{Store: s}); err != nil {
		t.Fatal(err)
	}
	keys := objectstore.KeyBuilder{RootPrefix: "simplebase"}
	svc := catalog.NewService(s.CatalogRepo(), keys, nil, objectstore.DuckLakeStorage{}, nil)

	in := catalog.CreateDatabaseInput{
		TenantID:  catalog.ReservedTenantID,
		ProjectID: "perfprj1",
		Name:      "conflict-db",
	}
	// Seed 只建系统项目：补建普通项目（幂等）。
	if err := s.CatalogRepo().CreateProject(ctx, catalog.Project{
		ID: "perfprj1", TenantID: catalog.ReservedTenantID,
		Name: "perfbench", CreatedAt: timeNow(),
	}); err != nil && !errors.Is(err, catalog.ErrAlreadyExists) {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.CreateDatabase(ctx, in); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// 第二次同名：必须 409 语义（ErrAlreadyExists），否则 HTTP 层变 500。
	_, err := svc.CreateDatabase(ctx, in)
	if !errors.Is(err, catalog.ErrAlreadyExists) {
		t.Fatalf("second create: want ErrAlreadyExists, got %v (%T)", err, err)
	}
}
