package catalog

// 系统库与 DevMode 种子使用的稳定 UUID。
// project_id 必须为 UUID（descriptor Validate）；不再使用字符串 "proj-01"。
const (
	DatabaseKindUser   = "user"
	DatabaseKindSystem = "system"

	// ReservedTenantID 是实例保留租户，系统库与 Dev 种子项目同属此租户。
	ReservedTenantID = "00000000-0000-0000-0000-000000000001"
	// ReservedSystemProjectID 承载 kind=system 的系统库行；以 admin 项目形式对 ProjectAdmin 可见。
	ReservedSystemProjectID = "00000000-0000-0000-0000-000000000099"
	// AdminProjectName 是系统项目在项目列表中的展示名。
	AdminProjectName = "admin"
	// DevProjectID 是 DevMode 种子项目（展示名「商城后台」）。
	DevProjectID = "00000000-0000-0000-0000-000000000002"
	// DevAPIKeyID 是 DevMode 种子 API Key 的记录 ID。
	DevAPIKeyID = "00000000-0000-0000-0000-000000000003"
)

// IsSystemProject 判断是否为系统（admin）项目。
func IsSystemProject(id string) bool {
	return id == ReservedSystemProjectID
}

// IsSystemDatabase 判断 catalog 行是否为系统库。
func IsSystemDatabase(d Database) bool {
	return d.Kind == DatabaseKindSystem
}
