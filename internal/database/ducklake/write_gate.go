package ducklake

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// conflictRetryAttempts 是首次失败之后的额外重试次数。
// DuckLake 冲突发生在提交失败时，语句没有落盘，重放是安全的。
const conflictRetryAttempts = 4

// writeGate 串行化同一打开库上的 DuckLake 写事务与 catalog sync 的
// flush/copy。连接池放开到多连接之后，flush_inlined_data 对内联行的删除
// 会和并发 INSERT/UPDATE 抢同一张表索引，提交报 Transaction conflict。
// 历史上 SetMaxOpenConns(1) 把所有语句排在一条连接上；现在读可以并行，
// 写和 flush/copy 仍走这把锁。
//
// 加锁顺序必须是「先拿到连接，再拿锁」。同步路径若先拿锁再向池申请连接，
// 会和「已占着连接、正在等锁」的写事务死锁。
type writeGate struct {
	mu             sync.Mutex
	holders        atomic.Int32
	maxHolders     atomic.Int32
	retryConflicts bool
}

type writeGateHeld struct{}

var dbGates sync.Map // *sql.DB -> *writeGate

func registerDBGate(db *sql.DB, g *writeGate) {
	if db == nil || g == nil {
		return
	}
	dbGates.Store(db, g)
}

func unregisterDBGate(db *sql.DB) {
	if db == nil {
		return
	}
	dbGates.Delete(db)
}

func gateFor(db *sql.DB) *writeGate {
	if db == nil {
		return nil
	}
	g, _ := dbGates.Load(db)
	gate, _ := g.(*writeGate)
	return gate
}

func (g *writeGate) lock() {
	g.mu.Lock()
	n := g.holders.Add(1)
	for {
		old := g.maxHolders.Load()
		if n <= old || g.maxHolders.CompareAndSwap(old, n) {
			return
		}
	}
}

func (g *writeGate) unlock() {
	g.holders.Add(-1)
	g.mu.Unlock()
}

// holdWriteGate 在当前 goroutine 持有 db 的写锁，并把持有标记放进 ctx。
// 同一 ctx 上的后续写语句不会再次加锁（避免 sync 的 flush 自己死锁）。
// 调用方必须已经从池里取出连接。没有注册写锁时原样返回。
func holdWriteGate(ctx context.Context, db *sql.DB) (context.Context, func()) {
	g := gateFor(db)
	if g == nil || ctx.Value(writeGateHeld{}) == g {
		return ctx, func() {}
	}
	g.lock()
	return context.WithValue(ctx, writeGateHeld{}, g), func() { g.unlock() }
}

// IsTransactionConflict 判断错误是否为 DuckLake 乐观并发冲突。
// 提交失败，语句没有生效。
func IsTransactionConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Transaction conflict")
}

// RetryOnConflict 在 DuckLake Transaction conflict 上有限次重放 fn。
// fn 必须幂等：冲突表示提交失败，重放不会叠加上一次的写入。
// 其它错误立即返回。ctx 取消时不再发起下一次。
func RetryOnConflict(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; ; attempt++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		err = fn()
		if err == nil || !IsTransactionConflict(err) || attempt >= conflictRetryAttempts {
			return err
		}
		delay := time.Duration(attempt+1) * 15 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// isMutatingSQL 判断语句是否会开启 DuckLake 写事务。
// 纯读不占写锁，保留多连接下的并发查询。
func isMutatingSQL(query string) bool {
	q := strings.TrimSpace(query)
	if q == "" {
		return false
	}
	upper := strings.ToUpper(q)
	switch {
	case strings.HasPrefix(upper, "SELECT"),
		strings.HasPrefix(upper, "EXPLAIN"),
		strings.HasPrefix(upper, "SHOW"),
		strings.HasPrefix(upper, "DESCRIBE"):
		return false
	case strings.HasPrefix(upper, "WITH"):
		return strings.Contains(upper, "INSERT") ||
			strings.Contains(upper, "UPDATE") ||
			strings.Contains(upper, "DELETE") ||
			strings.Contains(upper, "CREATE") ||
			strings.Contains(upper, "DROP") ||
			strings.Contains(upper, "ALTER")
	default:
		return true
	}
}
