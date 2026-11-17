// 文件说明：skills.go — 内嵌 SimpleBase 技能文档的加载、前缀白名单与对账解析
//
// 用法
//
//	skills.Known(id) / Prefixes(id) / IDs()  // 已登记子技能与允许的 argv[0]
//	skills.AllowArgv(ids, argv)               // argv[0] 是否落在已加载技能的前缀表内
//	skills.Prompt(ids) / Body(id)             // 注入 prompt 的正文（已剥离 frontmatter）
//	skills.EmbeddedAllows() / EmbeddedReadOnly()  // 文档注释解析，供对账测试
//
// 本包不依赖 cloudagent：module → skill 的映射（SkillsForModule）留在 cloudagent。

package skills

import (
	"io/fs"
	"sort"
	"strings"
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

// Known 报告 id 是否为已登记的子 skill。
func Known(id string) bool {
	_, ok := skillPrefixes[id]
	return ok
}

// Prefixes 返回某个 skill 允许的 argv[0]。未知 id 返回 nil。
// 对账测试专用（internal/agentbridge/skills_test.go）：生产判定走 AllowArgv。
func Prefixes(id string) []string {
	src := skillPrefixes[id]
	if src == nil {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
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

// AllowWriteArgv 报告 argv[0] 是否落在「可写技能」的前缀表内：即已加载技能中
// 去掉 readonly 子集之后仍包含该前缀。搭建会话的阶段限权用它判定一条写命令
// 是否属于当前阶段（例如 func 阶段里 database 只读，sql exec 不属于本阶段）。
func AllowWriteArgv(skills, readonly []string, argv []string) bool {
	if len(readonly) == 0 {
		return AllowArgv(skills, argv)
	}
	writable := make([]string, 0, len(skills))
	for _, id := range skills {
		ro := false
		for _, r := range readonly {
			if r == id {
				ro = true
				break
			}
		}
		if !ro {
			writable = append(writable, id)
		}
	}
	return AllowArgv(writable, argv)
}

// skillIndexPath 是顶层 skill（含 YAML frontmatter）；子模块文档位于 references/<id>.md。
const skillIndexPath = "simplebase/SKILL.md"

func skillDocPath(id string) string {
	return "simplebase/references/" + id + ".md"
}

// IDs 返回全部已登记子技能的 id（排序后），供宿主做文档对账。
// 对账测试专用（internal/agentbridge/skills_test.go）：生产运行时按需加载
// 单个 skill，不枚举全表。
//
// 只暴露 id 列表，不暴露内部前缀表结构：允许的命令前缀仍由 Prefixes
// 按 id 单独查询。
func IDs() []string {
	out := make([]string, 0, len(skillPrefixes))
	for id := range skillPrefixes {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Body 返回某个子技能的正文（已剥离 frontmatter）。未知 id 返回空串。
// 对账测试专用（internal/agentbridge/skills_test.go）：生产 prompt 组装走
// Prompt 批量拼接。
func Body(id string) string {
	return readSkill(skillDocPath(id))
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
			if !Known(id) {
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

// EmbeddedAllows 解析 embed 的 allow 注释，供测试与前缀表对账。
func EmbeddedAllows() (map[string][]string, error) {
	out := map[string][]string{}
	err := walkSkillDocs(func(id, body string) {
		out[id] = parseAllow(body)
	})
	return out, err
}

// EmbeddedReadOnly 解析各模块文档的 `<!-- readonly: a,b -->` 标注
// （planv6.0 v2 §4.2）：供测试与 sbcli.CommandReadOnly 对账，让「文档」与
// 「宿主判定」两个事实来源自动对齐。未标注的 skill 不出现在结果里。
// 对账测试专用（internal/agentbridge/skills_test.go）：生产只读判定走
// sbcli.CommandReadOnly。
func EmbeddedReadOnly() (map[string][]string, error) {
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
