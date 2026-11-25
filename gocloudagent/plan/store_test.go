// 文件说明：store_test.go — FileStore 原子写、单文件上限滚动、explore.jsonl 追加与 MemoryStore（planv6.8 §13.11.1-4）

package plan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileStoreDecisionRoundtrip(t *testing.T) {
	dir := t.TempDir()
	fs := &FileStore{Dir: filepath.Join(dir, "p")}
	d := validDecision()
	d.Steps = []Step{{ID: "s1", Title: "first", Status: StepInProgress}}
	if err := fs.SaveDecision(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	got, ok, err := fs.LoadDecision(context.Background())
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Goal != d.Goal || len(got.Dispatches) != 1 || got.Steps[0].ID != "s1" {
		t.Fatalf("roundtrip: %+v", got)
	}
	// plan.md 同步生成且包含 goal 与 dispatch。
	raw, err := os.ReadFile(filepath.Join(fs.Dir, "plan.md"))
	if err != nil {
		t.Fatalf("plan.md: %v", err)
	}
	if !strings.Contains(string(raw), "answer the question") || !strings.Contains(string(raw), "@a1") {
		t.Fatalf("plan.md content: %s", raw)
	}
	// Load 前置：目录不存在时 ok=false 而非错误。
	fresh := &FileStore{Dir: filepath.Join(dir, "none")}
	if _, ok, err := fresh.LoadDecision(context.Background()); err != nil || ok {
		t.Fatalf("missing store: ok=%v err=%v", ok, err)
	}
}

func TestFileStoreAppendBoundedAndFactsDedup(t *testing.T) {
	fs := &FileStore{Dir: t.TempDir(), MaxFileBytes: 400}
	ctx := context.Background()
	for i := 0; i < 40; i++ {
		if err := fs.AppendProgress(ctx, strings.Repeat("p", 30)); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(fs.Dir, "progress.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > fs.limit() {
		t.Fatalf("progress.md exceeds limit: %d > %d", len(raw), fs.limit())
	}
	// 头部标题保留。
	if !strings.HasPrefix(string(raw), "# Progress") {
		t.Fatalf("header lost: %q", string(raw[:20]))
	}

	// facts 去重追加。
	if err := fs.AppendFacts(ctx, []string{"f1", "f1", " f2 "}); err != nil {
		t.Fatal(err)
	}
	if err := fs.AppendFacts(ctx, []string{"f1", "f3"}); err != nil {
		t.Fatal(err)
	}
	facts, err := fs.Facts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(facts, ",") != "f1,f2,f3" {
		t.Fatalf("facts = %v", facts)
	}
}

func TestFileStoreExploreJSONL(t *testing.T) {
	fs := &FileStore{Dir: t.TempDir()}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := fs.AppendExplore(ctx, ExploreNote{
			Goal: "g", Questions: []string{"q"},
			Reads:    []ExploreRead{{Path: "a.go", Symbol: "F"}},
			Findings: []string{"found"}, Unknowns: []string{"unknown"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	notes, err := fs.Explores(ctx)
	if err != nil || len(notes) != 3 {
		t.Fatalf("explores = %v err = %v", notes, err)
	}
	if notes[0].Reads[0].Path != "a.go" || notes[2].Findings[0] != "found" {
		t.Fatalf("note content: %+v", notes[0])
	}
	// explore.jsonl 与其他文件互不干扰。
	if _, err := os.Stat(filepath.Join(fs.Dir, "explore.jsonl")); err != nil {
		t.Fatalf("explore.jsonl: %v", err)
	}
}

// 上限滚动只丢最旧的行，保留头部；超出部分整行丢弃。
// 注：findings 与 facts 是两条流（findings.md / facts.md），滚动断言直接读文件。
func TestFileStoreAppendBoundedDropsOldestLines(t *testing.T) {
	fs := &FileStore{Dir: t.TempDir(), MaxFileBytes: 200}
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		if err := fs.AppendFindings(ctx, []string{string(rune('a'+i%26)) + strings.Repeat("x", 25)}); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(fs.Dir, "findings.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 200 {
		t.Fatalf("findings.md exceeds limit: %d", len(raw))
	}
	if !strings.HasPrefix(string(raw), "# Findings") {
		t.Fatalf("header lost: %q", string(raw[:12]))
	}
	lines := strings.Count(string(raw), "\n- ")
	if lines < 2 {
		t.Fatalf("expected some retained lines, got %d in %q", lines, raw)
	}
}

func TestMemoryStore(t *testing.T) {
	m := &MemoryStore{}
	ctx := context.Background()
	if _, ok, _ := m.LoadDecision(ctx); ok {
		t.Fatal("empty store must report no decision")
	}
	d := validDecision()
	if err := m.SaveDecision(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := m.LoadDecision(ctx)
	if !ok || got.Goal != d.Goal {
		t.Fatalf("load: ok=%v %+v", ok, got)
	}
	if err := m.AppendProgress(ctx, "l1"); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendFindings(ctx, []string{"f", "f"}); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendFacts(ctx, []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendExplore(ctx, ExploreNote{Goal: "g"}); err != nil {
		t.Fatal(err)
	}
	if len(m.Progress()) != 1 || strings.Join(m.Findings(), "") != "f" {
		t.Fatalf("progress/findings: %v %v", m.Progress(), m.Findings())
	}
	facts, _ := m.Facts(ctx)
	notes, _ := m.Explores(ctx)
	if len(facts) != 1 || len(notes) != 1 {
		t.Fatalf("facts/notes: %v %v", facts, notes)
	}
}

// 空 Dir 拒绝写（fail-closed），不panic。
func TestFileStoreEmptyDir(t *testing.T) {
	fs := &FileStore{}
	if err := fs.SaveDecision(context.Background(), validDecision()); err == nil {
		t.Fatal("empty dir must fail")
	}
}

func TestFileStoreRejectsOversizedRecords(t *testing.T) {
	ctx := context.Background()
	fs := &FileStore{Dir: t.TempDir(), MaxFileBytes: 200}
	if err := fs.SaveDecision(ctx, validDecision()); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized decision error = %v, want ErrTooLarge", err)
	}
	if _, err := os.Stat(filepath.Join(fs.Dir, "decision.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized decision must not be written, stat err = %v", err)
	}
	if err := fs.AppendExplore(ctx, ExploreNote{Goal: strings.Repeat("x", 300)}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized explore error = %v, want ErrTooLarge", err)
	}
	notes, err := fs.Explores(ctx)
	if err != nil || len(notes) != 0 {
		t.Fatalf("oversized explore must not disappear as success: notes=%v err=%v", notes, err)
	}
}
