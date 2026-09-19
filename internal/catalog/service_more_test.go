package catalog

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

type hookRepo struct {
	Repository
	belongsFn    func(ctx context.Context, projectID, tenantID string) (bool, error)
	getTenantFn  func(ctx context.Context, projectID string) (string, error)
	listProjFn   func(ctx context.Context, tenantID string) ([]Project, error)
	transitionFn func(ctx context.Context, id string, from []DatabaseStatus, to DatabaseStatus, at time.Time) (Database, error)
	markDelFn    func(ctx context.Context, id string, at time.Time) error
	getDBFn      func(ctx context.Context, projectID, databaseID string) (Database, error)
}

func (r *hookRepo) ProjectBelongsToTenant(ctx context.Context, projectID, tenantID string) (bool, error) {
	if r.belongsFn != nil {
		return r.belongsFn(ctx, projectID, tenantID)
	}
	return r.Repository.ProjectBelongsToTenant(ctx, projectID, tenantID)
}

func (r *hookRepo) GetProjectTenant(ctx context.Context, projectID string) (string, error) {
	if r.getTenantFn != nil {
		return r.getTenantFn(ctx, projectID)
	}
	return r.Repository.GetProjectTenant(ctx, projectID)
}

func (r *hookRepo) ListProjectsByTenant(ctx context.Context, tenantID string) ([]Project, error) {
	if r.listProjFn != nil {
		return r.listProjFn(ctx, tenantID)
	}
	return r.Repository.ListProjectsByTenant(ctx, tenantID)
}

func (r *hookRepo) TransitionDatabase(ctx context.Context, id string, from []DatabaseStatus, to DatabaseStatus, at time.Time) (Database, error) {
	if r.transitionFn != nil {
		return r.transitionFn(ctx, id, from, to, at)
	}
	return r.Repository.TransitionDatabase(ctx, id, from, to, at)
}

func (r *hookRepo) MarkDeleted(ctx context.Context, id string, at time.Time) error {
	if r.markDelFn != nil {
		return r.markDelFn(ctx, id, at)
	}
	return r.Repository.MarkDeleted(ctx, id, at)
}

func (r *hookRepo) GetDatabase(ctx context.Context, projectID, databaseID string) (Database, error) {
	if r.getDBFn != nil {
		return r.getDBFn(ctx, projectID, databaseID)
	}
	return r.Repository.GetDatabase(ctx, projectID, databaseID)
}

type failPurger struct{ err error }

func (p failPurger) DeletePrefix(context.Context, string) error { return p.err }

