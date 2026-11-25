// 文件说明：todo.go — TodoManager 与 todo_write 工具（planv6.8 §13.6）
//
// 会话内的轻量任务清单：模型用 todo_write 整体替换或按 id 合并；最多一个 in_progress。
// 持久化由调用方通过 OnChange 回调决定（CLI 写会话 JSONL，云端写轮次事件）。

package tools

import (
	"context"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
)

// Todo 状态。
const (
	TodoPending    = "pending"
	TodoInProgress = "in_progress"
	TodoCompleted  = "completed"
	TodoCancelled  = "cancelled"
)

// TodoItem 是一条待办。
type TodoItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

// TodoManager 持有会话的待办列表（并发安全）。
type TodoManager struct {
	mu    sync.Mutex
	items []TodoItem
	// OnChange 在每次成功写入后以快照回调；nil 表示不持久化。
	OnChange func(items []TodoItem)
}

// maxTodos 限制列表长度，避免模型把它当日志用。
const maxTodos = 50

// Items 返回当前列表快照。
func (m *TodoManager) Items() []TodoItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]TodoItem(nil), m.items...)
}

// Write 应用一次写入：merge=false 整体替换；merge=true 按 id 更新已有项、追加新项。
func (m *TodoManager) Write(items []TodoItem, merge bool) ([]TodoItem, error) {
	for i := range items {
		items[i].ID = strings.TrimSpace(items[i].ID)
		items[i].Content = strings.TrimSpace(items[i].Content)
		items[i].Status = strings.TrimSpace(items[i].Status)
		if items[i].ID == "" {
			return nil, Failf("bad_arguments", "todo %d has empty id", i+1)
		}
	}
	m.mu.Lock()
	next := make([]TodoItem, 0, len(m.items)+len(items))
	if merge {
		next = append(next, m.items...)
	}
	for _, it := range items {
		idx := -1
		for j := range next {
			if next[j].ID == it.ID {
				idx = j
				break
			}
		}
		if idx < 0 {
			if it.Content == "" {
				m.mu.Unlock()
				return nil, Failf("bad_arguments", "todo %q needs content", it.ID)
			}
			if it.Status == "" {
				it.Status = TodoPending
			}
			next = append(next, it)
			continue
		}
		if it.Content != "" {
			next[idx].Content = it.Content
		}
		if it.Status != "" {
			next[idx].Status = it.Status
		}
	}
	inProgress := 0
	for _, it := range next {
		switch it.Status {
		case TodoPending, TodoCompleted, TodoCancelled:
		case TodoInProgress:
			inProgress++
		default:
			m.mu.Unlock()
			return nil, Failf("bad_arguments", "todo %q has invalid status %q", it.ID, it.Status)
		}
	}
	if inProgress > 1 {
		m.mu.Unlock()
		return nil, Failf("bad_arguments", "only one todo may be in_progress")
	}
	if len(next) > maxTodos {
		m.mu.Unlock()
		return nil, Failf("too_many", "at most %d todos", maxTodos)
	}
	m.items = next
	snapshot := append([]TodoItem(nil), next...)
	cb := m.OnChange
	m.mu.Unlock()
	if cb != nil {
		cb(snapshot)
	}
	return snapshot, nil
}

type todoWriteArgs struct {
	Todos []TodoItem `json:"todos" jsonschema:"description=todo items; status is pending|in_progress|completed|cancelled"`
	Merge bool       `json:"merge,omitempty" jsonschema:"description=true to update items by id and keep the rest; false replaces the whole list"`
}

// Tool 返回 todo_write 工具。
func (m *TodoManager) Tool() (tool.BaseTool, error) {
	return Exec("todo_write",
		"Create and maintain a short task list for the current session (3-7 coarse steps). Use merge=true to update statuses by id. At most one item may be in_progress. This only records progress; it does not execute anything.",
		func(_ context.Context, in todoWriteArgs) (any, error) {
			items, err := m.Write(in.Todos, in.Merge)
			if err != nil {
				return nil, err
			}
			done := 0
			for _, it := range items {
				if it.Status == TodoCompleted {
					done++
				}
			}
			return map[string]any{"ok": true, "todos": items, "completed": done, "total": len(items)}, nil
		})
}
