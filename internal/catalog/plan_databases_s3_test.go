package catalog

import (
	"context"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// 本文件对齐 plan/planv2.0/databases-and-s3-plan.md 验收（用户明确不做 MinIO e2e）。

type recordingDescriptorWriter struct {
	failNext bool
	calls    int
	last     objectstore.Descriptor
	lastKey  string
}

func (f *recordingDescriptorWriter) PutJSON(ctx context.Context, key string, value any, opts objectstore.PutOptions) error {
	f.calls++
	f.lastKey = key
	if f.failNext {
		return context.Canceled
	}
	if d, ok := value.(objectstore.Descriptor); ok {
		f.last = d
	} else if dp, ok := value.(*objectstore.Descriptor); ok && dp != nil {
		f.last = *dp
	}
	return nil
}

type recordingPurger struct {
	deleted []string
	objects map[string]struct{}
}

func newRecordingPurger(keys ...string) *recordingPurger {
	m := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		m[k] = struct{}{}
	}
	return &recordingPurger{objects: m}
}

func (p *recordingPurger) DeletePrefix(ctx context.Context, prefix string) error {
	p.deleted = append(p.deleted, prefix)
	for k := range p.objects {
		if k == prefix || strings.HasPrefix(k, prefix+"/") {
			delete(p.objects, k)
		}
	}
	return nil
}

// recordingProjectPurger 模拟项目单文件删除平面。
type recordingProjectPurger struct {
	purged  []string
	objects map[string]struct{}
}

func (p *recordingProjectPurger) PurgeProject(ctx context.Context, projectID string) error {
	p.purged = append(p.purged, projectID)
	prefix := "simplebase/" + projectID + "/database/"
	for k := range p.objects {
		if strings.HasPrefix(k, prefix) {
			delete(p.objects, k)
		}
	}
	return nil
}

func (p *recordingProjectPurger) DeletePrefix(context.Context, string) error { return nil }

func TestPlan_CreateDatabaseBecomesReady(t *testing.T) {
	desc := &recordingDescriptorWriter{}
	svc, _, tenantID, projectID := setupService(t, desc)

	db, err := svc.CreateDatabase(context.Background(), CreateDatabaseInput{
		TenantID: tenantID, ProjectID: projectID, Name: "plan_ready",
	})
	if err != nil {
		t.Fatalf("CreateDatabase: %v", err)
	}
	if db.Status != DatabaseReady {
		t.Fatalf("plan DoD: create must return ready, got %s", db.Status)
	}
	if desc.calls != 0 {
		t.Fatalf("project file model must not write DuckLake descriptor, got %d", desc.calls)
	}
	if err := ValidateProjectDatabasePath(projectID, db.DatabasePath); err != nil {
		t.Fatalf("database path: %v", err)
	}
	if db.StoragePrefix != "" {
		t.Fatalf("storage prefix must not be used for project files: %q", db.StoragePrefix)
	}
}

func TestPlan_DeleteDatabaseSyncPurgesPlaneBOnly(t *testing.T) {
	svc, _, tenantID, projectID := setupService(t, &recordingDescriptorWriter{})
	db, err := svc.CreateDatabase(context.Background(), CreateDatabaseInput{
		TenantID: tenantID, ProjectID: projectID, Name: "plan_purge",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	planeAKey := "simplebase/" + projectID + "/files/notes/hello.txt"
	projectPurger := &recordingProjectPurger{
		objects: map[string]struct{}{planeAKey: {}, "simplebase/" + projectID + "/database/snapshots/1.duckdb": {}},
	}
	closerCalled := false
	principal := newTestPrincipal(tenantID, projectID)

	out, err := svc.DeleteDatabaseSync(context.Background(), principal, projectID, db.ID,
		func(ctx context.Context, id string) error {
			closerCalled = true
			if id != db.ID {
				t.Fatalf("closer id=%s want %s", id, db.ID)
			}
			return nil
		},
		projectPurger,
	)
	if err != nil {
		t.Fatalf("DeleteDatabaseSync: %v", err)
	}
	if out.Status != DatabaseDeleted {
		t.Fatalf("expected deleted, got %s", out.Status)
	}
	if !closerCalled {
		t.Fatal("expected registry closer to be called")
	}
	if len(projectPurger.purged) != 1 || projectPurger.purged[0] != projectID {
		t.Fatalf("expected purge of project %q, got %v", projectID, projectPurger.purged)
	}
	if _, ok := projectPurger.objects[planeAKey]; !ok {
		t.Fatal("project files must survive database delete")
	}
	if _, ok := projectPurger.objects["simplebase/"+projectID+"/database/snapshots/1.duckdb"]; ok {
		t.Fatal("project database snapshot should be purged")
	}

	// catalog 侧应为 deleted
	got, err := svc.repo.GetDatabase(context.Background(), projectID, db.ID)
	if err != nil {
		t.Fatalf("GetDatabase after delete: %v", err)
	}
	if got.Status != DatabaseDeleted {
		t.Fatalf("catalog status=%s want deleted", got.Status)
	}
}

func TestPlan_DeleteFromCreatingAllowed(t *testing.T) {
	svc, repo, tenantID, projectID := setupService(t, nil)
	// 模拟历史卡住：直接插入 creating 记录
	id := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	prefix := "simplebase/test/tenants/" + tenantID + "/databases/" + id + "/data"
	now := svc.now()
	db := Database{
		ID: id, TenantID: tenantID, ProjectID: projectID, Name: "stuck",
		Status: DatabaseCreating, StoragePrefix: prefix,
		FormatVersion: objectstore.DescriptorFormatVersion,
		CreatedAt:     now, UpdatedAt: now,
	}
	if err := repo.CreateDatabase(context.Background(), db); err != nil {
		t.Fatalf("seed creating db: %v", err)
	}
	principal := newTestPrincipal(tenantID, projectID)
	out, err := svc.DeleteDatabaseSync(context.Background(), principal, projectID, id, nil, nil)
	if err != nil {
		t.Fatalf("delete stuck creating: %v", err)
	}
	if out.Status != DatabaseDeleted {
		t.Fatalf("got %s want deleted", out.Status)
	}
}

func TestPlan_DeleteWithoutPurgerStillMarksDeleted(t *testing.T) {
	// 本地模式：无 objectStore / purger
	svc, _, tenantID, projectID := setupService(t, nil)
	db, err := svc.CreateDatabase(context.Background(), CreateDatabaseInput{
		TenantID: tenantID, ProjectID: projectID, Name: "localmode_del",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	principal := newTestPrincipal(tenantID, projectID)
	out, err := svc.DeleteDatabaseSync(context.Background(), principal, projectID, db.ID, nil, nil)
	if err != nil {
		t.Fatalf("DeleteDatabaseSync: %v", err)
	}
	if out.Status != DatabaseDeleted {
		t.Fatalf("got %s want deleted", out.Status)
	}
}

func TestPlan_SetDatabaseReadyIdempotent(t *testing.T) {
	svc, _, tenantID, projectID := setupService(t, nil)
	db, err := svc.CreateDatabase(context.Background(), CreateDatabaseInput{
		TenantID: tenantID, ProjectID: projectID, Name: "idem",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.SetDatabaseReady(context.Background(), db.ID); err != nil {
		t.Fatalf("second SetDatabaseReady: %v", err)
	}
}
