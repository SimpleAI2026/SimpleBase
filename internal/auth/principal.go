// Package auth 提供 SimpleBase 的认证与授权原语。
//
// 本文件只定义 Principal 类型和权限常量，供 catalog/registry/api 共享。
// Service、middleware、api_key_repository 由 Plan 5 补充。
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
	UserID    string
	Username  string
	Role      Role
	SessionID string
	AccessJTI string
	TenantID  string
	ProjectIDs  map[string]struct{}
	Permissions map[Permission]struct{}
}

// HasPermission 判断 principal 是否拥有给定权限。
func (p Principal) HasPermission(perm Permission) bool {
	if p.Permissions == nil {
		return false
	}
	_, ok := p.Permissions[perm]
	return ok
}

// CanAccessProject 判断 principal 是否可访问指定 project。
func (p Principal) CanAccessProject(projectID string) bool {
	if p.ProjectIDs == nil {
		return false
	}
	_, ok := p.ProjectIDs[projectID]
	return ok
}
