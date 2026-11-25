// 文件说明：sqlval_test.go — godatabase/sqlval 的单元测试（TestBoolInt、TestNullTime 等）
//
// 用法
//
//	go test ./godatabase/sqlval -run 'TestBoolInt|TestNullTime|TestNullTimeValue|TestNullString'
//
// 调用架构
//
//	[上游] internal/auth, internal/catalog, internal/collection, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/sqlval/sqlval_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] sqlval.go
//
// 主要符号
//
//	TestBoolInt(t *testing.T)
//	TestNullTime(t *testing.T)
//	TestNullTimeValue(t *testing.T)
//	TestNullString(t *testing.T)

package sqlval

import (
	"testing"
	"time"
)

func TestBoolInt(t *testing.T) {
	if BoolInt(true) != 1 || BoolInt(false) != 0 {
		t.Fatal("BoolInt")
	}
}

func TestNullTime(t *testing.T) {
	if NullTime(nil) != nil {
		t.Fatal("nil should map to NULL")
	}
	loc := time.FixedZone("UTC+8", 8*3600)
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, loc)
	got, ok := NullTime(&ts).(time.Time)
	if !ok || got.Location() != time.UTC || !got.Equal(ts) {
		t.Fatalf("NullTime = %v", got)
	}
}

func TestNullTimeValue(t *testing.T) {
	if NullTimeValue(time.Time{}) != nil {
		t.Fatal("zero should map to NULL")
	}
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	got, ok := NullTimeValue(ts).(time.Time)
	if !ok || got.Location() != time.UTC || !got.Equal(ts) {
		t.Fatalf("NullTimeValue = %v", got)
	}
}

func TestNullString(t *testing.T) {
	if NullString("") != nil || NullString("x") != "x" {
		t.Fatal("NullString")
	}
}
