// 文件说明：store.go — 计划持久化（planning-with-files 布局）
//
// Store 是宿主端口：CLI 用 FileStore 写本地文件，云端由宿主实现（写轮次 route_json 与事件表），
// 测试与库调用可用 MemoryStore。
//
// FileStore 布局（根目录由调用方指定）：
//   plan.md        goal、steps checklist、当前分工、确认记录
//   progress.md    时间线：每次 update 的进度与任务结果摘要
//   findings.md    findings 去重追加
//   facts.md       reflect 抽取的持久事实
//   explore.jsonl  explore 原始记录
//   decision.json  最新 Decision（机器可读，Load 的来源）
// 写入采用「临时文件 + rename」；单文件上限超出时丢弃最旧的条目。

package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ExploreNote 是 explore 的一条记录。
type ExploreNote struct {
	Goal      string        `json:"goal"`
	Questions []string      `json:"questions,omitempty"`
	Reads     []ExploreRead `json:"reads,omitempty"`
	Findings  []string      `json:"findings,omitempty"`
	Unknowns  []string      `json:"unknowns,omitempty"`
	At        time.Time     `json:"at"`
}

// ExploreRead 是 explore 记录里的一个阅读目标。
type ExploreRead struct {
	Path   string `json:"path"`
	Symbol string `json:"symbol,omitempty"`
	Why    string `json:"why,omitempty"`
}

// Store 是计划持久化端口。实现必须并发安全。
type Store interface {
	SaveDecision(ctx context.Context, d Decision) error
	LoadDecision(ctx context.Context) (Decision, bool, error)
	AppendProgress(ctx context.Context, line string) error
	AppendFindings(ctx context.Context, findings []string) error
	AppendFacts(ctx context.Context, facts []string) error
	AppendExplore(ctx context.Context, note ExploreNote) error
	Explores(ctx context.Context) ([]ExploreNote, error)
	Facts(ctx context.Context) ([]string, error)
}

// MemoryStore 是进程内实现（测试、库调用、云端单轮 reflect 暂存）。
type MemoryStore struct {
	mu       sync.Mutex
	decision *Decision
	progress []string
	findings []string
	facts    []string
	explores []ExploreNote
}

func (m *MemoryStore) SaveDecision(_ context.Context, d Decision) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := d
	m.decision = &cp
	return nil
}

func (m *MemoryStore) LoadDecision(context.Context) (Decision, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.decision == nil {
		return Decision{}, false, nil
	}
	return *m.decision, true, nil
}

func (m *MemoryStore) AppendProgress(_ context.Context, line string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress = append(m.progress, line)
	return nil
}

func (m *MemoryStore) AppendFindings(_ context.Context, findings []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.findings = appendUnique(m.findings, findings)
	return nil
}

func (m *MemoryStore) AppendFacts(_ context.Context, facts []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.facts = appendUnique(m.facts, facts)
	return nil
}

func (m *MemoryStore) AppendExplore(_ context.Context, note ExploreNote) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.explores = append(m.explores, note)
	return nil
}

func (m *MemoryStore) Explores(context.Context) ([]ExploreNote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ExploreNote(nil), m.explores...), nil
}

func (m *MemoryStore) Facts(context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.facts...), nil
}

// Progress 返回进度时间线（测试与展示用）。
func (m *MemoryStore) Progress() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.progress...)
}

// Findings 返回去重后的发现。
func (m *MemoryStore) Findings() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.findings...)
}

func appendUnique(dst, add []string) []string {
	seen := make(map[string]bool, len(dst)+len(add))
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range add {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		dst = append(dst, s)
	}
	return dst
}

// FileStore 是 planning-with-files 布局的本地实现。
type FileStore struct {
	// Dir 是计划目录（不存在时自动创建，权限 0700）。
	Dir string
	// MaxFileBytes 是单文件上限；0 取 256 KiB。
	MaxFileBytes int
	// Now 便于测试固定时间；nil 用 time.Now。
	Now func() time.Time

	mu sync.Mutex
}

