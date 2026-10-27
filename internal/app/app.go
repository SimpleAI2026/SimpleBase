// 文件说明：app.go — 装配 SimpleBase 单实例服务的全部依赖并管理生命周期
//
// 用法
//
//	New(ctx context.Context, cfg config.Config) (*App, error)
//	(a *App) Close(ctx context.Context) error  // Close 反向关闭所有已注册资源
//	(a *App) Start() error  // Start 启动 HTTP server
//	同包调用：New() → cron_runner.go, reset.go；App → catalog_engine.go；ReadFile() → identity.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/app.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, godatabase, godatabase/cache, godatabase/ducklake 等
//	[同包] catalog_engine.go, cron_runner.go, identity.go, project_purger.go, reset.go 等 6 个
//	[关键引用] New() ← cron_runner.go, reset.go
//
// 主要符号
//
//	type App struct {…}
//	New(ctx context.Context, cfg config.Config) (*App, error)
//	NewWithRegistry(ctx context.Context, cfg config.Config, reg prometheus.Registerer) ...
//	(a *App) Close(ctx context.Context) error
//	(a *App) Start() error
//	(a *App) Handler() http.Handler
//	... 另有 43 个顶层符号

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/cache"
	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/kv"
	"github.com/linkxzhou/SimpleBase/godatabase/lease"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm/fake"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm/local"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/api"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/backup"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"github.com/linkxzhou/SimpleBase/internal/functionrun"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/sandbox/sshd"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/trigger"
	"github.com/linkxzhou/SimpleBase/internal/usage"
	// 云函数脚本可 import 的宿主标准库注册（blank import 必须保留，
	// 否则解释器 BuildProgram 找不到 fmt/json 等包，见 ui-gofunction-plan §4）
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
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
	fileFactory   *filedb.Factory
	fileSyncer    filedb.Syncer
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
	// sshHostKey 是沙盒终端的 SSH 主机密钥（setupSandboxTerminal 装配）。
	sshHostKey ssh.Signer

	// 多实例一致性（multi-instance-consistency-plan Phase A/B）。
	casSupported bool           // 启动探针结果；false 时禁用租约/manifest 互斥
	leaseMgr     *lease.Manager // per-database 写租约（nil = 未启用）
	leaseKeys    objectstore.KeyBuilder
	leaseGate    *lease.Gate       // 同库并发首写串行化（perf §1.5）
	onLostLease  func(dbID string) // 失租回调（租约获取处闭包捕获）

	delegations       *auth.DelegationRegistry
	cronScheduler     *cronjob.Scheduler
	triggerDispatcher *trigger.Dispatcher
	kvSweeper         *kv.Sweeper
	schedulerCancel   context.CancelFunc
	schedulerBaseCtx  context.Context

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
	if cfg.Instance.Writable && cfg.IsDBRemote() {
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
		api.NewDatabaseServiceAdapter(a.catalog, dbbridge.RegistryFrom(a.registry), newProjectPurger(a.fileFactory, a.fileSyncer)),
		cfg.Instance.Writable,
	)
	// 项目 KV 初始化器（key-value-ducklake-plan §2）：建项目后立即建 kv schema。
	// readonly 实例或无 registry 时为 nil（不初始化）；与种子库初始化共用同一实例。
	kvInit := a.kvInitializer()
	if a.fileFactory != nil {
		f := a.fileFactory
		dbHandler.SnapshotFor = func(databaseID string) *api.DatabaseSnapshot {
			return snapshotFromFileFactory(f, databaseID)
		}
	}
	// 数据库列表「数据量」列（v2.0 计划 §4-P1）：统计改为缓存 + 后台有界
	// 并发刷新，列表请求不再同步逐库 Acquire + COUNT(*)（N+1 同步请求链）。
	// 新鲜度 = TTL + 同步水位变化（写后失效）。
	sqlSvcForStats := api.NewSQLServiceAdapter(a.catalog, dbbridge.RegistryFrom(a.registry), a.systemStore)
	countRows := func(ctx context.Context, db catalog.Database) (int64, error) {
		return api.CountDatabaseRows(ctx, sqlSvcForStats, db)
	}
	dbHandler.RowCountFor = countRows
	var rowWatermark func(dbID string) (int64, int64)
	if a.fileFactory != nil {
		rowWatermark = a.fileFactory.SnapshotStatus
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
	realtimeHandler := api.NewRealtimeHandler(cfg.Limits.RealtimeMaxConns)
	recordEvents := &api.RecordEventFanout{}
	recordEvents.Add(realtimeHandler)
	var functionRunner *functionrun.Runner
	var triggerEvents *api.TriggerEventPublisher
	var functionTriggerHandler *api.FunctionTriggerHandler
	if a.systemStore != nil {
		functionRunner = functionrun.NewRunner(a.systemStore, cfg.Limits.MaxConcurrentGoRuns)
		a.triggerDispatcher = trigger.NewDispatcher(a.systemStore, functionRunner, 1024, 4)
		triggerEvents = api.NewTriggerEventPublisher(a.triggerDispatcher)
		recordEvents.Add(triggerEvents)
		functionTriggerHandler = api.NewFunctionTriggerHandler(a.systemStore, a.triggerDispatcher, cfg.Instance.Writable)
	}
	var collectionMetaHandler *api.CollectionMetaHandler
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
		sqlService := api.NewSQLServiceAdapter(a.catalog, dbbridge.RegistryFrom(a.registry), a.systemStore)
		sqlHandler = api.NewSQLHandler(sqlService, sqlLimits, cfg.Instance.Writable)
		if a.fileFactory != nil {
			f := a.fileFactory
			sqlHandler.DurabilityFor = f.DurabilityFor
		}
		dataHandler = api.NewDataHandler(sqlService.(api.DataService), cfg.Instance.Writable).WithEvents(recordEvents).WithImportLimit(cfg.Limits.MaxImportBytes)
		if a.systemStore != nil {
			schemas := collection.NewRepository(a.systemStore.DB(), a.systemStore.NotifyWrite)
			dataHandler.WithCollections(schemas, cfg.Limits.MaxFilterScanRows)
			collectionMetaHandler = api.NewCollectionMetaHandler(sqlService.(api.DataService), schemas, cfg.Instance.Writable, cfg.Limits.MaxFilterScanRows)
		}
		schemaHandler = api.NewSchemaHandler(sqlService.(api.DataService), cfg.Instance.Writable, sqlLimits)
		dbHandler.InitSQL = sqlService
		// 项目级 KV（key-value-ducklake-plan §2/§3）：catalog + registry 桥接。
		kvService = api.NewKVServiceAdapter(a.catalog, dbbridge.RegistryFrom(a.registry))
		kvHandler = api.NewKVHandler(kvService, cfg.Instance.Writable)
	}

	// 独立云沙盒模块：在 Agent 装配前创建，同一 Manager 同时供 Agent 与 API 使用。
	if cfg.Sandbox.Enabled && a.systemStore != nil {
		var driver vm.Driver
		driverLog := sandboxDriverLog(logger)
		switch cfg.Sandbox.EffectiveBackend() {
		case config.SandboxBackendFake:
			driver = fake.New()
		case config.SandboxBackendLocal:
			localDrv, err := local.New(ctx, sandboxLocalOptions(cfg, driverLog))
			if err == nil {
				driver = localDrv
			} else if logger != nil {
				// 隔离不可用时不退回裸执行：云沙盒功能整体不可用，其余服务照常启动。
				logger.Warn("sandbox disabled: local isolation unavailable (no unisolated fallback)", zap.Error(err))
			}
		}
		if driver != nil {
			driver = vm.WithLogging(driver, driverLog)
			a.sandboxMgr = sandbox.NewManager(cfg.Sandbox, sandboxStoreAdapter{s: a.systemStore}, driver)
			if logger != nil {
				a.sandboxMgr.SetLogger(logger)
				logger.Info("sandbox manager ready", zap.String("backend", cfg.Sandbox.EffectiveBackend()),
					zap.String("image", cfg.Sandbox.Image), zap.Int("max_per_project", cfg.Sandbox.MaxPerProject),
					zap.Duration("idle_timeout", cfg.Sandbox.IdleTimeout), zap.Duration("reap_interval", cfg.Sandbox.ReapInterval))
			}
			a.registerCloser(a.sandboxMgr)
			if a.schedulerCancel == nil {
				a.schedulerBaseCtx, a.schedulerCancel = context.WithCancel(context.Background())
			}
			a.sandboxMgr.StartReaper(a.schedulerBaseCtx)
		}
	}
	var terminal *api.TerminalBridge
	if a.sandboxMgr != nil && a.sandboxMgr.Available() && a.sandboxMgr.Interactive() {
		terminal = a.setupSandboxTerminal(cfg, logger)
	}

	// 统一定时任务调度器（ui-cronjob-plan §5.4 + planv6.0 gocloudagent-cli-and-cronjob-unification-plan §2）：
	// 同时承载云函数与 Agent 两类目标，仅在系统库齐备时创建。Agent 执行器仅在
	// Cloud Agent 运行时可用时挂接；缺失时 agent 任务触发会落 failed 记录而非静默跳过。
	var cronScheduler *cronjob.Scheduler
	runtime := a.cloudAgentRuntime()
	if a.systemStore != nil {
		cronScheduler = cronjob.NewScheduler(a.systemStore, &systemDBRunner{runner: functionRunner})
		if runtime != nil {
			var quota agentbridge.CronAgentQuota
			if a.usageSvc != nil {
				quota = a.usageSvc
			}
			cronScheduler.WithAgentRunner(agentbridge.NewCronAgentExecutor(a.systemStore, runtime, quota))
		}
		a.cronScheduler = cronScheduler
		if a.schedulerCancel == nil {
			a.schedulerBaseCtx, a.schedulerCancel = context.WithCancel(context.Background())
		}
		cronScheduler.Start(a.schedulerBaseCtx)
	}

	var backupHandler *api.BackupHandler
	if cfg.Backup.Enabled && a.systemStore != nil && a.registry != nil && a.fileFactory != nil {
		repo := backup.NewRepository(a.systemStore.DB(), a.systemStore.NotifyWrite)
		var blobs objectstore.BlobStore
		if a.objectStore != nil {
			blobs, _ = a.objectStore.(objectstore.BlobStore)
		}
		service := backup.NewService(cfg.Backup, cfg.Instance.ID, a.catalog, a.registry, a.fileFactory, repo, blobs, objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix})
		backupHandler = api.NewBackupHandler(service, api.NewAuditService(a.auditSvc), cfg.Instance.Writable)
		if cfg.Backup.Schedule != "" {
			scheduler, err := backup.NewScheduler(service, repo, cfg.Backup.Schedule)
			if err != nil {
				return nil, fmt.Errorf("backup schedule: %w", err)
			}
			if a.schedulerCancel == nil {
				a.schedulerBaseCtx, a.schedulerCancel = context.WithCancel(context.Background())
			}
			scheduler.Start(a.schedulerBaseCtx)
		}
	}

	// KV TTL 后台清扫（key-value-ducklake-plan §4.5）：仅可写实例 + registry 齐备时创建。
	if cfg.Instance.Writable && a.registry != nil {
		kvSweeper := kv.NewSweeper(a.registry, 60*time.Second, dbbridge.LoggerFrom(logger))
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
		MetricsRegistry:        metricsGatherer(reg),
		Health:                 a.health,
		Auth:                   a.auth,
		Sessions:               a.sessions,
		Users:                  a.users,
		Catalog:                a.catalog,
		Registry:               dbbridge.RegistryFrom(a.registry),
		DatabaseHandler:        dbHandler,
		SQLHandler:             sqlHandler,
		DataHandler:            dataHandler,
		CollectionMetaHandler:  collectionMetaHandler,
		BackupHandler:          backupHandler,
		RealtimeHandler:        realtimeHandler,
		FunctionRunner:         functionRunner,
		FunctionTriggerHandler: functionTriggerHandler,
		TriggerEvents:          triggerEvents,
		SchemaHandler:          schemaHandler,
		KVHandler:              kvHandler,
		KVService:              kvService,
		KVInit:                 kvInit,
		Cache:                  api.NewCacheService(a.cacheMgr),
		Usage:                  api.NewUsageService(a.usageSvc),
		Audit:                  api.NewAuditService(a.auditSvc),
		LLM:                    api.NewLLMService(a.llmSvc),
		S3FileStore:            a.fileStore,
		System:                 a.systemStore,
		CloudAgent:             runtime,
		Delegations:            a.delegations,
		CronScheduler:          cronScheduler,
		Sandbox:                a.sandboxMgr,
		SandboxUsage:           a.usageSvc,
		Terminal:               terminal,
	}
	a.echo = api.NewRouter(deps)
	if terminal != nil {
		// 认证器需要 SandboxHandler（API Key 校验、项目解析），而 handler 在 NewRouter 里才构造，
		// 所以认证器在这里挂上：用同一份 deps 构造一个只做认证的轻量 handler。
		terminal.Server = a.bindSandboxSSHAuth(terminal, deps)
	}

	writeTimeout := cfg.HTTP.WriteTimeout
	if a.sandboxMgr != nil && a.sandboxMgr.Available() {
		// 同步执行写超时须覆盖最长命令及启动预算。
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

// assembleDeps 按顺序创建运行期依赖：对象存储（可选）→ DuckLake factory →
// 系统库 bootstrap/migrate/seed → catalog/auth/registry。
// storage.database / storage.files 决定持久层落在本地盘还是对象存储。
func (a *App) assembleDeps(ctx context.Context) error {
	cfg := a.cfg
	dbRemote := cfg.IsDBRemote()
	filesRemote := cfg.IsFilesRemote()

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

	// 存储身份变更检测：改 storage 模式 / bucket / prefix 都等价于换了一个
	// 完全不同的持久层，必须拒绝启动并提示先 reset。
	if err := checkStorageIdentity(cfg); err != nil {
		return err
	}

	if cfg.Instance.Writable && (dbRemote || filesRemote) && s3cfg.Bucket != "" {
		store, err := objectstore.NewClient(ctx, s3cfg, a.logger, a.metrics)
		if err != nil {
			return fmt.Errorf("create objectstore client: %w", err)
		}
		a.objectStore = store

		// P0-3 启动探针：create-if-absent 条件写可用性验证（§4.5）。
		// 探针失败 → 记 Fatal + 指标归零 → 禁用一切依赖互斥的能力，
		// /health/ready 暴露原因；不让代码以为自己有保护。
		if dbRemote {
			probePrefix := objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix}.CatalogPrefix()
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
		}
	}

	if err := a.assembleFileStore(ctx, cfg, s3cfg, filesRemote); err != nil {
		return err
	}

	keys := objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix}
	if !cfg.Instance.Writable {
		return nil
	}

	// ducklake-duckdb-catalog-plan §4.2：catalog 引擎启动校验（CAS 探针之后、
	// 系统库 Bootstrap 之前）。与已有数据不一致 / 历史数据无标记 → 拒绝启动。
	if err := a.checkCatalogEngine(ctx); err != nil {
		return err
	}

	userCacheDir := cfg.Database.CacheDir
	if fs, err := os.Stat(userCacheDir); err != nil || !fs.IsDir() {
		if err := os.MkdirAll(userCacheDir, 0o755); err != nil {
			return fmt.Errorf("create cache dir: %w", err)
		}
	}
	userFactory := a.newUserDatabaseFactory(userCacheDir)
	sysFactory := a.newSystemDatabaseFactory()

	sysName := cfg.SystemDatabase.Name
	if sysName == "" {
		sysName = systemdb.DefaultName
	}
	store, err := systemdb.Bootstrap(ctx, systemdb.BootstrapInput{
		// locator 记录系统库 UUID，必须与 data_dir 同寿命：
		// 放进 cache_dir 会在清缓存后重新生成一个空系统库。
		LocatorDir: filepath.Join(cfg.Storage.DataDir, "_system"),
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
	a.catalog.SetDefaultEngine(cfg.Database.Engine)
	if err := a.catalog.RepairDatabasesOnStartup(ctx); err != nil {
		return fmt.Errorf("repair database availability: %w", err)
	}

	// P1-1：per-database 写租约（multi-instance-consistency-plan §4.6）。
	// 仅 writable + storage.database=s3 + 探针通过时启用；失租 → 释放该库句柄。
	regOpts := registry.Options{
		IdleTimeout: cfg.Database.IdleTimeout,
		MaxOpen:     cfg.Database.MaxOpen,
		Writable:    cfg.Instance.Writable,
	}
	a.assembleWriteLease(ctx, cfg, regOpts)

	a.registry = registry.New(userFactory, dbbridge.ResolverFrom(a.catalog), regOpts, dbbridge.LoggerFrom(a.logger), dbbridge.ObserverFrom(a.metrics))

	a.assembleAuthServices(store, cfg)
	if err := a.seedAndStartMaintenance(ctx, store, cfg); err != nil {
		return err
	}

	a.cacheMgr, err = cache.NewManager(cache.Options{
		Root:         cfg.Database.CacheDir,
		DataRoot:     cfg.Storage.DataDir,
		MaxBytes:     cfg.Database.CacheMaxBytes,
		MaxDatabases: cfg.Database.CacheMaxDatabases,
		Registry:     a.registry,
		Closer:       a.registry,
		Logger:       dbbridge.LoggerFrom(a.logger),
		Observer:     dbbridge.ObserverFrom(a.metrics),
	})
	if err != nil {
		return fmt.Errorf("create cache manager: %w", err)
	}

	catRepo := a.catalog.Repository()
	a.usageSvc = usage.NewService(catRepo, a.logger)
	a.auditSvc = audit.NewService(catRepo, a.logger)

	if cfg.LLM.Enabled {
		a.llmSvc = a.assembleLLMService(cfg)
	}

	return nil
}

// assembleAuthServices 装配 auth/user/session 服务（原 assembleDeps 连续块，
// B4 提取）。SYNC-01：user/session 仓储直接写系统库表，绕过 Store.notifyWrite；
// 注入写后同步通知，防止改密/禁用是唯一一次写入时 catalog 漏发布。
func (a *App) assembleAuthServices(store *systemdb.Store, cfg config.Config) {
	a.auth = auth.NewService(store.AuthRepo(), cfg.Auth.APIKeyHashSecret).WithPolicyLoader(func(ctx context.Context, keyID, projectID string) (*auth.KeyPolicy, error) {
		return auth.LoadKeyPolicy(ctx, store.DB(), keyID, projectID)
	})
	a.users = auth.NewUserService(auth.NewSQLUserRepository(store.DB()), catalog.ReservedTenantID)
	a.users.OnWrite(store.NotifyWrite)
	a.sessions = auth.NewSessionService(a.users, auth.NewSQLSessionRepository(store.DB()),
		cfg.Auth.APIKeyHashSecret, auth.SessionConfig{
			AccessTTL:  2 * time.Hour,
			RefreshTTL: 7 * 24 * time.Hour,
		})
	a.sessions.OnWrite(store.NotifyWrite) // SYNC-01
}

// seedAndStartMaintenance 执行系统库 Seed、日志保留与周期刷新，并启动
// 系统库周期维护（原 assembleDeps 连续块，B4 提取）。种子先于后台维护启动；
// 错误包装与 closer 注册顺序保持原样。
func (a *App) seedAndStartMaintenance(ctx context.Context, store *systemdb.Store, cfg config.Config) error {
	if err := systemdb.Seed(ctx, systemdb.SeedInput{
		Store:   store,
		Auth:    a.auth,
		Catalog: a.catalog,
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
			_, err := a.leaseMgr.Acquire(ctx, sysMeta.ProjectID, sysMeta.ID, cfg.Instance.ID, a.onLostLease)
			return err == nil
		}
	}
	maint := systemdb.NewMaintenanceRunner(store, duckLakeOptions(cfg.Database.DuckLake).Maintenance, maintWritable, a.logger)
	maint.Start()
	a.registerCloser(maint)
	return nil
}

// assembleWriteLease 装配 per-database 写租约（原 assembleDeps「lease manager
// 与回调」连续块，B4 提取）。仅 writable + storage.database=s3 + 探针通过时启用；
// 失租回调与 WriteGate 的闭包捕获时机、注册 closer 顺序保持原样。
func (a *App) assembleWriteLease(ctx context.Context, cfg config.Config, regOpts registry.Options) {
	dbRemote := cfg.IsDBRemote()
	if cfg.Instance.Writable && dbRemote && cfg.Instance.Lease.Enabled && a.casSupported {
		if blobs, ok := a.objectStore.(objectstore.BlobStore); ok {
			keys := objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix}
			lm := lease.NewManager(blobs, keys.DB(), lease.Config{
				Enabled:       true,
				TTL:           cfg.Instance.Lease.TTL,
				RenewInterval: cfg.Instance.Lease.RenewInterval,
				Grace:         cfg.Instance.Lease.Grace,
			})
			lm.SetLogger(zapAdapter{a.logger})
			lm.EnableProjectScope()
			a.leaseMgr = lm
			a.leaseKeys = keys
			a.leaseGate = lease.NewGate()
			a.registerCloser(leaseCloser{m: lm})
			a.logger.Info("per-database write lease enabled",
				zap.String("ttl", cfg.Instance.Lease.TTL.String()),
				zap.String("on_lost", cfg.Instance.Lease.OnLost),
			)

			// 失租回调：停同步 PUT + 释放句柄（§4.6 onLost 顺序）。
			a.onLostLease = func(scope string) {
				a.logger.Error("write lease LOST: releasing database handle and disabling further writes",
					zap.String("database_id", scope))
				if a.metrics != nil && a.metrics.LeaseLost != nil {
					a.metrics.LeaseLost.WithLabelValues(scope).Inc()
				}
				if a.metrics != nil && a.metrics.LeaseState != nil {
					a.metrics.LeaseState.WithLabelValues(scope).Set(0)
				}
				// 同步器立即进入失租模式：后续 Flush/Close 不再 PUT（§4.6 第 1 步）。
				if cs, ok := a.catalogSyncer.(*ducklake.CatalogSyncer); ok {
					cs.CloseNoFlush(scope)
				}
				if fs, ok := a.fileSyncer.(*filedb.FileSyncer); ok {
					fs.CloseNoFlush(scope)
				}
				if strings.EqualFold(cfg.Instance.Lease.OnLost, "exit") {
					a.logger.Fatal("write lease lost and on_lost=exit: terminating process")
				}
				a.registry.Remove(scope)
				// perf §1.5：失租后清除本进程的写门记录，允许后续重新竞争租约。
				a.leaseGate.Clear(scope)
			}

			// WriteGate：ReadWrite Acquire 前必须持租（惰性首次获取）。
			// 失租（ValidFor=false 且已有租约记录）→ 拒绝新写。
			// perf §1.5：同库并发首写在 leaseGate 上按 dbID 串行化，
			// 只触发一次 Acquire，其余请求复用结果（修复并发首写 45s + 500）。
			regOpts.WriteGate = func(db godatabase.Spec) error {
				scope := lm.ScopeKey(db.ProjectID, db.ID)
				if lm.ValidFor(scope) {
					return nil
				}
				// 已有租约记录但未持租 = 失租后：拒绝。
				if lm.Has(scope) {
					return fmt.Errorf("instance: write lease lost for database %s; writes rejected until lease recovered", db.ID)
				}
				// 首次写：获取租约。他人持租 → 拒绝（该实例应降级只读）。
				if err := a.leaseGate.TryDo(scope, func() error {
					if lm.ValidFor(scope) {
						return nil
					}
					if lm.Has(scope) {
						return fmt.Errorf("instance: write lease lost for database %s; writes rejected until lease recovered", db.ID)
					}
					if _, err := lm.Acquire(ctx, db.ProjectID, scope, cfg.Instance.ID, a.onLostLease); err != nil {
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
						a.metrics.LeaseEpoch.WithLabelValues(db.ID).Set(float64(lm.EpochFor(scope)))
					}
				}
				if cs, ok := a.catalogSyncer.(*ducklake.CatalogSyncer); ok {
					cs.SetWriterEpoch(db.ID, lm.EpochFor(scope))
				}
				if fs, ok := a.fileSyncer.(*filedb.FileSyncer); ok {
					fs.SetWriterEpoch(db.ProjectID, lm.EpochFor(scope))
				}
				return nil
			}
		}
	} else if cfg.Instance.Writable && dbRemote && cfg.Instance.Lease.Enabled && !a.casSupported {
		a.logger.Warn("write lease requested but disabled: create-if-absent probe failed",
			zap.String("impact", "single-writer enforcement is NOT active; replicas must stay 1"))
	}
}

// assembleFileStore 装配用户文件存储（原 assembleDeps「文件存储选择」连续块，
// B4 提取）：按 storage.files 选择对象存储或本地目录。位于 writable 提前返回
// 之前——只读实例同样装配 fileStore。错误文本保持原样。
func (a *App) assembleFileStore(ctx context.Context, cfg config.Config, s3cfg objectstore.Config, filesRemote bool) error {
	switch {
	case filesRemote:
		if s3cfg.Bucket == "" {
			return fmt.Errorf("storage.files=s3 requires s3.bucket")
		}
		if a.objectStore == nil {
			store, err := objectstore.NewClient(ctx, s3cfg, a.logger, a.metrics)
			if err != nil {
				return fmt.Errorf("create objectstore client: %w", err)
			}
			a.objectStore = store
		}
		fs, err := objectstore.NewS3FileStore(ctx, s3cfg, cfg.S3.Prefix, a.logger)
		if err != nil {
			return fmt.Errorf("create s3 file store: %w", err)
		}
		a.fileStore = fs
	default:
		fs, err := objectstore.NewLocalFileStore(cfg.Storage.DataDir)
		if err != nil {
			return fmt.Errorf("create local file store: %w", err)
		}
		a.fileStore = fs
		a.logger.Info("user files stored on local disk",
			zap.String("data_dir", cfg.Storage.DataDir))
	}
	return nil
}

// assembleLLMService 装配 LLM Gateway 服务（原 assembleDeps 尾部
// cfg.LLM.Enabled 连续块，B4 提取；resolver 链构造与优先级保持原样：
// 项目动态凭据（sys_llm_settings）→ catalog CredentialRef → 实例 YAML
// 供应商回退）。仅读取已装配的 a.catalog / a.systemStore / a.usageSvc /
// a.logger，不重新创建任何依赖。
func (a *App) assembleLLMService(cfg config.Config) llmgateway.Service {
	// 设置页把模型连接（API Key / Base URL）写进 sys_llm_settings。每次请求现读该行，
	// 保存后无需重启。catalog CredentialRef 与 YAML 实例供应商仅作回退。
	var resolver llmgateway.ProviderResolver = llmgateway.NewCatalogResolver(a.catalog, nil)
	resolver = llmgateway.NewFirstUsableResolver(
		llmgateway.NewProjectCredResolver(llmgateway.NewSystemCredSource(a.systemStore)),
		resolver,
	)
	if inst := instanceLLMProviders(cfg.LLM.Providers); len(inst) > 0 {
		resolver = llmgateway.NewFallbackResolver(resolver, inst)
	}
	return llmgateway.NewService(resolver, usage.NewLLMRecorder(a.usageSvc), a.logger)
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

	// 停止后台调度循环；随后等待统一定时任务调度器的 in-flight 执行收敛（受 ctx 超时约束）。
	if a.schedulerCancel != nil {
		a.schedulerCancel()
	}
	// 停止定时任务调度器（云函数 + Agent 两类目标共用）。
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
	if a.triggerDispatcher != nil {
		done := make(chan struct{})
		go func() {
			a.triggerDispatcher.Close()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			a.logger.Warn("trigger dispatcher stop timed out")
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

// newUserDatabaseFactory 返回项目单文件用户库工厂（DuckDB / SQLite）。
// dbRemote 为真时使用本地工作副本 + 不可变快照同步；本地模式直接持久化。
func (a *App) newUserDatabaseFactory(cacheDir string) godatabase.Factory {
	cfg := a.cfg
	dbRemote := cfg.IsDBRemote()
	var syncer filedb.Syncer = filedb.NewNopSyncer()
	if dbRemote && cfg.S3.Bucket != "" && a.objectStore != nil {
		if blobs, ok := a.objectStore.(objectstore.BlobStore); ok {
			fs := filedb.NewFileSyncer(blobs, objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix}.DB(), cacheDir,
				cfg.Database.DuckLake.CatalogSync.Interval, cfg.Database.DuckLake.CatalogSync.KeepVersions, dbbridge.LoggerFrom(a.logger))
			fs.Start(context.Background())
			syncer = fs
			a.registerCloser(fs)
		}
	}
	f := &filedb.Factory{
		DataRoot: cfg.Storage.DataDir,
		CacheDir: cacheDir,
		Remote:   dbRemote,
		Options: filedb.Options{
			MemoryLimit: cfg.Database.DuckLake.MemoryLimit,
			Threads:     cfg.Database.DuckLake.Threads,
		},
		Syncer: syncer,
		Logger: dbbridge.LoggerFrom(a.logger),
	}
	a.fileFactory = f
	a.fileSyncer = syncer
	return f
}

// newSystemDatabaseFactory 返回系统库专用 DuckLake Factory；与用户库无共享可变状态。
// 系统库 catalog 归到一个独立的缓存子目录，避免与用户库混用。
func (a *App) newSystemDatabaseFactory() *ducklake.Factory {
	cfg := a.cfg
	opts := duckLakeOptions(cfg.Database.DuckLake)
	f := &ducklake.Factory{
		CacheDir: filepath.Join(cfg.Database.CacheDir, "system", "dbs"),
		DataRoot: cfg.Storage.DataDir,
		Options:  opts,
		Logger:   dbbridge.LoggerFrom(a.logger),
		Observer: dbbridge.ObserverFrom(a.metrics),
	}
	if cfg.IsDBRemote() && cfg.S3.Bucket != "" {
		remote := ducklake.RemoteStorage{
			Enabled:        true,
			Endpoint:       cfg.S3.Endpoint,
			Region:         cfg.S3.Region,
			Bucket:         cfg.S3.Bucket,
			RootPrefix:     cfg.S3.Prefix,
			AccessKey:      cfg.S3.AccessKey,
			SecretKey:      cfg.S3.SecretKey,
			ForcePathStyle: cfg.S3.ForcePathStyle,
		}
		var blobs objectstore.BlobStore
		if a.objectStore != nil {
			if b, ok := a.objectStore.(objectstore.BlobStore); ok {
				blobs = b
			}
		}
		if blobs != nil {
			cs := ducklake.NewCatalogSyncer(blobs, remote, f.CacheDir, opts.CatalogSync, opts.CatalogEngine, dbbridge.LoggerFrom(a.logger), dbbridge.ObserverFrom(a.metrics))
			f.Remote = remote
			f.Blobs = blobs
			f.Syncer = cs
			a.catalogSyncer = cs
			a.registerCloser(syncerCloser{cs: cs})
		}
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
			a.kvInit = api.NewKVInitializer(dbbridge.RegistryFrom(a.registry), a.logger)
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
func (a *App) cloudAgentRuntime() *gocloudagent.Runtime {
	if a.systemStore == nil {
		return nil
	}
	cliPath := strings.TrimSpace(a.cfg.Agent.CLIPath)
	if cliPath == "" {
		if exe, err := os.Executable(); err == nil {
			cliPath = exe
		}
	}
	if a.cfg.Agent.SkillsCLI && a.logger != nil {
		if failure := gocloudagent.ValidateCLIPath(cliPath); failure != "" {
			a.logger.Warn("cloud agent CLI unavailable", zap.String("reason", failure))
		}
	}
	reg := auth.NewDelegationRegistry()
	a.delegations = reg
	rt := &gocloudagent.Runtime{
		LLM:            api.NewCloudAgentLLM(api.NewLLMService(a.llmSvc)),
		DB:             api.NewCloudAgentDB(a.catalog, dbbridge.RegistryFrom(a.registry)),
		Obj:            api.NewCloudAgentObj(a.fileStore, a.systemStore),
		Logs:           agentbridge.NewLogAccess(a.systemStore),
		Settings:       agentbridge.NewSettingsAccess(a.systemStore),
		MaxIterations:  a.cfg.LLM.EffectiveAgentMaxIterations(),
		RunTimeout:     a.cfg.LLM.EffectiveAgentRunTimeout(),
		ToolProtocol:   a.cfg.LLM.EffectiveAgentToolProtocol(),
		SkillsCLI:      a.cfg.Agent.SkillsCLI,
		CLIPath:        cliPath,
		CLI:            gocloudagent.SubprocessCLI{Path: cliPath},
		APIBaseURL:     loopbackBase(a.cfg.HTTP.Address),
		ConfirmTimeout: a.cfg.Agent.EffectiveConfirmTimeout(),
		DelegationTTL:  a.cfg.Agent.EffectiveDelegationTTL(),
		Issuer:         agentbridge.NewDelegationIssuer(a.cfg.Auth.APIKeyHashSecret, reg),
		// 命令只读/破坏性分类的唯一入口：规则在宿主（sbcli + sqlguard）。
		Policy: agentbridge.NewCommandPolicy(),
		// planv6.0 v2 §5：运行日志出口与内容层开关（默认关闭）。
		Logger:   a.logger,
		TraceLog: a.cfg.Agent.TraceLog,
	}
	if a.sandboxMgr != nil && a.sandboxMgr.Available() {
		rt.Sandbox = &sandboxAdapter{m: a.sandboxMgr}
	}
	return rt
}

// sandboxAdapter 只适配 gocloudagent 的线程工具接口，业务状态统一由 Manager 管理。
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
func (s *sandboxAdapter) Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (gocloudagent.SandboxOutput, error) {
	out, err := s.m.ExecForThread(ctx, projectID, threadID, sandbox.ExecInput{Cmd: cmd, Args: args})
	return gocloudagent.SandboxOutput{Stdout: out.Stdout, Stderr: out.Stderr, ExitCode: out.ExitCode}, err
}
func (s *sandboxAdapter) Shell(ctx context.Context, projectID, threadID, command string) (gocloudagent.SandboxOutput, error) {
	out, err := s.m.ExecForThread(ctx, projectID, threadID, sandbox.ExecInput{Command: command})
	return gocloudagent.SandboxOutput{Stdout: out.Stdout, Stderr: out.Stderr, ExitCode: out.ExitCode}, err
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

// snapshotFromFileFactory 返回项目文件同步水位（databaseID 或 projectID 均可）。
func snapshotFromFileFactory(f *filedb.Factory, databaseID string) *api.DatabaseSnapshot {
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

// sandboxLocalOptions 把配置转成本机沙盒驱动的显式选项，并把服务自己的数据
// （data_dir、缓存目录、配置文件与 .env）列为对沙盒内命令不可读的路径。
func sandboxLocalOptions(cfg config.Config, log vm.LogFunc) local.Options {
	root := cfg.Sandbox.Root
	if root == "" {
		root = filepath.Join(cfg.Storage.DataDir, "sandboxes")
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	opts := local.Options{
		Root: root, Workdir: cfg.Sandbox.Workdir,
		MaxFileBytes: cfg.Sandbox.MaxFileBytes, MaxOutputBytes: cfg.Sandbox.MaxOutputBytes,
		PidsMax: cfg.Sandbox.PidsMax, BwrapPath: cfg.Sandbox.BwrapPath, HelperPath: cfg.Sandbox.HelperPath,
		Log: log,
	}
	opts.ProtectDirs = []string{cfg.Storage.DataDir, cfg.Database.CacheDir}
	cfgPath := os.Getenv("SIMPLEBASE_CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	opts.ProtectFiles = []string{cfgPath, ".env"}
	return opts
}

// sandboxDriverLog 把 gosandbox 的 vm.LogFunc 适配到 observability.Logger；logger 为 nil 时返回 nil（不输出）。
func sandboxDriverLog(l observability.Logger) vm.LogFunc {
	if l == nil {
		return nil
	}
	return func(level, msg string, kv ...any) {
		fields := make([]zap.Field, 0, len(kv)/2+1)
		for i := 0; i+1 < len(kv); i += 2 {
			key, ok := kv[i].(string)
			if !ok {
				continue
			}
			fields = append(fields, zap.Any(key, kv[i+1]))
		}
		switch level {
		case "debug":
			l.Debug(msg, fields...)
		case "warn":
			l.Warn(msg, fields...)
		case "error":
			l.Error(msg, fields...)
		default:
			l.Info(msg, fields...)
		}
	}
}

// zapAdapter 把 observability.Logger 适配为 lease.Logger。
type zapAdapter struct{ l observability.Logger }

func (a zapAdapter) Printf(format string, args ...any) {
	if a.l != nil {
		a.l.Info(fmt.Sprintf(format, args...))
	}
}

// setupSandboxTerminal 装配浏览器终端与（可选的）外部 SSH 端口。失败只关闭终端，不影响沙盒的其它能力。
//
// 返回的 TerminalBridge 里 Server 先是 nil：它的认证器要等 api.NewRouter 构造出 handler 后才能挂上
// （见 bindSandboxSSHAuth）。因此这里只准备主机密钥与指纹。
func (a *App) setupSandboxTerminal(cfg config.Config, logger observability.Logger) *api.TerminalBridge {
	keyPath := cfg.Sandbox.SSH.HostKeyPath
	if keyPath == "" {
		keyPath = filepath.Join(cfg.Storage.DataDir, "sandbox_ssh_host_ed25519")
	}
	signer, err := sshd.LoadOrCreateHostKey(keyPath)
	if err != nil {
		if logger != nil {
			logger.Warn("sandbox terminal disabled: host key unavailable", zap.Error(err))
		}
		return nil
	}
	a.sshHostKey = signer
	return &api.TerminalBridge{Fingerprint: sshd.Fingerprint(signer), SSHListen: strings.TrimSpace(cfg.Sandbox.SSH.Listen)}
}

// bindSandboxSSHAuth 构造 SSH 服务端并按需开放外部端口。返回服务端供 TerminalBridge 使用。
func (a *App) bindSandboxSSHAuth(t *api.TerminalBridge, deps api.Dependencies) *sshd.Server {
	h := api.NewSandboxHandler(deps.Sandbox, deps.Config.Instance.Writable, deps.Audit, deps.System, deps.SandboxUsage, deps)
	audit := func(ctx context.Context, id sshd.Identity, kind string, d time.Duration, exit int) {
		if a.auditSvc == nil {
			return
		}
		_ = a.auditSvc.Record(ctx, audit.Event{ProjectID: id.ProjectID, PrincipalID: id.Actor, Kind: "sandbox.terminal." + kind,
			Status: "ok", Detail: fmt.Sprintf("%s duration_ms=%d exit=%d", id.SandboxID, d.Milliseconds(), exit)})
	}
	srv, err := sshd.New(sshd.Options{HostKey: a.sshHostKey, Authenticator: api.NewSSHAuthenticator(h), Sessions: a.sandboxMgr, Audit: audit})
	if err != nil {
		if a.logger != nil {
			a.logger.Warn("sandbox terminal disabled", zap.Error(err))
		}
		return nil
	}
	a.registerCloser(closerFunc(srv.Close))
	if t.SSHListen != "" {
		l, lerr := net.Listen("tcp", t.SSHListen)
		if lerr != nil {
			a.logger.Warn("sandbox ssh listener failed; the browser terminal still works", zap.String("listen", t.SSHListen), zap.Error(lerr))
			t.SSHListen = ""
		} else {
			go func() { _ = srv.Serve(l) }()
			a.logger.Info("sandbox ssh listening", zap.String("listen", t.SSHListen), zap.String("host_key", t.Fingerprint))
		}
	}
	return srv
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }
