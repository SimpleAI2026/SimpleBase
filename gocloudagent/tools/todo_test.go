// 文件说明：todo_test.go — TodoManager merge 语义、单 in_progress 约束与 todo_write 工具测试（planv6.8 §13.11.1-2）

package tools

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestTodoWriteReplaceAndMerge(t *testing.T) {
	m := &TodoManager{}
	items, err := m.Write([]TodoItem{
		{ID: "1", Content: "explore", Status: TodoCompleted},
		{ID: "2", Content: "implement", Status: TodoInProgress},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Status != TodoCompleted {
		t.Fatalf("items = %+v", items)
	}

	// merge=true：按 id 更新已有项，未知 id 追加，未提及的项保留。
	items, err = m.Write([]TodoItem{
		{ID: "2", Status: TodoCompleted}, // 只改状态
		{ID: "3", Content: "review"},     // 新项默认 pending
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("after merge: %+v", items)
	}
	if items[1].Content != "implement" || items[1].Status != TodoCompleted {
		t.Fatalf("merge must keep content and update status: %+v", items[1])
	}
	if items[2].Status != TodoPending {
		t.Fatalf("new item defaults to pending: %+v", items[2])
	}

	// merge=false：整体替换。
	items, err = m.Write([]TodoItem{{ID: "x", Content: "only"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "x" {
		t.Fatalf("replace: %+v", items)
	}
}

func TestTodoWriteSingleInProgress(t *testing.T) {
	m := &TodoManager{}
	// 两个 in_progress：拒绝，且列表不被修改。
	if _, err := m.Write([]TodoItem{
		{ID: "1", Content: "a", Status: TodoInProgress},
		{ID: "2", Content: "b", Status: TodoInProgress},
	}, false); err == nil || !strings.Contains(err.Error(), "in_progress") {
		t.Fatalf("two in_progress must be rejected, err = %v", err)
	}
	if got := m.Items(); len(got) != 0 {
		t.Fatalf("rejected write must not modify list: %+v", got)
	}
	// 一个 in_progress 允许。
	if _, err := m.Write([]TodoItem{
		{ID: "1", Content: "a", Status: TodoInProgress},
		{ID: "2", Content: "b", Status: TodoPending},
	}, false); err != nil {
		t.Fatal(err)
	}
	// merge 后累计出现第二个 in_progress 也拒绝。
	if _, err := m.Write([]TodoItem{{ID: "2", Status: TodoInProgress}}, true); err == nil {
		t.Fatal("merged second in_progress must be rejected")
	}
	if got := m.Items(); got[1].Status != TodoPending {
		t.Fatalf("rejected merge must not modify status: %+v", got)
	}
}

func TestTodoWriteValidation(t *testing.T) {
	m := &TodoManager{}
	if _, err := m.Write([]TodoItem{{ID: "", Content: "x"}}, false); err == nil || !strings.Contains(err.Error(), "empty id") {
		t.Fatalf("empty id: err = %v", err)
	}
	if _, err := m.Write([]TodoItem{{ID: "1", Content: "x", Status: "weird"}}, false); err == nil || !strings.Contains(err.Error(), "invalid status") {
		t.Fatalf("invalid status: err = %v", err)
	}
	// 新项必须带 content。
	if _, err := m.Write([]TodoItem{{ID: "1"}}, false); err == nil || !strings.Contains(err.Error(), "needs content") {
		t.Fatalf("missing content: err = %v", err)
	}
	// 上限 50 条。
	var many []TodoItem
	for i := 0; i < maxTodos+1; i++ {
		many = append(many, TodoItem{ID: string(rune('a'+i%26)) + itoa(i), Content: "t"})
	}
	if _, err := m.Write(many, false); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("too many todos: err = %v", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestTodoOnChangeSnapshot(t *testing.T) {
	var mu sync.Mutex
	var seen [][]TodoItem
	m := &TodoManager{OnChange: func(items []TodoItem) {
		mu.Lock()
		seen = append(seen, append([]TodoItem(nil), items...))
		mu.Unlock()
	}}
	if _, err := m.Write([]TodoItem{{ID: "1", Content: "a"}}, false); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || len(seen[0]) != 1 {
		t.Fatalf("onChange calls = %+v", seen)
	}
	// 快照与内部状态隔离：改回调里的切片不影响后续读。
	seen[0][0].Content = "mutated"
	if got := m.Items(); got[0].Content != "a" {
		t.Fatalf("snapshot leaked: %+v", got)
	}
}

// todo_write 工具经 JSON 参数往返：替换 / 合并 / 失败信封。
func TestTodoWriteToolRoundtrip(t *testing.T) {
	m := &TodoManager{}
	tl, err := m.Tool()
	if err != nil {
		t.Fatal(err)
	}

	out := invoke(t, tl, `{"todos":[{"id":"1","content":"first"}]}`)
	var res struct {
		OK      bool       `json:"ok"`
		Todos   []TodoItem `json:"todos"`
		Total   int        `json:"total"`
		Current int        `json:"completed"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("out = %s, err = %v", out, err)
	}
	if !res.OK || res.Total != 1 || res.Todos[0].Status != TodoPending {
		t.Fatalf("res = %+v", res)
	}

	// merge 更新状态。
	out = invoke(t, tl, `{"todos":[{"id":"1","status":"in_progress"}],"merge":true}`)
	if !strings.Contains(out, `"in_progress"`) {
		t.Fatalf("merge by id: %s", out)
	}

	// 非法状态 → bad_arguments 信封。
	out = invoke(t, tl, `{"todos":[{"id":"1","status":"zzz"}]}`)
	if !strings.Contains(out, "bad_arguments") {
		t.Fatalf("invalid status envelope: %s", out)
	}
}