func newHookedService(t *testing.T, descriptor DescriptorWriter, hook *hookRepo) (*Service, *hookRepo, string, string) {
	t.Helper()
	db := newTestDB(t)
	base := NewSQLRepository(db)
	hook.Repository = base
	keys := objectstore.KeyBuilder{RootPrefix: "simplebase", Environment: "test"}
	logger := observability.NewLogger("error", "json", io.Discard)
	svc := NewService(hook, keys, descriptor, objectstore.DuckLakeStorage{Region: "us-east-1", Bucket: "test-bucket"}, logger)
	tenantID := uuid.NewString()
	projectID := uuid.NewString()
	ctx := context.Background()
	if err := base.CreateTenant(ctx, Tenant{ID: tenantID, Name: "t1", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := base.CreateProject(ctx, Project{ID: projectID, TenantID: tenantID, Name: "p1", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return svc, hook, tenantID, projectID
}

func adminPrincipal(tenantID string, projectIDs ...string) auth.Principal {
	p := newTestPrincipal(tenantID, projectIDs...)
	p.Permissions[auth.ProjectAdmin] = struct{}{}
	return p
}

func TestService_CreateDatabaseValidationAndErrors(t *testing.T) {
	ctx := context.Background()
	svc, _, tenantID, projectID := setupService(t, &fakeDescriptorWriter{})

	if _, err := svc.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: ""}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("empty name: %v", err)
	}
	if _, err := svc.CreateDatabase(ctx, CreateDatabaseInput{Name: "x"}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("missing ids: %v", err)
	}

	hooked, hook, tenantID, projectID := newHookedService(t, &fakeDescriptorWriter{}, &hookRepo{})
	hook.belongsFn = func(context.Context, string, string) (bool, error) { return false, errors.New("db down") }
	if _, err := hooked.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "n"}); err == nil || !strings.Contains(err.Error(), "check project ownership") {
		t.Fatalf("belongs error: %v", err)
	}

	// non-UUID tenant fails KeyBuilder.DataPrefix
	raw := newTestDB(t)
	repo := NewSQLRepository(raw)
	keys := objectstore.KeyBuilder{RootPrefix: "simplebase", Environment: "test"}
	svc2 := NewService(repo, keys, nil, objectstore.DuckLakeStorage{}, nil)
	if err := repo.CreateTenant(ctx, Tenant{ID: "not-a-uuid", Name: "t", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	pid := uuid.NewString()
	if err := repo.CreateProject(ctx, Project{ID: pid, TenantID: "not-a-uuid", Name: "p", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.CreateDatabase(ctx, CreateDatabaseInput{TenantID: "not-a-uuid", ProjectID: pid, Name: "n"}); err == nil || !strings.Contains(err.Error(), "build storage prefix") {
		t.Fatalf("prefix error: %v", err)
	}

	// non-UUID project passes DataPrefix but fails descriptor.Validate
	svc3, repo3, tenant3, _ := setupService(t, &fakeDescriptorWriter{})
	badPID := "not-a-uuid-project----------------" // 32 chars, not UUID
	if err := repo3.CreateProject(ctx, Project{ID: badPID, TenantID: tenant3, Name: "bad", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	_, err := svc3.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenant3, ProjectID: badPID, Name: "n"})
	if !errors.Is(err, ErrDescriptorWrite) {
		t.Fatalf("expected descriptor validate fail, got %v", err)
	}

	// SetDatabaseReady after create fails → degrade
	failReady, hook, tenantID, projectID := newHookedService(t, &fakeDescriptorWriter{}, &hookRepo{})
	hook.transitionFn = func(ctx context.Context, id string, from []DatabaseStatus, to DatabaseStatus, at time.Time) (Database, error) {
		if to == DatabaseReady {
			return Database{}, errors.New("ready failed")
		}
		return hook.Repository.TransitionDatabase(ctx, id, from, to, at)
	}
	if _, err := failReady.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "n"}); err == nil || !strings.Contains(err.Error(), "mark ready after create") {
		t.Fatalf("ready fail: %v", err)
	}

	// degrade after create also fails (logger path)
	failAll, hook2, tenantID, projectID := newHookedService(t, &fakeDescriptorWriter{failNext: true}, &hookRepo{})
	hook2.transitionFn = func(context.Context, string, []DatabaseStatus, DatabaseStatus, time.Time) (Database, error) {
		return Database{}, errors.New("cannot degrade")
	}
	if _, err := failAll.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "n"}); !errors.Is(err, ErrDescriptorWrite) {
		t.Fatalf("descriptor+degrade fail: %v", err)
	}

	if svc.Repository() == nil {
		t.Fatal("Repository() should return repo")
	}
}

