package catalog

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSQLRepository_TenantsProjectsAndTouch(t *testing.T) {
	db := newTestDB(t)
	var touches int
	repo := NewSQLRepository(db, func(context.Context) { touches++ })
	ctx := context.Background()
	now := time.Now().UTC()

	t1 := Tenant{ID: uuid.NewString(), Name: "acme", CreatedAt: now}
	if err := repo.CreateTenant(ctx, t1); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTenant(ctx, t1); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("dup tenant: %v", err)
	}
	if touches == 0 {
		t.Fatal("afterWrite should run on create tenant")
	}

	p1 := Project{ID: uuid.NewString(), TenantID: t1.ID, Name: "alpha", CreatedAt: now}
	if err := repo.CreateProject(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateProject(ctx, p1); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("dup project id: %v", err)
	}
	if err := repo.CreateProject(ctx, Project{ID: uuid.NewString(), TenantID: t1.ID, Name: "alpha", CreatedAt: now}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("dup project name: %v", err)
	}

	listed, err := repo.ListProjectsByTenant(ctx, t1.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != p1.ID {
		t.Fatalf("list projects: %+v err=%v", listed, err)
	}
	empty, err := repo.ListProjectsByTenant(ctx, uuid.NewString())
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty list: %+v err=%v", empty, err)
	}

	ok, err := repo.ProjectBelongsToTenant(ctx, p1.ID, t1.ID)
	if err != nil || !ok {
		t.Fatalf("belongs: ok=%v err=%v", ok, err)
	}
	ok, err = repo.ProjectBelongsToTenant(ctx, p1.ID, uuid.NewString())
	if err != nil || ok {
		t.Fatalf("foreign tenant: ok=%v err=%v", ok, err)
	}

	gotTenant, err := repo.GetProjectTenant(ctx, p1.ID)
	if err != nil || gotTenant != t1.ID {
		t.Fatalf("GetProjectTenant: %s err=%v", gotTenant, err)
	}
	if _, err := repo.GetProjectTenant(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
}

