// 文件说明：toolset.go — 按名字组装本机工具集（CLI 与宿主共用，planv6.8 §13.6）
//
// Toolset 不是注册中心：只是把 Workspace / HTTP / Web / Todo / 额外工具按固定顺序拼起来，
// 再按 enabled 过滤。工具顺序稳定，便于提示词与测试断言。

package tools

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
)

// Toolset 汇总一次会话可用的通用工具。
type Toolset struct {
	// Workspace 提供 bash/read/write/edit/glob/grep；nil 表示不提供本机文件与 shell 工具。
	Workspace *Workspace
	// HTTP 提供 http/web_fetch；nil 表示不提供。
	HTTP *HTTPTools
	// Search 提供 web_search；未配置后端时自动跳过。
	Search *SearchClient
	// Todo 提供 todo_write；nil 表示不提供。
	Todo *TodoManager
	// Extra 是调用方追加的工具（plan/explore/reflect、MCP、业务工具），排在最后。
	Extra []tool.BaseTool
}

// Build 返回工具列表；enabled 非空时只保留其中的名字。
func (s Toolset) Build(enabled ...string) ([]tool.BaseTool, error) {
	var out []tool.BaseTool
	if s.Workspace != nil {
		local, err := s.Workspace.LocalTools()
		if err != nil {
			return nil, err
		}
		out = append(out, local...)
	}
	if s.HTTP != nil {
		for _, b := range []func() (tool.BaseTool, error){s.HTTP.HTTPTool, s.HTTP.WebFetchTool} {
			t, err := b()
			if err != nil {
				return nil, err
			}
			out = append(out, t)
		}
	}
	if s.Search != nil {
		t, err := s.Search.SearchTool()
		if err != nil {
			return nil, err
		}
		if t != nil {
			out = append(out, t)
		}
	}
	if s.Todo != nil {
		t, err := s.Todo.Tool()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	out = append(out, s.Extra...)
	if len(enabled) == 0 {
		return out, nil
	}
	want := make(map[string]bool, len(enabled))
	for _, n := range enabled {
		want[n] = true
	}
	filtered := out[:0]
	for _, t := range out {
		info, err := t.Info(context.Background())
		if err == nil && info != nil && want[info.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// Names 返回工具名列表（提示词与调试用）。
func Names(list []tool.BaseTool) []string {
	out := make([]string, 0, len(list))
	for _, t := range list {
		if info, err := t.Info(context.Background()); err == nil && info != nil {
			out = append(out, info.Name)
		}
	}
	return out
}
