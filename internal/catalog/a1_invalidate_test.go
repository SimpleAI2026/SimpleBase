package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// A1 表征（planv6.0 §7 A1）：建库成功后 KV/普通库的缓存可见性与失效行为。
// 不改变任何缓存逻辑，仅固化当前可观察行为，作为后续删除第二次失效的等价证据基础。

// belongsRepo 覆盖归属点查与创建路径，其余读路径沿用 countingRepo 行为。
type belongsRepo struct {
	countingRepo
	created map[string]Database // key: projectID+"/"+databaseID
}

func (r *belongsRepo) ProjectBelongsToTenant(ctx context.Context, projectID, tenantID string) (bool, error) {
	return true, nil
}

func (r *belongsRepo) GetProjectTenant(ctx context.Context, projectID string) (string, error) {
	return "t1", nil
}

func (r *belongsRepo) CreateDatabase(ctx context.Context, db Database) error {
	if r.created == nil {
		r.created = map[string]Database{}
	}
	r.created[db.ProjectID+"/"+db.ID] = db
	r.dbs[db.ProjectID+"/"+db.ID] = db
	r.kvRows[db.ProjectID] = db // KV 行按项目名唯一，创建后即可读
	return nil
}

func (r *belongsRepo) TransitionDatabase(ctx context.Context, id string, from []DatabaseStatus, to DatabaseStatus, at time.Time) (Database, error) {
	for k, db := range r.dbs {
		if db.ID == id {
			db.Status = to
			db.UpdatedAt = at
			r.dbs[k] = db
			if r.kvRows[db.ProjectID].ID == id {
				r.kvRows[db.ProjectID] = db
			}
			return db, nil
		}
	}
	return Database{}, ErrNotFound
}

func newSvcBelongs(t *testing.T) (*Service, *belongsRepo, string, string) {
	t.Helper()
	projectID := objectstore.NewProjectID()
	repo := &belongsRepo{countingRepo: countingRepo{
		tenant: map[string]string{projectID: "t1"},
		dbs:    map[string]Database{},
		kvRows: map[string]Database{},
	}}
	svc := NewService(repo, objectstore.KeyBuilder{RootPrefix: "simplebase"}, nil, objectstore.DuckLakeStorage{}, nil)
	return svc, repo, "t1", projectID
}

func TestCreateDatabase_InvalidateOnCreate(t *testing.T) {
	// 普通 SQL 库：创建后按 ID 读取必须可见新行（dbCache 失效正确）。
	{
		svc, repo, tenantID, projectID := newSvcBelongs(t)
		ctx := WithResolvedProjectTenant(context.Background(), projectID, tenantID)
		db, err := svc.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "sqldb"})
		if err != nil {
			t.Fatalf("CreateDatabase: %v", err)
		}
		if db.Kind != DatabaseKindUser {
			t.Fatalf("kind=%s, want user", db.Kind)
		}
		if _, err := svc.GetDatabase(ctx, newTestPrincipal(tenantID, projectID), projectID, db.ID); err != nil {
			t.Fatalf("GetDatabase after create: %v", err)
		}
		if repo.dbQueries != 1 {
			t.Fatalf("dbQueries=%d, want 1 (created row readable, invalidation prevents stale cache)", repo.dbQueries)
		}
	}

	// KV 库：创建后 GetKVDatabase 必须读到新行（kvCache 失效正确）。
	{
		svc, repo, tenantID, projectID := newSvcBelongs(t)
		ctx := WithResolvedProjectTenant(context.Background(), projectID, tenantID)
		db, err := svc.CreateKVDatabase(ctx, tenantID, projectID)
		if err != nil {
			t.Fatalf("CreateKVDatabase: %v", err)
		}
		if db.Kind != DatabaseKindKV {
			t.Fatalf("kind=%s, want kv", db.Kind)
		}
		if db.Name != KVDatabaseName {
			t.Fatalf("name=%s, want %s", db.Name, KVDatabaseName)
		}
		// CreateKVDatabase 前置 GetKVDatabase 幂等检查产生 1 次未命中查询；
		// 创建成功并全量失效后，后续读取各 1 次落库 + 缓存命中。
		got, err := svc.GetKVDatabase(ctx, tenantID, projectID)
		if err != nil {
			t.Fatalf("GetKVDatabase after create: %v", err)
		}
		if got.ID != db.ID {
			t.Fatalf("GetKVDatabase id=%s, want %s", got.ID, db.ID)
		}
		if _, err := svc.GetKVDatabase(ctx, tenantID, projectID); err != nil {
			t.Fatalf("GetKVDatabase second read: %v", err)
		}
		if repo.kvQueries != 2 {
			t.Fatalf("kvQueries=%d, want 2 (pre-check + post-invalidate reload; second read cached)", repo.kvQueries)
		}
	}

	// KV 库创建失败（仓储失败）：返回错误，不返回行。
	{
		svc, _, tenantID, projectID := newSvcBelongs(t)
		svc.repo = &failingCreateRepo{countingRepo: countingRepo{
			tenant: map[string]string{projectID: "t1"},
			dbs:    map[string]Database{},
			kvRows: map[string]Database{},
		}}
		ctx := WithResolvedProjectTenant(context.Background(), projectID, tenantID)
		if _, err := svc.CreateKVDatabase(ctx, tenantID, projectID); err == nil {
			t.Fatal("CreateKVDatabase with failing repo: want error")
		}
	}
}

// failingCreateRepo 让 CreateDatabase 恒失败，其余读路径沿用 map 行为。
type failingCreateRepo struct {
	countingRepo
}

func (f *failingCreateRepo) ProjectBelongsToTenant(ctx context.Context, projectID, tenantID string) (bool, error) {
	return true, nil
}

func (f *failingCreateRepo) GetProjectTenant(ctx context.Context, projectID string) (string, error) {
	return "t1", nil
}

func (f *failingCreateRepo) CreateDatabase(ctx context.Context, db Database) error {
	return errors.New("simulated create failure")
}
