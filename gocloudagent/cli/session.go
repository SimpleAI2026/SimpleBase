// 文件说明：session.go — gocloudagent/cli 中 SessionVersion、EntryType 等符号的实现
//
// 用法
//
//	NewSession(env []string, cfg Config, cwd string) (*Session, error)
//	OpenSession(path string) (*Session, error)  // OpenSession 打开已存在的会话文件
//	FindSession(env []string, prefix string) (string, error)  // FindSession 按 id 前缀在当前用户的 sessions 目录里定位会话文件
//	同包调用：Messages() → client.go, compaction.go 等；Session → compaction.go, repl.go 等；EntryCompaction → command.go, compaction.go
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/session.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] client.go, command.go, compaction.go, config.go, prompt.go 等 9 个
//	[关键引用] Messages() ← client.go, compaction.go, runner.go
//
// 主要符号
//
//	const SessionVersion
//	type EntryType string
//	const EntrySession
//	const EntryMessage
//	const EntryCompaction
//	type Entry struct {…}
//	... 另有 13 个顶层符号

package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SessionVersion 是 jsonl 格式版本；未来加字段不改格式时保持 1。
const SessionVersion = 1

// EntryType 是 jsonl 行的类型。
type EntryType string

const (
	// EntrySession 是首行，记录会话元信息。
	EntrySession EntryType = "session"
	// EntryMessage 是一条会话消息（user / assistant / tool）。
	EntryMessage EntryType = "message"
	// EntryCompaction 是一条压缩摘要（原文保留，只是不再进上下文）。
	EntryCompaction EntryType = "compaction"
)

// Entry 是 jsonl 的一行。
//
// Parent 形成树：第一阶段恒为前一条的 ID（线性），格式已预留分支，
// 后续加 branch/leaf 不需要迁移（planv6.0 §13.4）。
type Entry struct {
	Type EntryType `json:"type"`
	ID   string    `json:"id"`
	// Version 仅 session 行有值。
	Version int `json:"v,omitempty"`
	// Parent 为空表示根（session 行或第一条消息）。
	Parent string `json:"parent,omitempty"`
	// 以下字段仅 message 行有值。
	Role       string        `json:"role,omitempty"`
	Content    string        `json:"content,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
	IsError    bool          `json:"is_error,omitempty"`
	ToolCalls  []EntryToolCa `json:"tool_calls,omitempty"`
	// 以下字段仅 compaction 行有值。
	Summary string `json:"summary,omitempty"`
	// FirstKept 记录被折进摘要的最后一条消息：投影时只保留它之后的消息。
	//
	// 注意这与 Pi 的 firstKeptEntryId 语义相反（Pi 记「第一条保留的」，含端点）。
	// 这里沿用本仓库既有的取值口径——session_test.go 的
	// TestSessionMessagesHonorCompactionCut 就是按「该条不再保留」写的。
	FirstKept string `json:"first_kept,omitempty"`
	// 以下字段仅 session 行有值。
	Cwd       string    `json:"cwd,omitempty"`
	Endpoint  string    `json:"endpoint,omitempty"`
	ProjectID string    `json:"project,omitempty"`
	Model     string    `json:"model,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// EntryToolCa 是 assistant 消息里的工具调用（落盘形状）。
type EntryToolCa struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Session 是一个本地会话：一行一条 entry 的 append-only jsonl。
type Session struct {
	ID    string
	Path  string
	Meta  Entry
	Lines []Entry
}

// NewSession 创建会话文件并写入首行。
func NewSession(env []string, cfg Config, cwd string) (*Session, error) {
	dir := filepath.Join(HomeDir(env), "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	id := time.Now().UTC().Format("20060102T150405") + "-" + uuid.NewString()[:8]
	meta := Entry{
		Type: EntrySession, ID: uuid.NewString(), Version: SessionVersion,
		Cwd: cwd, Endpoint: cfg.Endpoint, ProjectID: cfg.ProjectID, Model: cfg.Model,
		CreatedAt: time.Now().UTC(),
	}
	s := &Session{ID: id, Path: filepath.Join(dir, id+".jsonl"), Meta: meta}
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(meta); err != nil {
		return nil, err
	}
	s.Lines = append(s.Lines, meta)
	return s, nil
}

// OpenSession 打开已存在的会话文件。
func OpenSession(path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("corrupt session line in %s: %w", path, err)
		}
		lines = append(lines, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(lines) == 0 || lines[0].Type != EntrySession {
		return nil, fmt.Errorf("not a session file: %s", path)
	}
	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	if id == "" {
		id = lines[0].ID
	}
	return &Session{ID: id, Path: path, Meta: lines[0], Lines: lines}, nil
}

// FindSession 按 id 前缀在当前用户的 sessions 目录里定位会话文件。
// 空前缀返回错误而不是匹配全部——否则 `sessions rm ""` 会删掉任一会话。
func FindSession(env []string, prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", errors.New("session id must not be empty")
	}
	dir := filepath.Join(HomeDir(env), "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var hits []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		if id == prefix || strings.HasPrefix(id, prefix) {
			hits = append(hits, filepath.Join(dir, e.Name()))
		}
	}
	switch len(hits) {
	case 0:
		return "", fmt.Errorf("session %q not found", prefix)
	case 1:
		return hits[0], nil
	default:
		return "", fmt.Errorf("session prefix %q is ambiguous (%d matches)", prefix, len(hits))
	}
}

