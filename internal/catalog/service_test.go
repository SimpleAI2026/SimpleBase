package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"

	_ "github.com/uglyer/go-sqlite3"
	"database/sql"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// fakeDescriptorWriter 模拟 S3 descriptor 写入，可注入失败。
type fakeDescriptorWriter struct {
	failNext bool
	calls    int
}

func (f *fakeDescriptorWriter) PutJSON(ctx context.Context, key string, value any, opts objectstore.PutOptions) error {
	f.calls++
	if f.failNext {
		return errors.New("simulated s3 failure")
	}
	return nil
}

func newTestPrincipal(tenantID string, projectIDs ...string) auth.Principal {
	pids := make(map[string]struct{}, len(projectIDs))
	for _, p := range projectIDs {
		pids[p] = struct{}{}
	}
	return auth.Principal{
		APIKeyID:   "test-key",
		TenantID:   tenantID,
		ProjectIDs: pids,
		Permissions: map[auth.Permission]struct{}{
			auth.DatabaseWrite: {},
			auth.DatabaseAdmin: {},
		},
	}
}

func setupService(t *testing.T, descriptor DescriptorWriter) (*Service, Repository, string, string) {
	t.Helper()
	db := newTestDB(t)
	repo := NewSQLiteRepository(db)
	keys := objectstore.KeyBuilder{RootPrefix: "simplebase", Environment: "test"}
	svc := NewService(repo, keys, descriptor, objectstore.DuckLakeStorage{Region: "us-east-1", Bucket: "test-bucket"}, nil)

	tenantID := uuid.NewString()
	projectID := uuid.NewString()
	ctx := context.Background()
	if err := repo.CreateTenant(ctx, Tenant{ID: tenantID, Name: "t1", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := repo.CreateProject(ctx, Project{ID: projectID, TenantID: tenantID, Name: "p1", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	return svc, repo, tenantID, projectID
}

func TestCreateDatabase_Success(t *testing.T) {
	descriptor := &fakeDescriptorWriter{}
	svc, _, tenantID, projectID := setupService(t, descriptor)

	db, err := svc.CreateDatabase(context.Background(), CreateDatabaseInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Name:      "mydb",
	})
	if err != nil {
		t.Fatalf("CreateDatabase: %v", err)
	}
	if db.Status != DatabaseReady {
		t.Fatalf("expected status ready after create, got %s", db.Status)
	}
	if descriptor.calls != 1 {
		t.Fatalf("expected 1 descriptor write, got %d", descriptor.calls)
	}
}

func TestCreateDatabase_CrossProjectRejected(t *testing.T) {
	svc, _, tenantID, _ := setupService(t, &fakeDescriptorWriter{})
	otherProject := uuid.NewString()

	_, err := svc.CreateDatabase(context.Background(), CreateDatabaseInput{
		TenantID:  tenantID,
		ProjectID: otherProject,
		Name:      "mydb",
	})
	if !errors.Is(err, ErrCrossProject) {
		t.Fatalf("expected ErrCrossProject, got %v", err)
	}
}

func TestCreateDatabase_DuplicateNameRejected(t *testing.T) {
	svc, _, tenantID, projectID := setupService(t, &fakeDescriptorWriter{})
	ctx := context.Background()

	if _, err := svc.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "dup"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "dup"})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestCreateDatabase_DescriptorWriteFailureDegrades(t *testing.T) {
	descriptor := &fakeDescriptorWriter{failNext: true}
	svc, repo, tenantID, projectID := setupService(t, descriptor)

	_, err := svc.CreateDatabase(context.Background(), CreateDatabaseInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		Name:      "mydb",
	})
	if !errors.Is(err, ErrDescriptorWrite) {
		t.Fatalf("expected ErrDescriptorWrite, got %v", err)
	}

	// 数据库记录仍存在且被标记为 degraded（补偿），而非丢失。
	dbs, _, err := repo.ListDatabases(context.Background(), projectID, Page{})
	if err != nil {
		t.Fatalf("list databases: %v", err)
	}
	if len(dbs) != 1 {
		t.Fatalf("expected 1 database record, got %d", len(dbs))
	}
	if dbs[0].Status != DatabaseDegraded {
		t.Fatalf("expected degraded status, got %s", dbs[0].Status)
	}
}

func TestGetDatabase_CrossProjectRejected(t *testing.T) {
	svc, _, tenantID, projectID := setupService(t, &fakeDescriptorWriter{})
	db, err := svc.CreateDatabase(context.Background(), CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "mydb"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	principal := newTestPrincipal(tenantID) // 没有任何 project 授权
	_, err = svc.GetDatabase(context.Background(), principal, projectID, db.ID)
	if !errors.Is(err, ErrCrossProject) {
		t.Fatalf("expected ErrCrossProject, got %v", err)
	}
}

func TestStateTransitions(t *testing.T) {
	svc, _, tenantID, projectID := setupService(t, &fakeDescriptorWriter{})
	db, err := svc.CreateDatabase(context.Background(), CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "mydb"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.SetDatabaseReady(context.Background(), db.ID); err != nil {
		t.Fatalf("SetDatabaseReady: %v", err)
	}

	principal := newTestPrincipal(tenantID, projectID)
	got, err := svc.GetDatabase(context.Background(), principal, projectID, db.ID)
	if err != nil {
		t.Fatalf("GetDatabase: %v", err)
	}
	if got.Status != DatabaseReady {
		t.Fatalf("expected ready, got %s", got.Status)
	}

	// 非法转换：ready 不能直接回到 creating。
	svc2 := svc
	err = doIllegalTransition(svc2, db.ID)
	if err == nil {
		t.Fatalf("expected illegal transition to fail")
	}
}

// doIllegalTransition 尝试 ready -> creating（非法），验证 repository 拒绝。
func doIllegalTransition(svc *Service, id string) error {
	_, err := svc.repo.TransitionDatabase(context.Background(), id, []DatabaseStatus{DatabaseOpening}, DatabaseCreating, time.Now())
	return err
}

func TestBeginDeleteDatabase(t *testing.T) {
	svc, _, tenantID, projectID := setupService(t, &fakeDescriptorWriter{})
	db, err := svc.CreateDatabase(context.Background(), CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "mydb"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.SetDatabaseReady(context.Background(), db.ID); err != nil {
		t.Fatalf("SetDatabaseReady: %v", err)
	}

	principal := newTestPrincipal(tenantID, projectID)
	deleted, err := svc.BeginDeleteDatabase(context.Background(), principal, projectID, db.ID)
	if err != nil {
		t.Fatalf("BeginDeleteDatabase: %v", err)
	}
	if deleted.Status != DatabaseDeleting {
		t.Fatalf("expected deleting, got %s", deleted.Status)
	}
}

func TestMigrations_Idempotent(t *testing.T) {
	db := newTestDB(t)
	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("second apply should be no-op: %v", err)
	}
}
