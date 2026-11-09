// 文件说明：principal.go — SimpleBase 的认证与授权原语
//
// 用法
//
//	(p Principal) HasPermission(perm Permission) bool  // HasPermission 判断 principal 是否拥有给定权限
//	(p Principal) CanAccessProject(projectID string) bool  // CanAccessProject 判断 principal 是否可访问指定 project
//	同包调用：Principal → cache.go, delegation.go 等；Permission → api_key_repository.go, delegation.go 等；HasPermission() → middleware.go, service.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/principal.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] api_key_repository.go, cache.go, delegation.go, id.go, jwt.go 等 15 个
//	[关键引用] Principal ← cache.go, delegation.go, middleware.go
//
// 主要符号
//
//	type Permission string
//	const DatabaseRead
//	const DatabaseWrite
//	const DatabaseAdmin
//	const LLMInvoke
//	const ProjectAdmin
//	... 另有 4 个顶层符号

package auth

// Permission 是 SimpleBase 的权限粒度。
type Permission string

const (
	DatabaseRead  Permission = "database:read"
	DatabaseWrite Permission = "database:write"
	DatabaseAdmin Permission = "database:admin"
	LLMInvoke     Permission = "llm:invoke"
	ProjectAdmin  Permission = "project:admin"
)

// Principal 表示一个已认证的调用方身份。
//
// 字段说明：
//   - APIKeyID：所使用的 API Key 摘要 ID，用于审计日志；不含 key 原文。
//   - TenantID：所属租户。
//   - ProjectIDs：该 key 被授权访问的 project 集合；handler 必须校验资源属于其中之一。
//   - Permissions：授予的权限集合。
//
// 所有字段都不可包含密钥原文。日志输出时只使用 APIKeyID/TenantID。
type Principal struct {
	APIKeyID string
	// UserID / Username / Role / SessionID / AccessJTI 仅登录态（JWT 通道）有值；
	// API Key 通道保持零值，向后兼容。
	UserID      string
	Username    string
	Role        Role
	SessionID   string
	AccessJTI   string
	TenantID    string
	ProjectIDs  map[string]struct{}
	Permissions map[Permission]struct{}
	Scope       *KeyPolicy
	// Delegated 标记助手委托令牌主体（planv6.0 D1）：路由级 delegable 标记
	// 由 api 包注入 Echo context，Require 据此拒绝未标记路由（默认拒绝）。
	Delegated bool
}

// HasPermission 判断 principal 是否拥有给定权限（含层级继承，见 PermissionImplied）。
func (p Principal) HasPermission(perm Permission) bool {
	return PermissionImplied(p.Permissions, perm)
}

// permissionImplies 是权限层级：拥有键即视为拥有值里的权限。
// database:admin（建库、删库）隐含 database:write 与 database:read，
// 发布 Key 只勾 admin 也能跑迁移、写函数与定时任务。write 不隐含 read，保持原语义。
var permissionImplies = map[Permission][]Permission{
	DatabaseAdmin: {DatabaseWrite, DatabaseRead},
}

// PermissionImplied 判断权限集合 set 是否直接或经层级继承拥有 perm（nil 集合视为无权限）。
// 鉴权与签发子集校验共用它，保证「能用」与「能签」口径一致。
func PermissionImplied(set map[Permission]struct{}, perm Permission) bool {
	if set == nil {
		return false
	}
	if _, ok := set[perm]; ok {
		return true
	}
	for holder, implied := range permissionImplies {
		if _, ok := set[holder]; !ok {
			continue
		}
		for _, p := range implied {
			if p == perm {
				return true
			}
		}
	}
	return false
}

// CanAccessProject 判断 principal 是否可访问指定 project。
func (p Principal) CanAccessProject(projectID string) bool {
	if p.ProjectIDs == nil {
		return false
	}
	_, ok := p.ProjectIDs[projectID]
	return ok
}

// CheckProjectAccess 校验租户与项目归属；系统项目仅允许管理员或管理 Key。
func CheckProjectAccess(p Principal, projectID, tenantID string, isSystem bool) bool {
	if p.TenantID == "" || p.TenantID != tenantID {
		return false
	}
	if p.Role.IsUser() {
		return !isSystem && p.CanAccessProject(projectID)
	}
	if p.Role.IsSuper() || p.Role.IsAdmin() || p.HasPermission(ProjectAdmin) {
		return true
	}
	return !isSystem && p.CanAccessProject(projectID)
}
