// 文件说明：assemble_api.go — 数据库、函数面、调度器、API 依赖与 HTTP server 装配
//
// 用法
//
//	NewWithRegistry 按依赖顺序调用本文件的装配函数；各函数不处理跨阶段失败回收，
//	备份装配失败仍由 NewWithRegistry 统一调用 App.Close。
//
// 调用架构
//
//	NewWithRegistry（app.go）
//	  → assembleDatabaseHandler → assembleFunctionPlane → assembleScheduler
//	  → buildAPIDeps → buildHTTPServers
//
// 主要符号
//
//	type apiHandlers struct {…}
//	func (a *App) assembleDatabaseHandler(...)
//	func (a *App) assembleFunctionPlane(...)
//	func (a *App) assembleScheduler(...)
//	func (a *App) buildAPIDeps(...)
//	func (a *App) buildHTTPServers(...)
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/godatabase/kv"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/api"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"github.com/linkxzhou/SimpleBase/internal/functionrun"
	"github.com/linkxzhou/SimpleBase/internal/trigger"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type apiHandlers struct {
	database        *api.DatabaseHandler
	sql             *api.SQLHandler
	data            *api.DataHandler
	collectionMeta  *api.CollectionMetaHandler
	backup          *api.BackupHandler
	realtime        *api.RealtimeHandler
	functionRunner  *functionrun.Runner
	functionTrigger *api.FunctionTriggerHandler
	events          *eventFanout
	schema          *api.SchemaHandler
	kv              *api.KVHandler
	kvService       api.KVService
	kvInit          api.KVInitializer
	terminal        *api.TerminalBridge
	runtime         *cloudagent.Runtime
	agentRun        *agentrun.Service
	cronScheduler   *cronjob.Scheduler
}

func (a *App) assembleDatabaseHandler(cfg config.Config) (*api.DatabaseHandler, api.KVInitializer) {
	// 删库清空用户数据（保留 KV / 项目文件 / 备份），并同步清理集合元数据。
	var purgerSchemas collectionCleaner
	if a.systemStore != nil {
		purgerSchemas = collection.NewRepository(a.systemStore.DB(), a.systemStore.NotifyWrite)
	}
	dbHandler := api.NewDatabaseHandler(
		api.NewDatabaseServiceAdapter(a.catalog, dbbridge.RegistryFrom(a.registry), newProjectPurger(a.fileFactory, a.fileSyncer, a.registry, purgerSchemas)),
		cfg.Instance.Writable,
	)
	kvInit := a.kvInitializer()
	if a.fileFactory != nil {
		f := a.fileFactory
		dbHandler.SnapshotFor = func(databaseID string) *api.DatabaseSnapshot {
			return snapshotFromFileFactory(f, databaseID)
		}
	}
	// 数据库列表「数据量」列使用缓存 + 后台有界并发刷新，避免逐库同步 COUNT(*)。
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
		Logger:    a.logger,
	})
	return dbHandler, kvInit
}

// assembleFunctionPlane 构造云函数执行器与触发器队列。不向 fan-out 登记订阅方：
// 领域事件的接线由 NewWithRegistry 在本函数返回后一次性完成（app.go、event_fanout.go）。
func (a *App) assembleFunctionPlane(cfg config.Config) (*functionrun.Runner, *api.FunctionTriggerHandler) {
	if a.systemStore == nil {
		return nil, nil
	}
	functionRunner := functionrun.NewRunner(a.systemStore, cfg.Limits.MaxConcurrentGoRuns)
	a.triggerDispatcher = trigger.NewDispatcher(a.systemStore, functionRunner, 1024, 4)
	functionTriggerHandler := api.NewFunctionTriggerHandler(a.systemStore, a.triggerDispatcher, cfg.Instance.Writable)
	return functionRunner, functionTriggerHandler
}

