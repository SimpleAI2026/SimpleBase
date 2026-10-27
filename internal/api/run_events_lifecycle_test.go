package api

import (
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
)

// LIFE-05：没有 end 事件的 run 缓存必须被兜底清理，不能永久留在内存。
// 修复前只有收到 end 才安排 drop：中途 panic / 提前返回的 run 会一直占用。
func TestRunEventHub_NoEndStillDropped(t *testing.T) {
	h := newRunEventHub()
	h.idleTTL = 200 * time.Millisecond // 测试用短 TTL
	runID := "run-no-end"
	h.append(runID, gocloudagent.Event{Type: "run", RunID: runID})
	h.append(runID, gocloudagent.Event{Type: "token", RunID: runID, Content: "x"})
	// 未写 end —— 修复前该条目永不清理。

	if len(h.runs) != 1 {
		t.Fatalf("runs=%d want 1", len(h.runs))
	}
	// 生命周期兜底：未完成的 run 在 idleTTL 后也必须被清理。
	time.Sleep(300 * time.Millisecond)
	h.sweep()
	if len(h.runs) != 0 {
		t.Fatalf("run without end event never dropped: %d entries remain", len(h.runs))
	}
}

// LIFE-05：已有 end 的 run 保持原有 2 分钟延迟清理语义（sweep 不提前删）。
func TestRunEventHub_EndKeepsDelay(t *testing.T) {
	h := newRunEventHub()
	runID := "run-ended"
	h.append(runID, gocloudagent.Event{Type: "run", RunID: runID})
	h.append(runID, gocloudagent.Event{Type: "end", RunID: runID})
	h.sweep()
	if len(h.runs) != 1 {
		t.Fatalf("ended run swept too early: %d entries", len(h.runs))
	}
}

// LIFE-05：缓存总量上限——超出 maxRuns 时最旧的未活跃条目被淘汰，
// 防止大量未完成 run 堆积导致内存无限增长。
func TestRunEventHub_TotalCap(t *testing.T) {
	h := newRunEventHub()
	h.maxRuns = 5
	for i := 0; i < 20; i++ {
		h.append(string(rune('a'+i)), gocloudagent.Event{Type: "run"})
	}
	if len(h.runs) > h.maxRuns {
		t.Fatalf("runs=%d exceeds cap %d", len(h.runs), h.maxRuns)
	}
	// 最新写入的 run 必须仍在缓存中。
	last := string(rune('a' + 19))
	if _, ok := h.runs[last]; !ok {
		t.Fatalf("latest run %q evicted", last)
	}
}

// LIFE-05：并发 append 与 sweep 不产生 race（-race 下验证）。
func TestRunEventHub_ConcurrentAppendSweep(t *testing.T) {
	h := newRunEventHub()
	h.maxRuns = 8
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				h.append(string(rune('a'+n)), gocloudagent.Event{Type: "token", Content: "x"})
			}
		}(i % 20)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			h.sweep()
		}
	}()
	wg.Wait()
}
