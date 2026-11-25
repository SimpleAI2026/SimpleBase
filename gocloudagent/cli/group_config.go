// 文件说明：group_config.go — 本地群定义（~/.gocloudagent/groups/<name>.json，planv6.8 §13.8 CLI 落地）
//
// 用法
//
//	LoadGroup(env, name) (GroupConfig, error)     // 读取并校验一个群定义
//	ListGroups(env) ([]string, error)             // 列出群名（文件名去掉 .json，排序）
//	SaveGroup(env, cfg) (string, error)           // 写入（0600），返回路径
//	ScaffoldGroup(env, name) (string, error)      // 不存在时生成一个单成员样例；已存在则原样返回
//
// 群定义只描述「有哪些成员、各自怎么工作」：成员 = 模型 + 指令 + 工具子集；
// 分工由 gocloudagent/group 的 Plan 栅栏在运行时决定，不在配置里写死。
// 计划文件（plan.md 等）与轮次状态落在 groups/<name>/ 目录（plan.FileStore + group store）。

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// GroupMember 是群里的一个助手成员。
type GroupMember struct {
	// ID 是成员标识，也是 @ 点名时使用的名字（小写字母、数字、-、_）。
	ID string `json:"id"`
	// Instruction 是该成员的系统提示词（职责、工作方式、输出要求）。
	Instruction string `json:"instruction"`
	// Tools 是工具子集（工具名）；空表示使用全部已装配工具。
	Tools []string `json:"tools,omitempty"`
	// Model 覆盖成员使用的模型；空用全局 model。
	Model string `json:"model,omitempty"`
}

// GroupConfig 是一个本地群的定义文件。
type GroupConfig struct {
	Name    string        `json:"name"`
	Members []GroupMember `json:"members"`
	// RoutingPrompt 是群路由提示词（用户数据，只影响分工建议，不授予权限）。
	RoutingPrompt string `json:"routing_prompt,omitempty"`
	// FallbackMember 是模型决策失败时的兜底成员；空表示无兜底。
	FallbackMember string `json:"fallback_member,omitempty"`
	// ConfirmPolicy：auto（默认，模型建议 ∪ 规则）/ always / never（只忽略模型建议，不放过 destructive）。
	ConfirmPolicy string `json:"confirm_policy,omitempty"`
	// MaxReplans 是每轮最大再规划次数；0 取 2，负数关闭。
	MaxReplans int `json:"max_replans,omitempty"`
	// HumanTasks 为 false 时群不产生人工任务（@me 也无效）；nil 视为 true。
	HumanTasks *bool `json:"human_tasks,omitempty"`
}

// ErrGroupInvalid 表示群定义不合法（文件头、成员、字段取值）。
var ErrGroupInvalid = errors.New("group config is invalid")

// ErrGroupNotFound 表示群定义文件不存在。
var ErrGroupNotFound = errors.New("group is not defined")

var groupIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// GroupsDir 返回群定义目录（尊重 GOCLOUDAGENT_HOME）。
func GroupsDir(env []string) string { return filepath.Join(HomeDir(env), "groups") }

// groupPath 返回群定义文件路径；名字非法时返回空串。
func groupPath(env []string, name string) string {
	name = strings.TrimSpace(name)
	if !groupIDPattern.MatchString(name) {
		return ""
	}
	return filepath.Join(GroupsDir(env), name+".json")
}

// GroupStateDir 返回群的运行时目录（计划文件与轮次状态）。
func GroupStateDir(env []string, name string) string {
	return filepath.Join(GroupsDir(env), name)
}

// ListGroups 列出已定义的群名（按名字排序）。目录不存在视为空。
func ListGroups(env []string) ([]string, error) {
	entries, err := os.ReadDir(GroupsDir(env))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(out)
	return out, nil
}

