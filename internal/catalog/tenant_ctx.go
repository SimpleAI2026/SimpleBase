// tenant_ctx.go 实现 §7.2 P1.1：请求路径上避免重复解析 project→tenant。
//
// api 的 projectContextMiddleware 已通过 ResolveProjectTenant 解析出
// {projectID, tenantID}；下游 catalog.ensureProjectAccess / GetKVDatabase
// 再查一次 GetProjectTenant / ProjectBelongsToTenant 是同一数据的重复点查
//（每次约 4ms）。本文件提供 context 通道：上游解析后注入，下游命中即免查。
//
// 语义：ctx 中的 tenant 仅为「免重复查询」的捷径；值必须来自
// ResolveProjectTenant 同一请求的解析结果（非缓存，无失效问题）。
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
