// 文件说明：写 handler 装配阶段：writeHandlers、KV 初始化器、调度上下文（原 app.go 拆出）
package app

import (
	"context"
	"github.com/linkxzhou/SimpleBase/internal/api/cataloghttp"
	"github.com/linkxzhou/SimpleBase/internal/api/datahttp"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
)

// writeHandlers 是可写实例的 SQL/数据/Schema/KV handler 组（只读实例全部为 nil）。
type writeHandlers struct {
	sql            *datahttp.SQLHandler
	data           *datahttp.DataHandler
	collectionMeta *datahttp.CollectionMetaHandler
	schema         *datahttp.SchemaHandler
	kvService      datahttp.KVService
	kv             *datahttp.KVHandler
}

// assembleWriteHandlers 构造可写实例的 handler 组，并为 dbHandler 注入 InitSQL。
// 调用方负责 Writable && registry != nil 判断；本方法不启动 goroutine、不注册 closer。
func (a *App) assembleWriteHandlers(cfg config.Config, dbHandler *datahttp.DatabaseHandler, events *eventFanout) writeHandlers {
	var w writeHandlers
	sqlLimits := datahttp.SQLLimits{
		QueryTimeout:       int64(cfg.Limits.QueryTimeout),
		MaxQueryRows:       cfg.Limits.MaxQueryRows,
		MaxConcurrent:      cfg.Limits.MaxConcurrentQueries,
		MaxBatchStatements: cfg.Limits.MaxBatchStatements,
		MaxSQLBytes:        cfg.Limits.MaxSQLBytes,
		MaxRequestBytes:    cfg.Limits.MaxRequestBytes,
	}
	sqlService := datahttp.NewSQLServiceAdapter(a.catalog, dbbridge.RegistryFrom(a.registry), a.systemStore)
	w.sql = datahttp.NewSQLHandler(sqlService, sqlLimits, cfg.Instance.Writable)
	if a.fileFactory != nil {
		f := a.fileFactory
		w.sql.DurabilityFor = f.DurabilityFor
	}
	w.data = datahttp.NewDataHandler(sqlService.(datahttp.DataService), cfg.Instance.Writable).WithEvents(events).WithImportLimit(cfg.Limits.MaxImportBytes).WithAggregateLimits(cfg.Limits.QueryTimeout, 0)
	if a.systemStore != nil {
		schemas := collection.NewRepository(a.systemStore.DB(), a.systemStore.NotifyWrite)
		w.data.WithCollections(schemas, cfg.Limits.MaxFilterScanRows)
		w.data.WithAccessRules(
			func(ctx context.Context, projectID, key string) (string, error) {
				kv, err := a.systemStore.GetProjectSetting(ctx, projectID, key)
				if err != nil {
					return "", err
				}
				return kv.ValueJSON, nil
			},
			func(ctx context.Context, projectID, key, valueJSON string) error {
				return a.systemStore.PutProjectSetting(ctx, projectID, key, valueJSON)
			},
		)
		if a.auditSvc != nil {
			w.data.WithAudit(a.auditSvc)
		}
		w.collectionMeta = datahttp.NewCollectionMetaHandler(sqlService.(datahttp.DataService), schemas, cfg.Instance.Writable, cfg.Limits.MaxFilterScanRows)
	}
	w.schema = datahttp.NewSchemaHandler(sqlService.(datahttp.DataService), cfg.Instance.Writable, sqlLimits).WithAggregateLimits(cfg.Limits.QueryTimeout, 0)
	dbHandler.InitSQL = sqlService
	// 项目级 KV（key-value-ducklake-plan §2/§3）：catalog + registry 桥接。
	w.kvService = datahttp.NewKVServiceAdapter(a.catalog, dbbridge.RegistryFrom(a.registry))
	w.kv = datahttp.NewKVHandler(w.kvService, cfg.Instance.Writable)
	return w
}

func (a *App) ensureSchedulerCtx() context.Context {
	if a.schedulerCancel == nil {
		a.schedulerBaseCtx, a.schedulerCancel = context.WithCancel(context.Background())
	}
	return a.schedulerBaseCtx
}

// kvInitializer 惰性构造共享的 KV 系统表初始化器（key-value-ducklake-plan §2）；
// 只读实例或 registry 未装配时返回 nil（不初始化）。
func (a *App) kvInitializer() cataloghttp.KVInitializer {
	a.kvInitOnce.Do(func() {
		if a.cfg.Instance.Writable && a.registry != nil {
			a.kvInit = datahttp.NewKVInitializer(dbbridge.RegistryFrom(a.registry), a.logger)
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
