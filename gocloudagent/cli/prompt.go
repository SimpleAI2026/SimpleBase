// 文件说明：prompt.go — gocloudagent/cli 中 PromptTemplate、PromptsDir 等符号的实现
//
// 用法
//
//	PromptsDir(env []string) string// PromptsDir 返回模板目录
//	LoadPromptTemplates(env []string) (map[string]PromptTemplate, error)  // LoadPromptTemplates 读取全部模板
//	同包调用：PromptTemplate → repl.go；LoadPromptTemplates() → repl.go；ExpandSlashCommand() → repl.go
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/prompt.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] client.go, command.go, compaction.go, config.go, repeat_detector.go 等 9 个
//	[关键引用] PromptTemplate ← repl.go
//
// 主要符号
//
//	type PromptTemplate struct {…}
//	PromptsDir(env []string) string
//	LoadPromptTemplates(env []string) (map[string]PromptTemplate, error)
//	ExpandPromptTemplate(tpl PromptTemplate, args []string) string
//	PromptTemplateNames(tpls map[string]PromptTemplate) []string
//	ExpandSlashCommand(tpls map[string]PromptTemplate, line string) (string, bool)
//	... 另有 3 个顶层符号

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PromptTemplate 是 ~/.gocloudagent/prompts/*.md 定义的一个提示词模板（§13.7）。
//
// 与 skill 的严格区分：`/name` 只做文本展开，不授予任何写权限；
// `@skill` 才是能力授权（走 AllowArgv 前缀集合）。两者不混用。
type PromptTemplate struct {
	Name         string
	Description  string
	ArgumentHint string
	Content      string
}

// promptArgRe 匹配 $1..$9、$@ 与 ${N:-默认值}。
var promptArgRe = regexp.MustCompile(`\$\{(\d+):-([^}]*)\}|\$(\d+)|\$@`)

// PromptsDir 返回模板目录。
func PromptsDir(env []string) string { return filepath.Join(HomeDir(env), "prompts") }

// LoadPromptTemplates 读取全部模板；目录不存在返回空（不是错误）。
func LoadPromptTemplates(env []string) (map[string]PromptTemplate, error) {
	dir := PromptsDir(env)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]PromptTemplate{}, nil
		}
		return nil, err
	}
	out := make(map[string]PromptTemplate, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".md")
		b, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			continue
		}
		body, meta := splitFrontmatter(string(b))
		name := meta["name"]
		if name == "" {
			name = base
		}
		out[name] = PromptTemplate{
			Name:         name,
			Description:  meta["description"],
			ArgumentHint: meta["argument-hint"],
			Content:      strings.TrimSpace(body),
		}
		if name != base {
			// 文件名也能唤起（`/review` → review.md，即使 frontmatter 里换了名）。
			if _, exists := out[base]; !exists {
				out[base] = out[name]
			}
		}
	}
	return out, nil
}

// splitFrontmatter 拆出 YAML frontmatter（只支持 `key: value`，不引 yaml 依赖）。
func splitFrontmatter(s string) (body string, meta map[string]string) {
	meta = map[string]string{}
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return s, meta
	}
	rest := s[strings.Index(s, "\n")+1:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return s, meta
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.TrimSpace(strings.ToLower(key))] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	after := rest[end+1:]
	if nl := strings.Index(after, "\n"); nl >= 0 {
		after = after[nl+1:]
	} else {
		after = ""
	}
	return after, meta
}

// ExpandPromptTemplate 用参数展开模板：$1..$9、$@（全部参数）、${N:-默认值}。
// 不做嵌套、不做表达式——与 Pi 的 substituteArgs 同范围。
func ExpandPromptTemplate(tpl PromptTemplate, args []string) string {
	return promptArgRe.ReplaceAllStringFunc(tpl.Content, func(m string) string {
		if m == "$@" {
			return strings.Join(args, " ")
		}
		if strings.HasPrefix(m, "${") {
			inner := strings.TrimSuffix(strings.TrimPrefix(m, "${"), "}")
			idxPart, def, _ := strings.Cut(inner, ":-")
			n, err := strconv.Atoi(idxPart)
			if err != nil || n <= 0 || n > len(args) || strings.TrimSpace(args[n-1]) == "" {
				return def
			}
			return args[n-1]
		}
		n, err := strconv.Atoi(strings.TrimPrefix(m, "$"))
		if err != nil || n <= 0 || n > len(args) {
			return ""
		}
		return args[n-1]
	})
}

// PromptTemplateNames 返回排序后的模板名（供帮助与补全）。
func PromptTemplateNames(tpls map[string]PromptTemplate) []string {
	names := make([]string, 0, len(tpls))
	for name := range tpls {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ExpandSlashCommand 处理 REPL 里的 `/name args`。
//
// 返回 (展开后的文本, 是否已处理)。非斜杠输入原样返回 false。
// 导出是因为父包需要断言「展开模板不授予任何权限」（§13.7）。
func ExpandSlashCommand(tpls map[string]PromptTemplate, line string) (string, bool) {
	if !strings.HasPrefix(line, "/") {
		return line, false
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return line, false
	}
	name := strings.TrimPrefix(fields[0], "/")
	tpl, ok := tpls[name]
	if !ok {
		return line, false
	}
	return ExpandPromptTemplate(tpl, fields[1:]), true
}

// promptTemplateHelp 渲染 `/<name> <argument-hint>` 列表。
func promptTemplateHelp(tpls map[string]PromptTemplate) string {
	if len(tpls) == 0 {
		return ""
	}
	var b strings.Builder
	for _, name := range PromptTemplateNames(tpls) {
		tpl := tpls[name]
		fmt.Fprintf(&b, "/%s", name)
		if tpl.ArgumentHint != "" {
			fmt.Fprintf(&b, " %s", tpl.ArgumentHint)
		}
		if tpl.Description != "" {
			fmt.Fprintf(&b, "  %s", tpl.Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}
