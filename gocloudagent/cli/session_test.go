package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestSession(t *testing.T) *Session {
	t.Helper()
	env := testEnv(t)
	s, err := NewSession(env, Config{Endpoint: "http://e", ProjectID: "p1"}, "/tmp/work")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// 每行必须是合法 JSON，且 parent 链完整（jsonl 格式契约）。
func TestSessionAppendWritesValidJSONLines(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.AppendMessage("user", "hello"); err != nil {
		t.Fatal(err)
	}
	call := EntryToolCa{ID: "c1", Name: "shell", Arguments: `{"command":"ls"}`}
	if _, err := s.AppendToolCall("", []EntryToolCa{call}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendToolResult("c1", "shell", `{"ok":true}`, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage("assistant", "done"); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 5 { // session + 4 entries
		t.Fatalf("lines = %d, want 5", len(lines))
	}
	prev := ""
	for i, line := range lines {
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("line %d is not JSON: %v", i, err)
		}
		if i == 0 {
			if e.Type != EntrySession || e.Version != SessionVersion {
				t.Fatalf("header = %+v", e)
			}
		} else if e.Parent != prev {
			t.Fatalf("line %d parent = %q, want %q", i, e.Parent, prev)
		}
		prev = e.ID
	}
}

// 重新打开后内容与顺序一致（resume 的正确性基础）。
func TestSessionReopenRoundTrip(t *testing.T) {
	s := newTestSession(t)
	_, _ = s.AppendMessage("user", "第一条")
	_, _ = s.AppendMessage("assistant", "回答")

	reopened, err := OpenSession(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID != s.ID {
		t.Fatalf("id = %q, want %q", reopened.ID, s.ID)
	}
	msgs := reopened.Messages()
	if len(msgs) != 2 || msgs[0].Content != "第一条" || msgs[1].Role != "assistant" {
		t.Fatalf("messages = %+v", msgs)
	}
	if reopened.Meta.ProjectID != "p1" || reopened.Meta.Cwd != "/tmp/work" {
		t.Fatalf("meta = %+v", reopened.Meta)
	}
}

// compaction 之后只保留该点之后的消息（原文仍在文件里）。
func TestSessionMessagesHonorCompactionCut(t *testing.T) {
	s := newTestSession(t)
	_, _ = s.AppendMessage("user", "old1")
	_, _ = s.AppendMessage("assistant", "old2")
	pivot := s.Lines[len(s.Lines)-1].ID
	if err := s.Append(Entry{Type: EntryCompaction, Parent: pivot, Summary: "早期对话摘要", FirstKept: pivot}); err != nil {
		t.Fatal(err)
	}
	_, _ = s.AppendMessage("user", "new1")

	msgs := s.Messages()
	if len(msgs) != 1 || msgs[0].Content != "new1" {
		t.Fatalf("messages = %+v, want only the post-compaction message", msgs)
	}
	// 摘要要能被取到（供提示词拼装）。
	c, ok := s.LatestCompaction()
	if !ok || c.Summary != "早期对话摘要" {
		t.Fatalf("compaction = %+v ok=%v", c, ok)
	}
	// 原文不删：文件里仍有 5 行。
	b, _ := os.ReadFile(s.Path)
	if n := len(strings.Split(strings.TrimSpace(string(b)), "\n")); n != 5 {
		t.Fatalf("file lines = %d, want 5 (originals must be preserved)", n)
	}
}

func TestFindSessionByPrefixAndAmbiguity(t *testing.T) {
	env := testEnv(t)
	if _, err := NewSession(env, Config{}, "/w"); err != nil {
		t.Fatal(err)
	}
	sessions, err := ListSessions(env)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("list = %+v err=%v", sessions, err)
	}
	id := sessions[0].ID
	prefix := id[:8]
	path, err := FindSession(env, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != id+".jsonl" {
		t.Fatalf("path = %q", path)
	}
	if _, err := FindSession(env, "nope-nope"); err == nil {
		t.Fatal("missing session must error")
	}
	if _, err := FindSession(env, ""); err == nil {
		t.Fatal("empty prefix must not match everything")
	}
}

func TestRemoveSessionDeletesFile(t *testing.T) {
	s := newTestSession(t)
	if err := RemoveSession(s.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestOpenSessionRejectsForeignFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"message","role":"user"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSession(path); err == nil {
		t.Fatal("file without a session header must be rejected")
	}
}

func TestOpenSessionRejectsCorruptLine(t *testing.T) {
	s := newTestSession(t)
	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{not json}\n")
	f.Close()
	if _, err := OpenSession(s.Path); err == nil {
		t.Fatal("corrupt line must be reported, not silently skipped")
	}
}
