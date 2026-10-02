package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// countingRepo 统计读查询次数，验证缓存命中与写路径失效（§7.2 P1.3/P1.4）。
type countingRepo struct {
	Repository // 组合真实实现由具体测试决定；此处仅用到被计数的方法

	tenantQueries int
	dbQueries     int
	kvQueries     int

	tenant map[string]string
	dbs    map[string]Database // key: projectID+"/"+databaseID
	kvRows map[string]Database // key: projectID
}

func (r *countingRepo) GetProjectTenant(ctx context.Context, projectID string) (string, error) {
	r.tenantQueries++
	if t, ok := r.tenant[projectID]; ok {
		return t, nil
	}
	return "", ErrNotFound
}

func (r *countingRepo) GetDatabase(ctx context.Context, projectID, databaseID string) (Database, error) {
	r.dbQueries++
	if db, ok := r.dbs[projectID+"/"+databaseID]; ok {
		return db, nil
	}
	return Database{}, ErrNotFound
}

func (r *countingRepo) GetDatabaseByName(ctx context.Context, projectID, name string) (Database, error) {
	r.kvQueries++
	if db, ok := r.kvRows[projectID]; ok {
		return db, nil
	}
	return Database{}, ErrNotFound
}

// ProjectBelongsToTenant 走缓存免查路径（ctx 注入后不调用）；留桩防御。
func (r *countingRepo) ProjectBelongsToTenant(ctx context.Context, projectID, tenantID string) (bool, error) {
	return r.tenant[projectID] == tenantID, nil
}

func TestCatalogReadCacheReducesQueries(t *testing.T) {
	repo := &countingRepo{
		tenant: map[string]string{"p1": "t1"},
		dbs:    map[string]Database{"p1/d1": {ID: "d1", ProjectID: "p1", Status: DatabaseReady}},
		kvRows: map[string]Database{"p1": {ID: "kv1", ProjectID: "p1", Kind: DatabaseKindKV, Status: DatabaseReady}},
	}
	svc := NewService(repo, objectstore.KeyBuilder{}, nil, objectstore.DuckLakeStorage{}, nil)
	principal := auth.Principal{APIKeyID: "k1", TenantID: "t1"}
	principal.Permissions = map[auth.Permission]struct{}{auth.ProjectAdmin: {}}
	ctx := context.Background()

	// 三条读路径各查 3 次：只有首次落库。GetDatabase 带上 ctx 注入
	//（模拟 project 中间件），ensureTenantMatch 也免查（§7.2 P1.1）。
	for i := 0; i < 3; i++ {
		if _, err := svc.ResolveProjectTenant(ctx, "p1"); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		rctx := WithResolvedProjectTenant(ctx, "p1", "t1")
		if _, err := svc.GetDatabase(rctx, principal, "p1", "d1"); err != nil {
			t.Fatalf("get db: %v", err)
		}
		if _, err := svc.GetKVDatabase(rctx, "t1", "p1"); err != nil {
			t.Fatalf("get kv: %v", err)
		}
	}
	if repo.tenantQueries != 1 || repo.dbQueries != 1 || repo.kvQueries != 1 {
		t.Fatalf("queries tenant=%d db=%d kv=%d, want all 1", repo.tenantQueries, repo.dbQueries, repo.kvQueries)
	}

	// 失效后重新查库。
	svc.InvalidateAllCatalogCache()
	if _, err := svc.ResolveProjectTenant(ctx, "p1"); err != nil {
		t.Fatalf("resolve after invalidate: %v", err)
	}
	if repo.tenantQueries != 2 {
		t.Fatalf("tenant queries=%d after invalidate, want 2", repo.tenantQueries)
	}

	// 未命中项目仍返回 ErrNotFound（不缓存错误）。
	if _, err := svc.ResolveProjectTenant(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
}

func TestCatalogCacheTTLExpiry(t *testing.T) {
	repo := &countingRepo{tenant: map[string]string{"p1": "t1"}}
	svc := NewService(repo, objectstore.KeyBuilder{}, nil, objectstore.DuckLakeStorage{}, nil)
	// 注入时钟越过 TTL。
	svc.tenantCache.now = func() time.Time { return fakeClockNow }
	fakeClockNow = time.Now()
	ctx := context.Background()
	if _, err := svc.ResolveProjectTenant(ctx, "p1"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	fakeClockNow = fakeClockNow.Add(projectTenantTTL + time.Second)
	if _, err := svc.ResolveProjectTenant(ctx, "p1"); err != nil {
		t.Fatalf("resolve after ttl: %v", err)
	}
	if repo.tenantQueries != 2 {
		t.Fatalf("queries=%d, want 2 after TTL expiry", repo.tenantQueries)
	}
}

var fakeClockNow time.Time
