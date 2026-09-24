// Package app 装配 SimpleBase 单实例服务的全部依赖并管理生命周期。
// 依赖创建顺序：S3 client → catalog DB → database factory/registry →
// auth → usage → LLM gateway → API router。
// 任一失败时反向关闭已创建资源。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/api"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/cache"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/usage"
	// 云函数脚本可 import 的宿主标准库注册（blank import 必须保留，
	// 否则解释器 BuildProgram 找不到 fmt/json 等包，见 ui-gofunction-plan §4）
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// App 是运行中的 SimpleBase 实例。它持有 HTTP server 与已创建资源。
type App struct {
	cfg     config.Config
	logger  observability.Logger
	metrics *observability.Metrics

	httpServer *http.Server
	echo       *echo.Echo

	objectStore   objectstore.Client
	fileStore     objectstore.FileStore
	catalogSyncer ducklake.Syncer
	duckFactory   *ducklake.Factory
	systemStore   *systemdb.Store
	catalog       *catalog.Service
	registry      *registry.Registry
	auth          *auth.Service
	users         *auth.UserService
	sessions      *auth.SessionService
	cacheMgr      *cache.Manager
	usageSvc      *usage.Service
	auditSvc      *audit.Service
	llmSvc        llmgateway.Service

	agentScheduler   *cloudagent.Scheduler
	cronScheduler    *cronjob.Scheduler
	schedulerCancel  context.CancelFunc
	schedulerBaseCtx context.Context

	health *healthService

	closerMu sync.Mutex
	closers  []io.Closer
}

// healthService 实现 api.HealthChecker。
type healthService struct {
	app *App
}

func (h *healthService) Live(ctx context.Context) error {
	// 进程存活即 live 通过
	return nil
}

func (h *healthService) Ready(ctx context.Context) error {
	cfg := h.app.cfg
	if cfg.Instance.Writable {
		if h.app.systemStore == nil {
			return errors.New("system database not ready")
		}
		if err := h.app.systemStore.Ping(ctx); err != nil {
			return fmt.Errorf("system database: %w", err)
		}
	}
	if cfg.Instance.Writable && !cfg.DevMode {
		if cfg.S3.Bucket == "" {
			return errors.New("s3 bucket not configured")
		}
		if h.app.objectStore != nil {
			if err := h.app.objectStore.Check(ctx); err != nil {
				return fmt.Errorf("s3 check failed: %w", err)
			}
		}
	}
	return nil
}

// New 按 plan1 规定的顺序构造 App。
// 任何依赖失败时反向关闭已创建资源，避免句柄/连接泄漏。
// 指标注册到 prometheus.DefaultRegisterer；测试请使用 NewWithRegistry。
func New(ctx context.Context, cfg config.Config) (*App, error) {
	return NewWithRegistry(ctx, cfg, nil)
}

