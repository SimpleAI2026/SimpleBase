// Package app 装配 SimpleBase 单实例服务的全部依赖并管理生命周期。
// 依赖创建顺序：S3 client → catalog DB → database factory/registry →
// auth → usage → LLM gateway → API router。
// 任一失败时反向关闭已创建资源。
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/api"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/cache"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
	"github.com/linkxzhou/SimpleBase/internal/database/turso"
	"github.com/linkxzhou/SimpleBase/internal/jobs"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/usage"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	// DevMode 下使用 :memory: SQLite；驱动注册名 "sqlite3"。
	_ "github.com/uglyer/go-sqlite3"
)

// App 是运行中的 SimpleBase 实例。它持有 HTTP server 与已创建资源。
type App struct {
	cfg      config.Config
	logger   observability.Logger
	metrics  *observability.Metrics

	httpServer *http.Server
	echo       *echo.Echo

	objectStore objectstore.Client
	catalog     *catalog.Service
	registry    *registry.Registry
	auth        *auth.Service
	cacheMgr    *cache.Manager
	usageSvc    *usage.Service
	auditSvc    *audit.Service
	llmSvc      llmgateway.Service
	jobWorker   *jobs.Worker
	jobEnqueuer *jobs.Enqueuer
	workerCancel context.CancelFunc

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
	if cfg.Instance.Writable && !cfg.DevMode {
		if cfg.S3.Bucket == "" {
			return errors.New("s3 bucket not configured")
		}
		// Writable 实例必须能连通 S3 与 catalog
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
	logger := observability.NewLogger(cfg.Observability.LogLevel, cfg.Observability.LogFormat, nil)
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
		api.NewDatabaseServiceAdapter(a.catalog, a.registry),
		cfg.Instance.Writable,
	)

	// Plan 6：SQL handler。readonly 实例 SQLHandler 为 nil，路由不挂载写操作；
	// query 路由也只在 writable 实例提供（首期 readonly 不开放 SQL API）。
	var sqlHandler *api.SQLHandler
	if cfg.Instance.Writable && a.registry != nil {
		sqlLimits := api.SQLLimits{
			QueryTimeout:       int64(cfg.Limits.QueryTimeout),
			MaxQueryRows:       cfg.Limits.MaxQueryRows,
			MaxConcurrent:      cfg.Limits.MaxConcurrentQueries,
			MaxBatchStatements: cfg.Limits.MaxBatchStatements,
			MaxSQLBytes:        cfg.Limits.MaxSQLBytes,
			MaxRequestBytes:    cfg.Limits.MaxRequestBytes,
		}
		sqlHandler = api.NewSQLHandler(
			api.NewSQLServiceAdapter(a.catalog, a.registry),
			sqlLimits,
			cfg.Instance.Writable,
		)
	}

	deps := api.Dependencies{
		Config:          cfg,
		Logger:          logger,
		Metrics:         metrics,
		Health:          a.health,
		Auth:            a.auth,
		Catalog:         a.catalog,
		Registry:        a.registry,
		DatabaseHandler: dbHandler,
		SQLHandler:      sqlHandler,
		Cache:           api.NewCacheService(a.cacheMgr),
		Usage:           api.NewUsageService(a.usageSvc),
		Audit:           api.NewAuditService(a.auditSvc),
		LLM:             api.NewLLMService(a.llmSvc),
		JobEnqueuer:     api.NewJobEnqueuer(a.jobEnqueuer),
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

// assembleDeps 按顺序创建运行期依赖：S3 → catalog DB → catalog service →
// database factory → registry → auth。失败时已创建的资源由调用方通过 Close 反向释放。
func (a *App) assembleDeps(ctx context.Context) error {
	cfg := a.cfg

	// 1) objectstore client（S3）
	// DevMode 旁路 S3，catalog 与用户库使用 :memory: SQLite。
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
	}

	// 2) catalog 系统数据库连接 + 迁移（Writable 实例需要）
	// Readonly 实例首期不打开 catalog；auth/DB API 在 readonly 模式下不可用。
	keys := objectstore.KeyBuilder{
		RootPrefix:  cfg.S3.Prefix,
		Environment: cfg.Instance.ID,
	}
	if cfg.Instance.Writable {
		// DevMode：catalog 使用 :memory: SQLite，不走 turso/S3。
		if cfg.DevMode {
			catalogDB, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				return fmt.Errorf("open catalog database (dev mode): %w", err)
			}
			catalogDB.SetMaxOpenConns(1) // :memory: 每连接独立，限单连接保证共享
			catalogDB.SetMaxIdleConns(1)
			a.registerCloser(catalogDB)
			if err := catalog.ApplyMigrations(ctx, catalogDB); err != nil {
				return fmt.Errorf("apply catalog migrations: %w", err)
			}

			repo := catalog.NewSQLiteRepository(catalogDB)
			a.catalog = catalog.NewService(repo, keys, nil, a.logger)

			// DevMode factory：用户库也用 :memory: SQLite。
			factory := &database.MemoryFactory{
				Logger:  a.logger,
				Metrics: a.metrics,
			}
			a.registry = registry.New(factory, a.catalog, registry.Options{
				IdleTimeout: cfg.Database.IdleTimeout,
				MaxOpen:     cfg.Database.MaxOpen,
				Writable:    cfg.Instance.Writable,
			}, a.logger, a.metrics)

			// auth service
			a.auth = auth.NewService(auth.NewSQLiteAPIKeyRepository(catalogDB), cfg.Auth.APIKeyHashSecret)

			// cache manager（DevMode 下 Root 可空，仅占位）
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
				resolver := llmgateway.NewCatalogResolver(a.catalog, nil)
				a.llmSvc = llmgateway.NewService(resolver, usage.NewLLMRecorder(a.usageSvc), a.logger)
			}

			if cfg.Instance.Writable {
				handlers := []jobs.Handler{
					jobs.NewDeleteDatabaseHandler(a.catalog, a.registry, nil, a.logger),
				}
				a.jobEnqueuer = jobs.NewEnqueuer(catRepo)
				a.jobWorker, err = jobs.NewWorker(catRepo, handlers, jobs.Options{
					Logger:  a.logger,
					Metrics: a.metrics,
				})
				if err != nil {
					return fmt.Errorf("create job worker: %w", err)
				}
			}
			a.logger.Info("running in dev mode: :memory: SQLite, S3 bypassed")
			return nil
		}

		catalogPrefix := keys.CatalogPrefix()
		catalogStorage := database.StorageConfig{
			Endpoint:       cfg.S3.Endpoint,
			Region:         cfg.S3.Region,
			Bucket:         cfg.S3.Bucket,
			KMSKeyID:       cfg.S3.KMSKeyID,
			ForcePathStyle: cfg.S3.ForcePathStyle,
		}
		catalogPool := turso.PoolOptions{
			MaxOpen: 5,
			MaxIdle: 2,
		}
		catalogDB, err := turso.Open(ctx, turso.OpenOptions{
			DatabaseID: cfg.Catalog.DatabaseID,
			Writable:   cfg.Instance.Writable,
			Storage: turso.StorageConfig{
				Endpoint:       catalogStorage.Endpoint,
				Region:         catalogStorage.Region,
				Bucket:         catalogStorage.Bucket,
				Prefix:         catalogPrefix,
				KMSKeyID:       catalogStorage.KMSKeyID,
				ForcePathStyle: catalogStorage.ForcePathStyle,
			},
		}, catalogPool, a.logger, a.metrics, nil)
		if err != nil {
			return fmt.Errorf("open catalog database: %w", err)
		}
		a.registerCloser(catalogDB)
		if err := catalog.ApplyMigrations(ctx, catalogDB); err != nil {
			return fmt.Errorf("apply catalog migrations: %w", err)
		}

		repo := catalog.NewSQLiteRepository(catalogDB)
		var descWriter catalog.DescriptorWriter
		if a.objectStore != nil {
			descWriter = a.objectStore
		}
		a.catalog = catalog.NewService(repo, keys, descWriter, a.logger)

		// 3) database factory + registry
		factory := &database.TursoFactory{
			Storage:  catalogStorage,
			CacheDir: cfg.Database.CacheDir,
			Pool: turso.PoolOptions{
				MaxOpen: cfg.Database.MaxOpen,
				MaxIdle: cfg.Database.MaxIdle,
			},
			Logger:  a.logger,
			Metrics: a.metrics,
		}
		a.registry = registry.New(factory, a.catalog, registry.Options{
			IdleTimeout: cfg.Database.IdleTimeout,
			MaxOpen:     cfg.Database.MaxOpen,
			Writable:    cfg.Instance.Writable,
		}, a.logger, a.metrics)

		// 4) auth service
		a.auth = auth.NewService(auth.NewSQLiteAPIKeyRepository(catalogDB), cfg.Auth.APIKeyHashSecret)

		// 5) Plan 7-9 依赖：cache manager / usage / audit / llm gateway / jobs
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

		// LLM Gateway（仅在 LLM 启用时装配）。
		if cfg.LLM.Enabled {
			resolver := llmgateway.NewCatalogResolver(a.catalog, nil)
			a.llmSvc = llmgateway.NewService(resolver, usage.NewLLMRecorder(a.usageSvc), a.logger)
		}

		// 后台任务 worker（仅 writable 实例启动）。
		if cfg.Instance.Writable {
			handlers := []jobs.Handler{
				jobs.NewDeleteDatabaseHandler(a.catalog, a.registry, a.objectStore, a.logger),
			}
			a.jobEnqueuer = jobs.NewEnqueuer(catRepo)
			a.jobWorker, err = jobs.NewWorker(catRepo, handlers, jobs.Options{
				Logger:  a.logger,
				Metrics: a.metrics,
			})
			if err != nil {
				return fmt.Errorf("create job worker: %w", err)
			}
		}
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

// Start 启动 HTTP server 与后台 worker。阻塞调用者直到 Shutdown。
func (a *App) Start() error {
	// 启动后台任务 worker（若已配置）。
	if a.jobWorker != nil {
		ctx, cancel := context.WithCancel(context.Background())
		a.workerCancel = cancel
		go a.jobWorker.Start(ctx)
	}
	a.logger.Info("http server listening", zap.String("address", a.cfg.HTTP.Address))
	err := a.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown 优雅关闭：停止 job worker → HTTP server → registry → catalog → LLM。
// 超时由 ctx 控制。
func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("shutdown started")
	var firstErr error

	// 停止后台任务 worker（等待当前任务完成）。
	if a.workerCancel != nil {
		a.workerCancel()
	}
	if a.jobWorker != nil {
		a.jobWorker.Stop()
	}

	if err := a.httpServer.Shutdown(ctx); err != nil {
		a.logger.Error("http shutdown error", zap.String("err", err.Error()))
		if firstErr == nil {
			firstErr = err
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
