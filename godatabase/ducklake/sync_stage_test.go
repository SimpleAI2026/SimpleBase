// 文件说明：sync_stage_test.go — godatabase/ducklake 的单元测试（TestClassifySyncError 等）
//
// 用法
//
//	go test ./godatabase/ducklake -run 'TestClassifySyncError'
//
// 调用架构
//
//	[上游] internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/sync_stage_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] c4_syncer_lease_test.go, c4_syncer_ordering_test.go, catalog_syncer.go, catalog_syncer_test.go, coverage_test.go 等 35 个
//
// 主要符号
//
//	TestClassifySyncError(t *testing.T)

// sync_stage_test.go 验证 planv5.0 §4 P0.3 的同步错误归类。
package ducklake

import (
	"errors"
	"fmt"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"testing"
)

func TestClassifySyncError(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{
			fmt.Errorf("ducklake: flush inlined data: %w", errors.New(`Transaction conflict - attempting to delete from table with index "22"`)),
			"transaction_conflict",
		},
		{fmt.Errorf("wrap: %w", blob.ErrPreconditionFailed), "precondition_failed"},
		{errors.New("ducklake: snapshot changed during copy (5 -> 7)"), "snapshot_moved"},
		{errors.New("something else"), "other"},
	}
	for _, tc := range cases {
		if got := classifySyncError(tc.err); got != tc.want {
			t.Errorf("classify(%v)=%q want %q", tc.err, got, tc.want)
		}
	}
}
