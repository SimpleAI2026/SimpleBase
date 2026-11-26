// 文件说明：subpackage_aliases_test.go — 仅供同包测试使用的业务域子包类型别名
//
// 用法
//
//	同包路由级 / 守卫用例用旧名引用 internal/api/datahttp 等子包的类型（如 fakeSQLService 返回的 SQLLease）
//
// 调用架构
//
//	[上游] agent_kernel_helpers_test.go 等同包测试
//	         │ 同包别名
//	         ▼
//	[本文件] internal/api/subpackage_aliases_test.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/datahttp
//
// 主要符号
//
//	type SQLLease = datahttp.SQLLease
//	type SQLLimits / DatabaseService / SQLStatementRequest / QueryRequest 等（按需追加）

package api

import (
	"github.com/linkxzhou/SimpleBase/internal/api/agenthttp"
	"github.com/linkxzhou/SimpleBase/internal/api/cataloghttp"
	"github.com/linkxzhou/SimpleBase/internal/api/datahttp"
	"github.com/linkxzhou/SimpleBase/internal/api/functionhttp"
	"github.com/linkxzhou/SimpleBase/internal/api/iamhttp"
	"github.com/linkxzhou/SimpleBase/internal/api/llmhttp"
	"github.com/linkxzhou/SimpleBase/internal/api/opshttp"
)

type (
	SQLLease               = datahttp.SQLLease
	SQLLimits              = datahttp.SQLLimits
	DatabaseService        = datahttp.DatabaseService
	SQLService             = datahttp.SQLService
	SystemStoreRef         = datahttp.SystemStoreRef
	SQLStatementRequest    = datahttp.SQLStatementRequest
	QueryRequest           = datahttp.QueryRequest
	ExecuteRequest         = datahttp.ExecuteRequest
	BatchRequest           = datahttp.BatchRequest
	BatchResultItem        = datahttp.BatchResultItem
	DatabaseSnapshot       = datahttp.DatabaseSnapshot
	RowCountCacheOptions   = datahttp.RowCountCacheOptions
	QueryResponse          = datahttp.QueryResponse
	ExecuteResponse        = datahttp.ExecuteResponse
	BatchResponse          = datahttp.BatchResponse
	BatchError             = datahttp.BatchError
	DeleteDatabaseResponse = datahttp.DeleteDatabaseResponse
	CreateDatabaseRequest  = datahttp.CreateDatabaseRequest
	DatabaseResponse       = datahttp.DatabaseResponse
)

// 数据面构造函数的同包转发（路由级用例直接构造 handler 用）。
var (
	NewDatabaseHandler        = datahttp.NewDatabaseHandler
	NewSQLHandler             = datahttp.NewSQLHandler
	NewDataHandler            = datahttp.NewDataHandler
	NewCollectionMetaHandler  = datahttp.NewCollectionMetaHandler
	NewSchemaHandler          = datahttp.NewSchemaHandler
	NewKVHandler              = datahttp.NewKVHandler
	NewRealtimeHandler        = datahttp.NewRealtimeHandler
	NewKVServiceAdapter       = datahttp.NewKVServiceAdapter
	NewSQLServiceAdapter      = datahttp.NewSQLServiceAdapter
	NewDatabaseServiceAdapter = datahttp.NewDatabaseServiceAdapter
	NewRowCountCache          = datahttp.NewRowCountCache
	CountDatabaseRows         = datahttp.CountDatabaseRows
)

// 身份 / LLM / Agent 入口的同包转发（路由级用例用）。
var (
	NewEndUsersHandler   = iamhttp.NewEndUsersHandler
	NewAuthHandler       = iamhttp.NewAuthHandler
	NewUsersHandler      = iamhttp.NewUsersHandler
	NewAPIKeysHandler    = iamhttp.NewAPIKeysHandler
	NewGoFunctionHandler = functionhttp.NewGoFunctionHandler
	NewCronJobHandler    = functionhttp.NewCronJobHandler
	NewMigrationHandler  = opshttp.NewMigrationHandler
)

type (
	LLMHandler        = llmhttp.LLMHandler
	AuthHandler       = iamhttp.AuthHandler
	UsersHandler      = iamhttp.UsersHandler
	APIKeysHandler    = iamhttp.APIKeysHandler
	EndUsersHandler   = iamhttp.EndUsersHandler
	CloudAgentHandler = agenthttp.CloudAgentHandler
)

const oaiSSEDone = llmhttp.SSEDone

var WrapTimedLease = datahttp.WrapTimedLease

// 项目目录 DTO 的同包别名（cataloghttp）。
type (
	ProjectResponse      = cataloghttp.ProjectResponse
	ProjectListResponse  = cataloghttp.ProjectListResponse
	CreateProjectRequest = cataloghttp.CreateProjectRequest
)
