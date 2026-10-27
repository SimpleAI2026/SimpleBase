// 文件说明：role.go — internal/auth 中 Role、RoleSuperAdmin 等符号的实现
//
// 用法
//
//	(r Role) Valid() bool  // Valid 判断是否为合法角色值
//	(r Role) IsSuper() bool// IsSuper / IsAdmin / IsUser 便捷判定
//	同包调用：Role → delegation.go, jwt.go 等；Valid() → api_key_repository.go, policy.go 等；IsSuper() → principal.go, user.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/role.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] api_key_repository.go, cache.go, delegation.go, id.go, jwt.go 等 15 个
//	[关键引用] Role ← delegation.go, jwt.go, principal.go
//
// 主要符号
//
//	type Role string
//	const RoleSuperAdmin
//	const RoleAdmin
//	const RoleUser
//	const UserAdmin
//	(r Role) Valid() bool
//	... 另有 7 个顶层符号

package auth

// Role 是控制台账号的三级角色（planv3.0 login-auth-plan §2）。
// 空 Role 表示 API Key 通道，不参与角色判定。
type Role string

const (
	RoleSuperAdmin Role = "superadminl1"
	RoleAdmin      Role = "admin"
	RoleUser       Role = "user"
)

// UserAdmin 允许管理用户（仅 superadminl1）。
const UserAdmin Permission = "user:admin"

// Valid 判断是否为合法角色值。
func (r Role) Valid() bool {
	switch r {
	case RoleSuperAdmin, RoleAdmin, RoleUser:
		return true
	default:
		return false
	}
}

// IsSuper / IsAdmin / IsUser 便捷判定。
func (r Role) IsSuper() bool { return r == RoleSuperAdmin }
func (r Role) IsAdmin() bool { return r == RoleAdmin }
func (r Role) IsUser() bool  { return r == RoleUser }

// CanManageUsers 仅 superadminl1 可写用户。
func (r Role) CanManageUsers() bool { return r == RoleSuperAdmin }

// CanViewUsers super 与 admin 可看用户列表。
func (r Role) CanViewUsers() bool { return r == RoleSuperAdmin || r == RoleAdmin }

// CanWrite 业务写（数据库/对象/函数等）。admin 全局只读。
func (r Role) CanWrite() bool {
	return r == RoleSuperAdmin || r == RoleUser
}

// PermissionsForRole 展开角色到权限位。admin 只读，不授予任何 write/admin 位。
func PermissionsForRole(r Role) map[Permission]struct{} {
	mk := func(ps ...Permission) map[Permission]struct{} {
		out := make(map[Permission]struct{}, len(ps))
		for _, p := range ps {
			out[p] = struct{}{}
		}
		return out
	}
	switch r {
	case RoleSuperAdmin:
		return mk(DatabaseRead, DatabaseWrite, DatabaseAdmin, LLMInvoke, ProjectAdmin, UserAdmin)
	case RoleAdmin:
		// 只读：能看信息，不能修改（planv3.0 D4）。
		return mk(DatabaseRead, LLMInvoke)
	case RoleUser:
		return mk(DatabaseRead, DatabaseWrite, DatabaseAdmin, LLMInvoke, ProjectAdmin)
	default:
		return map[Permission]struct{}{}
	}
}