// ListSessions 返回按创建时间倒序的会话摘要。
func ListSessions(env []string) ([]Entry, error) {
	dir := filepath.Join(HomeDir(env), "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		if sc.Scan() {
			var meta Entry
			if json.Unmarshal(sc.Bytes(), &meta) == nil && meta.Type == EntrySession {
				meta.ID = strings.TrimSuffix(e.Name(), ".jsonl")
				out = append(out, meta)
			}
		}
		f.Close()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// RemoveSession 真删除会话文件（不做墓碑，§5.2）。
func RemoveSession(path string) error { return os.Remove(path) }

// Append 追加一条 entry 并落盘（fsync 失败只影响耐久性，不改会话语义）。
func (s *Session) Append(e Entry) error {
	if s == nil || s.Path == "" {
		return errors.New("session is not initialized")
	}
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	if e.Parent == "" && len(s.Lines) > 0 {
		e.Parent = s.Lines[len(s.Lines)-1].ID
	}
	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(e); err != nil {
		return err
	}
	s.Lines = append(s.Lines, e)
	return nil
}

// AppendMessage 追加一条消息行。content 由调用方保证已脱敏（§5.2 决策：
// CLI 落盘「脱敏但不截断」，与云端「不落正文又截断」刻意不同）。
func (s *Session) AppendMessage(role, content string) (Entry, error) {
	e := Entry{Type: EntryMessage, Role: role, Content: content}
	return e, s.Append(e)
}

// AppendToolCall 追加 assistant 消息（带工具调用）。
func (s *Session) AppendToolCall(content string, calls []EntryToolCa) (Entry, error) {
	e := Entry{Type: EntryMessage, Role: "assistant", Content: content, ToolCalls: calls}
	return e, s.Append(e)
}

// AppendToolResult 追加工具结果消息。
func (s *Session) AppendToolResult(callID, name, content string, isErr bool) (Entry, error) {
	e := Entry{Type: EntryMessage, Role: "tool", ToolCallID: callID, Name: name, Content: content, IsError: isErr}
	return e, s.Append(e)
}

// Messages 返回根到当前叶子的消息序列（第一阶段即全部消息行，跳过 session 与 compaction）。
//
// 树已建好，但投影按 §13.4 的渐进路径实现：从最后一条回溯 Parent 到根，
// 遇到最近的 compaction 时只保留该点之后的消息，摘要本身作为 system 提示词由调用方拼装。
func (s *Session) Messages() []Entry {
	if s == nil || len(s.Lines) == 0 {
		return nil
	}
	byID := make(map[string]Entry, len(s.Lines))
	for _, e := range s.Lines {
		byID[e.ID] = e
	}
	// 从最后一条向上回溯到根。
	var chain []Entry
	cur := s.Lines[len(s.Lines)-1].ID
	for cur != "" {
		e, ok := byID[cur]
		if !ok {
			break
		}
		chain = append(chain, e)
		cur = e.Parent
	}
	// 反转成根 → 叶子顺序。
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	var out []Entry
	// 最近一次压缩折进的最后一条消息（FirstKept 记的是「最后被压掉的」，
	// 不是「第一条保留的」——见 Entry.FirstKept 的注释）。
	cut := ""
	for _, e := range chain {
		if e.Type == EntryCompaction {
			cut = e.FirstKept
		}
	}
	skipping := cut != ""
	for _, e := range chain {
		if e.Type != EntryMessage {
			continue
		}
		if skipping {
			if e.ID == cut {
				skipping = false
			}
			continue
		}
		out = append(out, e)
	}
	return out
}

// LatestCompaction 返回链上最近的一条压缩摘要（供提示词拼装）。
func (s *Session) LatestCompaction() (Entry, bool) {
	for i := len(s.Lines) - 1; i >= 0; i-- {
		if s.Lines[i].Type == EntryCompaction {
			return s.Lines[i], true
		}
	}
	return Entry{}, false
}
