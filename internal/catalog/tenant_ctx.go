// 文件说明：tenant_ctx.go — §7.2 P1.1：请求路径上避免重复解析 project→tenant
//
// 用法
//
//	WithResolvedProjectTenant(ctx context.Context, projectID, tenantID string) ...  // WithResolvedProjectTenant 把已解析的
//	type resolvedTenantKey（包内使用）
//	同包调用：tenantMatches() → service.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/catalog/tenant_ctx.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] cache.go, data_model.go, errors.go, model.go, project_storage.go 等 9 个
//	[关键引用] tenantMatches() ← service.go
//
// 主要符号
//
//	WithResolvedProjectTenant(ctx context.Context, projectID, tenantID string) ...
//	... 另有 3 个顶层符号

package catalog

import "context"

type resolvedTenantKey struct{}

// WithResolvedProjectTenant 把已解析的 {projectID, tenantID} 注入 ctx。
// tenantID 为空时不注入（上游解析失败由上游返回错误）。
func WithResolvedProjectTenant(ctx context.Context, projectID, tenantID string) context.Context {
	if ctx == nil || projectID == "" || tenantID == "" {
		return ctx
	}
	return context.WithValue(ctx, resolvedTenantKey{}, [2]string{projectID, tenantID})
}

// resolvedProjectTenant 返回 ctx 上已解析的 {projectID, tenantID}。
func resolvedProjectTenant(ctx context.Context) (projectID, tenantID string, ok bool) {
	if ctx == nil {
		return "", "", false
	}
	v, hit := ctx.Value(resolvedTenantKey{}).([2]string)
	if !hit || v[0] == "" || v[1] == "" {
		return "", "", false
	}
	return v[0], v[1], true
}

// tenantMatches 校验 ctx 上已解析的 tenant 与期望值一致（免查库比对）。
// projectID 不匹配（跨项目复用 ctx 的防御）时返回 false，调用方回退查库。
func tenantMatches(ctx context.Context, wantProjectID, principalTenantID string) bool {
	pid, tenant, ok := resolvedProjectTenant(ctx)
	return ok && pid == wantProjectID && tenant == principalTenantID
}