const defaultMaxFileBytes = 256 << 10

func (f *FileStore) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *FileStore) limit() int {
	if f.MaxFileBytes > 0 {
		return f.MaxFileBytes
	}
	return defaultMaxFileBytes
}

func (f *FileStore) path(name string) (string, error) {
	if strings.TrimSpace(f.Dir) == "" {
		return "", errors.New("plan: file store dir is empty")
	}
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(f.Dir, name), nil
}

// writeAtomic 用临时文件 + rename 写入。
func (f *FileStore) writeAtomic(name string, data []byte) error {
	p, err := f.path(name)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+name+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// appendBounded 追加若干行；超出上限时丢弃最旧的行（保留文件头部第一行标题）。
// 单条新记录自身超限必须报错，不能把它整行丢弃后返回成功。
func (f *FileStore) appendBounded(name, header string, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	p, err := f.path(name)
	if err != nil {
		return err
	}
	old, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	limit := f.limit()
	head := ""
	if header != "" {
		head = header + "\n\n"
	}
	for _, line := range lines {
		if len(head)+len(line)+1 > limit {
			return fmt.Errorf("%w: %s record is %d bytes, limit is %d", ErrTooLarge, name, len(line)+1, limit)
		}
	}
	content := string(old)
	if content == "" {
		content = head
	}
	for _, line := range lines {
		content += line + "\n"
	}
	if len(content) > limit {
		body := content[len(head):]
		for len(head)+len(body) > limit {
			i := strings.IndexByte(body, '\n')
			if i < 0 {
				return fmt.Errorf("%w: %s has no complete record within %d bytes", ErrTooLarge, name, limit)
			}
			body = body[i+1:]
		}
		content = head + body
	}
	return f.writeAtomic(name, []byte(content))
}

func (f *FileStore) SaveDecision(_ context.Context, d Decision) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	markdown := []byte(RenderMarkdown(d))
	if len(raw) > f.limit() {
		return fmt.Errorf("%w: decision.json is %d bytes, limit is %d", ErrTooLarge, len(raw), f.limit())
	}
	if len(markdown) > f.limit() {
		return fmt.Errorf("%w: plan.md is %d bytes, limit is %d", ErrTooLarge, len(markdown), f.limit())
	}
	if err := f.writeAtomic("decision.json", raw); err != nil {
		return err
	}
	if err := f.writeAtomic("plan.md", markdown); err != nil {
		return err
	}
	if len(d.Findings) > 0 {
		return f.appendFindingsLocked(d.Findings)
	}
	return nil
}

func (f *FileStore) LoadDecision(context.Context) (Decision, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, err := f.path("decision.json")
	if err != nil {
		return Decision{}, false, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return Decision{}, false, nil
	}
	if err != nil {
		return Decision{}, false, err
	}
	var d Decision
	if err := json.Unmarshal(raw, &d); err != nil {
		return Decision{}, false, err
	}
	return d, true, nil
}

func (f *FileStore) AppendProgress(_ context.Context, line string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.appendBounded("progress.md", "# Progress", []string{
		fmt.Sprintf("- %s %s", f.now().UTC().Format(time.RFC3339), strings.TrimSpace(line)),
	})
}

func (f *FileStore) AppendFindings(_ context.Context, findings []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.appendFindingsLocked(findings)
}

func (f *FileStore) appendFindingsLocked(findings []string) error {
	return f.appendUniqueLines("findings.md", "# Findings", findings)
}

func (f *FileStore) AppendFacts(_ context.Context, facts []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.appendUniqueLines("facts.md", "# Facts", facts)
}

