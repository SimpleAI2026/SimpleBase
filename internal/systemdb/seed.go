package systemdb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// DevRawAPIKey 是 DevMode 种子明文 Key（仅本地开发；禁止用于生产）。
const DevRawAPIKey = "sb_live_dev_key_12345"

const (
	devTenantName  = "Dev Tenant"
	devProjectName = "商城后台"
	systemProjName = "system"
)

// SeedInput 控制空库种子。
type SeedInput struct {
	Store   *Store
	Auth    *auth.Service
	Catalog *catalog.Service
	DevMode bool
}

// Seed 幂等写入保留租户 / 系统项目；DevMode 额外写入 UUID 项目、Dev Key 与 default 库。
func Seed(ctx context.Context, in SeedInput) error {
	if in.Store == nil {
		return fmt.Errorf("systemdb: seed store is required")
	}
	repo := in.Store.CatalogRepo()
	now := time.Now().UTC()

	if err := seedTenant(ctx, repo, now); err != nil {
		return err
	}
	if err := seedProject(ctx, repo, catalog.ReservedSystemProjectID, catalog.ReservedTenantID, systemProjName, now); err != nil {
		return err
	}

	if in.DevMode {
		if err := seedProject(ctx, repo, catalog.DevProjectID, catalog.ReservedTenantID, devProjectName, now); err != nil {
			return err
		}
		if in.Auth != nil {
			if err := seedDevAPIKey(ctx, in.Store, in.Auth, now); err != nil {
				return fmt.Errorf("systemdb: seed api key: %w", err)
			}
		}
		if in.Catalog != nil {
			if _, err := in.Catalog.CreateDatabase(ctx, catalog.CreateDatabaseInput{
				TenantID:  catalog.ReservedTenantID,
				ProjectID: catalog.DevProjectID,
				Name:      "default",
			}); err != nil && !errors.Is(err, catalog.ErrAlreadyExists) {
				return fmt.Errorf("systemdb: seed default database: %w", err)
			}
		}
		if err := repo.UpsertQuota(ctx, catalog.ProjectQuota{
			ProjectID:     catalog.DevProjectID,
			PeriodSeconds: 3600,
			UpdatedAt:     now,
		}); err != nil {
			return fmt.Errorf("systemdb: seed quota: %w", err)
		}
	}

	in.Store.notifyWrite(ctx)
	return nil
}

func seedTenant(ctx context.Context, repo catalog.Repository, now time.Time) error {
	err := repo.CreateTenant(ctx, catalog.Tenant{
		ID: catalog.ReservedTenantID, Name: devTenantName, CreatedAt: now,
	})
	if err != nil && !errors.Is(err, catalog.ErrAlreadyExists) {
		return fmt.Errorf("systemdb: seed tenant: %w", err)
	}
	return nil
}

func seedProject(ctx context.Context, repo catalog.Repository, id, tenantID, name string, now time.Time) error {
	err := repo.CreateProject(ctx, catalog.Project{
		ID: id, TenantID: tenantID, Name: name, CreatedAt: now,
	})
	if err != nil && !errors.Is(err, catalog.ErrAlreadyExists) {
		return fmt.Errorf("systemdb: seed project %s: %w", name, err)
	}
	return nil
}

func seedDevAPIKey(ctx context.Context, store *Store, authSvc *auth.Service, now time.Time) error {
	perms := []auth.Permission{
		auth.DatabaseRead, auth.DatabaseWrite, auth.DatabaseAdmin,
		auth.LLMInvoke, auth.ProjectAdmin,
	}
	err := auth.CreateAPIKey(ctx, store.DB(), catalog.DevAPIKeyID, catalog.DevProjectID,
		authSvc.HashKey(DevRawAPIKey), perms, now)
	if err != nil && !isDuplicate(err) {
		return err
	}
	return nil
}

func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "already exists")
}
