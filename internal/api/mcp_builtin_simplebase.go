// 文件说明：mcp_builtin_simplebase.go — internal/api 的内部实现（simplebaseAPIMCP、simplebaseTools 等）
//
// 用法
//
//	同包调用：simplebaseAPIMCP → mcp_builtin.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/mcp_builtin_simplebase.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backup_handler.go, data_compat.go, error.go, file_compat.go, function_compat.go, health.go, mcp_builtin.go, mcp_builtin_admin.go 等 28 个
//
// 主要符号
//
//	var simplebaseAPIMCP
//	var simplebaseTools
//	buildSimplebaseTools(b *builtinMCPBuilder, pc ProjectContext) *mcp.ServerSpec

// mcp_builtin_simplebase.go — 内置 MCP Server `simplebase` 的工具表（planv6.7 §3.3）
package api

import (
	"encoding/json"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
)

var simplebaseAPIMCP = builtinMCP{
	Name:      "simplebase",
	TitleKey:  "components.mcpServers.builtin.simplebaseTitle",
	Available: func(deps Dependencies) bool { return true },
	Build:     buildSimplebaseTools,
}

// 工具表：每条是一个 `/v1` 路由的薄封装。schema 以 handler 实际定义为准；
// PathArgs 会 PathEscape；BodyArg=""：整个 arguments 做 body，"-"：无 body。
var simplebaseTools = []builtinAPITool{
	{
		Name:        "project_info",
		Description: "获取项目设置和配额信息",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/settings",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		ReadOnly:    true,
	},
	{
		Name:        "list_databases",
		Description: "列出项目中的数据库",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/databases",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		ReadOnly:    true,
	},
	{
		Name:        "get_schema",
		Description: "查询数据库表结构（列信息、索引、约束）",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/databases/{databaseID}/schema",
		PathArgs:    []string{"databaseID"},
		Schema: json.RawMessage(`{"type":"object","properties":{
			"databaseID":{"type":"string","description":"数据库ID；可省略，默认使用项目唯一库"}
		}}`),
		ReadOnly: true,
	},
	{
		Name:        "sql_query",
		Description: "执行 SQL 查询（SELECT 等只读语句）",
		Method:      "POST",
		Path:        "/v1/projects/{projectID}/databases/{databaseID}/query",
		PathArgs:    []string{"databaseID"},
		BodyArg:     "",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"databaseID":{"type":"string","description":"数据库ID；可省略"},
			"sql":{"type":"string","description":"SQL 语句"},
			"args":{"type":"array","items":{},"description":"参数化查询参数（可选）"},
			"max_rows":{"type":"integer","description":"返回行数上限（可选）"}
		},"required":["sql"]}`),
		ReadOnly: true,
	},
	{
		Name:        "sql_execute",
		Description: "执行 SQL 写操作（INSERT / UPDATE / DELETE / DDL）",
		Method:      "POST",
		Path:        "/v1/projects/{projectID}/databases/{databaseID}/execute",
		PathArgs:    []string{"databaseID"},
		BodyArg:     "",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"databaseID":{"type":"string"},
			"sql":{"type":"string","description":"SQL 语句"},
			"args":{"type":"array","items":{}}
		},"required":["sql"]}`),
		Destructive: true,
	},
	{
		Name:        "list_collections",
		Description: "列出数据库中的集合（文档表）",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/databases/{databaseID}/data/collections",
		PathArgs:    []string{"databaseID"},
		Schema: json.RawMessage(`{"type":"object","properties":{
			"databaseID":{"type":"string"}
		}}`),
		ReadOnly: true,
	},
	{
		Name:        "query_documents",
		Description: "查询集合文档（支持过滤、排序、分页）",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/databases/{databaseID}/data/collections/{collection}/documents",
		PathArgs:    []string{"databaseID", "collection"},
		QueryArgs:   []string{"filter", "limit", "offset", "sort", "fields"},
		BodyArg:     "-",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"databaseID":{"type":"string"},
			"collection":{"type":"string","description":"集合名"},
			"filter":{"type":"string","description":"筛选条件，如 age>18 AND name~'李'"},
			"limit":{"type":"integer"},
			"offset":{"type":"integer"},
			"sort":{"type":"string","description":"排序字段，如 created_at desc"},
			"fields":{"type":"string","description":"返回字段，逗号分隔"}
		},"required":["collection"]}`),
		ReadOnly: true,
	},
	{
		Name:        "get_document",
		Description: "按 ID 获取单个文档",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/databases/{databaseID}/data/collections/{collection}/documents/{id}",
		PathArgs:    []string{"databaseID", "collection", "id"},
		BodyArg:     "-",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"databaseID":{"type":"string"},
			"collection":{"type":"string"},
			"id":{"type":"string","description":"文档ID"}
		},"required":["collection","id"]}`),
		ReadOnly: true,
	},
	{
		Name:        "create_document",
		Description: "在集合中创建新文档",
		Method:      "POST",
		Path:        "/v1/projects/{projectID}/databases/{databaseID}/data/collections/{collection}/documents",
		PathArgs:    []string{"databaseID", "collection"},
		BodyArg:     "",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"databaseID":{"type":"string"},
			"collection":{"type":"string"},
			"_id":{"type":"string","description":"文档ID（可选）"},
			"_collection":{"type":"string","description":"忽略"}
		},"required":["collection"],"additionalProperties":true}`),
	},
	{
		Name:        "update_document",
		Description: "更新文档（部分字段或整体替换）",
		Method:      "PATCH",
		Path:        "/v1/projects/{projectID}/databases/{databaseID}/data/collections/{collection}/documents/{id}",
		PathArgs:    []string{"databaseID", "collection", "id"},
		BodyArg:     "",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"databaseID":{"type":"string"},
			"collection":{"type":"string"},
			"id":{"type":"string"}
		},"required":["collection","id"],"additionalProperties":true}`),
	},
	{
		Name:        "delete_document",
		Description: "删除文档",
		Method:      "DELETE",
		Path:        "/v1/projects/{projectID}/databases/{databaseID}/data/collections/{collection}/documents/{id}",
		PathArgs:    []string{"databaseID", "collection", "id"},
		BodyArg:     "-",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"databaseID":{"type":"string"},
			"collection":{"type":"string"},
			"id":{"type":"string"}
		},"required":["collection","id"]}`),
		Destructive: true,
	},
	{
		Name:        "aggregate_collection",
		Description: "聚合查询（groupBy / sum / avg / count 等）",
		Method:      "POST",
		Path:        "/v1/projects/{projectID}/databases/{databaseID}/data/collections/{collection}/aggregate",
		PathArgs:    []string{"databaseID", "collection"},
		BodyArg:     "",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"databaseID":{"type":"string"},
			"collection":{"type":"string"},
			"groupBy":{"type":"array","items":{"type":"string"}},
			"metrics":{"type":"array","items":{"type":"object"}},
			"filter":{"type":"string"}
		},"required":["collection"]}`),
		ReadOnly: true,
	},
	{
		Name:        "kv_command",
		Description: "执行 KV 命令（redis 风格 API）；读命令可免确认",
		Method:      "POST",
		Path:        "/v1/projects/{projectID}/kv",
		BodyArg:     "",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"type":{"type":"string","enum":["cmd","string","list","hash","set","zset"],"description":"cmd=命令行模式，其他为对象形式"},
			"argvs":{"type":"array","items":{"type":"string"},"description":"仅 type=cmd：argvs[0]=命令名"},
			"args":{"type":"object","description":"数据类型参数"}
		},"required":["type"]}`),
	},
	{
		Name:        "list_objects",
		Description: "列出对象存储中的文件（支持前缀、分隔符）",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/s3/objects",
		QueryArgs:   []string{"prefix", "delimiter"},
		BodyArg:     "-",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"prefix":{"type":"string","description":"前缀过滤"},
			"delimiter":{"type":"string","description":"目录分隔符，如 /"}
		}}`),
		ReadOnly: true,
	},
	{
		Name:        "get_object_link",
		Description: "生成对象限时下载链接（不直接返回文件正文）",
		Method:      "POST",
		Path:        "/v1/projects/{projectID}/s3/link",
		BodyArg:     "",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"key":{"type":"string","description":"对象键"}
		},"required":["key"]}`),
		ReadOnly: true,
	},
	{
		Name:        "list_gofunctions",
		Description: "列出项目中的云函数",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/gofunctions",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		ReadOnly:    true,
	},
	{
		Name:        "invoke_gofunction",
		Description: "调用云函数",
		Method:      "POST",
		Path:        "/go/{projectID}/{name}/{functionName}",
		PathArgs:    []string{"name", "functionName"},
		BodyArg:     "",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"name":{"type":"string","description":"函数组名称"},
			"functionName":{"type":"string","description":"函数入口点"}
		},"required":["name","functionName"],"additionalProperties":true}`),
	},
	{
		Name:        "list_cron_jobs",
		Description: "列出定时任务",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/cron-jobs",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		ReadOnly:    true,
	},
	{
		Name:        "trigger_cron_job",
		Description: "手动触发定时任务",
		Method:      "POST",
		Path:        "/v1/projects/{projectID}/cron-jobs/{jobID}/trigger",
		PathArgs:    []string{"jobID"},
		BodyArg:     "-",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"jobID":{"type":"string","description":"定时任务ID"}
		},"required":["jobID"]}`),
	},
	{
		Name:        "list_agents",
		Description: "列出云助手（agent）配置",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/agents",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		ReadOnly:    true,
	},
	{
		Name:        "query_logs",
		Description: "查询项目日志（支持级别、时间范围、关键词）",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/logs",
		QueryArgs:   []string{"level", "q", "from", "to", "limit"},
		BodyArg:     "-",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"level":{"type":"string","enum":["debug","info","warn","error"]},
			"q":{"type":"string","description":"关键词搜索"},
			"from":{"type":"string","format":"date-time","description":"起始时间（RFC3339）"},
			"to":{"type":"string","format":"date-time"},
			"limit":{"type":"integer"}
		}}`),
		ReadOnly: true,
	},
	{
		Name:        "site_status",
		Description: "查询对象存储站点状态（公开访问配置）",
		Method:      "GET",
		Path:        "/v1/projects/{projectID}/s3/site",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		ReadOnly:    true,
	},
}

func buildSimplebaseTools(b *builtinMCPBuilder, pc httpkit.ProjectContext) *mcp.ServerSpec {
	return b.buildServerFromTools(simplebaseTools, pc)
}
