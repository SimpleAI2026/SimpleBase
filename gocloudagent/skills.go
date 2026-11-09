// 文件说明：skills.go — gocloudagent 中 KnownSkill、SkillPrefixes 等符号的实现
//
// 用法
//
//	KnownSkill(id string) bool  // KnownSkill 报告 id 是否为已登记的子 skill
//	SkillPrefixes(id string) []string  // SkillPrefixes 返回某个 skill 允许的 argv[0]
//	同包调用：AllowArgv() → cli_tool.go；truncateSkill() → prompt.go；SkillPrompt() → runtime.go
//
// 调用架构
//
//	[上游] gocloudagent/cmd/gocloudagent, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/skills.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/skills
//	[同包] access.go, cli_entry.go, cli_runner.go, cli_tool.go, errors.go 等 11 个
//	[关键引用] AllowArgv() ← cli_tool.go
//
// 主要符号
//
//	KnownSkill(id string) bool
//	SkillPrefixes(id string) []string
//	SkillsForModule(module string) []string
//	AllowArgv(skills []string, argv []string) bool
//	KnownSkillIDs() []string
//	SkillBody(id string) string
//	... 另有 14 个顶层符号

package gocloudagent

import (
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"

	builtinSkills "github.com/linkxzhou/SimpleBase/gocloudagent/skills"
)

var skillFS = builtinSkills.Simplebase

const maxSkillChars = 8192

// skillPrefixes 是宿主拒绝表。Markdown 里的 allow 注释必须与这里一致。
// planv6.0 D4：users 下线（助手不再管理用户）；project 收缩为 quota/audit，
// user/apikey/settings 命令保留给人类操作者但不进任何 skill。
var skillPrefixes = map[string][]string{
	"database":   {"database", "sql", "schema", "collection", "document"},
	"kv":         {"kv"},
	"s3":         {"object"},
	"gofunction": {"function"},
	"cron":       {"cron"},
	"sandbox":    {"sandbox"},
	"logs":       {"log"},
	"project":    {"quota", "audit"},
}

// KnownSkill 报告 id 是否为已登记的子 skill。
func KnownSkill(id string) bool {
	_, ok := skillPrefixes[id]
	return ok
}

// SkillPrefixes 返回某个 skill 允许的 argv[0]。未知 id 返回 nil。
func SkillPrefixes(id string) []string {
	src := skillPrefixes[id]
	if src == nil {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
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

// AllowArgv 报告 argv[0] 是否落在已加载 skill 的前缀表内。
func AllowArgv(skills []string, argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	resource := argv[0]
	for _, id := range skills {
		for _, p := range skillPrefixes[id] {
			if p == resource {
				return true
			}
		}
	}
	return false
}

// skillIndexPath 是顶层 skill（含 YAML frontmatter）；子模块文档位于 references/<id>.md。
const skillIndexPath = "simplebase/SKILL.md"

func skillDocPath(id string) string {
	return "simplebase/references/" + id + ".md"
}

// KnownSkillIDs 返回全部已登记子技能的 id（排序后），供宿主做文档对账。
//
// 只暴露 id 列表，不暴露内部前缀表结构：允许的命令前缀仍由 SkillPrefixes
// 按 id 单独查询。
func KnownSkillIDs() []string {
	out := make([]string, 0, len(skillPrefixes))
	for id := range skillPrefixes {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// SkillBody 返回某个子技能的正文（已剥离 frontmatter）。未知 id 返回空串。
func SkillBody(id string) string {
	return readSkill(skillDocPath(id))
}

// SkillPrompt 返回本轮要注入的 skill 正文。空 skills 只有顶层索引。
func SkillPrompt(ids []string) string {
	if len(ids) == 0 {
		return readSkill(skillIndexPath)
	}
	var b strings.Builder
	for _, id := range ids {
		body := readSkill(skillDocPath(id))
		if body == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(body)
	}
	return b.String()
}

// readSkill 读取 embed 文档并去掉 YAML frontmatter（frontmatter 只给 skill 加载器用，不进 prompt）。
func readSkill(path string) string {
	b, err := skillFS.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(stripFrontmatter(string(b)))
}

func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---\n") {
		return s
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		return s
	}
	rest := s[4+end+len("\n---"):]
	return strings.TrimPrefix(rest, "\n")
}

// walkSkillDocs 遍历顶层索引（id="index"）与 references/ 下已登记的子模块文档。
// references/ 中未登记的文件（如 authoring.md）是维护文档，不参与对账。
func walkSkillDocs(fn func(id, body string)) error {
	return fs.WalkDir(skillFS, "simplebase", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		var id string
		switch {
		case path == skillIndexPath:
			id = "index"
		case strings.HasPrefix(path, "simplebase/references/"):
			id = strings.TrimSuffix(strings.TrimPrefix(path, "simplebase/references/"), ".md")
			if !KnownSkill(id) {
				return nil
			}
		default:
			return nil
		}
		body, err := skillFS.ReadFile(path)
		if err != nil {
			return err
		}
		fn(id, string(body))
		return nil
	})
}

// EmbeddedSkillAllows 解析 embed 的 allow 注释，供测试与前缀表对账。
func EmbeddedSkillAllows() (map[string][]string, error) {
	out := map[string][]string{}
	err := walkSkillDocs(func(id, body string) {
		out[id] = parseAllow(body)
	})
	return out, err
}

// EmbeddedSkillReadOnly 解析各模块文档的 `<!-- readonly: a,b -->` 标注
// （planv6.0 v2 §4.2）：供测试与 sbcli.CommandReadOnly 对账，让「文档」与
// 「宿主判定」两个事实来源自动对齐。未标注的 skill 不出现在结果里。
func EmbeddedSkillReadOnly() (map[string][]string, error) {
	out := map[string][]string{}
	err := walkSkillDocs(func(id, body string) {
		if ro := parseCommentList(body, "<!-- readonly:"); len(ro) > 0 {
			out[id] = ro
		}
	})
	return out, err
}

func parseAllow(body string) []string {
	return parseCommentList(body, "<!-- allow:")
}

// parseCommentList 解析形如 `<!-- name: a,b -->` 的注释为排序后的列表。
func parseCommentList(body, marker string) []string {
	i := strings.Index(body, marker)
	if i < 0 {
		return nil
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, "-->")
	if j < 0 {
		return nil
	}
	raw := strings.TrimSpace(rest[:j])
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func truncateSkill(s string) string {
	if utf8.RuneCountInString(s) <= maxSkillChars {
		return s
	}
	return truncate(s, maxSkillChars)
}
