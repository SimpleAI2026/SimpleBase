// 文件说明：modules.go — 助手模块 / 内置工具 id 常量、模板与 SkillsForModule

package cloudagent

import "strings"

const (
	ModuleDatabase   = "database"
	ModuleKV         = "kv"
	ModuleGoFunction = "gofunction"
	ModuleCron       = "cron"
	ModuleS3         = "s3"
	ModuleLogs       = "logs"
	ModuleGeneral    = "general"
	ModuleSandbox    = "sandbox"

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

// sandboxToolIDs 是四个云沙盒工具 id（cloud-agent-sandbox-plan §7）。
func sandboxToolIDs() []string {
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
- Built-in data tools are readonly. Never invent INSERT/UPDATE/DELETE/DROP or object uploads with them.
- Never output secrets: S3 keys, provider API keys, passwords, tokens, DSN, or credential_ref values.
- If a tool errors, explain the error; do not fabricate rows or keys.
- Prefer tools over guessing schema or object lists.
- Answer in the user's language.`

// databaseModulePrompt 不再写死「Do not run writes」：能力口径由
// PromptParts.Writable 统一生成（planv6.0 v2 §3.5），模块层只描述职责。
const databaseModulePrompt = `Module: database.
You inspect DuckLake databases in this project.
Use list_databases, list_collections, and readonly_sql (SELECT/WITH/EXPLAIN/DESCRIBE/SHOW only). If CLI skills are enabled, use the database skill for anything beyond that. Do not ATTACH, COPY, PRAGMA, or SET.`

const kvModulePrompt = `Module: kv.
You help manage project-scoped key-value storage and cached statistics.
When CLI skills are enabled, use only the kv skill loaded for this turn. Otherwise explain that KV tools are unavailable; never invent results.`

const goFunctionModulePrompt = `Module: gofunction.
You help manage cloud functions for project-scoped APIs and aggregations.
When CLI skills are enabled, use only the gofunction skill loaded for this turn. Otherwise explain that function tools are unavailable; never invent results.`

const cronModulePrompt = `Module: cron.
You help manage scheduled aggregation jobs in this project.
When CLI skills are enabled, use only the cron skill loaded for this turn. Otherwise explain that cron tools are unavailable; never invent results.`

const s3ModulePrompt = `Module: s3.
You inspect the project's object store.
Use list_objects and head_object. Keys are project-relative (no physical prefix).
The built-in tools cannot upload or delete; if CLI skills are enabled, use the s3 skill. Do not request file bodies.`

const logsModulePrompt = `Module: logs.
You search sys_log_events for this project.
Use search_logs and log_level_stats. Retention changes go through the logs skill when enabled.`

const generalModulePrompt = `Module: general.
You help with database, object storage, and log inspection in this project. Choose tools according to the user's request; list resources before assuming names.
Built-in database, object storage, and log tools are read-only. If CLI skills are enabled, writes go through the simplebase tool and the loaded skills.
If sandbox tools are available, commands and files are isolated in this thread's sandbox workspace. Do not request or expose credentials.`

const sandboxModulePrompt = `Module: sandbox.
Commands and files live only in this thread's sandbox workspace (local process isolation, shared kernel, no network).
sandbox_read_file / sandbox_write_file take absolute paths under /workspace. Inside sandbox_exec / sandbox_shell the current directory is that same workspace, so use relative paths there; the literal path /workspace does not exist in the shell.
Use sandbox_exec (argv command), sandbox_shell (/bin/sh -c), sandbox_read_file, sandbox_write_file.
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
		{ID: ModuleGeneral, Name: "通用助手", Description: "跨数据库、对象存储和日志的项目助手", DefaultTools: generalTools(sandboxAvailable), TeamSupported: false, SandboxAvailable: sandboxAvailable},
		{ID: ModuleDatabase, Name: "Database", Description: "Readonly database inspection and SQL", DefaultTools: []string{ToolListDatabases, ToolListCollections, ToolReadonlySQL}, TeamSupported: false},
		{ID: ModuleKV, Name: "Key-Value", Description: "Project key-value statistics and cache (requires CLI skills)", DefaultTools: []string{}, TeamSupported: false},
		{ID: ModuleGoFunction, Name: "Go Functions", Description: "Project cloud functions and aggregation APIs (requires CLI skills)", DefaultTools: []string{}, TeamSupported: false},
		{ID: ModuleCron, Name: "Scheduled Tasks", Description: "Scheduled aggregation jobs (requires CLI skills)", DefaultTools: []string{}, TeamSupported: false},
		{ID: ModuleS3, Name: "S3", Description: "Readonly object list and head", DefaultTools: []string{ToolListObjects, ToolHeadObject}, TeamSupported: false},
		{ID: ModuleLogs, Name: "Logs", Description: "Search logs and level stats", DefaultTools: []string{ToolSearchLogs, ToolLogLevelStats}, TeamSupported: false},
		{ID: ModuleSandbox, Name: "Sandbox", Description: "Run commands and manage files in an isolated sandbox workspace", DefaultTools: sandboxToolIDs(), TeamSupported: false, SandboxAvailable: sandboxAvailable},
	}
}

// generalTools returns project-scoped read-only tools plus optional sandbox tools.
func generalTools(sandboxAvailable bool) []string {
	tools := []string{ToolListDatabases, ToolListCollections, ToolReadonlySQL, ToolListObjects, ToolHeadObject, ToolSearchLogs, ToolLogLevelStats}
	if sandboxAvailable {
		tools = append(tools, sandboxToolIDs()...)
	}
	return tools
}

// KnownModule reports whether module is a built-in id.
func KnownModule(module string) bool {
	switch strings.ToLower(strings.TrimSpace(module)) {
	case ModuleDatabase, ModuleKV, ModuleGoFunction, ModuleCron, ModuleS3, ModuleLogs, ModuleGeneral, ModuleSandbox:
		return true
	default:
		return false
	}
}

// moduleTemplate is the module-layer system prompt.
func moduleTemplate(module string) string {
	switch strings.ToLower(strings.TrimSpace(module)) {
	case ModuleDatabase:
		return databaseModulePrompt
	case ModuleKV:
		return kvModulePrompt
	case ModuleGoFunction:
		return goFunctionModulePrompt
	case ModuleCron:
		return cronModulePrompt
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

// SkillsForModule 返回某个助手 module 自动加载的 skill id。
//
// 技能归属由助手本身决定，不由用户消息里的 `@` 选择：`@` 只用于点名助手。
// 专用助手加载自己 module 对应的技能；general 与未知 module 不加载技能，
// 只注入顶层索引（与既有的空 skills 行为一致）。
func SkillsForModule(module string) []string {
	switch strings.ToLower(strings.TrimSpace(module)) {
	case ModuleDatabase, ModuleKV, ModuleGoFunction, ModuleCron, ModuleS3, ModuleLogs, ModuleSandbox:
		return []string{strings.ToLower(strings.TrimSpace(module))}
	default:
		return nil
	}
}

// 搭建会话阶段（home-starter-prompts-plan §2.7）。
const (
	StarterStagePlan   = "plan"
	StarterStageData   = "data"
	StarterStageFunc   = "func"
	StarterStageCron   = "cron"
	StarterStageSite   = "site"
	StarterStageVerify = "verify"
)

// starterTemplates 是服务端接受的搭建模板白名单。B1 只开放「数据通道为只读快照」
// 的模板；需要匿名写入的模板（todo / booking / survey）依赖 B3 的公开函数面，
// 在安全评审通过前不接受。
var starterTemplates = map[string]struct{}{
	"dashboard": {},
	"portfolio": {},
	"notice":    {},
}

// IsStarterTemplate 报告 id 是否为服务端允许的搭建模板。
func IsStarterTemplate(id string) bool {
	_, ok := starterTemplates[strings.TrimSpace(id)]
	return ok
}

// starterStageSkills 是各阶段加载的技能（write）与只读技能（readonly）。
// readonly 中的技能同样出现在 skills 里（用于放行命令与注入文档），
// 但执行时只允许只读命令。
var starterStageSkills = map[string]struct{ skills, readonly []string }{
	StarterStagePlan:   {},
	StarterStageData:   {skills: []string{"database"}},
	StarterStageFunc:   {skills: []string{"gofunction", "database"}, readonly: []string{"database"}},
	StarterStageCron:   {skills: []string{"cron", "gofunction"}, readonly: []string{"gofunction"}},
	StarterStageSite:   {skills: []string{"s3", "site"}},
	StarterStageVerify: {skills: []string{"database", "gofunction", "site"}, readonly: []string{"database", "gofunction", "site"}},
}

// StarterStage 返回搭建会话某个阶段要加载的技能与其中只读的技能。
// 技能只由服务端按阶段白名单决定，客户端不能传技能 id；未知阶段 ok=false。
func StarterStage(stage string) (skills, readonly []string, ok bool) {
	spec, ok := starterStageSkills[strings.TrimSpace(stage)]
	if !ok {
		return nil, nil, false
	}
	return append([]string(nil), spec.skills...), append([]string(nil), spec.readonly...), true
}

// StarterStages 按执行顺序返回全部阶段。
func StarterStages() []string {
	return []string{StarterStagePlan, StarterStageData, StarterStageFunc, StarterStageCron, StarterStageSite, StarterStageVerify}
}

const starterModulePrompt = `Build mode: you are building a small application for the user in this project, one stage per turn. The current stage and the skills loaded this turn are listed in the project env.
Rules:
- Do only the current stage. Never run commands that belong to another stage, never trigger a cron job, and never delete any resource.
- Migrations are append-only: use CREATE TABLE IF NOT EXISTS and never DROP or rewrite existing tables.
- A cloud function is a single file with package main; run function test and read its result before you claim success.
- The front end is one web/index.html: inline CSS and JS, hash routing, no build step, no CDN, at most 200KiB, and no key or token of any kind. Upload it with object upload and put the content in stdin. Only the web/ prefix may be published, never the object root.
- The site surface is read-only and has no data API. To show data, write the result to web/data/*.json and read it with a relative fetch.
- If site status reports publishable=false, skip publishing and say why.
- If a command fails, report the error code and stop that step; never invent results.
- In the verify stage, end the final message with exactly one line: <!-- sb:delivery {"site_url":"","functions":[{"name":"","url":"/go/<project_id>/<function>/<export>"}],"tables":[],"collections":[],"cron":[{"name":"","expr":""}],"next":[]} --> filled in only with facts you confirmed through tools.`

// starterPrompt 返回搭建会话在某阶段追加的模块层提示词。
func starterPrompt(stage string) string {
	return starterModulePrompt + "\nCurrent stage: " + strings.TrimSpace(stage) + "."
}
