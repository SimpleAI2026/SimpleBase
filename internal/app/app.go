// 文件说明：app.go — 装配 SimpleBase 单实例服务的全部依赖并管理生命周期
//
// 用法
//
//	New(ctx context.Context, cfg config.Config) (*App, error)
//	(a *App) Close(ctx context.Context) error  // Close 反向关闭所有已注册资源
//	(a *App) Start() error  // Start 启动 HTTP server
//	同包调用：New() → cron_runner.go, reset.go；App → catalog_engine.go；ReadFile() → identity.go
//	装配阶段：assembleDeps 内部按序调用 assemble_{storage,handlers,sandbox,backup,auth,agent,api}.go
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
//	(a *App) assembleDeps(ctx context.Context) error
//	(a *App) Shutdown(ctx context.Context) error
//	(a *App) RunWithSignal(shutdownTimeout time.Duration) error

package app

import (
	"context"
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
	"github.com/linkxzhou/SimpleBase/godatabase/cache"
	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/kv"
	"github.com/linkxzhou/SimpleBase/godatabase/lease"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/gosandbox/image"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/alerts"
	"github.com/linkxzhou/SimpleBase/internal/api"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
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
	// clock 记录启动阶段耗时（startup.go）；nil 时 step 为空操作。
	clock *startupClock

	httpServer    *http.Server
	metricsServer *http.Server // §8.1.7 独立观测监听（nil = 未启用）
	echo          *echo.Echo

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
	// endUsers 是终端用户服务（§8.2.6；独立密钥域 aud=enduser）。
	endUsers   *auth.EndUserService
	cacheMgr   *cache.Manager
	usageSvc   *usage.Service
	auditSvc   *audit.Service
	llmSvc     llmgateway.Service
	sandboxMgr *sandbox.Manager
	// imageStore 是沙盒镜像的 store；未配置 sandbox.images_dir 时为 nil。
	imageStore *image.Store
	// sshHostKey 是沙盒终端的 SSH 主机密钥（setupSandboxTerminal 装配）。
	sshHostKey ssh.Signer

	// 多实例一致性（multi-instance-consistency-plan Phase A/B）。
	casSupported bool           // 启动探针结果；false 时禁用租约/manifest 互斥
	leaseMgr     *lease.Manager // per-database 写租约（nil = 未启用）
	leaseKeys    objectstore.KeyBuilder
	leaseGate    *lease.Gate       // 同库并发首写串行化（perf §1.5）
	onLostLease  func(dbID string) // 失租回调（租约获取处闭包捕获）

	delegations   *auth.DelegationRegistry
	cronScheduler *cronjob.Scheduler
	// agentRun 是 Agent 协作应用服务：生命周期归 App（Shutdown 第 0 步取消、第 5 步等收尾）。
	agentRun          *agentrun.Service
	triggerDispatcher *trigger.Dispatcher
	kvSweeper         *kv.Sweeper
	handleEvictor     *registry.Evictor // R1：周期回收空闲句柄
	schedulerCancel   context.CancelFunc
	schedulerBaseCtx  context.Context
	// alertBus 是进程内告警总线（§8.3.5）；nil = 未装配（测试路径）。
	alertBus *alerts.Bus
	// backupStoreReader 提供最近备份时刻（§8.3.6 readiness「48h 内有成功备份」）；nil = 不检查。
	backupStoreReader interface {
		LastBackupAt(ctx context.Context) (time.Time, error)
	}

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
	// §8.3.6：data_dir 磁盘水位低于 5% 时 readiness 置 503（强制 LB 摘除）。
	if cfg.Storage.DataDir != "" {
		ratio, err := alerts.DiskFreeRatio(cfg.Storage.DataDir)
		if err == nil && ratio < 0.05 {
			return fmt.Errorf("disk almost full: %.1f%% free on %s", ratio*100, cfg.Storage.DataDir)
		}
	}
	// §8.3.6：production 且启用备份的 writable 实例，检查「上次成功备份 < 48h」。
	if cfg.IsProduction() && cfg.Backup.Enabled && cfg.Instance.Writable && h.app.systemStore != nil {
		if h.app.backupStoreReader != nil {
			if last, err := h.app.backupStoreReader.LastBackupAt(ctx); err == nil && !last.IsZero() && time.Since(last) > 48*time.Hour {
				return fmt.Errorf("last successful backup older than 48h (%s)", last.Format(time.RFC3339))
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
	logStartupBanner(logger, cfg)
	redacted := cfg.Redacted()
	logger.Info("config", zap.Any("config", redacted))
	// planv6.5 §9：控制台与用户站点同注册域时提示同站风险。
	if w := config.SitesSameSiteWarning(cfg); w != "" {
		logger.Warn("sites same-site risk", zap.String("detail", w))
	}
	logSitesMode(logger, cfg)

	metrics := observability.NewMetrics(reg)

	a := &App{
		cfg:     cfg,
		logger:  logger,
		metrics: metrics,
		clock:   newStartupClock(logger),
		health:  nil,
	}

	// 装配依赖（顺序见文件头注释）。任一失败时反向关闭已创建资源。
	if err := a.assembleDeps(ctx); err != nil {
		// 启动失败必须留下明确一行：容器里只看得到最后一条错误时才知道断在哪一步。
		logger.Error("startup failed during dependency assembly", zap.Error(err))
		_ = a.Close(context.Background())
		return nil, err
	}
	a.step("deps assembled",
		zap.Bool("object_store", a.objectStore != nil),
		zap.Bool("file_store", a.fileStore != nil),
		zap.Bool("system_store", a.systemStore != nil),
		zap.Bool("registry", a.registry != nil),
		zap.Bool("cron_scheduler", a.cronScheduler != nil),
	)
	a.health = &healthService{app: a}

	var h apiHandlers
	h.database, h.kvInit = a.assembleDatabaseHandler(cfg)
	h.realtime = api.NewRealtimeHandler(cfg.Limits.RealtimeMaxConns)
	// 函数面（执行器 + 触发器队列）先于数据写 handler：领域事件的订阅方在构造 fan-out 时
	// 一次性登记（internal-ddd-hexagonal-implementation-plan §4、决策 D5）——记录事件先实时、后触发器，
	// 对象事件只给触发器；之后不再有可变的 Add()。
	h.functionRunner, h.functionTrigger = a.assembleFunctionPlane(cfg)
	trig := newTriggerEventPublisher(a.triggerDispatcher)
	recordSubs := []recordEventPublisher{h.realtime}
	var objectSubs []objectEventPublisher
	if trig != nil {
		recordSubs = append(recordSubs, trig)
		objectSubs = append(objectSubs, trig)
	}
	h.events = newEventFanout(recordSubs, objectSubs)

	// Plan 6：SQL handler。readonly 实例 SQLHandler 为 nil，路由不挂载写操作。
	if cfg.Instance.Writable && a.registry != nil {
		w := a.assembleWriteHandlers(cfg, h.database, h.events)
		h.sql, h.data = w.sql, w.data
		h.collectionMeta, h.schema = w.collectionMeta, w.schema
		h.kvService, h.kv = w.kvService, w.kv
	}

	// 独立云沙盒模块：在 Agent 装配前创建，同一 Manager 同时供 Agent 与 API 使用。
	h.terminal = a.assembleSandbox(ctx, cfg, logger)
	a.step("sandbox assembled",
		zap.Bool("sandbox_enabled", cfg.Sandbox.Enabled),
		zap.Bool("sandbox_available", a.sandboxMgr != nil && a.sandboxMgr.Available()),
		zap.Bool("terminal", h.terminal != nil),
	)

	// 统一定时任务调度器同时承载云函数与 Agent；群定时执行器先于 cron 执行器创建。
	// Agent 协作：确认表先于运行时构造并注入（router 不再回写 Runtime.Confirm）；
	// 应用服务被 cron（群定时）、HTTP（run / 轮次）与关停流程共用（internal-ddd-hexagonal-implementation-plan §3.4）。
	confirms := agentrun.NewConfirmHub()
	h.runtime = a.cloudAgentRuntime(confirms)
	a.agentRun = a.assembleAgentRun(h.runtime, confirms)
	h.agentRun = a.agentRun
	h.cronScheduler = a.assembleScheduler(h.functionRunner, h.runtime, h.agentRun)
	a.step("cron scheduler started",
		zap.Bool("enabled", h.cronScheduler != nil),
		zap.Bool("cloud_agent_runtime", h.runtime != nil),
	)

	backupHandler, err := a.assembleBackup(cfg)
	if err != nil {
		// 此前已打开系统库/registry 并启动 reaper、cron 等协程，失败时同样反向关闭。
		logger.Error("startup failed during backup assembly", zap.Error(err))
		_ = a.Close(context.Background())
		return nil, err
	}
	h.backup = backupHandler
	a.step("backup assembled",
		zap.Bool("backup_enabled", cfg.Backup.Enabled),
		zap.Bool("handler", backupHandler != nil),
		zap.String("schedule", cfg.Backup.Schedule),
	)
	a.startKVSweeper(cfg)

	// 启动收敛：上一进程残留的 running 轮次落 interrupted；先于 HTTP 监听（Start 在 New 之后）。
	h.agentRun.Converge(ctx)

	deps := a.buildAPIDeps(cfg, reg, h)
	a.buildHTTPServers(cfg, deps)

	logger.Info("simplebase initialized",
		zap.String("http_address", cfg.HTTP.Address),
		zap.String("metrics_address", cfg.Observability.MetricsAddress),
		zap.Int64("startup_ms", a.elapsedMs()),
	)
	observability.Sync(logger)
	return a, nil
}

// elapsedMs 返回自启动计时器创建以来的毫秒数；未初始化计时器时返回 0。
func (a *App) elapsedMs() int64 {
	if a == nil || a.clock == nil {
		return 0
	}
	return time.Since(a.clock.start).Milliseconds()
}

// assembleDeps 按顺序创建运行期依赖：对象存储（可选）→ DuckLake factory →
// 系统库 bootstrap/migrate/seed → catalog/auth/registry。
// storage.database / storage.files 决定持久层落在本地盘还是对象存储。
func (a *App) assembleDeps(ctx context.Context) error {
	cfg := a.cfg
	dbRemote := cfg.IsDBRemote()
	filesRemote := cfg.IsFilesRemote()

	// 启动第一段：把「数据落在哪」写成一行，容器里最常见的问题就是路径/桶不对。
	a.logger.Info("resolving persistence",
		zap.String("storage_database", cfg.Storage.Database),
		zap.String("storage_files", cfg.Storage.Files),
		zap.String("data_dir", cfg.Storage.DataDir),
		zap.String("cache_dir", cfg.Database.CacheDir),
		zap.Bool("database_remote", dbRemote),
		zap.Bool("files_remote", filesRemote),
		zap.String("s3_endpoint", cfg.S3.Endpoint),
		zap.String("s3_region", cfg.S3.Region),
		zap.String("s3_bucket", cfg.S3.Bucket),
		zap.String("s3_prefix", cfg.S3.Prefix),
	)

	s3cfg := s3ConfigFrom(cfg)

	// 存储身份变更检测：改 storage 模式 / bucket / prefix 都等价于换了一个
	// 完全不同的持久层，必须拒绝启动并提示先 reset。
	if err := checkStorageIdentity(cfg); err != nil {
		return err
	}
	a.step("storage identity checked")
	if err := a.connectObjectStore(ctx, cfg, s3cfg, dbRemote, filesRemote); err != nil {
		return err
	}
	if err := a.assembleFileStore(ctx, cfg, s3cfg, filesRemote); err != nil {
		return err
	}
	a.step("file store ready", zap.Bool("files_remote", filesRemote))

	keys := objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix}
	if !cfg.Instance.Writable {
		// 只读实例到此为止：写路径（系统库/registry/调度器）全部不装配。
		a.logger.Info("readonly instance: skipping system database, registry and schedulers")
		return nil
	}

	// ducklake-duckdb-catalog-plan §4.2：catalog 引擎启动校验（CAS 探针之后、
	// 系统库 Bootstrap 之前）。与已有数据不一致 / 历史数据无标记 → 拒绝启动。
	if err := a.checkCatalogEngine(ctx); err != nil {
		return err
	}

	userCacheDir := cfg.Database.CacheDir
	if err := os.MkdirAll(userCacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	userFactory := a.newUserDatabaseFactory(userCacheDir)
	sysFactory := a.newSystemDatabaseFactory()
	a.step("database factories ready", zap.String("engine", cfg.Database.Engine))

	store, err := a.bootSystemDatabase(ctx, cfg, keys, sysFactory)
	if err != nil {
		return err
	}
	a.systemStore = store
	a.registerCloser(store)

	if err := a.assembleRegistryAndCache(ctx, cfg, userFactory); err != nil {
		return err
	}

	a.assembleAuthServices(store, cfg)
	a.step("auth services ready")
	if err := a.seedAndStartMaintenance(ctx, store, cfg); err != nil {
		return err
	}
	a.step("system seed + maintenance started")

	a.assembleUsageServices(cfg)
	return nil
}

func (a *App) assembleUsageServices(cfg config.Config) {
	catRepo := a.catalog.Repository()
	a.usageSvc = usage.NewService(catRepo, a.logger)
	a.auditSvc = audit.NewService(catRepo, a.logger)
	if cfg.LLM.Enabled {
		a.llmSvc = a.assembleLLMService(cfg)
	}
	a.step("usage/audit/llm ready",
		zap.Bool("llm_enabled", cfg.LLM.Enabled),
		zap.Int("llm_providers", len(cfg.LLM.Providers)),
	)
}

// Start 启动 HTTP server（阻塞直到关闭）；配置了独立观测监听时并行启动，
// 监听失败视为启动失败（§8.1.7）。
func (a *App) Start() error {
	a.logger.Info("starting http server",
		zap.String("address", a.cfg.HTTP.Address),
		zap.String("profile", a.cfg.ProfileName()),
		zap.Int64("startup_ms", a.elapsedMs()),
	)
	if a.metricsServer != nil {
		metricsErr := make(chan error, 1)
		go func() {
			a.logger.Info("metrics server listening", zap.String("address", a.cfg.Observability.MetricsAddress))
			if err := a.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				metricsErr <- err
				return
			}
			metricsErr <- nil
		}()
		// 等待首个结果：独立监听绑定失败必须让主服务启动失败。
		select {
		case err := <-metricsErr:
			if err != nil {
				_ = a.httpServer.Close()
				a.logger.Error("metrics server listen failed",
					zap.String("address", a.cfg.Observability.MetricsAddress), zap.Error(err))
				return fmt.Errorf("metrics server listen %s: %w", a.cfg.Observability.MetricsAddress, err)
			}
		default:
		}
		// 异步失败（运行期掉线）也透传给阻塞调用方。
		go func() {
			if err := <-metricsErr; err != nil {
				a.logger.Error("metrics server failed", zap.String("err", err.Error()))
				_ = a.httpServer.Close()
			}
		}()
	}
	a.logger.Info("http server listening", zap.String("address", a.cfg.HTTP.Address))
	err := a.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		// 端口被占用 / 无权限绑定是最常见的容器启动失败原因，单独记一行。
		a.logger.Error("http server stopped with error",
			zap.String("address", a.cfg.HTTP.Address), zap.Error(err))
		return err
	}
	a.logger.Info("http server stopped")
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

// agentShutdownGrace 是关停时等待 Agent run / 群轮次收尾落库的上限（计划 §3.4：常量，不新增配置项）。
// 收尾只是几条系统库 UPDATE，10s 足够；超时记告警并列出残留轮次，不阻塞后续关停。
const agentShutdownGrace = 10 * time.Second

// Shutdown 优雅关闭：Agent run 开始收尾 → HTTP server → 观测监听 → 调度器 → 触发器 →
// 等 Agent 收尾落库 → KV 清扫器 → 句柄回收器 → registry / catalog / LLM。超时由 ctx 控制。
func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("shutdown started")
	var firstErr error

	// 第 0 步（D6-a）：先让在途 Agent run / 群轮次开始收尾（非阻塞），再关 HTTP ——
	// 否则 httpServer.Shutdown 会被在途 LLM 调用拖满整个预算，后面的收尾等待拿不到时间。
	if a.agentRun != nil {
		a.agentRun.BeginShutdown()
	}
	if err := a.httpServer.Shutdown(ctx); err != nil {
		a.logger.Error("http shutdown error", zap.String("err", err.Error()))
		if firstErr == nil {
			firstErr = err
		}
	}
	// §8.1.7：关闭独立观测监听。
	if a.metricsServer != nil {
		if err := a.metricsServer.Shutdown(ctx); err != nil {
			a.logger.Error("metrics shutdown error", zap.String("err", err.Error()))
			if firstErr == nil {
				firstErr = err
			}
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

	// 第 5 步（D6-a）：等 Agent run / 群轮次的收尾写库完成。用独立于主预算的收尾预算：
	// 第 1 步可能被长期 SSE 占满，不能让「等收尾」因此拿不到时间；超时只告警、列出残留，不阻塞后续关停。
	if a.agentRun != nil {
		graceCtx, cancelGrace := context.WithTimeout(context.Background(), agentShutdownGrace)
		if err := a.agentRun.Wait(graceCtx); err != nil {
			a.logger.Warn("agent runs did not settle before deadline",
				zap.String("err", err.Error()), zap.Any("active_turns", a.agentRun.ActiveTurns()))
		}
		cancelGrace()
	}

	// 停止 KV TTL 清扫器（schedulerCancel 已停其循环，此处等待退出）。
	if a.kvSweeper != nil {
		if err := a.kvSweeper.Close(); err != nil {
			a.logger.Warn("kv sweeper close error", zap.String("err", err.Error()))
		}
	}

	// R1：先停句柄回收器，避免与 registry.Shutdown 竞争关闭同一句柄。
	if a.handleEvictor != nil {
		a.handleEvictor.Stop()
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
	if firstErr != nil {
		a.logger.Warn("shutdown finished with errors", zap.Error(firstErr))
	} else {
		a.logger.Info("shutdown complete")
	}
	observability.Sync(a.logger)
	return firstErr
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
	case sig := <-sigCh:
		// 容器里 SIGTERM 是常态（docker stop / 滚动重启），记一行便于对齐时间线。
		a.logger.Info("signal received, shutting down",
			zap.String("signal", sig.String()),
			zap.String("timeout", shutdownTimeout.String()),
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return a.Shutdown(ctx)
}
