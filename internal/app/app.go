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
	"strings"
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
	"github.com/linkxzhou/SimpleBase/internal/database/kv"
	"github.com/linkxzhou/SimpleBase/internal/database/lease"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
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
	sandboxMgr    *sandbox.Manager

	// 多实例一致性（multi-instance-consistency-plan Phase A/B）。
	casSupported bool           // 启动探针结果；false 时禁用租约/manifest 互斥
	leaseMgr     *lease.Manager // per-database 写租约（nil = 未启用）
	leaseKeys    objectstore.KeyBuilder
	leaseGate    *lease.Gate       // 同库并发首写串行化（perf §1.5）
	onLostLease  func(dbID string) // 失租回调（租约获取处闭包捕获）

	delegations      *auth.DelegationRegistry
	agentScheduler   *cloudagent.Scheduler
	cronScheduler    *cronjob.Scheduler
	kvSweeper        *kv.Sweeper
	schedulerCancel  context.CancelFunc
	schedulerBaseCtx context.Context

	kvInitOnce sync.Once
	kvInit     api.KVInitializer // 惰性单例：路由装配与 systemdb 种子共用

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
	// 项目 KV 初始化器（key-value-ducklake-plan §2）：建项目后立即建 kv schema。
	// readonly 实例或无 registry 时为 nil（不初始化）；与种子库初始化共用同一实例。
	kvInit := a.kvInitializer()
	if a.duckFactory != nil {
		f := a.duckFactory
		dbHandler.SnapshotFor = func(databaseID string) *api.DatabaseSnapshot {
			return snapshotFromFactory(f, databaseID)
		}
	}
	// 数据库列表「数据量」列（v2.0 计划 §4-P1）：统计改为缓存 + 后台有界
	// 并发刷新，列表请求不再同步逐库 Acquire + COUNT(*)（N+1 同步请求链）。
	// 新鲜度 = TTL + 同步水位变化（写后失效）。
	sqlSvcForStats := api.NewSQLServiceAdapter(a.catalog, a.registry, a.systemStore)
	countRows := func(ctx context.Context, db catalog.Database) (int64, error) {
		return api.CountDatabaseRows(ctx, sqlSvcForStats, db)
	}
	dbHandler.RowCountFor = countRows
	var rowWatermark func(dbID string) (int64, int64)
	if a.duckFactory != nil {
		rowWatermark = a.duckFactory.SnapshotStatus
	}
	dbHandler.RowCounts = api.NewRowCountCache(api.RowCountCacheOptions{
		Count:     countRows,
		Watermark: rowWatermark,
		Logger:    logger,
	})

	// Plan 6：SQL handler。readonly 实例 SQLHandler 为 nil，路由不挂载写操作；
	// query 路由也只在 writable 实例提供（首期 readonly 不开放 SQL API）。
	var sqlHandler *api.SQLHandler
	var dataHandler *api.DataHandler
	var schemaHandler *api.SchemaHandler
	var kvHandler *api.KVHandler
	var kvService api.KVService
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
		schemaHandler = api.NewSchemaHandler(sqlService.(api.DataService), cfg.Instance.Writable, sqlLimits)
		dbHandler.InitSQL = sqlService
		// 项目级 KV（key-value-ducklake-plan §2/§3）：catalog + registry 桥接。
		kvService = api.NewKVServiceAdapter(a.catalog, a.registry)
		kvHandler = api.NewKVHandler(kvService, cfg.Instance.Writable)
	}

	// 独立云沙盒模块：在 Agent 装配前创建，同一 Manager 同时供 Agent 与 API 使用。
	if cfg.Sandbox.Enabled && a.systemStore != nil {
		var driver sandbox.Driver
		if cfg.Sandbox.EffectiveBackend() == config.SandboxBackendFake && cfg.DevMode {
			driver = sandbox.NewFakeDriver()
		} else if cfg.Sandbox.EffectiveBackend() == config.SandboxBackendCloud {
			cloud, err := sandbox.NewCloudDriver(cfg.Sandbox, cfg.Database.CacheDir, logger)
			if err == nil {
				driver = cloud
			} else if logger != nil {
				logger.Warn("sandbox disabled: cloud backend unavailable (no local fallback)")
			}
		}
		if driver != nil {
			a.sandboxMgr = sandbox.NewManager(cfg.Sandbox, sandboxStoreAdapter{s: a.systemStore}, driver)
			a.registerCloser(a.sandboxMgr)
			if a.schedulerCancel == nil {
				a.schedulerBaseCtx, a.schedulerCancel = context.WithCancel(context.Background())
			}
			a.sandboxMgr.StartReaper(a.schedulerBaseCtx)
		}
	}

	// Cloud Agent 定时执行调度器：仅在运行时与系统库齐备时创建。
	var agentScheduler *cloudagent.Scheduler
	runtime := a.cloudAgentRuntime()
	if runtime != nil && a.systemStore != nil {
		agentScheduler = cloudagent.NewScheduler(a.systemStore, runtime, a.usageSvc)
		a.agentScheduler = agentScheduler
		if a.schedulerCancel == nil {
			a.schedulerBaseCtx, a.schedulerCancel = context.WithCancel(context.Background())
		}
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

	// KV TTL 后台清扫（key-value-ducklake-plan §4.5）：仅可写实例 + registry 齐备时创建。
	if cfg.Instance.Writable && a.registry != nil {
		kvSweeper := kv.NewSweeper(a.registry, 60*time.Second, logger)
		a.kvSweeper = kvSweeper
		if a.schedulerCancel == nil {
			a.schedulerBaseCtx, a.schedulerCancel = context.WithCancel(context.Background())
		}
		kvSweeper.Start(a.schedulerBaseCtx)
	}

	deps := api.Dependencies{
		Config:  cfg,
		Logger:  logger,
		Metrics: metrics,
		// /metrics 输出源与 Metrics 同 registry（perfbench 抓 stage 指标依赖）。
		MetricsRegistry: metricsGatherer(reg),
		Health:          a.health,
		Auth:            a.auth,
		Sessions:        a.sessions,
		Users:           a.users,
		Catalog:         a.catalog,
		Registry:        a.registry,
		DatabaseHandler: dbHandler,
		SQLHandler:      sqlHandler,
		DataHandler:     dataHandler,
		SchemaHandler:   schemaHandler,
		KVHandler:       kvHandler,
		KVService:       kvService,
		KVInit:          kvInit,
		Cache:           api.NewCacheService(a.cacheMgr),
		Usage:           api.NewUsageService(a.usageSvc),
		Audit:           api.NewAuditService(a.auditSvc),
		LLM:             api.NewLLMService(a.llmSvc),
		S3FileStore:     a.fileStore,
		System:          a.systemStore,
		CloudAgent:      runtime,
		Delegations:     a.delegations,
		AgentScheduler:  agentScheduler,
		CronScheduler:   cronScheduler,
		Sandbox:         a.sandboxMgr,
		SandboxUsage:    a.usageSvc,
	}
	a.echo = api.NewRouter(deps)

	writeTimeout := cfg.HTTP.WriteTimeout
	if a.sandboxMgr != nil && a.sandboxMgr.Available() {
		// 同步执行包含 Cloud 冷启动，写超时须覆盖最长命令及启动预算。
		minTimeout := cfg.Sandbox.ExecTimeoutMax + time.Minute
		if minTimeout > writeTimeout {
			writeTimeout = minTimeout
		}
	}
	a.httpServer = &http.Server{
		Addr:         cfg.HTTP.Address,
		Handler:      a.echo,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	return a, nil
}

// metricsGatherer 把 Registerer 适配为 Gatherer；仅 *prometheus.Registry
// 两者兼备，其余实现回退 DefaultGatherer（保持旧装配行为）。
func metricsGatherer(reg prometheus.Registerer) prometheus.Gatherer {
	if g, ok := reg.(prometheus.Gatherer); ok {
		return g
	}
	return prometheus.DefaultGatherer
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

	// P0-4：instance.id / s3.prefix 变更检测（multi-instance-consistency-plan §4.8）。
	// 改 id 等价于换全新空存储前缀，必须拒绝并指向迁移流程。
	if err := checkInstanceIdentity(cfg.Database.CacheDir, cfg.Instance.ID, cfg.S3.Prefix); err != nil {
		return err
	}

	if cfg.Instance.Writable && !cfg.DevMode && s3cfg.Bucket != "" {
		store, err := objectstore.NewClient(ctx, s3cfg, a.logger, a.metrics)
		if err != nil {
			return fmt.Errorf("create objectstore client: %w", err)
		}
		a.objectStore = store

		// P0-3 启动探针：create-if-absent 条件写可用性验证（§4.5）。
		// 探针失败 → 记 Fatal + 指标归零 → 禁用一切依赖互斥的能力，
		// /health/ready 暴露原因；不让代码以为自己有保护。
		probePrefix := objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix, Environment: cfg.Instance.ID}.CatalogPrefix()
		if bs, ok := store.(objectstore.BlobStore); ok {
			res := objectstore.ProbePutIfAbsent(ctx, bs, probePrefix)
			a.casSupported = res.CASSupported
			if a.metrics != nil {
				if res.CASSupported {
					a.metrics.CASSupported.Set(1)
				} else {
					a.metrics.CASSupported.Set(0)
				}
			}
			if !res.CASSupported {
				a.logger.Error("objectstore create-if-absent probe FAILED; mutual-exclusion capabilities disabled",
					zap.Error(res.Err),
					zap.String("hint", "endpoint does not enforce create-if-absent (bucket versioning enabled on COS?)"),
				)
				// §3.3：配置要求写租约但探针失败时阻止启动——不得退化到
				// 静默无保护的单写（可写实例 + 远端同步器 + 无互斥）。
				if cfg.Instance.Lease.Enabled {
					return fmt.Errorf("objectstore create-if-absent probe failed: %w; "+
						"instance.lease.enabled=true 要求对象存储支持条件写，拒绝在无单写保护下启动可写实例 "+
						"(hint: COS 存储桶开启版本控制会使 x-cos-forbid-overwrite 失效)", res.Err)
				}
			} else {
				a.logger.Info("objectstore create-if-absent probe passed")
			}
		}

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

	// ducklake-duckdb-catalog-plan §4.2：catalog 引擎启动校验（CAS 探针之后、
	// 系统库 Bootstrap 之前）。与已有数据不一致 / 历史数据无标记 → 拒绝启动。
	if err := a.checkCatalogEngine(ctx); err != nil {
		return err
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

	// P1-1：per-database 写租约（multi-instance-consistency-plan §4.6）。
	// 仅 writable + 非 DevMode + 探针通过时启用；失租 → 释放该库句柄。
	regOpts := registry.Options{
		IdleTimeout: cfg.Database.IdleTimeout,
		MaxOpen:     cfg.Database.MaxOpen,
		Writable:    cfg.Instance.Writable,
	}
	if cfg.Instance.Writable && !cfg.DevMode && cfg.Instance.Lease.Enabled && a.casSupported {
		if blobs, ok := a.objectStore.(objectstore.BlobStore); ok {
			keys := objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix, Environment: cfg.Instance.ID}
			lm := lease.NewManager(blobs, keys, lease.Config{
				Enabled:       true,
				TTL:           cfg.Instance.Lease.TTL,
				RenewInterval: cfg.Instance.Lease.RenewInterval,
				Grace:         cfg.Instance.Lease.Grace,
			})
			lm.SetLogger(zapAdapter{a.logger})
			a.leaseMgr = lm
			a.leaseKeys = keys
			a.leaseGate = lease.NewGate()
			a.registerCloser(leaseCloser{m: lm})
			a.logger.Info("per-database write lease enabled",
				zap.String("ttl", cfg.Instance.Lease.TTL.String()),
				zap.String("on_lost", cfg.Instance.Lease.OnLost),
			)

			// 失租回调：停同步 PUT + 释放句柄（§4.6 onLost 顺序）。
			a.onLostLease = func(dbID string) {
				a.logger.Error("write lease LOST: releasing database handle and disabling further writes",
					zap.String("database_id", dbID))
				if a.metrics != nil && a.metrics.LeaseLost != nil {
					a.metrics.LeaseLost.WithLabelValues(dbID).Inc()
				}
				if a.metrics != nil && a.metrics.LeaseState != nil {
					a.metrics.LeaseState.WithLabelValues(dbID).Set(0)
				}
				// 同步器立即进入失租模式：后续 Flush/Close 不再 PUT（§4.6 第 1 步）。
				if cs, ok := a.catalogSyncer.(*ducklake.CatalogSyncer); ok {
					cs.CloseNoFlush(dbID)
				}
				if strings.EqualFold(cfg.Instance.Lease.OnLost, "exit") {
					a.logger.Fatal("write lease lost and on_lost=exit: terminating process")
				}
				a.registry.Remove(dbID)
				// perf §1.5：失租后清除本进程的写门记录，允许后续重新竞争租约。
				a.leaseGate.Clear(dbID)
			}

			// WriteGate：ReadWrite Acquire 前必须持租（惰性首次获取）。
			// 失租（ValidFor=false 且已有租约记录）→ 拒绝新写。
			// perf §1.5：同库并发首写在 leaseGate 上按 dbID 串行化，
			// 只触发一次 Acquire，其余请求复用结果（修复并发首写 45s + 500）。
			regOpts.WriteGate = func(db catalog.Database) error {
				if lm.ValidFor(db.ID) {
					return nil
				}
				// 已有租约记录但未持租 = 失租后：拒绝。
				if lm.Has(db.ID) {
					return fmt.Errorf("instance: write lease lost for database %s; writes rejected until lease recovered", db.ID)
				}
				// 首次写：获取租约。他人持租 → 拒绝（该实例应降级只读）。
				if err := a.leaseGate.TryDo(db.ID, func() error {
					if lm.ValidFor(db.ID) {
						return nil
					}
					if lm.Has(db.ID) {
						return fmt.Errorf("instance: write lease lost for database %s; writes rejected until lease recovered", db.ID)
					}
					if _, err := lm.Acquire(ctx, db.TenantID, db.ID, cfg.Instance.ID, a.onLostLease); err != nil {
						if lease.IsHeld(err) {
							if a.metrics != nil && a.metrics.LeaseHeldRejected != nil {
								a.metrics.LeaseHeldRejected.WithLabelValues(db.ID).Inc()
							}
							return fmt.Errorf("instance: write lease held by another instance for database %s: %w", db.ID, err)
						}
						return fmt.Errorf("instance: acquire write lease: %w", err)
					}
					return nil
				}); err != nil {
					if errors.Is(err, lease.ErrAcquiring) {
						// 同库首租获取在途：立即返回可识别状态，不排队挂等 TTL+Grace。
						return fmt.Errorf("instance: write lease acquisition in progress for database %s: %w", db.ID, err)
					}
					return err
				}
				if a.metrics != nil {
					if a.metrics.LeaseState != nil {
						a.metrics.LeaseState.WithLabelValues(db.ID).Set(1)
					}
					if a.metrics.LeaseEpoch != nil {
						a.metrics.LeaseEpoch.WithLabelValues(db.ID).Set(float64(lm.EpochFor(db.ID)))
					}
				}
				if cs, ok := a.catalogSyncer.(*ducklake.CatalogSyncer); ok {
					cs.SetWriterEpoch(db.ID, lm.EpochFor(db.ID))
				}
				return nil
			}
		}
	} else if cfg.Instance.Writable && !cfg.DevMode && cfg.Instance.Lease.Enabled && !a.casSupported {
		a.logger.Warn("write lease requested but disabled: create-if-absent probe failed",
			zap.String("impact", "single-writer enforcement is NOT active; replicas must stay 1"))
	}

	a.registry = registry.New(userFactory, a.catalog, regOpts, a.logger, a.metrics)

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
		InitKV:  a.initKVFunc(),
	}); err != nil {
		return fmt.Errorf("seed system database: %w", err)
	}
	store.SetDefaultLogKeepDays(cfg.SystemDatabase.LogKeepDays)
	if err := store.SeedGlobalRetention(ctx, cfg.SystemDatabase.LogKeepDays); err != nil {
		return fmt.Errorf("seed log retention: %w", err)
	}
	store.StartPeriodicFlush(cfg.SystemDatabase.LogFlushInterval, cfg.SystemDatabase.MetricsFlushInterval)

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
			_, err := a.leaseMgr.Acquire(ctx, sysMeta.TenantID, sysMeta.ID, cfg.Instance.ID, a.onLostLease)
			return err == nil
		}
	}
	maint := systemdb.NewMaintenanceRunner(store, duckLakeOptions(cfg.Database.DuckLake).Maintenance, maintWritable, a.logger)
	maint.Start()
	a.registerCloser(maint)

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
		// 设置页把厂商凭证写进 sys_llm_provider_creds。每次请求现读该表，
		// 保存后无需重启。catalog CredentialRef 与 YAML 实例供应商仅作回退。
		var resolver llmgateway.ProviderResolver = llmgateway.NewCatalogResolver(a.catalog, nil)
		resolver = llmgateway.NewFirstUsableResolver(
			llmgateway.NewProjectCredResolver(llmgateway.NewSystemCredSource(a.systemStore)),
			resolver,
		)
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
	if a.schedulerCancel != nil {
		a.schedulerCancel()
	}
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
// Start 启动 HTTP server（阻塞直到关闭）。
func (a *App) Start() error {
	a.logger.Info("http server listening", zap.String("address", a.cfg.HTTP.Address))
	err := a.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Handler 返回路由 Handler（perfbench 等进程内压测挂 httptest 用）。
// 未装配 HTTP server（测试/压测环境先行关闭）时返回 nil。
func (a *App) Handler() http.Handler {
	if a.echo == nil {
		return nil
	}
	return a.echo
}

// SystemStore 返回系统库句柄（perfbench 在用例间排空日志/指标缓冲用；
// 生产代码不得经此绕过 catalog 读写系统数据）。
func (a *App) SystemStore() *systemdb.Store {
	return a.systemStore
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

	// 停止 KV TTL 清扫器（schedulerCancel 已停其循环，此处等待退出）。
	if a.kvSweeper != nil {
		if err := a.kvSweeper.Close(); err != nil {
			a.logger.Warn("kv sweeper close error", zap.String("err", err.Error()))
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
			cs := ducklake.NewCatalogSyncer(blobs, remote, cacheDir, opts.CatalogSync, opts.CatalogEngine, a.logger, a.metrics)
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

// kvInitializer 惰性构造共享的 KV 系统表初始化器（key-value-ducklake-plan §2）；
// 只读实例或 registry 未装配时返回 nil（不初始化）。
func (a *App) kvInitializer() api.KVInitializer {
	a.kvInitOnce.Do(func() {
		if a.cfg.Instance.Writable && a.registry != nil {
			a.kvInit = api.NewKVInitializer(a.registry, a.logger)
		}
	})
	return a.kvInit
}

// initKVFunc 返回 KV 系统表初始化函数（供 systemdb 种子库使用）；
// 只读实例或 registry 未装配时返回 nil（不初始化）。
func (a *App) initKVFunc() func(ctx context.Context, db catalog.Database) error {
	init := a.kvInitializer()
	if init == nil {
		return nil
	}
	return init.InitKV
}

// cloudAgentRuntime 装配云 Agent 运行时；沙盒按配置注入（cloud-agent-sandbox-plan §3.1）。
func (a *App) cloudAgentRuntime() *cloudagent.Runtime {
	if a.systemStore == nil {
		return nil
	}
	cliPath := strings.TrimSpace(a.cfg.Agent.CLIPath)
	if cliPath == "" {
		if exe, err := os.Executable(); err == nil {
			cliPath = filepath.Join(filepath.Dir(exe), "simplebase")
		}
	}
	reg := auth.NewDelegationRegistry()
	a.delegations = reg
	rt := &cloudagent.Runtime{
		LLM:            api.NewCloudAgentLLM(api.NewLLMService(a.llmSvc)),
		DB:             api.NewCloudAgentDB(a.catalog, a.registry),
		Obj:            api.NewCloudAgentObj(a.fileStore, a.systemStore),
		Logs:           cloudagent.NewLogAccess(a.systemStore),
		Settings:       cloudagent.NewSettingsAccess(a.systemStore),
		MaxIterations:  a.cfg.LLM.EffectiveAgentMaxIterations(),
		RunTimeout:     a.cfg.LLM.EffectiveAgentRunTimeout(),
		ToolProtocol:   a.cfg.LLM.EffectiveAgentToolProtocol(),
		SkillsCLI:      a.cfg.Agent.SkillsCLI,
		CLIPath:        cliPath,
		CLI:            cloudagent.SubprocessCLI{Path: cliPath},
		APIBaseURL:     loopbackBase(a.cfg.HTTP.Address),
		ConfirmTimeout: a.cfg.Agent.EffectiveConfirmTimeout(),
		DelegationTTL:  a.cfg.Agent.EffectiveDelegationTTL(),
		Issuer:         auth.HMACDelegationIssuer{Secret: a.cfg.Auth.APIKeyHashSecret, Reg: reg},
	}
	if a.sandboxMgr != nil && a.sandboxMgr.Available() {
		rt.Sandbox = &sandboxAdapter{m: a.sandboxMgr}
	}
	return rt
}

// sandboxAdapter 只适配 cloudagent 的线程工具接口，业务状态统一由 Manager 管理。
type sandboxAdapter struct{ m *sandbox.Manager }

func loopbackBase(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	host, port, err := splitHostPortLoose(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + host + ":" + port
}

func splitHostPortLoose(addr string) (string, string, error) {
	if strings.HasPrefix(addr, "[") {
		end := strings.LastIndex(addr, "]")
		if end < 0 || end+1 >= len(addr) || addr[end+1] != ':' {
			return "", "", errors.New("bad addr")
		}
		return addr[:end+1], addr[end+2:], nil
	}
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return "", "", errors.New("bad addr")
	}
	return addr[:i], addr[i+1:], nil
}

func (s *sandboxAdapter) Available() bool { return s.m.Available() }
func (s *sandboxAdapter) Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (cloudagent.SandboxOutput, error) {
	out, err := s.m.ExecForThread(ctx, projectID, threadID, sandbox.ExecInput{Cmd: cmd, Args: args})
	return cloudagent.SandboxOutput{Stdout: out.Stdout, Stderr: out.Stderr, ExitCode: out.ExitCode}, err
}
func (s *sandboxAdapter) Shell(ctx context.Context, projectID, threadID, command string) (cloudagent.SandboxOutput, error) {
	out, err := s.m.ExecForThread(ctx, projectID, threadID, sandbox.ExecInput{Command: command})
	return cloudagent.SandboxOutput{Stdout: out.Stdout, Stderr: out.Stderr, ExitCode: out.ExitCode}, err
}
func (s *sandboxAdapter) ReadFile(ctx context.Context, projectID, threadID, path string) (string, error) {
	file, err := s.m.ReadFileForThread(ctx, projectID, threadID, path)
	return string(file.Content), err
}
func (s *sandboxAdapter) WriteFile(ctx context.Context, projectID, threadID, path, content string) error {
	return s.m.WriteFileForThread(ctx, projectID, threadID, path, []byte(content))
}
func (s *sandboxAdapter) ReleaseThread(ctx context.Context, projectID, threadID string) error {
	return s.m.ReleaseThread(ctx, projectID, threadID)
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
		CatalogEngine:        cfg.CatalogEngine,
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
			Interval:     cfg.CatalogSync.Interval,
			MaxLag:       cfg.CatalogSync.MaxLag,
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

// leaseCloser 进程关闭时停止全部续约。
type leaseCloser struct{ m *lease.Manager }

func (c leaseCloser) Close() error {
	if c.m == nil {
		return nil
	}
	c.m.StopAll()
	return nil
}

// zapAdapter 把 observability.Logger 适配为 lease.Logger。
type zapAdapter struct{ l observability.Logger }

func (a zapAdapter) Printf(format string, args ...any) {
	if a.l != nil {
		a.l.Info(fmt.Sprintf(format, args...))
	}
}
