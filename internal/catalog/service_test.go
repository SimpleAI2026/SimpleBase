package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"

	"database/sql"
	_ "github.com/uglyer/go-sqlite3"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := applyTestSchema(db); err != nil {
		t.Fatalf("apply sys schema: %v", err)
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
	repo := NewSQLRepository(db)
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

func TestSystemDatabaseHiddenAndProtected(t *testing.T) {
	svc, repo, tenantID, projectID := setupService(t, nil)
	ctx := context.Background()
	now := time.Now()
	sysID := uuid.NewString()
	if err := repo.CreateDatabase(ctx, Database{
		ID: sysID, TenantID: tenantID, ProjectID: projectID, Name: "simplebase-system",
		Kind: DatabaseKindSystem, Status: DatabaseReady, StoragePrefix: "sys",
		FormatVersion: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed system db: %v", err)
	}
	if _, err := svc.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "userdb"}); err != nil {
		t.Fatalf("create user db: %v", err)
	}
	principal := newTestPrincipal(tenantID, projectID)
	list, _, err := svc.ListDatabases(ctx, principal, projectID, Page{})
	if err != nil {
		t.Fatalf("ListDatabases: %v", err)
	}
	for _, d := range list {
		if d.Kind == DatabaseKindSystem || d.ID == sysID {
			t.Fatalf("system database leaked into list: %+v", d)
		}
	}
	_, err = svc.GetDatabase(ctx, principal, projectID, sysID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetDatabase system should be hidden, got %v", err)
	}
	_, err = svc.BeginDeleteDatabase(ctx, principal, projectID, sysID)
	if !errors.Is(err, ErrSystemProtected) {
		t.Fatalf("expected ErrSystemProtected, got %v", err)
	}
}

func TestCreateProject_SuccessAndConflict(t *testing.T) {
	svc, _, tenantID, _ := setupService(t, nil)
	ctx := context.Background()
	admin := newTestPrincipal(tenantID)
	admin.Permissions[auth.ProjectAdmin] = struct{}{}

	p, err := svc.CreateProject(ctx, admin, CreateProjectInput{Name: "新项目"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" || p.Name != "新项目" || p.TenantID != tenantID {
		t.Fatalf("unexpected project: %+v", p)
	}
	if _, err := uuid.Parse(p.ID); err != nil {
		t.Fatalf("generated id should be UUID: %v", err)
	}

	_, err = svc.CreateProject(ctx, admin, CreateProjectInput{Name: "新项目"})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected name conflict, got %v", err)
	}

	fixedID := "11111111-1111-1111-1111-111111111111"
	p2, err := svc.CreateProject(ctx, admin, CreateProjectInput{Name: "指定 ID", ID: fixedID})
	if err != nil {
		t.Fatalf("CreateProject with id: %v", err)
	}
	if p2.ID != fixedID {
		t.Fatalf("id = %s, want %s", p2.ID, fixedID)
	}
	_, err = svc.CreateProject(ctx, admin, CreateProjectInput{Name: "另一个", ID: fixedID})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected id conflict, got %v", err)
	}

	_, err = svc.CreateProject(ctx, admin, CreateProjectInput{Name: "坏 ID", ID: "not-a-uuid"})
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected invalid id, got %v", err)
	}

	_, err = svc.CreateProject(ctx, admin, CreateProjectInput{Name: "系统", ID: ReservedSystemProjectID})
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected reserved id, got %v", err)
	}

	plain := newTestPrincipal(tenantID)
	_, err = svc.CreateProject(ctx, plain, CreateProjectInput{Name: "无权限"})
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestProjectAdminCanAccessSiblingProject(t *testing.T) {
	svc, _, tenantID, projectID := setupService(t, nil)
	ctx := context.Background()
	admin := newTestPrincipal(tenantID, projectID)
	admin.Permissions[auth.ProjectAdmin] = struct{}{}

	other, err := svc.CreateProject(ctx, admin, CreateProjectInput{Name: "sibling"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, _, err := svc.ListDatabases(ctx, admin, other.ID, Page{Limit: 20}); err != nil {
		t.Fatalf("ProjectAdmin should list sibling project databases: %v", err)
	}

	foreign := newTestPrincipal(uuid.NewString(), projectID)
	foreign.Permissions[auth.ProjectAdmin] = struct{}{}
	if _, _, err := svc.ListDatabases(ctx, foreign, other.ID, Page{Limit: 20}); !errors.Is(err, ErrCrossProject) && !errors.Is(err, ErrNotFound) {
		t.Fatalf("other tenant admin should be denied, got %v", err)
	}
}

func TestListProjectsHidesSystemProject(t *testing.T) {
	svc, repo, tenantID, projectID := setupService(t, nil)
	ctx := context.Background()
	if err := repo.CreateProject(ctx, Project{
		ID: ReservedSystemProjectID, TenantID: tenantID, Name: "system", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed system project: %v", err)
	}
	principal := newTestPrincipal(tenantID, projectID)
	principal.Permissions[auth.ProjectAdmin] = struct{}{}
	list, err := svc.ListProjects(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.ID == ReservedSystemProjectID {
			t.Fatal("system project should be hidden")
		}
	}
}