func TestSQLRepository_DatabasesPaginationAndStates(t *testing.T) {
	db := newTestDB(t)
	repo := NewSQLRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	tenantID, projectID := uuid.NewString(), uuid.NewString()
	if err := repo.CreateTenant(ctx, Tenant{ID: tenantID, Name: "t", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateProject(ctx, Project{ID: projectID, TenantID: tenantID, Name: "p", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	deletedAt := now.Add(-time.Hour)
	first := Database{
		ID: uuid.NewString(), TenantID: tenantID, ProjectID: projectID, Name: "one",
		Status: DatabaseReady, StoragePrefix: "p1", FormatVersion: 2,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateDatabase(ctx, first); err != nil {
		t.Fatal(err)
	}
	// empty kind defaults to user
	second := Database{
		ID: uuid.NewString(), TenantID: tenantID, ProjectID: projectID, Name: "two",
		Kind: "", Status: DatabaseReady, StoragePrefix: "p2", FormatVersion: 2,
		CreatedAt: now.Add(time.Second), UpdatedAt: now,
	}
	if err := repo.CreateDatabase(ctx, second); err != nil {
		t.Fatal(err)
	}
	third := Database{
		ID: uuid.NewString(), TenantID: tenantID, ProjectID: projectID, Name: "three",
		Kind: DatabaseKindUser, Status: DatabaseReady, StoragePrefix: "p3", FormatVersion: 2,
		CreatedAt: now.Add(2 * time.Second), UpdatedAt: now, DeletedAt: &deletedAt,
	}
	if err := repo.CreateDatabase(ctx, third); err != nil {
		t.Fatal(err)
	}
	sys := Database{
		ID: uuid.NewString(), TenantID: tenantID, ProjectID: projectID, Name: "one",
		Kind: DatabaseKindSystem, Status: DatabaseReady, StoragePrefix: "sys", FormatVersion: 2,
		CreatedAt: now.Add(3 * time.Second), UpdatedAt: now,
	}
	if err := repo.CreateDatabase(ctx, sys); err != nil {
		t.Fatal(err)
	}
	// system kind skips name uniqueness against user "one"
	sys2 := sys
	sys2.ID = uuid.NewString()
	sys2.Name = "one"
	if err := repo.CreateDatabase(ctx, sys2); err != nil {
		t.Fatalf("system dbs may share names: %v", err)
	}

	if err := repo.CreateDatabase(ctx, first); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("dup id: %v", err)
	}
	if err := repo.CreateDatabase(ctx, Database{
		ID: uuid.NewString(), TenantID: tenantID, ProjectID: projectID, Name: "one",
		Status: DatabaseReady, StoragePrefix: "x", FormatVersion: 2, CreatedAt: now, UpdatedAt: now,
	}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("dup user name: %v", err)
	}

	got, err := repo.GetDatabase(ctx, projectID, first.ID)
	if err != nil || got.Kind != DatabaseKindUser || got.Status != DatabaseReady {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if _, err := repo.GetDatabase(ctx, projectID, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing db: %v", err)
	}

	// third has deleted_at so list hides it; page limit default and overflow
	page1, cursor, err := repo.ListDatabases(ctx, projectID, Page{Limit: 1})
	if err != nil || len(page1) != 1 || cursor == "" {
		t.Fatalf("page1 n=%d cursor=%q err=%v", len(page1), cursor, err)
	}
	all, cursor, err := repo.ListDatabases(ctx, projectID, Page{Limit: 0})
	if err != nil || len(all) != 2 || cursor != "" {
		t.Fatalf("default limit list n=%d cursor=%q err=%v", len(all), cursor, err)
	}
	wide, _, err := repo.ListDatabases(ctx, projectID, Page{Limit: 500})
	if err != nil || len(wide) != 2 {
		t.Fatalf("clamped limit: n=%d err=%v", len(wide), err)
	}
	sysList, _, err := repo.ListDatabasesByKind(ctx, projectID, DatabaseKindSystem, Page{Limit: 10})
	if err != nil || len(sysList) != 2 {
		t.Fatalf("system kind list: n=%d err=%v", len(sysList), err)
	}

	if _, err := repo.TransitionDatabase(ctx, first.ID, nil, DatabaseClosing, now); err == nil {
		t.Fatal("empty from should fail")
	}
	if _, err := repo.TransitionDatabase(ctx, uuid.NewString(), []DatabaseStatus{DatabaseReady}, DatabaseClosing, now); err == nil {
		t.Fatal("missing id should fail")
	} else if _, ok := err.(*StateTransitionError); !ok && !errors.Is(err, ErrInvalidState) {
		t.Fatalf("want StateTransitionError, got %T %v", err, err)
	}
	if _, err := repo.TransitionDatabase(ctx, first.ID, []DatabaseStatus{DatabaseOpening}, DatabaseReady, now); err == nil {
		t.Fatal("wrong from should fail")
	}
	moved, err := repo.TransitionDatabase(ctx, first.ID, []DatabaseStatus{DatabaseReady}, DatabaseClosing, now)
	if err != nil || moved.Status != DatabaseClosing {
		t.Fatalf("transition: %+v err=%v", moved, err)
	}

	if err := repo.MarkDeleted(ctx, first.ID, now); err != nil {
		t.Fatal(err)
	}
	marked, err := repo.GetDatabase(ctx, projectID, first.ID)
	if err != nil || marked.DeletedAt == nil || marked.Status != DatabaseDeleted {
		t.Fatalf("mark deleted: %+v err=%v", marked, err)
	}
	if err := repo.MarkDeleted(ctx, uuid.NewString(), now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mark missing: %v", err)
	}
}

func TestSQLRepository_ProvidersUsageQuotaOperations(t *testing.T) {
	db := newTestDB(t)
	repo := NewSQLRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	projectID := uuid.NewString()

	cfg := LLMProviderConfig{
		ID: uuid.NewString(), ProjectID: projectID, Provider: "openai",
		CredentialRef: "ref-1", Enabled: true, AllowedModelsJSON: `["gpt-4o"]`,
		DefaultModel: "gpt-4o", CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.UpsertProviderConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.CredentialRef = "ref-2"
	cfg.Enabled = false
	cfg.UpdatedAt = now.Add(time.Second)
	if err := repo.UpsertProviderConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	enabled, err := repo.ListEnabledProviders(ctx, projectID)
	if err != nil || len(enabled) != 0 {
		t.Fatalf("disabled should be hidden: %+v err=%v", enabled, err)
	}
	cfg.Enabled = true
	if err := repo.UpsertProviderConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	enabled, err = repo.ListEnabledProviders(ctx, projectID)
	if err != nil || len(enabled) != 1 || enabled[0].CredentialRef != "ref-2" || !enabled[0].Enabled {
		t.Fatalf("enabled: %+v err=%v", enabled, err)
	}

	got, err := repo.GetLLMProviders(ctx, projectID)
	if err != nil || got.Default != "openai" || len(got.Providers) != 1 {
		t.Fatalf("GetLLMProviders: %+v err=%v", got, err)
	}
	if err := repo.SetLLMProviders(ctx, projectID, LLMProviders{Providers: []LLMProviderConfig{{
		ID: uuid.NewString(), Provider: "anthropic", CredentialRef: "r3",
		CreatedAt: now, UpdatedAt: now,
	}}}); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetLLMProviders(ctx, projectID)
	if err != nil || len(got.Providers) != 2 {
		t.Fatalf("after set: %+v err=%v", got, err)
	}

	if err := repo.AppendUsage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendUsage(ctx, []UsageEvent{{
		ID: uuid.NewString(), ProjectID: projectID, Kind: "llm", Provider: "openai", Model: "gpt-4o",
		InputTokens: 10, OutputTokens: 5, CostMicros: 1, RequestID: "r1", OccurredAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	sum, err := repo.SumUsageSince(ctx, projectID, now.Add(-time.Minute))
	if err != nil || sum.LLMRequests != 1 || sum.LLMTokens != 15 {
		t.Fatalf("sum: %+v err=%v", sum, err)
	}

	q, err := repo.GetQuota(ctx, projectID)
	if err != nil || q.PeriodSeconds != 3600 || q.ProjectID != projectID {
		t.Fatalf("default quota: %+v err=%v", q, err)
	}
	if err := repo.UpsertQuota(ctx, ProjectQuota{ProjectID: projectID, MaxDatabases: 3}); err != nil {
		t.Fatal(err)
	}
	q, err = repo.GetQuota(ctx, projectID)
	if err != nil || q.MaxDatabases != 3 || q.PeriodSeconds != 3600 {
		t.Fatalf("inserted quota: %+v err=%v", q, err)
	}
	if err := repo.UpsertQuota(ctx, ProjectQuota{ProjectID: projectID, MaxDatabases: 9, PeriodSeconds: 7200, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	q, err = repo.GetQuota(ctx, projectID)
	if err != nil || q.MaxDatabases != 9 || q.PeriodSeconds != 7200 {
		t.Fatalf("updated quota: %+v err=%v", q, err)
	}

	if err := repo.AppendOperation(ctx, Operation{
		ID: uuid.NewString(), DatabaseID: "db1", ProjectID: projectID, PrincipalID: "k1",
		Kind: "create", RequestID: "req", Status: "ok", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendOperation(ctx, Operation{
		ID: uuid.NewString(), DatabaseID: "db2", ProjectID: "other", PrincipalID: "k1",
		Kind: "delete", Status: "ok", CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	ops, err := repo.ListOperations(ctx, "", "", 0)
	if err != nil || len(ops) != 2 {
		t.Fatalf("all ops: n=%d err=%v", len(ops), err)
	}
	ops, err = repo.ListOperations(ctx, projectID, "db1", 10)
	if err != nil || len(ops) != 1 || ops[0].Kind != "create" {
		t.Fatalf("filtered ops: %+v err=%v", ops, err)
	}
	ops, err = repo.ListOperations(ctx, "", "db2", 500)
	if err != nil || len(ops) != 1 {
		t.Fatalf("db filter + clamp: %+v err=%v", ops, err)
	}
}

func TestSQLRepository_QueryErrorsOnClosedDB(t *testing.T) {
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyTestSchema(raw); err != nil {
		t.Fatal(err)
	}
	repo := NewSQLRepository(raw)
	_ = raw.Close()
	ctx := context.Background()
	now := time.Now()

	if err := repo.CreateTenant(ctx, Tenant{ID: "t", Name: "n", CreatedAt: now}); err == nil {
		t.Fatal("expected closed-db error")
	}
	if err := repo.CreateProject(ctx, Project{ID: "p", TenantID: "t", Name: "n", CreatedAt: now}); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, err := repo.ListProjectsByTenant(ctx, "t"); err == nil {
		t.Fatal("expected closed-db error")
	}
	if err := repo.CreateDatabase(ctx, Database{ID: "d", TenantID: "t", ProjectID: "p", Name: "n", Status: DatabaseReady, CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, err := repo.GetDatabase(ctx, "p", "d"); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, _, err := repo.ListDatabases(ctx, "p", Page{}); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, err := repo.TransitionDatabase(ctx, "d", []DatabaseStatus{DatabaseReady}, DatabaseClosed, now); err == nil {
		t.Fatal("expected closed-db error")
	}
	if err := repo.MarkDeleted(ctx, "d", now); err == nil {
		t.Fatal("expected closed-db error")
	}
	if err := repo.UpsertProviderConfig(ctx, LLMProviderConfig{ProjectID: "p", Provider: "x"}); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, err := repo.ListEnabledProviders(ctx, "p"); err == nil {
		t.Fatal("expected closed-db error")
	}
	if err := repo.AppendUsage(ctx, []UsageEvent{{ID: "u", ProjectID: "p", OccurredAt: now}}); err == nil {
		t.Fatal("expected closed-db error")
	}
	if err := repo.AppendOperation(ctx, Operation{ID: "o", CreatedAt: now}); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, err := repo.ProjectBelongsToTenant(ctx, "p", "t"); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, err := repo.GetProjectTenant(ctx, "p"); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, err := repo.GetLLMProviders(ctx, "p"); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, err := repo.GetQuota(ctx, "p"); err == nil {
		t.Fatal("expected closed-db error")
	}
	if err := repo.UpsertQuota(ctx, ProjectQuota{ProjectID: "p"}); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, err := repo.SumUsageSince(ctx, "p", now); err == nil {
		t.Fatal("expected closed-db error")
	}
	if _, err := repo.ListOperations(ctx, "p", "", 10); err == nil {
		t.Fatal("expected closed-db error")
	}
}
