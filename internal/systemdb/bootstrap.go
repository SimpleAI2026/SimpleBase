package systemdb

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

const DefaultName = "simplebase-system"

// BootstrapInput 引导系统库所需的最小依赖（不经过 CatalogService）。
type BootstrapInput struct {
	LocatorDir string
	Name       string
	Factory    *ducklake.Factory
	Keys       objectstore.KeyBuilder
	Logger     observability.Logger
}

// Bootstrap 创建或打开系统 DuckLake，应用迁移，回填 sys_databases。
func Bootstrap(ctx context.Context, in BootstrapInput) (*Store, error) {
	if in.Factory == nil {
		return nil, fmt.Errorf("systemdb: factory is required")
	}
	if in.LocatorDir == "" {
		return nil, fmt.Errorf("systemdb: locator dir is required")
	}
	name := in.Name
	if name == "" {
		name = DefaultName
	}

	loc, ok, err := LoadLocator(in.LocatorDir)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if !ok {
		loc = Locator{
			DatabaseID: uuid.NewString(),
			TenantID:   catalog.ReservedTenantID,
			Name:       name,
			CreatedAt:  now,
		}
		if err := SaveLocator(in.LocatorDir, loc); err != nil {
			return nil, err
		}
	}
	if loc.Name == "" {
		loc.Name = name
	}
	if loc.TenantID == "" {
		loc.TenantID = catalog.ReservedTenantID
	}

	prefix := ""
	if p, err := in.Keys.DataPrefix(loc.TenantID, loc.DatabaseID); err == nil {
		prefix = p
	}

	meta := catalog.Database{
		ID:            loc.DatabaseID,
		TenantID:      loc.TenantID,
		ProjectID:     catalog.ReservedSystemProjectID,
		Name:          loc.Name,
		Kind:          catalog.DatabaseKindSystem,
		Status:        catalog.DatabaseReady,
		StoragePrefix: prefix,
		FormatVersion: objectstore.DescriptorFormatVersion,
		CreatedAt:     loc.CreatedAt,
		UpdatedAt:     now,
	}

	db, err := in.Factory.Open(ctx, meta, database.ReadWrite)
	if err != nil {
		return nil, fmt.Errorf("systemdb: open: %w", err)
	}

	if err := ApplySystemMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("systemdb: migrate: %w", err)
	}

	store := &Store{
		db:      db,
		meta:    meta,
		factory: in.Factory,
		locator: loc,
		logger:  in.Logger,
	}

	if err := backfillSystemRow(ctx, store, meta); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("systemdb: backfill sys_databases: %w", err)
	}
	store.notifyWrite(ctx)

	if in.Logger != nil {
		in.Logger.Info("system database ready",
			zap.String("database_id", meta.ID),
			zap.String("name", meta.Name),
			zap.String("locator", in.LocatorDir),
		)
	}
	return store, nil
}

func backfillSystemRow(ctx context.Context, store *Store, meta catalog.Database) error {
	repo := store.CatalogRepo()
	existing, err := repo.GetDatabase(ctx, meta.ProjectID, meta.ID)
	if err == nil && existing.ID == meta.ID {
		return nil
	}
	if err != nil && !catalog.IsNotFound(err) {
		return err
	}
	if err := repo.CreateDatabase(ctx, meta); err != nil && !catalog.IsAlreadyExists(err) {
		return err
	}
	return nil
}
