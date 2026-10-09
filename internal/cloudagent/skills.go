package cloudagent

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"
)

//go:embed skills/simplebase
var skillFS embed.FS

const maxSkillChars = 8192

// skillPrefixes 是宿主拒绝表。Markdown 里的 allow 注释必须与这里一致。
var skillPrefixes = map[string][]string{
	"database":   {"database", "sql", "schema", "collection", "document"},
	"kv":         {"kv"},
	"s3":         {"object"},
	"gofunction": {"function"},
	"cron":       {"cron"},
	"sandbox":    {"sandbox"},
	"logs":       {"log"},
	"users":      {"user"},
	"project":    {"project", "apikey", "settings", "quota", "audit"},
}

// skillAliases 中文精确匹配；拉丁别名按小写登记。
var skillAliases = map[string]string{
	"数据库": "database", "database": "database", "db": "database",
	"键值": "kv", "kv": "kv",
	"对象存储": "s3", "s3": "s3", "对象": "s3",
	"云函数": "gofunction", "函数": "gofunction", "gofunction": "gofunction",
	"定时任务": "cron", "定时": "cron", "cron": "cron",
	"沙盒": "sandbox", "sandbox": "sandbox",
	"日志": "logs", "logs": "logs",
	"用户": "users", "users": "users",
	"项目": "project", "project": "project",
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

// ResolveAtToken 把一个 @ token 分成 skill 或助手名。
// 精确命中别名（含「数据库」）时 skill 优先；精确命中助手名（如 Database）时不算 skill。
func ResolveAtToken(token string, agentNames []string) (skillID string, isAgent bool) {
	token = strings.TrimSpace(strings.TrimPrefix(token, "@"))
	if token == "" {
		return "", false
	}
	if id, ok := skillAliases[token]; ok {
		return id, false
	}
	for _, name := range agentNames {
		if name == token {
			return "", true
		}
	}
	if id, ok := skillAliases[strings.ToLower(token)]; ok {
		return id, false
	}
	return "", false
}

// NormalizeSkills 校验本轮 skills。未知 id 返回错误。空列表时按助手 module 回落。
func NormalizeSkills(ids []string, module string) ([]string, error) {
	if len(ids) == 0 {
		switch module {
		case ModuleDatabase, ModuleS3, ModuleLogs, ModuleSandbox:
			return []string{module}, nil
		default:
			return nil, nil
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(strings.ToLower(id))
		if id == "" {
			continue
		}
		if !KnownSkill(id) {
			return nil, fmt.Errorf("unknown_skill")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
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

// SkillPrompt 返回本轮要注入的 skill 正文。空 skills 只有顶层索引。
func SkillPrompt(ids []string) string {
	if len(ids) == 0 {
		return readSkill("skills/simplebase/SKILL.md")
	}
	var b strings.Builder
	for _, id := range ids {
		body := readSkill("skills/simplebase/" + id + "/SKILL.md")
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

func readSkill(path string) string {
	b, err := skillFS.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// EmbeddedSkillAllows 解析 embed 的 allow 注释，供测试与前缀表对账。
func EmbeddedSkillAllows() (map[string][]string, error) {
	out := map[string][]string{}
	err := fs.WalkDir(skillFS, "skills/simplebase", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "SKILL.md") {
			return err
		}
		body, err := skillFS.ReadFile(path)
		if err != nil {
			return err
		}
		allow := parseAllow(string(body))
		id := "index"
		if path != "skills/simplebase/SKILL.md" {
			id = strings.TrimSuffix(strings.TrimPrefix(path, "skills/simplebase/"), "/SKILL.md")
		}
		out[id] = allow
		return nil
	})
	return out, err
}

func parseAllow(body string) []string {
	const marker = "<!-- allow:"
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
