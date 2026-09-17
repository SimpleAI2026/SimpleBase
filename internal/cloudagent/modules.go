package cloudagent

import "strings"

const (
	ModuleDatabase = "database"
	ModuleS3       = "s3"
	ModuleLogs     = "logs"
	ModuleGeneral  = "general"

	ToolListDatabases   = "list_databases"
	ToolListCollections = "list_collections"
	ToolReadonlySQL     = "readonly_sql"
	ToolListObjects     = "list_objects"
	ToolHeadObject      = "head_object"
	ToolSearchLogs      = "search_logs"
	ToolLogLevelStats   = "log_level_stats"
)

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
You are a general assistant for this SimpleBase project. You have no extra tools unless the agent config lists them.`

// ModuleInfo is returned by GET /agents/modules.
type ModuleInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	DefaultTools  []string `json:"default_tools"`
	TeamSupported bool     `json:"team_supported"`
}

// Modules returns the built-in module catalog (Team is a Phase 4 stub).
func Modules() []ModuleInfo {
	return []ModuleInfo{
		{ID: ModuleDatabase, Name: "Database", Description: "Readonly database inspection and SQL", DefaultTools: []string{ToolListDatabases, ToolListCollections, ToolReadonlySQL}, TeamSupported: false},
		{ID: ModuleS3, Name: "S3", Description: "Readonly object list and head", DefaultTools: []string{ToolListObjects, ToolHeadObject}, TeamSupported: false},
		{ID: ModuleLogs, Name: "Logs", Description: "Search logs and level stats", DefaultTools: []string{ToolSearchLogs, ToolLogLevelStats}, TeamSupported: false},
		{ID: ModuleGeneral, Name: "General", Description: "No default tools; custom prompt only", DefaultTools: nil, TeamSupported: false},
	}
}

// KnownModule reports whether module is a built-in id.
func KnownModule(module string) bool {
	switch strings.ToLower(strings.TrimSpace(module)) {
	case ModuleDatabase, ModuleS3, ModuleLogs, ModuleGeneral:
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
	default:
		return generalModulePrompt
	}
}

// DefaultToolsForModule returns the default readonly tool ids.
func DefaultToolsForModule(module string) []string {
	for _, m := range Modules() {
		if m.ID == module {
			return append([]string(nil), m.DefaultTools...)
		}
	}
	return nil
}

// KnownTool reports whether id is a Phase-1 readonly tool.
func KnownTool(id string) bool {
	switch id {
	case ToolListDatabases, ToolListCollections, ToolReadonlySQL, ToolListObjects, ToolHeadObject, ToolSearchLogs, ToolLogLevelStats:
		return true
	default:
		return false
	}
}