func TestService_GetListResolveAndProjects(t *testing.T) {
	ctx := context.Background()
	svc, repo, tenantID, projectID := setupService(t, nil)

	if _, err := svc.ResolveProjectTenant(ctx, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolveProjectTenant(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}

	db, err := svc.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "userdb"})
	if err != nil {
		t.Fatal(err)
	}
	plain := newTestPrincipal(tenantID, projectID)
	got, err := svc.GetDatabase(ctx, plain, projectID, db.ID)
	if err != nil || got.ID != db.ID {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if _, err := svc.GetDatabase(ctx, plain, projectID, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing db: %v", err)
	}

	list, cursor, err := svc.ListDatabases(ctx, plain, projectID, Page{Limit: 20})
	if err != nil || len(list) != 1 || cursor != "" {
		t.Fatalf("list: n=%d cursor=%q err=%v", len(list), cursor, err)
	}

	// admin project listing system dbs
	if err := repo.CreateProject(ctx, Project{ID: ReservedSystemProjectID, TenantID: tenantID, Name: "system", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	sysID := uuid.NewString()
	if err := repo.CreateDatabase(ctx, Database{
		ID: sysID, TenantID: tenantID, ProjectID: ReservedSystemProjectID, Name: "simplebase-system",
		Kind: DatabaseKindSystem, Status: DatabaseReady, StoragePrefix: "sys", FormatVersion: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	admin := adminPrincipal(tenantID, projectID)
	sysList, _, err := svc.ListDatabases(ctx, admin, ReservedSystemProjectID, Page{})
	if err != nil || len(sysList) != 1 || sysList[0].ID != sysID {
		t.Fatalf("admin list: %+v err=%v", sysList, err)
	}
	gotSys, err := svc.GetDatabase(ctx, admin, ReservedSystemProjectID, sysID)
	if err != nil || gotSys.ID != sysID {
		t.Fatalf("admin get system db: %+v err=%v", gotSys, err)
	}
	// non-admin cannot access system project
	if _, _, err := svc.ListDatabases(ctx, plain, ReservedSystemProjectID, Page{}); !errors.Is(err, ErrCrossProject) {
		t.Fatalf("plain admin project: %v", err)
	}

	// ListProjects: admin first from seed, plus synthetic when missing
	projects, err := svc.ListProjects(ctx, admin)
	if err != nil || len(projects) == 0 || projects[0].ID != ReservedSystemProjectID {
		t.Fatalf("admin list projects: %+v err=%v", projects, err)
	}
	plainList, err := svc.ListProjects(ctx, plain)
	if err != nil || len(plainList) != 1 || plainList[0].ID != projectID {
		t.Fatalf("plain list: %+v err=%v", plainList, err)
	}
	if _, err := svc.ListProjects(ctx, auth.Principal{}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("no tenant: %v", err)
	}

	hooked, hook, tenantID, _ := newHookedService(t, nil, &hookRepo{})
	hook.listProjFn = func(context.Context, string) ([]Project, error) { return nil, errors.New("list fail") }
	if _, err := hooked.ListProjects(ctx, adminPrincipal(tenantID)); err == nil {
		t.Fatal("list projects error")
	}
	// no admin row → synthetic admin for ProjectAdmin
	hook.listProjFn = func(context.Context, string) ([]Project, error) {
		return []Project{{ID: uuid.NewString(), Name: "only-user", TenantID: tenantID}}, nil
	}
	syn, err := hooked.ListProjects(ctx, adminPrincipal(tenantID))
	if err != nil || len(syn) < 1 || syn[0].ID != ReservedSystemProjectID || syn[0].Name != AdminProjectName {
		t.Fatalf("synthetic admin: %+v err=%v", syn, err)
	}

	// CreateProject: no tenant
	if _, err := svc.CreateProject(ctx, auth.Principal{Permissions: map[auth.Permission]struct{}{auth.ProjectAdmin: {}}}, CreateProjectInput{Name: "x"}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("create project no tenant: %v", err)
	}
}

func TestService_EnsureProjectAccessAndDeletePaths(t *testing.T) {
	ctx := context.Background()
	svc, repo, tenantID, projectID := setupService(t, &fakeDescriptorWriter{})
	logger := observability.NewLogger("warn", "json", io.Discard)
	svc.logger = logger

	db, err := svc.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "delme"})
	if err != nil {
		t.Fatal(err)
	}
	plain := newTestPrincipal(tenantID, projectID)
	admin := adminPrincipal(tenantID)

	// admin empty tenant
	if _, _, err := svc.ListDatabases(ctx, auth.Principal{Permissions: map[auth.Permission]struct{}{auth.ProjectAdmin: {}}}, projectID, Page{}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("admin no tenant: %v", err)
	}
	// admin missing project
	if _, _, err := svc.ListDatabases(ctx, admin, uuid.NewString(), Page{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("admin missing project: %v", err)
	}

	if _, err := svc.BeginDeleteDatabase(ctx, plain, projectID, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}

	// illegal state: already deleting then delete again
	if _, err := svc.BeginDeleteDatabase(ctx, plain, projectID, db.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BeginDeleteDatabase(ctx, plain, projectID, db.ID); err == nil {
		t.Fatal("second delete should fail transition")
	}

	// closer warn + purger fail
	db2, err := svc.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "purge-fail"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.DeleteDatabaseSync(ctx, plain, projectID, db2.ID,
		func(context.Context, string) error { return errors.New("close failed") },
		failPurger{err: errors.New("s3 down")},
	)
	if err == nil || !strings.Contains(err.Error(), "sync purge") {
		t.Fatalf("purger fail: %v", err)
	}

	// DatabasePrefix fallback: non-UUID tenant/id so keys.DatabasePrefix fails
	oddID := "not-uuid-db"
	if err := repo.CreateDatabase(ctx, Database{
		ID: oddID, TenantID: "not-uuid-tenant", ProjectID: projectID, Name: "odd",
		Kind: DatabaseKindUser, Status: DatabaseReady, StoragePrefix: "fallback-prefix",
		FormatVersion: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	rec := &recordingPurger{}
	out, err := svc.DeleteDatabaseSync(ctx, plain, projectID, oddID, nil, rec)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != DatabaseDeleted {
		t.Fatalf("status %s", out.Status)
	}
	if len(rec.deleted) != 1 || rec.deleted[0] != "fallback-prefix" {
		t.Fatalf("fallback prefix: %v", rec.deleted)
	}

	// MarkDatabaseDeleted idempotent on already-deleted
	if err := svc.MarkDatabaseDeleted(ctx, oddID, time.Now()); err != nil {
		t.Fatal(err)
	}

	// SetDatabaseReady / SetDatabaseDegraded
	db3, err := svc.CreateDatabase(ctx, CreateDatabaseInput{TenantID: tenantID, ProjectID: projectID, Name: "deg"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDatabaseReady(ctx, db3.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDatabaseDegraded(ctx, db3.ID, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDatabaseDegraded(ctx, uuid.NewString(), nil); err == nil {
		t.Fatal("degrade missing should fail")
	}

	// MarkDatabaseDeleted non-InvalidState error + DeleteDatabaseSync mark-deleted error
	hooked, hook, hTenant, hProject := newHookedService(t, nil, &hookRepo{})
	hook.transitionFn = func(context.Context, string, []DatabaseStatus, DatabaseStatus, time.Time) (Database, error) {
		return Database{}, errors.New("disk fail")
	}
	if err := hooked.MarkDatabaseDeleted(ctx, "x", time.Now()); err == nil {
		t.Fatal("mark deleted should surface non-state error")
	}
	hook.transitionFn = nil
	db4, err := hooked.CreateDatabase(ctx, CreateDatabaseInput{TenantID: hTenant, ProjectID: hProject, Name: "markfail"})
	if err != nil {
		t.Fatal(err)
	}
	hook.markDelFn = func(context.Context, string, time.Time) error { return errors.New("mark fail") }
	_, err = hooked.DeleteDatabaseSync(ctx, newTestPrincipal(hTenant, hProject), hProject, db4.ID, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "mark fail") {
		t.Fatalf("mark fail: %v", err)
	}
}

func TestService_ProvidersAndUsage(t *testing.T) {
	ctx := context.Background()
	svc, _, tenantID, projectID := setupService(t, nil)
	plain := newTestPrincipal(tenantID, projectID)
	denied := newTestPrincipal(tenantID)

	if err := svc.UpsertProviderConfig(ctx, denied, LLMProviderConfig{ProjectID: projectID, Provider: "openai"}); !errors.Is(err, ErrCrossProject) {
		t.Fatalf("upsert denied: %v", err)
	}
	if err := svc.UpsertProviderConfig(ctx, plain, LLMProviderConfig{ProjectID: projectID, Provider: "openai", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpsertProviderConfig(ctx, plain, LLMProviderConfig{ID: uuid.NewString(), ProjectID: projectID, Provider: "anthropic", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListEnabledProviders(ctx, plain, projectID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list providers: n=%d err=%v", len(list), err)
	}
	if _, err := svc.ListEnabledProviders(ctx, denied, projectID); !errors.Is(err, ErrCrossProject) {
		t.Fatalf("list denied: %v", err)
	}
	got, err := svc.GetLLMProviders(ctx, projectID)
	if err != nil || len(got.Providers) != 2 {
		t.Fatalf("GetLLMProviders: %+v err=%v", got, err)
	}
	if err := svc.AppendUsage(ctx, []UsageEvent{{
		ID: uuid.NewString(), ProjectID: projectID, Kind: "llm", Provider: "openai",
		InputTokens: 1, OccurredAt: time.Now(),
	}}); err != nil {
		t.Fatal(err)
	}
}
