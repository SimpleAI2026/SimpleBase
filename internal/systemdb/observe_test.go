// observe_test.go 验证 planv5.0 §4 P0.2 的连接池窗口差与水位辅助函数。
package systemdb

import (
	"database/sql"
	"testing"
	"time"
)

func TestObserveConnWaitWindowDelta(t *testing.T) {
	before := sql.DBStats{WaitCount: 10, WaitDuration: 5 * time.Second}
	after := sql.DBStats{WaitCount: 12, WaitDuration: 7 * time.Second}
	count, d := ObserveConnWait(before, after)
	if count != 2 {
		t.Fatalf("count=%d want 2", count)
	}
	if d != 2*time.Second {
		t.Fatalf("duration=%v want 2s", d)
	}
}

func TestObserveConnWaitReset(t *testing.T) {
	// 计数器回退（重建连接池等）：不得产生负值。
	before := sql.DBStats{WaitCount: 100, WaitDuration: time.Minute}
	after := sql.DBStats{WaitCount: 0, WaitDuration: 0}
	count, d := ObserveConnWait(before, after)
	if count != 0 || d != 0 {
		t.Fatalf("expect zeros, got count=%d d=%v", count, d)
	}
}

func TestConnStatsNilSafe(t *testing.T) {
	if got := ConnStats(nil); got != (sql.DBStats{}) {
		t.Fatalf("nil db should return zero stats, got %+v", got)
	}
}
