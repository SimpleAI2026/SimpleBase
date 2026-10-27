// 文件说明：reserved.go — internal/catalog 中 DatabaseKindUser、DatabaseKindSystem 等符号的实现
//
// 用法
//
//	IsSystemProject(id string) bool  // IsSystemProject 判断是否为系统（admin）项目
//	IsSystemDatabase(d Database) bool  // IsSystemDatabase 判断 catalog 行是否为系统库
//	同包调用：DatabaseKindSystem → data_model.go, service.go 等；DatabaseKindKV → data_model.go, service.go 等；DataModelCollection → data_model.go, sql_repository.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/catalog/reserved.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] cache.go, data_model.go, errors.go, model.go, project_storage.go 等 9 个
//	[关键引用] DatabaseKindSystem ← data_model.go, service.go, sql_repository.go
//
// 主要符号
//
//	const DatabaseKindUser
//	const DatabaseKindSystem
//	const DatabaseKindKV
//	const KVDatabaseName
//	const ReservedTenantID
//	const ReservedSystemProjectID
//	... 另有 6 个顶层符号

package catalog

// 系统库使用的稳定 ID。
// project_id 必须为 8 位 [A-Za-z0-9-]（objectstore.ValidateProjectID）；
// tenant_id / database_id 仍为 UUID。
const (
	DatabaseKindUser   = "user"
	DatabaseKindSystem = "system"
	// DatabaseKindKV 承载项目专属的 Key-Value catalog 行（key-value-ducklake-plan §2）。
	// 与用户库同引擎（DuckLake），但不出现在用户库列表，name 固定为 "kv"。
	DatabaseKindKV = "kv"

	// KVDatabaseName 是 kind=kv 行在项目内的保留名（唯一性约束 (project_id,name)）。
	KVDatabaseName = "kv"

	// ReservedTenantID 是实例保留租户。租户不再参与存储路径（路径按项目 ID 组织），
	// 仅保留在 catalog 行中以兼容既有数据模型。
	ReservedTenantID = "00000000-0000-0000-0000-000000000001"
	// ReservedSystemProjectID 承载 kind=system 的系统库行；以 admin 项目形式对 ProjectAdmin 可见。
	ReservedSystemProjectID = "sb-admin"
	// AdminProjectName 是系统项目在项目列表中的展示名。
	AdminProjectName = "admin"
)

// IsSystemProject 判断是否为系统（admin）项目。
func IsSystemProject(id string) bool {
	return id == ReservedSystemProjectID
}

// IsSystemDatabase 判断 catalog 行是否为系统库。
func IsSystemDatabase(d Database) bool {
	return d.Kind == DatabaseKindSystem
}

// IsKVDatabase 判断 catalog 行是否为项目 KV catalog（不出现在用户库列表）。
func IsKVDatabase(d Database) bool {
	return d.Kind == DatabaseKindKV
}

// 用户库数据形态。创建时选定；省略视为集合文档。
const (
	DataModelCollection = "collection"
	DataModelSQL        = "sql"
)
