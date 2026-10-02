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
	keys := objectstore.KeyBuilder{RootPrefix: "simplebase", Environment: "test"}
	svc := catalog.NewService(s.CatalogRepo(), keys, nil, objectstore.DuckLakeStorage{}, nil)

	in := catalog.CreateDatabaseInput{
		TenantID:  catalog.ReservedTenantID,
		ProjectID: catalog.DevProjectID,
		Name:      "conflict-db",
	}
	// Seed 不建 dev-shop 项目：补建（幂等）。
	if err := s.CatalogRepo().CreateProject(ctx, catalog.Project{
		ID: catalog.DevProjectID, TenantID: catalog.ReservedTenantID,
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
