// 文件说明：k4_race_test.go — godatabase/kv 的单元测试（TestConcurrentSetSameKeyDuplicateRows 等）
//
// 用法
//
//	go test ./godatabase/kv -run 'TestConcurrentSetSameKeyDuplicateRows'
//
// 调用架构
//
//	[上游] godatabase/cmd, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/kv/k4_race_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bugfix_test.go, collections_test.go, conn_schema_test.go, core.go, executor.go 等 20 个
//
// 主要符号
//
//	TestConcurrentSetSameKeyDuplicateRows(t *testing.T)

package kv

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// K4 复现：并发 SET 同一新 key，检查 __sb_kv_keys 是否出现重复行。
// DuckDB 用户库允许 10 连接 + 乐观并发，两个并发 Update 的 guardWrite
// 都可能看到「不存在」而各自 insertKey。复现后用 per-DB 写互斥修复。
func TestConcurrentSetSameKeyDuplicateRows(t *testing.T) {
	if testing.Short() {
		t.Skip("skip concurrency stress in short mode")
	}
	db := openTestStoreRaw(t)
	ctx := context.Background()

	const rounds = 30
	var dupSeen atomic.Int64
	for round := 0; round < rounds; round++ {
		key := fmt.Sprintf("race-%03d", round)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				s := New(db)
				errs[idx] = s.Update(ctx, func(tx *Tx) error {
					return tx.Str().Set(ctx, key, []byte("v"))
				})
			}(i)
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatalf("round %d: concurrent set: %v", round, err)
			}
		}
		var rows int64
		if err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM __sb_kv_keys WHERE "key" = ?`, key).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows > 1 {
			dupSeen.Add(rows - 1)
		}
	}
	if n := dupSeen.Load(); n > 0 {
		t.Fatalf("duplicate meta rows detected in %d key(s); K4 confirmed", n)
	}
}
