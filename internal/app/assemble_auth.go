// 文件说明：认证装配阶段：auth 服务与种子/维护循环（原 app.go 拆出）
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"go.uber.org/zap"
)

// assembleAuthServices 装配 auth/user/session 服务。SYNC-01：user/session 仓储直接写系统库表，绕过 Store.notifyWrite；
// 注入写后同步通知，防止改密/禁用是唯一一次写入时 catalog 漏发布。
func (a *App) assembleAuthServices(store *systemdb.Store, cfg config.Config) {
	a.auth = auth.NewService(store.AuthRepo(), cfg.Auth.APIKeyHashSecret).WithPolicyLoader(func(ctx context.Context, keyID, projectID string) (*auth.KeyPolicy, error) {
		return auth.LoadKeyPolicy(ctx, store.DB(), keyID, projectID)
	})
	// §8.2.4：版本化密钥环（SIMPLEBASE_AUTH_APIKEY_SECRETS=v2:<new>,v1:<old>）；
	// 未配置时为零值环，维持单盐行为。
	if strings.TrimSpace(cfg.Auth.APIKeyHashSecrets) != "" {
		ring, err := auth.ParseKeyring(cfg.Auth.APIKeyHashSecrets)
		if err != nil {
			a.logger.Error("invalid api key keyring, fallback to single secret", zap.String("err", err.Error()))
		} else {
			a.auth.WithKeyring(ring)
		}
	}
	a.users = auth.NewUserService(auth.NewSQLUserRepository(store.DB()), catalog.ReservedTenantID)
	a.users.OnWrite(store.NotifyWrite)
	// §8.3.5：登录软锁定回调（审计 + 告警信号）。
	// §8.2.6：终端用户服务（独立密钥域：aud=enduser）。
	a.endUsers = auth.NewEndUserService(systemdb.NewSQLEndUserRepository(store.DB(), store.NotifyWrite), cfg.Auth.SessionSecret)
	// §8.2.3 密钥域拆分：会话 JWT 用独立 session secret（SIMPLEBASE_AUTH_SESSION_SECRET；
	// 未配置时 Load 阶段已从 Key 盐派生），轮换 Key 盐不再杀掉登录态。
	a.sessions = auth.NewSessionService(a.users, auth.NewSQLSessionRepository(store.DB()),
		cfg.Auth.SessionSecret, auth.SessionConfig{
			AccessTTL:  2 * time.Hour,
			RefreshTTL: 7 * 24 * time.Hour,
		})
	a.sessions.OnWrite(store.NotifyWrite) // SYNC-01
}

// seedAndStartMaintenance 执行系统库 Seed、日志保留与周期刷新，并启动
// 系统库周期维护。种子先于后台维护启动，closer 注册顺序决定关闭顺序。
func (a *App) seedAndStartMaintenance(ctx context.Context, store *systemdb.Store, cfg config.Config) error {
	if err := systemdb.Seed(ctx, systemdb.SeedInput{
		// §8.2.2：引导口令注入（SIMPLEBASE_BOOTSTRAP_PASSWORD）。
		BootstrapPassword: cfg.Auth.BootstrapPassword,
		Store:             store,
		Auth:              a.auth,
		Catalog:           a.catalog,
		Logger:            a.logger,
		InitKV:            a.initKVFunc(),
	}); err != nil {
		return fmt.Errorf("seed system database: %w", err)
	}
	store.SetDefaultLogKeepDays(cfg.SystemDatabase.LogKeepDays)
	if err := store.SeedGlobalRetention(ctx, cfg.SystemDatabase.LogKeepDays); err != nil {
		return fmt.Errorf("seed log retention: %w", err)
	}
	store.StartPeriodicFlush(cfg.SystemDatabase.LogFlushInterval, cfg.SystemDatabase.MetricsFlushInterval)

	// §8.3.5：告警总线装配（audit 记录 + metrics 计数两个订阅方）。
	a.assembleAlertBus(cfg, store)

	// perf §1 P1-C：系统库周期维护（merge/expire/cleanup；cleanup 首轮 dry-run）。
	// 仅 writer 执行；启用租约时必须持有系统库写租约（惰性 Acquire，他人持租则跳过）。
	maintWritable := func() bool { return cfg.Instance.Writable }
	if a.leaseMgr != nil {
		sysMeta := store.Meta()
		maintWritable = func() bool {
			if !cfg.Instance.Writable {
				return false
			}
			if a.leaseMgr.ValidFor(sysMeta.ID) {
				return true
			}
			if a.leaseMgr.Has(sysMeta.ID) {
				return false
			}
			_, err := a.leaseMgr.Acquire(ctx, sysMeta.ProjectID, sysMeta.ID, cfg.Instance.ID, a.onLostLease)
			return err == nil
		}
	}
	maint := systemdb.NewMaintenanceRunner(store, duckLakeOptions(cfg.Database.DuckLake).Maintenance, maintWritable, a.logger)
	maint.Start()
	a.registerCloser(maint)
	return nil
}
