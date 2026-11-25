// 文件说明：skills_test.go — internal/agentbridge/agentskills 的单元测试（TestDatabaseSkillUsesSupportedDataModel、TestSkillIndexFrontmatterStripped 等）
//
// 用法
//
//	go test ./internal/agentbridge/agentskills -run 'TestDatabaseSkillUsesSupportedDataModel|TestSkillIndexFrontmatterStripped|TestSkillBodiesHaveReasonableSize|TestEmbeddedAllowsMatchPrefixes'
//
// 调用架构
//
//	[上游] internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/agentskills/skills_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] embed.go, skills.go
//
// 主要符号
//
//	TestDatabaseSkillUsesSupportedDataModel(t *testing.T)
//	TestSkillIndexFrontmatterStripped(t *testing.T)
//	TestSkillBodiesHaveReasonableSize(t *testing.T)
//	TestEmbeddedAllowsMatchPrefixes(t *testing.T)
//	Known(id string) bool
//	Prefixes(id string) []string
//	Body(id string) string
//	walkSkillDocs(fn func(id, body string)) error
//	... 另有 7 个顶层符号

package skills

import (
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/sbcli"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

func TestDatabaseSkillUsesSupportedDataModel(t *testing.T) {
	body := readSkill(skillDocPath("database"))
	if !strings.Contains(body, "--data-model sql") || strings.Contains(body, "--data-model relational") {
		t.Fatalf("database create 示例必须使用 sql: %s", body)
	}
}

// 顶层 SKILL.md 带 frontmatter（skill-creator 规范），注入 prompt 时必须剥离。
func TestSkillIndexFrontmatterStripped(t *testing.T) {
	raw, err := skillFS.ReadFile(skillIndexPath)
	if err != nil || !strings.HasPrefix(string(raw), "---\nname: simplebase\n") {
		t.Fatalf("index frontmatter missing: err=%v", err)
	}
	body := Prompt(nil)
	if strings.HasPrefix(body, "---") || strings.Contains(body, "description:") || !strings.HasPrefix(body, "# SimpleBase") {
		t.Fatalf("frontmatter leaked into prompt: %.80q", body)
	}
}

// planv6.0 v2 §4.3：每份 SKILL.md 的体积下限与上限。
// 下限防止「没有写 flag 与示例」的薄文本回归；上限与 MaxChars 对齐，
// 超限会被截断导致尾部示例丢失。
func TestSkillBodiesHaveReasonableSize(t *testing.T) {
	const minBytes, maxBytes = 600, MaxChars
	if got := len(readSkill(skillIndexPath)); got < 300 || got > maxBytes {
		t.Fatalf("index size=%d", got)
	}
	for id := range skillPrefixes {
		body := readSkill(skillDocPath(id))
		if n := len(body); n < minBytes || n > maxBytes {
			t.Fatalf("skill %s size=%d want %d..%d", id, n, minBytes, maxBytes)
		}
	}
}

func TestEmbeddedAllowsMatchPrefixes(t *testing.T) {
	if Prompt(nil) == "" {
		t.Fatal("embedded skill index is empty")
	}
	allows, err := EmbeddedAllows()
	if err != nil {
		t.Fatal(err)
	}
	if len(allows) != len(skillPrefixes)+1 {
		t.Fatalf("embedded skills=%d want %d", len(allows), len(skillPrefixes)+1)
	}
	if _, ok := allows["index"]; !ok {
		t.Fatal("embedded skill index missing")
	}
	if len(allows["index"]) != 0 {
		t.Fatalf("index allow %v", allows["index"])
	}
	for id, want := range skillPrefixes {
		got := append([]string(nil), allows[id]...)
		exp := append([]string(nil), want...)
		sort.Strings(got)
		sort.Strings(exp)
		if !reflect.DeepEqual(got, exp) {
			t.Fatalf("skill %s allow %v want %v", id, got, exp)
		}
	}
}

// ── 对账辅助：只服务于测试，不属于生产 API ──

// Known 报告 id 是否为已登记的子 skill。
func Known(id string) bool {
	_, ok := skillPrefixes[id]
	return ok
}

// Prefixes 返回某个 skill 允许的 argv[0]。未知 id 返回 nil。
func Prefixes(id string) []string {
	src := skillPrefixes[id]
	if src == nil {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// Body 返回某个子技能的正文（已剥离 frontmatter）。未知 id 返回空串。
func Body(id string) string {
	return readSkill(skillDocPath(id))
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
	out := strutil.SplitCSV(raw)
	sort.Strings(out)
	return out
}

// 技能文档里的 `<!-- readonly: … -->` 标注必须与宿主 sbcli 判定一致，
// 否则模型会按文档把写命令当成只读。
func TestSkillReadOnlyAnnotationsMatchSbcli(t *testing.T) {
	annotations, err := EmbeddedReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := annotations["index"]; ok {
		t.Fatal("索引不应有 readonly 标注")
	}
	if _, ok := annotations["kv"]; ok {
		t.Fatal("kv 不应对 readonly 做任何声明")
	}
	for id, cmds := range annotations {
		allows := Prefixes(id)
		for _, cmd := range cmds {
			parts := strings.SplitN(cmd, " ", 2)
			if len(parts) != 2 {
				t.Fatalf("skill %s 的 readonly 项格式错误: %q", id, cmd)
			}
			allowed := false
			for _, p := range allows {
				if p == parts[0] {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Fatalf("skill %s 标注了不属于自己的命令 %q", id, cmd)
			}
			if !sbcli.CommandReadOnlyWithArgv(parts) {
				t.Fatalf("skill %s 标注 %q 为只读，但 sbcli 判定为写", id, cmd)
			}
		}
	}

	for _, id := range IDs() {
		body := Body(id)
		declared := map[string]bool{}
		for _, c := range annotations[id] {
			declared[c] = true
		}
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "simplebase ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 3 || !sbcli.CommandReadOnly(fields[1], fields[2]) {
				continue
			}
			if !declared[fields[1]+" "+fields[2]] {
				t.Fatalf("skill %s 的只读命令 %q 未在 readonly 标注中登记", id, fields[1]+" "+fields[2])
			}
		}
	}
}

func TestSkillCommandsDispatchable(t *testing.T) {
	for _, id := range IDs() {
		body := Body(id)
		if body == "" {
			t.Fatalf("skill %s 正文缺失", id)
		}
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "simplebase ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 3 && !sbcli.DispatchKnown(fields[1], fields[2]) {
				t.Fatalf("skill %s 的示例无法分发: %q", id, line)
			}
		}
	}
}

func TestUsersIsNotAKnownSkill(t *testing.T) {
	if Known("users") {
		t.Fatal("users is not a delegable skill")
	}
}
