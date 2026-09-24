package registry

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"

	_ "github.com/uglyer/go-sqlite3"
)

func newAlwaysOpenCatalog(t *testing.T) (*catalog.Service, catalog.Repository, auth.Principal, string, string) {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	for _, stmt := range []string{
		`CREATE TABLE sys_tenants (id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE sys_projects (id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE sys_databases (
			id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, name VARCHAR NOT NULL,
			kind VARCHAR NOT NULL, status VARCHAR NOT NULL, storage_prefix VARCHAR NOT NULL, format_version BIGINT NOT NULL,
			deleted_at TIMESTAMP, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	repo := catalog.NewSQLRepository(raw)
	keys := objectstore.KeyBuilder{RootPrefix: "simplebase", Environment: "test"}
	svc := catalog.NewService(repo, keys, nil, objectstore.DuckLakeStorage{}, nil)
	tenantID := uuid.NewString()
	projectID := uuid.NewString()
	now := time.Now().UTC()
	ctx := context.Background()
	if err := repo.CreateTenant(ctx, catalog.Tenant{ID: tenantID, Name: "t", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateProject(ctx, catalog.Project{ID: projectID, TenantID: tenantID, Name: "p", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	principal := auth.Principal{
		APIKeyID:   "k",
		TenantID:   tenantID,
		ProjectIDs: map[string]struct{}{projectID: {}},
	}
	return svc, repo, principal, tenantID, projectID
}

func TestPlan_LegacyClosedRowListsReadyAndQueries(t *testing.T) {
	svc, repo, principal, tenantID, projectID := newAlwaysOpenCatalog(t)
	ctx := context.Background()
	now := time.Now().UTC()
	id := uuid.NewString()
	if err := repo.CreateDatabase(ctx, catalog.Database{
		ID: id, TenantID: tenantID, ProjectID: projectID, Name: "legacy",
		Kind: catalog.DatabaseKindUser, Status: catalog.DatabaseClosed,
		StoragePrefix: "p", FormatVersion: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	listed, _, err := svc.ListDatabases(ctx, principal, projectID, catalog.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Status != catalog.DatabaseReady || listed[0].DeletedAt != nil {
		t.Fatalf("list=%+v", listed)
	}

	reg := New(&fakeFactory{}, svc, Options{Writable: true}, nil, nil)
	lease, err := reg.Acquire(ctx, listed[0], database.ReadWrite)
	if err != nil {
		t.Fatalf("acquire closed-promoted db: %v", err)
	}
	if _, err := lease.Handle.Query(ctx, database.Statement{SQL: "SELECT 1"}, 1); err != nil {
		t.Fatalf("query: %v", err)
	}
	lease.Release()

	stored, err := repo.GetDatabase(ctx, projectID, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != catalog.DatabaseReady || stored.DeletedAt != nil {
		t.Fatalf("stored=%+v", stored)
	}
}

func TestPlan_CloseIdleThenQueryKeepsCatalogReady(t *testing.T) {
	svc, _, principal, tenantID, projectID := newAlwaysOpenCatalog(t)
	ctx := context.Background()
	db, err := svc.CreateDatabase(ctx, catalog.CreateDatabaseInput{
		TenantID: tenantID, ProjectID: projectID, Name: "live",
	})
	if err != nil {
		t.Fatal(err)
	}
	if db.Status != catalog.DatabaseReady {
		t.Fatalf("create status=%s", db.Status)
	}

	reg := New(&fakeFactory{}, svc, Options{Writable: true, IdleTimeout: time.Millisecond}, nil, nil)
	lease, err := reg.Acquire(ctx, db, database.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lease.Handle.Query(ctx, database.Statement{SQL: "SELECT 1"}, 1); err != nil {
		t.Fatal(err)
	}
	lease.Release()
	time.Sleep(5 * time.Millisecond)
	if err := reg.CloseIdle(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}

	got, err := svc.GetDatabase(ctx, principal, projectID, db.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != catalog.DatabaseReady || got.DeletedAt != nil {
		t.Fatalf("after idle: %+v", got)
	}

	lease2, err := reg.Acquire(ctx, got, database.ReadWrite)
	if err != nil {
		t.Fatalf("re-acquire after idle: %v", err)
	}
	if _, err := lease2.Handle.Query(ctx, database.Statement{SQL: "SELECT 1"}, 1); err != nil {
		t.Fatalf("query after idle: %v", err)
	}
	lease2.Release()

	again, err := svc.GetDatabase(ctx, principal, projectID, db.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != catalog.DatabaseReady || again.DeletedAt != nil {
		t.Fatalf("after re-query: %+v", again)
	}
}