// NewWithRegistry 允许注入独立的 Prometheus Registerer（测试隔离用）。
// reg 为 nil 时使用 prometheus.DefaultRegisterer。
func NewWithRegistry(ctx context.Context, cfg config.Config, reg prometheus.Registerer) (*App, error) {
	logger := observability.NewLogger(cfg.Observability.LogLevel, cfg.Observability.LogFormat, observability.WriterFor(cfg.Observability.LogOutput))
	logger.Info("starting simplebase",
		zap.String("instance_id", cfg.Instance.ID),
		zap.Bool("writable", cfg.Instance.Writable),
	)
	redacted := cfg.Redacted()
	logger.Info("config", zap.Any("config", redacted))

	metrics := observability.NewMetrics(reg)

	a := &App{
		cfg:     cfg,
		logger:  logger,
		metrics: metrics,
		health:  nil,
	}

	// 装配依赖（顺序见文件头注释）。任一失败时反向关闭已创建资源。
	if err := a.assembleDeps(ctx); err != nil {
		_ = a.Close(context.Background())
		return nil, err
	}
	a.health = &healthService{app: a}

	dbHandler := api.NewDatabaseHandler(
		api.NewDatabaseServiceAdapter(a.catalog, a.registry, a.objectStore),
		cfg.Instance.Writable,
	)
	if a.duckFactory != nil {
		f := a.duckFactory
		dbHandler.SnapshotFor = func(databaseID string) *api.DatabaseSnapshot {
			return snapshotFromFactory(f, databaseID)
		}
	}
	// 数据库列表「数据量」列：统计各库用户表总行数（只读，单库限时在 handler）。
	sqlSvcForStats := api.NewSQLServiceAdapter(a.catalog, a.registry, a.systemStore)
	dbHandler.RowCountFor = func(ctx context.Context, db catalog.Database) (int64, error) {
		return api.CountDatabaseRows(ctx, sqlSvcForStats, db)
	}

	// Plan 6：SQL handler。readonly 实例 SQLHandler 为 nil，路由不挂载写操作；
	// query 路由也只在 writable 实例提供（首期 readonly 不开放 SQL API）。
	var sqlHandler *api.SQLHandler
	var dataHandler *api.DataHandler
	if cfg.Instance.Writable && a.registry != nil {
		sqlLimits := api.SQLLimits{
			QueryTimeout:       int64(cfg.Limits.QueryTimeout),
			MaxQueryRows:       cfg.Limits.MaxQueryRows,
			MaxConcurrent:      cfg.Limits.MaxConcurrentQueries,
			MaxBatchStatements: cfg.Limits.MaxBatchStatements,
			MaxSQLBytes:        cfg.Limits.MaxSQLBytes,
			MaxRequestBytes:    cfg.Limits.MaxRequestBytes,
		}
		sqlService := api.NewSQLServiceAdapter(a.catalog, a.registry, a.systemStore)
		sqlHandler = api.NewSQLHandler(sqlService, sqlLimits, cfg.Instance.Writable)
		if a.duckFactory != nil {
			f := a.duckFactory
			sqlHandler.DurabilityFor = f.DurabilityFor
		}
		dataHandler = api.NewDataHandler(sqlService.(api.DataService), cfg.Instance.Writable)
	}

	// Cloud Agent 定时执行调度器：仅在运行时与系统库齐备时创建。
	var agentScheduler *cloudagent.Scheduler
	runtime := a.cloudAgentRuntime()
	if runtime != nil && a.systemStore != nil {
		agentScheduler = cloudagent.NewScheduler(a.systemStore, runtime, a.usageSvc)
		a.agentScheduler = agentScheduler
		a.schedulerBaseCtx, a.schedulerCancel = context.WithCancel(context.Background())
		agentScheduler.Start(a.schedulerBaseCtx)
	}

	// 云函数定时任务调度器（ui-cronjob-plan §5.4）：仅在系统库齐备时创建。
	var cronScheduler *cronjob.Scheduler
	if a.systemStore != nil {
		cronScheduler = cronjob.NewScheduler(a.systemStore, &systemDBRunner{store: a.systemStore})
		a.cronScheduler = cronScheduler
		if a.schedulerCancel == nil {
			a.schedulerBaseCtx, a.schedulerCancel = context.WithCancel(context.Background())
		}
		cronScheduler.Start(a.schedulerBaseCtx)
	}

	deps := api.Dependencies{
		Config:          cfg,
		Logger:          logger,
		Metrics:         metrics,
		Health:          a.health,
		Auth:            a.auth,
		Sessions:        a.sessions,
		Users:           a.users,
		Catalog:         a.catalog,
		Registry:        a.registry,
		DatabaseHandler: dbHandler,
		SQLHandler:      sqlHandler,
		DataHandler:     dataHandler,
		Cache:           api.NewCacheService(a.cacheMgr),
		Usage:           api.NewUsageService(a.usageSvc),
		Audit:           api.NewAuditService(a.auditSvc),
		LLM:             api.NewLLMService(a.llmSvc),
		S3FileStore:     a.fileStore,
		System:          a.systemStore,
		CloudAgent:      runtime,
		AgentScheduler:  agentScheduler,
		CronScheduler:   cronScheduler,
	}
	a.echo = api.NewRouter(deps)

	a.httpServer = &http.Server{
		Addr:         cfg.HTTP.Address,
		Handler:      a.echo,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	return a, nil
}

// assembleDeps 按顺序创建运行期依赖：S3 → DuckLake factory → 系统库 bootstrap/migrate/seed
// → catalog/auth/registry。DevMode 只影响 DATA_PATH 是否本地，不再使用 SQLite catalog。
func (a *App) assembleDeps(ctx context.Context) error {
	cfg := a.cfg

	if cfg.Instance.Writable && !cfg.DevMode {
		if cfg.S3.Bucket == "" || cfg.S3.Region == "" || cfg.S3.Prefix == "" {
			return fmt.Errorf("writable non-dev instance requires s3.bucket, s3.region and s3.prefix")
		}
	}

	s3cfg := objectstore.Config{
		Endpoint:       cfg.S3.Endpoint,
		Region:         cfg.S3.Region,
		Bucket:         cfg.S3.Bucket,
		Prefix:         cfg.S3.Prefix,
		AccessKey:      cfg.S3.AccessKey,
		SecretKey:      cfg.S3.SecretKey,
		KMSKeyID:       cfg.S3.KMSKeyID,
		ForcePathStyle: cfg.S3.ForcePathStyle,
	}
	if cfg.Instance.Writable && !cfg.DevMode && s3cfg.Bucket != "" {
		store, err := objectstore.NewClient(ctx, s3cfg, a.logger, a.metrics)
		if err != nil {
			return fmt.Errorf("create objectstore client: %w", err)
		}
		a.objectStore = store

		filePrefix := cfg.S3.Prefix + "/" + cfg.Instance.ID + "/files"
		fs, err := objectstore.NewS3FileStore(ctx, s3cfg, filePrefix, a.logger)
		if err != nil {
			return fmt.Errorf("create s3 file store: %w", err)
		}
		a.fileStore = fs
	}

	keys := objectstore.KeyBuilder{
		RootPrefix:  cfg.S3.Prefix,
		Environment: cfg.Instance.ID,
	}
	if !cfg.Instance.Writable {
		return nil
	}

	userCacheDir := cfg.Database.CacheDir
	if cfg.DevMode {
		userCacheDir = filepath.Join(cfg.Database.CacheDir, "dev", "dbs")
		if err := os.MkdirAll(userCacheDir, 0o755); err != nil {
			return fmt.Errorf("create dev dbs dir: %w", err)
		}
	}
	userFactory := a.newUserDatabaseFactory(userCacheDir)
	sysFactory := a.newSystemDatabaseFactory()

	sysName := cfg.SystemDatabase.Name
	if sysName == "" {
		sysName = systemdb.DefaultName
	}
	store, err := systemdb.Bootstrap(ctx, systemdb.BootstrapInput{
		LocatorDir: filepath.Join(cfg.Database.CacheDir, "system"),
		Name:       sysName,
		Factory:    sysFactory,
		Keys:       keys,
		Logger:     a.logger,
	})
	if err != nil {
		return fmt.Errorf("bootstrap system database: %w", err)
	}
	a.systemStore = store
	a.registerCloser(store)

	repo := store.CatalogRepo()
	var descWriter catalog.DescriptorWriter
	if a.objectStore != nil {
		descWriter = a.objectStore
	}
	ducklakeStore := objectstore.DuckLakeStorage{
		Endpoint:       cfg.S3.Endpoint,
		Region:         cfg.S3.Region,
		Bucket:         cfg.S3.Bucket,
		ForcePathStyle: cfg.S3.ForcePathStyle,
		KMSKeyIDRef:    cfg.S3.KMSKeyID,
	}
	a.catalog = catalog.NewService(repo, keys, descWriter, ducklakeStore, a.logger)
	if err := a.catalog.RepairDatabasesOnStartup(ctx); err != nil {
		return fmt.Errorf("repair database availability: %w", err)
	}

	a.registry = registry.New(userFactory, a.catalog, registry.Options{
		IdleTimeout: cfg.Database.IdleTimeout,
		MaxOpen:     cfg.Database.MaxOpen,
		Writable:    cfg.Instance.Writable,
	}, a.logger, a.metrics)

	a.auth = auth.NewService(store.AuthRepo(), cfg.Auth.APIKeyHashSecret)
	a.users = auth.NewUserService(auth.NewSQLUserRepository(store.DB()), catalog.ReservedTenantID)
	a.sessions = auth.NewSessionService(a.users, auth.NewSQLSessionRepository(store.DB()),
		cfg.Auth.APIKeyHashSecret, auth.SessionConfig{
			AccessTTL:  2 * time.Hour,
			RefreshTTL: 7 * 24 * time.Hour,
		})

	if err := systemdb.Seed(ctx, systemdb.SeedInput{
		Store:   store,
		Auth:    a.auth,
		Catalog: a.catalog,
		DevMode: cfg.DevMode,
		Logger:  a.logger,
	}); err != nil {
		return fmt.Errorf("seed system database: %w", err)
	}
	store.SetDefaultLogKeepDays(cfg.SystemDatabase.LogKeepDays)
	if err := store.SeedGlobalRetention(ctx, cfg.SystemDatabase.LogKeepDays); err != nil {
		return fmt.Errorf("seed log retention: %w", err)
	}
	store.StartPeriodicFlush(cfg.SystemDatabase.LogFlushInterval, cfg.SystemDatabase.MetricsFlushInterval)

	a.cacheMgr, err = cache.NewManager(cache.Options{
		Root:         cfg.Database.CacheDir,
		MaxBytes:     cfg.Database.CacheMaxBytes,
		MaxDatabases: cfg.Database.CacheMaxDatabases,
		Registry:     a.registry,
		Closer:       a.registry,
		Logger:       a.logger,
		Metrics:      a.metrics,
	})
	if err != nil {
		return fmt.Errorf("create cache manager: %w", err)
	}

	catRepo := a.catalog.Repository()
	a.usageSvc = usage.NewService(catRepo, a.logger)
	a.auditSvc = audit.NewService(catRepo, a.logger)

	if cfg.LLM.Enabled {
		var resolver llmgateway.ProviderResolver = llmgateway.NewCatalogResolver(a.catalog, nil)
		if inst := instanceLLMProviders(cfg.LLM.Providers); len(inst) > 0 {
			resolver = llmgateway.NewFallbackResolver(resolver, inst)
		}
		a.llmSvc = llmgateway.NewService(resolver, usage.NewLLMRecorder(a.usageSvc), a.logger)
	}

	if cfg.DevMode {
		fs, err := objectstore.NewLocalFileStore(filepath.Join(cfg.Database.CacheDir, "dev", "files"))
		if err != nil {
			return fmt.Errorf("create local file store: %w", err)
		}
		a.fileStore = fs
		a.logger.Info("running in dev mode: local DuckLake DATA_PATH, S3 bypassed",
			zap.String("user_dbs", userCacheDir),
			zap.String("system_dir", filepath.Join(cfg.Database.CacheDir, "system")),
		)
	}

	return nil
}

// Close 反向关闭所有已注册资源。供 New 失败路径和测试 cleanup 调用。
func (a *App) Close(ctx context.Context) error {
	var firstErr error
	if a.registry != nil {
		if err := a.registry.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	a.closerMu.Lock()
	closers := a.closers
	a.closers = nil
	a.closerMu.Unlock()
	for i := len(closers) - 1; i >= 0; i-- {
		if err := closers[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Start 启动 HTTP server。阻塞调用者直到 Shutdown。
func (a *App) Start() error {
	a.logger.Info("http server listening", zap.String("address", a.cfg.HTTP.Address))
	err := a.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown 优雅关闭：HTTP server → 调度器 → registry → catalog → LLM。
// 超时由 ctx 控制。
func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("shutdown started")
	var firstErr error

	if err := a.httpServer.Shutdown(ctx); err != nil {
		a.logger.Error("http shutdown error", zap.String("err", err.Error()))
		if firstErr == nil {
			firstErr = err
		}
	}

	// 停止 Cloud Agent 调度器并等待 in-flight 执行收敛（受 ctx 超时约束）。
	if a.schedulerCancel != nil {
		a.schedulerCancel()
	}
	if a.agentScheduler != nil {
		done := make(chan struct{})
		go func() {
			a.agentScheduler.Stop()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			a.logger.Warn("agent scheduler stop timed out")
		}
	}
	// 停止云函数定时任务调度器（同上收敛语义）。
	if a.cronScheduler != nil {
		done := make(chan struct{})
		go func() {
			a.cronScheduler.Stop()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			a.logger.Warn("cron scheduler stop timed out")
		}
	}

	// 关闭 registry + 所有已注册资源（catalog DB 等）
	if err := a.Close(ctx); err != nil {
		a.logger.Error("resource close error", zap.String("err", err.Error()))
		if firstErr == nil {
			firstErr = err
		}
	}

	if err := observability.Sync(a.logger); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// newUserDatabaseFactory 返回 DuckLake 用户库工厂（唯一引擎）。
func (a *App) newUserDatabaseFactory(cacheDir string) database.Factory {
	cfg := a.cfg
	opts := duckLakeOptions(cfg.Database.DuckLake)
	remote := ducklake.RemoteStorage{}
	var syncer ducklake.Syncer = ducklake.NewLocalSyncer()
	var blobs objectstore.BlobStore

	// 生产（非 DevMode）启用 S3 catalog 同步与 DATA_PATH。
	if !cfg.DevMode && cfg.S3.Bucket != "" {
		remote = ducklake.RemoteStorage{
			Enabled:        true,
			Endpoint:       cfg.S3.Endpoint,
			Region:         cfg.S3.Region,
			Bucket:         cfg.S3.Bucket,
			RootPrefix:     cfg.S3.Prefix,
			Environment:    cfg.Instance.ID,
			AccessKey:      cfg.S3.AccessKey,
			SecretKey:      cfg.S3.SecretKey,
			ForcePathStyle: cfg.S3.ForcePathStyle,
		}
		if a.objectStore != nil {
			if b, ok := a.objectStore.(objectstore.BlobStore); ok {
				blobs = b
			}
		}
		if blobs != nil {
			cs := ducklake.NewCatalogSyncer(blobs, remote, cacheDir, opts.CatalogSync, a.logger, a.metrics)
			syncer = cs
			a.catalogSyncer = cs
			a.registerCloser(syncerCloser{cs: cs})
		}
	}

	f := &ducklake.Factory{
		CacheDir: cacheDir,
		Options:  opts,
		Syncer:   syncer,
		Remote:   remote,
		Blobs:    blobs,
		Logger:   a.logger,
		Metrics:  a.metrics,
	}
	a.duckFactory = f
	return f
}

// newSystemDatabaseFactory 返回系统库专用 Factory；与用户库共享 Remote/Syncer，CacheDir 隔离。
func (a *App) newSystemDatabaseFactory() *ducklake.Factory {
	cfg := a.cfg
	opts := duckLakeOptions(cfg.Database.DuckLake)
	f := &ducklake.Factory{
		CacheDir: filepath.Join(cfg.Database.CacheDir, "system", "dbs"),
		Options:  opts,
		Logger:   a.logger,
		Metrics:  a.metrics,
	}
	if a.duckFactory != nil {
		f.Remote = a.duckFactory.Remote
		f.Blobs = a.duckFactory.Blobs
		f.Syncer = a.duckFactory.Syncer
	}
	if a.catalogSyncer != nil {
		f.Syncer = a.catalogSyncer
	}
	if f.Syncer == nil {
		f.Syncer = ducklake.NewLocalSyncer()
	}
	return f
}

func (a *App) cloudAgentRuntime() *cloudagent.Runtime {
	if a.systemStore == nil {
		return nil
	}
	return &cloudagent.Runtime{
		LLM:      api.NewCloudAgentLLM(api.NewLLMService(a.llmSvc)),
		DB:       api.NewCloudAgentDB(a.catalog, a.registry),
		Obj:      api.NewCloudAgentObj(a.fileStore, a.systemStore),
		Logs:     cloudagent.NewLogAccess(a.systemStore),
		Settings: cloudagent.NewSettingsAccess(a.systemStore),
	}
}

// snapshotFromFactory maps DuckLake sync watermarks to the API snapshot DTO.
// Extracted so tests can cover both empty and non-empty watermarks without HTTP.
func snapshotFromFactory(f *ducklake.Factory, databaseID string) *api.DatabaseSnapshot {
	last, lag := f.SnapshotStatus(databaseID)
	if last == 0 && lag == 0 {
		return nil
	}
	return &api.DatabaseSnapshot{LastSyncedSnapshot: last, SyncLag: lag}
}

func instanceLLMProviders(in map[string]config.ProviderConfig) map[string]llmgateway.InstanceProvider {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]llmgateway.InstanceProvider, len(in))
	for name, p := range in {
		out[name] = llmgateway.InstanceProvider{
			APIKey:        p.APIKey,
			BaseURL:       p.BaseURL,
			DefaultModel:  p.DefaultModel,
			AllowedModels: p.AllowedModels,
			Timeout:       p.Timeout,
		}
	}
	return out
}

func duckLakeOptions(cfg config.DuckLakeConfig) ducklake.Options {
	return ducklake.Options{
		MemoryLimit:          cfg.MemoryLimit,
		Threads:              cfg.Threads,
		ExtensionDir:         cfg.ExtensionDir,
		DataInliningRowLimit: cfg.DataInliningRowLimit,
		ParquetCompression:   cfg.ParquetCompression,
		TargetFileSize:       cfg.TargetFileSize,
		RequireCommitMessage: cfg.RequireCommitMessage,
		CatalogSync: ducklake.CatalogSyncOptions{
			Mode:         cfg.CatalogSync.Mode,
			Debounce:     cfg.CatalogSync.Debounce,
			KeepVersions: cfg.CatalogSync.KeepVersions,
		},
		Maintenance: ducklake.MaintenanceOptions{
			CheckpointInterval:     cfg.Maintenance.CheckpointInterval,
			ExpireOlderThan:        cfg.Maintenance.ExpireOlderThan,
			DeleteOlderThan:        cfg.Maintenance.DeleteOlderThan,
			RewriteDeleteThreshold: cfg.Maintenance.RewriteDeleteThreshold,
		},
	}
}

// registerCloser 让各 plan 注册需要反向关闭的资源。
func (a *App) registerCloser(c io.Closer) {
	a.closerMu.Lock()
	defer a.closerMu.Unlock()
	a.closers = append(a.closers, c)
}

// RunWithSignal 启动服务并在收到 SIGINT/SIGTERM 时优雅关闭。
func (a *App) RunWithSignal(shutdownTimeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return a.Shutdown(ctx)
}

type syncerCloser struct{ cs *ducklake.CatalogSyncer }

func (c syncerCloser) Close() error {
	if c.cs == nil {
		return nil
	}
	return c.cs.Close(context.Background())
}
