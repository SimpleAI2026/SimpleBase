// 文件说明：cachetest.go — B2 RunJSON 编译缓存正确性测试：
// fork 隔离（并发请求 globals 互不干扰）、init 重放语义、
// LRU 淘汰与容量调节、缓存命中/未命中计数。
package interp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

// TestRunJSONCacheSemantics 两次 RunJSON 同源码：结果一致且计数命中。
// 全局变量每次 fork 重放，脚本内自增不应跨请求泄漏。
func TestRunJSONCacheSemantics(t *testing.T) {
	SetProgramCacheLimit(8)
	defer SetProgramCacheLimit(128)

	src := `
package main

var counter = 0

func Run(body struct{ X int }) map[string]int {
	counter += body.X
	return map[string]int{"counter": counter}
}
`
	var out map[string]int
	for i := 0; i < 2; i++ {
		raw, err := RunJSON(context.Background(), "seq", "main", src, "Run", []byte(`{"X": 5}`))
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out["counter"] != 5 {
			t.Fatalf("run %d: counter = %d, want 5（全局不应跨请求累积）", i, out["counter"])
		}
	}
	if runJSONCache.hits == 0 || runJSONCache.misses == 0 {
		t.Fatalf("hits=%d misses=%d, want both > 0", runJSONCache.hits, runJSONCache.misses)
	}
}

// TestRunJSONCacheForkIsolation 并发 RunJSON 同源码：各请求独立实例，
// globals 无竞态（-race 验证）、结果互不串扰。
func TestRunJSONCacheForkIsolation(t *testing.T) {
	SetProgramCacheLimit(8)
	defer SetProgramCacheLimit(128)

	src := `
package main

var data = map[int]int{}

func Run(body struct{ K int }) map[int]int {
	data[body.K] = body.K
	return data
}
`
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			raw, err := RunJSON(context.Background(), "seq", "main", src, "Run",
				[]byte(`{"K": `+string(rune('0'+k))+`}`))
			if err != nil {
				t.Errorf("goroutine %d: %v", k, err)
				return
			}
			var out map[int]int
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Errorf("goroutine %d: unmarshal: %v", k, err)
				return
			}
			if len(out) != 1 || out[k] != k {
				t.Errorf("goroutine %d: map = %v, want single entry {%d:%d}", k, out, k, k)
			}
		}(i)
	}
	wg.Wait()
}

// TestRunJSONCacheLRUEviction 容量 2 写入 3 个源码：最旧被淘汰，
// 再访问第一个应重新编译（misses 增加）。
func TestRunJSONCacheLRUEviction(t *testing.T) {
	SetProgramCacheLimit(2)
	defer SetProgramCacheLimit(128)

	// 同源码只占 1 缓存槽；用三个不同源码验证淘汰
	srcs := []string{
		"package main\nfunc Run(b struct{ X int }) int { return b.X }",
		"package main\nfunc Run(b struct{ X int }) int { return b.X + 0 }",
		"package main\nfunc Run(b struct{ X int }) int { return b.X + 1 - 1 }",
	}
	call := func(i int) {
		raw, err := RunJSON(context.Background(), "seq", "main", srcs[i], "Run", []byte(`{"X": 7}`))
		if err != nil {
			t.Fatalf("src %d: %v", i, err)
		}
		var out int
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("src %d: unmarshal: %v", i, err)
		}
		if out != 7 {
			t.Fatalf("src %d: out = %d, want 7", i, out)
		}
	}
	call(0)
	call(1)
	call(2) // 触发淘汰 src0
	runJSONCache.mu.Lock()
	size := len(runJSONCache.m)
	runJSONCache.mu.Unlock()
	if size != 2 {
		t.Fatalf("cache size = %d, want 2", size)
	}
	if runJSONCache.get(sha256Of("main", srcs[0])) != nil {
		t.Fatalf("src0 应已被淘汰")
	}
	if runJSONCache.get(sha256Of("main", srcs[2])) == nil {
		t.Fatalf("src2 应在缓存中")
	}
}

// TestRunJSONCacheDisabled limit<=0 关闭：两次调用各自全量编译，
// 结果仍正确。
func TestRunJSONCacheDisabled(t *testing.T) {
	SetProgramCacheLimit(0)
	defer SetProgramCacheLimit(128)

	src := "package main\nfunc Run(b struct{ X int }) int { return b.X * 3 }"
	for i := 0; i < 2; i++ {
		raw, err := RunJSON(context.Background(), "seq", "main", src, "Run", []byte(`{"X": 2}`))
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		var out int
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("run %d: unmarshal: %v", i, err)
		}
		if out != 6 {
			t.Fatalf("run %d: out = %d, want 6", i, out)
		}
	}
	runJSONCache.mu.Lock()
	size := len(runJSONCache.m)
	runJSONCache.mu.Unlock()
	if size != 0 {
		t.Fatalf("缓存应关闭，size = %d", size)
	}
}

func sha256Of(fname, source string) [32]byte {
	return sha256Sum(fname + "\x00" + source)
}
