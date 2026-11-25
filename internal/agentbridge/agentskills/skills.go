// 文件说明：skills.go — 内嵌 SimpleBase 技能文档的加载与前缀白名单
//
// 用法
//
//	skills.IDs()                              // 已登记子技能 id（排序后）
//	skills.AllowArgv(ids, argv)               // argv[0] 是否落在已加载技能的前缀表内
//	skills.Prompt(ids)                        // 注入 prompt 的正文（已剥离 frontmatter）
//
// 文档注释解析（allow / readonly 对账）只在 skills_test.go 中，不属于生产 API。
//
// 文档读取委托通用 skills.ReadDocument；module → skill 的映射、命令白名单由本包与父包 agentbridge 负责。

package skills

import (
	"sort"
	"strings"

	coreskills "github.com/linkxzhou/SimpleBase/gocloudagent/skills"
)

var skillFS = Simplebase

// MaxChars 是单次注入 prompt 的技能正文字符上限（超限会被截断，尾部示例丢失）。
const MaxChars = 8192

// skillPrefixes 是宿主拒绝表。Markdown 里的 allow 注释必须与这里一致。
// planv6.0 D4：users 下线（助手不再管理用户）；project 收缩为 quota/audit，
// user/apikey/settings 命令保留给人类操作者但不进任何 skill。
var skillPrefixes = map[string][]string{
	"database":   {"database", "sql", "schema", "collection", "document"},
	"kv":         {"kv"},
	"s3":         {"object"},
	"site":       {"site"},
	"gofunction": {"function"},
	"cron":       {"cron"},
	"sandbox":    {"sandbox"},
	"logs":       {"log"},
	"project":    {"quota", "audit"},
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

// IDs 返回全部已登记子技能的 id（排序后），供宿主做文档对账。
// 只暴露 id 列表，不暴露内部前缀表结构。
func IDs() []string {
	out := make([]string, 0, len(skillPrefixes))
	for id := range skillPrefixes {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Prompt 返回本轮要注入的 skill 正文。空 skills 只有顶层索引。
func Prompt(ids []string) string {
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

// readSkill 读取 embed 文档（去 frontmatter）；读取失败视为文档缺失，返回空串。
func readSkill(path string) string {
	body, err := coreskills.ReadDocument(skillFS, path)
	if err != nil {
		return ""
	}
	return body
}