// assembleScheduler 构造统一定时任务调度器（云函数 + Agent 两类目标）。群定时执行器就是
// Agent 协作应用服务本身（实现 agentbridge.GroupCronRunner），在调度器之前已构造，不再延迟绑定。
func (a *App) assembleScheduler(runner *functionrun.Runner, runtime *cloudagent.Runtime, agentRun *agentrun.Service) *cronjob.Scheduler {
	if a.systemStore == nil {
		return nil
	}
	cronScheduler := cronjob.NewScheduler(a.systemStore, &systemDBRunner{runner: runner})
	if runtime != nil && agentRun != nil {
		var quota agentbridge.CronAgentQuota
		if a.usageSvc != nil {
			quota = a.usageSvc
		}
		// cron 触发的助手 run 由应用服务统一收尾（RunFinalizer），群定时也直接调用同一个服务。
		cronScheduler.WithAgentRunner(agentbridge.NewCronAgentExecutor(a.systemStore, runtime, quota, agentRun).WithGroupRunner(agentRun))
	}
	a.cronScheduler = cronScheduler
	cronScheduler.Start(a.ensureSchedulerCtx())
	return cronScheduler
}

func (a *App) startKVSweeper(cfg config.Config) {
	if cfg.Instance.Writable && a.registry != nil {
		kvSweeper := kv.NewSweeper(a.registry, 60*time.Second, dbbridge.LoggerFrom(a.logger))
		a.kvSweeper = kvSweeper
		kvSweeper.Start(a.ensureSchedulerCtx())
	}
}

func (a *App) buildAPIDeps(cfg config.Config, reg prometheus.Registerer, h apiHandlers) api.Dependencies {
	return api.Dependencies{
		Config:                 cfg,
		Logger:                 a.logger,
		Metrics:                a.metrics,
		MetricsRegistry:        metricsGatherer(reg),
		Health:                 a.health,
		Auth:                   a.auth,
		Sessions:               a.sessions,
		EndUsers:               a.endUsers,
		Users:                  a.users,
		Catalog:                a.catalog,
		Registry:               dbbridge.RegistryFrom(a.registry),
		DatabaseHandler:        h.database,
		SQLHandler:             h.sql,
		DataHandler:            h.data,
		CollectionMetaHandler:  h.collectionMeta,
		BackupHandler:          h.backup,
		RealtimeHandler:        h.realtime,
		FunctionRunner:         h.functionRunner,
		FunctionTriggerHandler: h.functionTrigger,
		TriggerEvents:          h.events,
		SchemaHandler:          h.schema,
		KVHandler:              h.kv,
		KVService:              h.kvService,
		KVInit:                 h.kvInit,
		Cache:                  api.NewCacheService(a.cacheMgr),
		Usage:                  api.NewUsageService(a.usageSvc),
		Audit:                  api.NewAuditService(a.auditSvc),
		LLM:                    api.NewLLMService(a.llmSvc),
		S3FileStore:            a.fileStore,
		System:                 a.systemStore,
		CloudAgent:             h.runtime,
		AgentRun:               h.agentRun,
		Delegations:            a.delegations,
		CronScheduler:          h.cronScheduler,
		Sandbox:                a.sandboxMgr,
		Images:                 a.imageStore,
		SandboxUsage:           a.usageSvc,
		Terminal:               h.terminal,
	}
}

func (a *App) buildHTTPServers(cfg config.Config, deps api.Dependencies) {
	// SSH 认证器在路由树之前构造并挂到 TerminalBridge（internal-ddd-hexagonal-implementation-plan §5.2）：
	// app 是唯一组装点，NewRouter 之后不再回填任何依赖；浏览器终端与 SSH 认证经桥共享同一份票据存储。
	if deps.Terminal != nil {
		deps.Terminal.Server = a.bindSandboxSSHAuth(deps.Terminal, deps)
	}
	a.echo = api.NewRouter(deps)
	a.step("router assembled", zap.Int("routes", len(a.echo.Routes())))

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
	if cfg.Observability.MetricsAddress != "" {
		a.metricsServer = &http.Server{
			Addr:    cfg.Observability.MetricsAddress,
			Handler: api.NewObservabilityHandler(deps),
		}
	}
}