func (f *FileStore) appendUniqueLines(name, header string, items []string) error {
	existing, err := f.readBullets(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	seen := make(map[string]bool, len(existing))
	for _, e := range existing {
		seen[e] = true
	}
	var lines []string
	for _, it := range items {
		it = strings.TrimSpace(strings.ReplaceAll(it, "\n", " "))
		if it == "" || seen[it] {
			continue
		}
		seen[it] = true
		lines = append(lines, "- "+it)
	}
	return f.appendBounded(name, header, lines)
}

func (f *FileStore) readBullets(name string) ([]string, error) {
	p, err := f.path(name)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(l, "- ") {
			out = append(out, strings.TrimPrefix(l, "- "))
		}
	}
	return out, nil
}

func (f *FileStore) AppendExplore(_ context.Context, note ExploreNote) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := json.Marshal(note)
	if err != nil {
		return err
	}
	return f.appendBounded("explore.jsonl", "", []string{string(raw)})
}

func (f *FileStore) Explores(context.Context) ([]ExploreNote, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, err := f.path("explore.jsonl")
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []ExploreNote
	for i, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var note ExploreNote
		if err := json.Unmarshal([]byte(line), &note); err != nil {
			return nil, fmt.Errorf("plan: decode explore.jsonl line %d: %w", i+1, err)
		}
		out = append(out, note)
	}
	return out, nil
}

func (f *FileStore) Facts(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out, err := f.readBullets("facts.md")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return out, err
}

// RenderMarkdown 把决策渲染成 plan.md。
func RenderMarkdown(d Decision) string {
	var b strings.Builder
	b.WriteString("# Plan\n\n")
	if d.Goal != "" {
		fmt.Fprintf(&b, "**Goal**: %s\n\n", d.Goal)
	}
	fmt.Fprintf(&b, "- action: %s\n- complexity: %s\n", d.Action, d.Complexity)
	if d.Reason != "" {
		fmt.Fprintf(&b, "- reason: %s\n", d.Reason)
	}
	if d.Progress.Summary != "" || d.Progress.Percent > 0 {
		fmt.Fprintf(&b, "- progress: %d%% %s\n", d.Progress.Percent, d.Progress.Summary)
	}
	if d.Progress.Next != "" {
		fmt.Fprintf(&b, "- next: %s\n", d.Progress.Next)
	}
	if len(d.Steps) > 0 {
		b.WriteString("\n## Steps\n\n")
		for _, s := range d.Steps {
			mark := " "
			switch s.Status {
			case StepDone:
				mark = "x"
			case StepInProgress:
				mark = "~"
			case StepSkipped:
				mark = "-"
			case StepBlocked:
				mark = "!"
			}
			fmt.Fprintf(&b, "- [%s] %s %s", mark, s.ID, s.Title)
			if s.Evidence != "" {
				fmt.Fprintf(&b, " — %s", s.Evidence)
			}
			b.WriteString("\n")
		}
	}
	if len(d.Dispatches) > 0 {
		b.WriteString("\n## Dispatches\n\n")
		for _, t := range d.Dispatches {
			target := "@" + t.TargetID
			if t.TargetKind == TargetHuman {
				target = "@human(" + t.TargetID + ")"
			}
			fmt.Fprintf(&b, "- %s %s: %s", t.Ref, target, strings.ReplaceAll(t.Message, "\n", " "))
			if len(t.DependsOn) > 0 {
				fmt.Fprintf(&b, " (after %s)", strings.Join(t.DependsOn, ","))
			}
			if t.Risk != "" {
				fmt.Fprintf(&b, " [%s]", t.Risk)
			}
			b.WriteString("\n")
		}
	}
	if d.Confirm.Required {
		fmt.Fprintf(&b, "\n## Confirmation\n\n- required: %s\n", d.Confirm.Reason)
		if d.Confirm.Question != "" {
			fmt.Fprintf(&b, "- question: %s\n", d.Confirm.Question)
		}
	}
	if len(d.Unknowns) > 0 {
		b.WriteString("\n## Unknowns\n\n")
		for _, u := range d.Unknowns {
			fmt.Fprintf(&b, "- %s\n", u)
		}
	}
	return b.String()
}
