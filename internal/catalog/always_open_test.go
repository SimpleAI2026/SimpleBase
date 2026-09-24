package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

type headDescriptor struct {
	missing bool
	headErr error
	heads   int
}

func (h *headDescriptor) PutJSON(context.Context, string, any, objectstore.PutOptions) error {
	return nil
}

func (h *headDescriptor) Head(context.Context, string) (objectstore.ObjectInfo, error) {
	h.heads++
	if h.headErr != nil {
		return objectstore.ObjectInfo{}, h.headErr
	}
	if h.missing {
		return objectstore.ObjectInfo{}, objectstore.ErrNotFound
	}
	return objectstore.ObjectInfo{Size: 1}, nil
}

func seedStatus(t *testing.T, repo Repository, tenantID, projectID string, status DatabaseStatus, deleted bool) Database {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.NewString()
	db := Database{
		ID:            id,
		TenantID:      tenantID,
		ProjectID:     projectID,
		Name:          "n-" + string(status) + "-" + id[:8],
		Kind:          DatabaseKindUser,
		Status:        status,
		StoragePrefix: "simplebase/test/data",
		FormatVersion: objectstore.DescriptorFormatVersion,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if deleted {
		at := now
		db.DeletedAt = &at
	}
	if err := repo.CreateDatabase(context.Background(), db); err != nil {
		t.Fatalf("seed %s: %v", status, err)
	}
	return db
}

func TestPlan_RetiredOpenStatusListsAsReady(t *testing.T) {
	svc, repo, tenantID, projectID := setupService(t, nil)
	principal := newTestPrincipal(tenantID, projectID)
	ctx := context.Background()

	closed := seedStatus(t, repo, tenantID, projectID, DatabaseClosed, false)
	opening := seedStatus(t, repo, tenantID, projectID, DatabaseOpening, false)
	closing := seedStatus(t, repo, tenantID, projectID, DatabaseClosing, false)
	recovering := seedStatus(t, repo, tenantID, projectID, DatabaseRecovering, false)
	degraded := seedStatus(t, repo, tenantID, projectID, DatabaseDegraded, false)
	creating := seedStatus(t, repo, tenantID, projectID, DatabaseCreating, false)
	softClosed := seedStatus(t, repo, tenantID, projectID, DatabaseClosed, true)

	listed, _, err := svc.ListDatabases(ctx, principal, projectID, Page{Limit: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byID := map[string]Database{}
	for _, db := range listed {
		byID[db.ID] = db
		if db.DeletedAt != nil {
			t.Fatalf("list included soft-deleted %s", db.ID)
		}
	}
	for _, id := range []string{closed.ID, opening.ID, closing.ID, recovering.ID} {
		got, ok := byID[id]
		if !ok {
			t.Fatalf("missing %s in list", id)
		}
		if got.Status != DatabaseReady {
			t.Fatalf("%s listed as %s, want ready", id, got.Status)
		}
		stored, err := repo.GetDatabase(ctx, projectID, id)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Status != DatabaseReady {
			t.Fatalf("writeback %s status=%s", id, stored.Status)
		}
		if stored.DeletedAt != nil {
			t.Fatalf("retired status %s was soft-deleted", id)
		}
	}
	if byID[degraded.ID].Status != DatabaseDegraded {
		t.Fatalf("degraded became %s", byID[degraded.ID].Status)
	}
	if byID[creating.ID].Status != DatabaseCreating {
		t.Fatalf("creating was promoted on list to %s", byID[creating.ID].Status)
	}
	if _, ok := byID[softClosed.ID]; ok {
		t.Fatal("soft-deleted closed row must stay off the list")
	}
	storedSoft, err := repo.GetDatabase(ctx, projectID, softClosed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedSoft.Status != DatabaseClosed || storedSoft.DeletedAt == nil {
		t.Fatalf("soft-deleted closed row changed: %+v", storedSoft)
	}

	one, err := svc.GetDatabase(ctx, principal, projectID, closed.ID)
	if err != nil || one.Status != DatabaseReady || one.DeletedAt != nil {
		t.Fatalf("get closed: status=%s deleted=%v err=%v", one.Status, one.DeletedAt, err)
	}
}

func TestPlan_SetDatabaseReadyDoesNotPromoteDegraded(t *testing.T) {
	svc, repo, tenantID, projectID := setupService(t, nil)
	db := seedStatus(t, repo, tenantID, projectID, DatabaseDegraded, false)
	if err := svc.SetDatabaseReady(context.Background(), db.ID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetDatabase(context.Background(), projectID, db.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DatabaseDegraded {
		t.Fatalf("degraded became %s", got.Status)
	}
}

func TestPlan_StartupRepairCreating(t *testing.T) {
	ctx := context.Background()

	t.Run("storage disabled becomes ready", func(t *testing.T) {
		svc, repo, tenantID, projectID := setupService(t, nil)
		db := seedStatus(t, repo, tenantID, projectID, DatabaseCreating, false)
		deg := seedStatus(t, repo, tenantID, projectID, DatabaseDegraded, false)
		if err := svc.RepairDatabasesOnStartup(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetDatabase(ctx, projectID, db.ID)
		if err != nil || got.Status != DatabaseReady {
			t.Fatalf("creating status=%s err=%v", got.Status, err)
		}
		still, err := repo.GetDatabase(ctx, projectID, deg.ID)
		if err != nil || still.Status != DatabaseDegraded {
			t.Fatalf("degraded status=%s err=%v", still.Status, err)
		}
	})

	t.Run("descriptor present becomes ready", func(t *testing.T) {
		head := &headDescriptor{}
		svc, repo, tenantID, projectID := setupService(t, head)
		db := seedStatus(t, repo, tenantID, projectID, DatabaseCreating, false)
		if err := svc.RepairDatabasesOnStartup(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetDatabase(ctx, projectID, db.ID)
		if err != nil || got.Status != DatabaseReady {
			t.Fatalf("status=%s err=%v", got.Status, err)
		}
		if head.heads != 1 {
			t.Fatalf("heads=%d", head.heads)
		}
	})

	t.Run("descriptor missing becomes degraded", func(t *testing.T) {
		head := &headDescriptor{missing: true}
		svc, repo, tenantID, projectID := setupService(t, head)
		db := seedStatus(t, repo, tenantID, projectID, DatabaseCreating, false)
		if err := svc.RepairDatabasesOnStartup(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetDatabase(ctx, projectID, db.ID)
		if err != nil || got.Status != DatabaseDegraded {
			t.Fatalf("status=%s err=%v", got.Status, err)
		}
	})

	t.Run("head error leaves creating", func(t *testing.T) {
		head := &headDescriptor{headErr: errors.New("s3 timeout")}
		svc, repo, tenantID, projectID := setupService(t, head)
		db := seedStatus(t, repo, tenantID, projectID, DatabaseCreating, false)
		if err := svc.RepairDatabasesOnStartup(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetDatabase(ctx, projectID, db.ID)
		if err != nil || got.Status != DatabaseCreating {
			t.Fatalf("status=%s err=%v", got.Status, err)
		}
	})
}

func TestPlan_DeleteClosedIsSoftDelete(t *testing.T) {
	svc, repo, tenantID, projectID := setupService(t, nil)
	db := seedStatus(t, repo, tenantID, projectID, DatabaseClosed, false)
	principal := newTestPrincipal(tenantID, projectID)
	purger := newRecordingPurger()
	out, err := svc.DeleteDatabaseSync(context.Background(), principal, projectID, db.ID, func(context.Context, string) error {
		return nil
	}, purger)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != DatabaseDeleted {
		t.Fatalf("status=%s want deleted", out.Status)
	}
	got, err := repo.GetDatabase(context.Background(), projectID, db.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DatabaseDeleted || got.DeletedAt == nil {
		t.Fatalf("soft delete incomplete: status=%s deleted_at=%v", got.Status, got.DeletedAt)
	}
	if len(purger.deleted) == 0 {
		t.Fatal("expected object prefix purge")
	}
}
