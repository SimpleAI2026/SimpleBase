// 文件说明：prompt.go — internal/agentbridge 中 PromptParts、AssembleInstruction 等符号的实现
//
// 用法
//
//	type PromptParts struct{…}
//	AssembleInstruction(p PromptParts) string
//	同包调用：truncate() → prompt_test.go, tools.go
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/prompt.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//	[关键引用] PromptParts ← prompt_test.go, runtime_provider.go；AssembleInstruction ← prompt_test.go, runtime_provider.go；truncate ← prompt_test.go, tools.go
//
// 主要符号
//
//	const maxPromptSection
//	type PromptParts struct{…}
//	const platformSkillsPrompt
//	const capabilityWritable / capabilityReadonly
//	AssembleInstruction(p PromptParts) string
//	truncate(s string, max int) string
//	truncateSkill(s string) string

package agentbridge

import (
	"strings"
	"unicode/utf8"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	agentskills "github.com/linkxzhou/SimpleBase/internal/agentbridge/agentskills"
)

const maxPromptSection = 4000

type PromptParts struct {
	ModuleTemplate string
	AgentPrompt    string
	ProjectEnv     string
	Snapshot       string
	SkillsCLI      bool
	SkillText      string
	Writable       bool
}

const platformSkillsPrompt = `You are SimpleBase Cloud Agent, a project-scoped assistant.
Rules:
- Stay in the current project. Never guess another tenant or project.
- Writes go only through the simplebase tool, and only for skills loaded this turn.
- Never invent rows, object keys, or command results.
- Never output secrets: S3 keys, provider API keys, passwords, tokens, DSN, or credential_ref values. Never ask the user to paste a token.
- If a tool errors, explain the error code; do not fabricate a success.
- Answer in the user's language.`

const (
	capabilityWritable = `Capability: writable. You may run write commands for the skills loaded this turn (e.g. sql exec, document insert, object upload, function create). Destructive commands still require user confirmation. Do not claim a write if the tool did not run.`
	capabilityReadonly = `Capability: readonly. Only run read commands. Write commands (sql exec, document insert/update/delete, object upload/delete, function create/update/delete, cron create/update/delete/trigger, kv exec, schema create-table/add-column, database create/delete, log retention set) are rejected with write_not_allowed, before any confirmation prompt. Do not retry or work around it. Tell the user this agent is read-only and that the "Allow write operations" switch in the agent's edit dialog turns writing on.`
)

func AssembleInstruction(p PromptParts) string {
	base := platformBasePrompt
	if p.SkillsCLI {
		base = platformSkillsPrompt
	}
	capability := capabilityReadonly
	if p.SkillsCLI && p.Writable {
		capability = capabilityWritable
	}
	return cloudagent.AssembleInstruction(
		base,
		capability,
		strings.TrimSpace(p.ModuleTemplate),
		truncate(strings.TrimSpace(p.AgentPrompt), maxPromptSection),
		truncateSkill(strings.TrimSpace(p.SkillText)),
		truncate(strings.TrimSpace(p.ProjectEnv), maxPromptSection),
		truncate(strings.TrimSpace(p.Snapshot), maxPromptSection),
	)
}

func truncate(s string, max int) string { return agentcore.Truncate(s, max) }

func truncateSkill(s string) string {
	if utf8.RuneCountInString(s) <= agentskills.MaxChars {
		return s
	}
	return truncate(s, agentskills.MaxChars)
}
