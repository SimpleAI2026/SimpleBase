// 文件说明：dbbridge.go — SimpleBase 宿主与数据库运行时库（godatabase）之间的适配层
//
// 用法
//
//	SpecOf(db catalog.Database) godatabase.Spec  // SpecOf 把 catalog.Database 转换为库的 Spec
//	CatalogOf(s godatabase.Spec) catalog.Database  // CatalogOf 是 SpecOf 的反向，仅用于测试与过渡：缺失的业务字段保持零值
//
// 调用架构
//
//	[上游] internal/app, internal/backup, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/dbbridge/dbbridge.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/registry, internal/catalog, internal/observability
//
// 主要符号
//
//	SpecOf(db catalog.Database) godatabase.Spec
//	CatalogOf(s godatabase.Spec) catalog.Database
//	IsSystem(s godatabase.Spec) bool
//	ResolverFrom(svc *catalog.Service) godatabase.PathResolver
//	LoggerFrom(l observability.Logger) godatabase.Logger
//	ObserverFrom(m *observability.Metrics) godatabase.Observer
//	... 另有 17 个顶层符号

package dbbridge

import (
	"context"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// SpecOf 把 catalog.Database 转换为库的 Spec。
// 只复制库读取的字段；Kind 与 Status 的取值一一对应，不做任何改写。
func SpecOf(db catalog.Database) godatabase.Spec {
	return godatabase.Spec{
		ID:           db.ID,
		TenantID:     db.TenantID,
		ProjectID:    db.ProjectID,
		Kind:         db.Kind,
		Status:       godatabase.Status(db.Status),
		DatabasePath: db.DatabasePath,
	}
}

// CatalogOf 是 SpecOf 的反向，仅用于测试与过渡：缺失的业务字段保持零值。
func CatalogOf(s godatabase.Spec) catalog.Database {
	return catalog.Database{
		ID:           s.ID,
		TenantID:     s.TenantID,
		ProjectID:    s.ProjectID,
		Kind:         s.Kind,
		Status:       catalog.DatabaseStatus(s.Status),
		DatabasePath: s.DatabasePath,
	}
}

// IsSystem 保持与 catalog.IsSystemDatabase 的判定一致。
func IsSystem(s godatabase.Spec) bool { return s.IsSystem() }

// resolver 用 catalog.Service 补全 DatabasePath；租户归属校验仍由 catalog 完成。
type resolver struct{ svc *catalog.Service }

// ResolverFrom 返回基于 catalog.Service 的 PathResolver。svc 为 nil 时返回 nil 接口
// （registry 据此跳过补全），避免把 nil 指针装进非 nil 接口。
func ResolverFrom(svc *catalog.Service) godatabase.PathResolver {
	if svc == nil {
		return nil
	}
	return resolver{svc: svc}
}

func (r resolver) ResolveDatabasePath(ctx context.Context, tenantID, projectID string) (string, error) {
	target, err := r.svc.ResolveProjectStorage(ctx, tenantID, projectID)
	if err != nil {
		return "", err
	}
	return target.DatabasePath, nil
}

// LoggerFrom 把 observability.Logger 适配为库的 Logger。
//
// observability.Logger 是 *zapLogger：nil 指针装进接口会得到非 nil 接口，库内的
// `logger != nil` 判断会失效并在调用时 panic。因此 nil 必须返回无类型 nil。
func LoggerFrom(l observability.Logger) godatabase.Logger {
	if l == nil {
		return nil
	}
	return l
}

type observer struct{ m *observability.Metrics }

// ObserverFrom 把 *observability.Metrics 适配为库的 Observer。m 为 nil 时返回无类型 nil。
// 单个指标字段为 nil 时对应调用静默跳过，与重构前的字段级 nil 判断一致。
func ObserverFrom(m *observability.Metrics) godatabase.Observer {
	if m == nil {
		return nil
	}
	return observer{m: m}
}

func (o observer) OpenHandleDelta(delta int) {
	if o.m.DBOpenHandles == nil {
		return
	}
	o.m.DBOpenHandles.Add(float64(delta))
}

func (o observer) CacheBytes(bytes int64) { o.m.ObserveCacheBytes(bytes) }

func (o observer) CacheEvicted() { o.m.IncCacheEvictions() }

func (o observer) CatalogSync(outcome string, d time.Duration, failed bool) {
	if failed && o.m.CatalogSyncFailures != nil {
		o.m.CatalogSyncFailures.Inc()
	}
	if o.m.CatalogSyncTotal != nil {
		o.m.CatalogSyncTotal.WithLabelValues(outcome).Inc()
	}
	if o.m.CatalogSyncDuration != nil {
		o.m.CatalogSyncDuration.Observe(d.Seconds())
	}
}

func (o observer) CatalogSyncLag(dbID string, lag int64) {
	if o.m.CatalogSyncLag != nil {
		o.m.CatalogSyncLag.WithLabelValues(dbID).Set(float64(lag))
	}
}

func (o observer) CatalogSyncConflict(dbID string) {
	if o.m.CatalogSyncConflicts != nil {
		o.m.CatalogSyncConflicts.WithLabelValues(dbID).Inc()
	}
}

func (o observer) CatalogSize(dbID string, bytes int64) {
	if o.m.CatalogSizeBytes != nil {
		o.m.CatalogSizeBytes.WithLabelValues(dbID).Set(float64(bytes))
	}
}

func (o observer) CatalogLocalAhead() {
	if o.m.CatalogLocalAhead != nil {
		o.m.CatalogLocalAhead.Inc()
	}
}

// Registry 把库的 *registry.Registry 适配为宿主侧以 catalog.Database 为参数的接口，
// 使 api 层的 RegistryService 签名与全部 handler 保持不变。
type Registry struct{ R *registry.Registry }

// RegistryService 是宿主侧对 Registry 的最小接口，与 api.RegistryService 同形。
type RegistryService interface {
	Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (*registry.Lease, error)
	CloseDatabase(ctx context.Context, databaseID string) error
}

// RegistryFrom 包装 *registry.Registry。r 为 nil 时返回无类型 nil 接口：
// api 层用 `reg == nil` 区分只读实例，nil 指针装进接口会得到非 nil 接口并破坏该判断。
func RegistryFrom(r *registry.Registry) RegistryService {
	if r == nil {
		return nil
	}
	return Registry{R: r}
}

// Acquire 在入口把 catalog.Database 转成 Spec。
func (r Registry) Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (*registry.Lease, error) {
	return r.R.Acquire(ctx, SpecOf(db), mode)
}

// CloseDatabase 透传。
func (r Registry) CloseDatabase(ctx context.Context, databaseID string) error {
	return r.R.CloseDatabase(ctx, databaseID)
}

// WriteGate 把宿主以 catalog.Database 为参数的写门禁适配为库的 Options.WriteGate。
func WriteGate(f func(db catalog.Database) error) func(godatabase.Spec) error {
	if f == nil {
		return nil
	}
	return func(s godatabase.Spec) error { return f(CatalogOf(s)) }
}
