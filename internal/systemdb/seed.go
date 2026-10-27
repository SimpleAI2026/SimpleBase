// 文件说明：seed.go — internal/systemdb 中 SeedInput、Seed 等符号的实现
//
// 用法
//
//	Seed(ctx context.Context, in SeedInput) error  // Seed 幂等写入保留租户与系统项目
//	type SeedInput（导出类型，同包与上层均可使用）
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/seed.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/ducklake, internal/auth, internal/catalog, internal/observability
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//
// 主要符号
//
//	type SeedInput struct {…}
//	Seed(ctx context.Context, in SeedInput) error
//	... 另有 6 个顶层符号

package systemdb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

const (
	tenantName     = "SimpleBase"
	systemProjName = "system"
)

// SeedInput 控制空库种子。
type SeedInput struct {
	Store *Store
	// Auth 保留以便后续扩展种子流程；当前不再用于签发种子 Key。
	Auth    *auth.Service
	Catalog *catalog.Service
	Logger  observability.Logger
	// InitKV 可选：项目 KV catalog 创建后初始化 kv schema
	// （key-value-ducklake-plan §2）。失败只记日志，不阻断种子。
	InitKV func(ctx context.Context, db catalog.Database) error
}

// Seed 幂等写入保留租户与系统项目。不创建演示项目、也不签发任何明文 Key：
// 项目与 Key 由超管登录后通过公开 API 创建。
// 整段可重入：DuckLake Transaction conflict 表示提交失败，重放不会叠写。
// 周期同步在成功返回后才恢复，避免 flush 与 seed 写并发。
func Seed(ctx context.Context, in SeedInput) error {
	if in.Store == nil {
		return fmt.Errorf("systemdb: seed store is required")
	}
	if err := ducklake.RetryOnConflict(ctx, func() error {
		return seedOnce(ctx, in)
	}); err != nil {
		return err
	}
	resumeCatalogSync(in.Store)
	return nil
}

func seedOnce(ctx context.Context, in SeedInput) error {
	repo := in.Store.CatalogRepo()
	now := time.Now().UTC()
	// 远端 S3 DATA_PATH 冷启动时每条首查都要拉数据文件，seed 可能
	// 耗时数十秒；分阶段观测（debug）避免这段静默被误判为卡死。
	stage := func(name string) func() {
		start := time.Now()
		return func() {
			if in.Logger != nil {
				in.Logger.Debug("systemdb seed stage",
					zap.String("stage", name),
					zap.Duration("duration", time.Since(start)),
				)
			}
		}
	}

	done := stage("seed_tenant")
	if err := seedTenant(ctx, repo, now); err != nil {
		return err
	}
	done()
	done = stage("seed_system_project")
	if err := seedProject(ctx, repo, catalog.ReservedSystemProjectID, catalog.ReservedTenantID, systemProjName, now); err != nil {
		return err
	}
	done()

	// login-auth-plan §3.5：幂等种子超管 simplebase2026 / simplebase2026。
	done = stage("seed_bootstrap_user")
	userRepo := auth.NewSQLUserRepository(in.Store.DB())
	users := auth.NewUserService(userRepo, catalog.ReservedTenantID)
	boot, created, err := users.EnsureBootstrapUser(ctx)
	if err != nil {
		return fmt.Errorf("systemdb: seed bootstrap user: %w", err)
	}
	done()
	if created && in.Logger != nil {
		in.Logger.Info("seeded bootstrap user", zap.String("username", boot.Username))
	}
	// 项目归属：admin 系统项目 → 超管。
	done = stage("seed_admin_project_owner")
	if err := userRepo.SetProjectOwner(ctx, catalog.ReservedSystemProjectID, boot.ID, now); err != nil {
		return fmt.Errorf("systemdb: seed admin project owner: %w", err)
	}
	done()

	in.Store.notifyWrite(ctx)
	return nil
}

func seedTenant(ctx context.Context, repo catalog.Repository, now time.Time) error {
	err := repo.CreateTenant(ctx, catalog.Tenant{
		ID: catalog.ReservedTenantID, Name: tenantName, CreatedAt: now,
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

func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "already exists")
}
