package cloudagent

import "strings"

const (
	ModuleDatabase = "database"
	ModuleS3       = "s3"
	ModuleLogs     = "logs"
	ModuleGeneral  = "general"
	ModuleSandbox  = "sandbox"

	ToolListDatabases   = "list_databases"
	ToolListCollections = "list_collections"
	ToolReadonlySQL     = "readonly_sql"
	ToolListObjects     = "list_objects"
	ToolHeadObject      = "head_object"
	ToolSearchLogs      = "search_logs"
	ToolLogLevelStats   = "log_level_stats"

	ToolSandboxExec      = "sandbox_exec"
	ToolSandboxShell     = "sandbox_shell"
	ToolSandboxReadFile  = "sandbox_read_file"
	ToolSandboxWriteFile = "sandbox_write_file"
)

// SandboxToolIDs 是四个云沙盒工具 id（cloud-agent-sandbox-plan §7）。
func SandboxToolIDs() []string {
	return []string{ToolSandboxExec, ToolSandboxShell, ToolSandboxReadFile, ToolSandboxWriteFile}
}

// IsSandboxTool 报告 id 是否为沙盒工具。
func IsSandboxTool(id string) bool {
	switch id {
	case ToolSandboxExec, ToolSandboxShell, ToolSandboxReadFile, ToolSandboxWriteFile:
		return true
	default:
		return false
	}
}

const platformBasePrompt = `You are SimpleBase Cloud Agent, a project-scoped assistant.
Rules:
- Stay in the current project. Never guess another tenant or project.
- Tools are readonly. Never invent INSERT/UPDATE/DELETE/DROP or object uploads.
- Never output secrets: S3 keys, provider API keys, passwords, tokens, DSN, or credential_ref values.
- If a tool errors, explain the error; do not fabricate rows or keys.
- Prefer tools over guessing schema or object lists.
- Answer in the user's language.`

const databaseModulePrompt = `Module: database.
You inspect DuckLake databases in this project.
Use list_databases, list_collections, and readonly_sql (SELECT/WITH/EXPLAIN/DESCRIBE/SHOW only).
Do not run writes. Do not ATTACH, COPY, PRAGMA, or SET.`

const s3ModulePrompt = `Module: s3.
You inspect the project's object store.
Use list_objects and head_object. Keys are project-relative (no physical prefix).
Do not upload, delete, or request file bodies.`

const logsModulePrompt = `Module: logs.
You search sys_log_events for this project.
Use search_logs and log_level_stats. Do not change retention.`

const generalModulePrompt = `Module: general.
You help with database, object storage, and log inspection in this project. Choose tools according to the user's request; list resources before assuming names.
Database, object storage, and log tools are read-only. Never perform writes to project data.
If sandbox tools are available, commands and files are isolated in this thread's cloud sandbox under /workspace. Do not request or expose credentials.`

const sandboxModulePrompt = `Module: sandbox.
Commands and files live only in this thread's cloud sandbox (microVM), working directory /workspace.
Use sandbox_exec (argv command), sandbox_shell (/bin/sh -c), sandbox_read_file, sandbox_write_file.
Paths for read/write must be absolute and under /workspace.
The environment is recycled after the idle timeout or max lifetime; files may not survive.
Readonly tools (database/s3/logs) still apply to this project; the sandbox has no SimpleBase credentials and cannot reach the system database.
Never ask for or print S3 keys, provider keys, DSN, or tokens.`

// ModuleInfo is returned by GET /agents/modules.
type ModuleInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	DefaultTools  []string `json:"default_tools"`
	TeamSupported bool     `json:"team_supported"`
	// SandboxAvailable 仅 sandbox 模块返回：enabled 且 backend 核对为 cloud。
	SandboxAvailable bool `json:"sandbox_available,omitempty"`
}

// Modules returns the built-in module catalog (Team is a Phase 4 stub).
// sandboxAvailable 控制 sandbox 模块的 sandbox_available 字段；其余模块不含。
func Modules(sandboxAvailable bool) []ModuleInfo {
	return []ModuleInfo{
		{ID: ModuleGeneral, Name: "通用助手", Description: "跨数据库、对象存储和日志的项目助手", DefaultTools: GeneralTools(sandboxAvailable), TeamSupported: false, SandboxAvailable: sandboxAvailable},
		{ID: ModuleDatabase, Name: "Database", Description: "Readonly database inspection and SQL", DefaultTools: []string{ToolListDatabases, ToolListCollections, ToolReadonlySQL}, TeamSupported: false},
		{ID: ModuleS3, Name: "S3", Description: "Readonly object list and head", DefaultTools: []string{ToolListObjects, ToolHeadObject}, TeamSupported: false},
		{ID: ModuleLogs, Name: "Logs", Description: "Search logs and level stats", DefaultTools: []string{ToolSearchLogs, ToolLogLevelStats}, TeamSupported: false},
		{ID: ModuleSandbox, Name: "Sandbox", Description: "Run commands and manage files in a cloud microVM", DefaultTools: SandboxToolIDs(), TeamSupported: false, SandboxAvailable: sandboxAvailable},
	}
}

// GeneralTools returns project-scoped read-only tools plus optional cloud sandbox tools.
func GeneralTools(sandboxAvailable bool) []string {
	tools := []string{ToolListDatabases, ToolListCollections, ToolReadonlySQL, ToolListObjects, ToolHeadObject, ToolSearchLogs, ToolLogLevelStats}
	if sandboxAvailable {
		tools = append(tools, SandboxToolIDs()...)
	}
	return tools
}

// KnownModule reports whether module is a built-in id.
func KnownModule(module string) bool {
	switch strings.ToLower(strings.TrimSpace(module)) {
	case ModuleDatabase, ModuleS3, ModuleLogs, ModuleGeneral, ModuleSandbox:
		return true
	default:
		return false
	}
}

// ModuleTemplate is the module-layer system prompt.
func ModuleTemplate(module string) string {
	switch strings.ToLower(strings.TrimSpace(module)) {
	case ModuleDatabase:
		return databaseModulePrompt
	case ModuleS3:
		return s3ModulePrompt
	case ModuleLogs:
		return logsModulePrompt
	case ModuleSandbox:
		return sandboxModulePrompt
	default:
		return generalModulePrompt
	}
}

// DefaultToolsForModule returns the default tool ids for the module.
func DefaultToolsForModule(module string) []string {
	for _, m := range Modules(false) {
		if m.ID == module {
			return append([]string(nil), m.DefaultTools...)
		}
	}
	return nil
}

// KnownTool reports whether id is a built-in tool (readonly or sandbox).
func KnownTool(id string) bool {
	switch id {
	case ToolListDatabases, ToolListCollections, ToolReadonlySQL, ToolListObjects, ToolHeadObject, ToolSearchLogs, ToolLogLevelStats:
		return true
	default:
		return IsSandboxTool(id)
	}
}