// LoadGroup 读取并校验群定义。文件不存在返回 ErrGroupNotFound；不合法返回 ErrGroupInvalid。
func LoadGroup(env []string, name string) (GroupConfig, error) {
	path := groupPath(env, name)
	if path == "" {
		return GroupConfig{}, fmt.Errorf("%w: %q is not a valid group name", ErrGroupInvalid, name)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return GroupConfig{}, fmt.Errorf("%w: %s (file %s)", ErrGroupNotFound, name, path)
		}
		return GroupConfig{}, err
	}
	var cfg GroupConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return GroupConfig{}, fmt.Errorf("%w: %s: %v", ErrGroupInvalid, path, err)
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = strings.TrimSpace(name)
	}
	if err := cfg.Validate(); err != nil {
		return GroupConfig{}, fmt.Errorf("%w: %s: %v", ErrGroupInvalid, path, err)
	}
	return cfg, nil
}

// Validate 校验群定义：名字合法、至少一个成员、成员 id 唯一且有指令、策略取值合法。
func (g GroupConfig) Validate() error {
	name := strings.TrimSpace(g.Name)
	if !groupIDPattern.MatchString(name) {
		return fmt.Errorf("name %q must match %s", name, groupIDPattern.String())
	}
	if len(g.Members) == 0 {
		return errors.New("at least one member is required")
	}
	seen := make(map[string]bool, len(g.Members))
	for i, m := range g.Members {
		id := strings.TrimSpace(m.ID)
		if !groupIDPattern.MatchString(id) {
			return fmt.Errorf("member %d: id %q must match %s", i+1, id, groupIDPattern.String())
		}
		if seen[id] {
			return fmt.Errorf("member %d: duplicate id %q", i+1, id)
		}
		seen[id] = true
		if strings.TrimSpace(m.Instruction) == "" {
			return fmt.Errorf("member %q: instruction is required", id)
		}
	}
	switch strings.ToLower(strings.TrimSpace(g.ConfirmPolicy)) {
	case "", "auto", "always", "never":
	default:
		return fmt.Errorf("confirm_policy %q must be auto, always or never", g.ConfirmPolicy)
	}
	if g.FallbackMember != "" && !seen[strings.TrimSpace(g.FallbackMember)] {
		return fmt.Errorf("fallback_member %q is not a member", g.FallbackMember)
	}
	return nil
}

// Member 按 id 找成员。
func (g GroupConfig) Member(id string) (GroupMember, bool) {
	for _, m := range g.Members {
		if m.ID == id {
			return m, true
		}
	}
	return GroupMember{}, false
}

// HumanEnabled 报告群是否允许人工任务（默认允许）。
func (g GroupConfig) HumanEnabled() bool { return g.HumanTasks == nil || *g.HumanTasks }

// SaveGroup 写入群定义（0600），返回文件路径。定义必须先通过 Validate。
func SaveGroup(env []string, cfg GroupConfig) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrGroupInvalid, err)
	}
	path := groupPath(env, cfg.Name)
	if path == "" {
		return "", fmt.Errorf("%w: %q is not a valid group name", ErrGroupInvalid, cfg.Name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// ScaffoldGroup 在群不存在时生成一个单成员样例并返回路径与是否新建；
// 已存在时原样返回（created=false），不覆盖用户的定义。
func ScaffoldGroup(env []string, name string) (path string, created bool, err error) {
	name = strings.TrimSpace(name)
	path = groupPath(env, name)
	if path == "" {
		return "", false, fmt.Errorf("%w: %q is not a valid group name", ErrGroupInvalid, name)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		return path, false, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", false, statErr
	}
	sample := GroupConfig{
		Name:           name,
		RoutingPrompt:  "把用户请求拆给最合适的成员；需要多步时按依赖顺序分工。",
		FallbackMember: "assistant",
		Members: []GroupMember{{
			ID:          "assistant",
			Instruction: "You are a careful local coding assistant. Inspect before you change, keep answers short, and cite file:line.",
		}},
	}
	p, err := SaveGroup(env, sample)
	return p, err == nil, err
}
