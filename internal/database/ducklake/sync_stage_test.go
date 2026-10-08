// sync_stage_test.go 验证 planv5.0 §4 P0.3 的同步错误归类。
package ducklake

import (
	"errors"
	"fmt"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/objectstore"
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
		{fmt.Errorf("wrap: %w", objectstore.ErrPreconditionFailed), "precondition_failed"},
		{errors.New("ducklake: snapshot changed during copy (5 -> 7)"), "snapshot_moved"},
		{errors.New("something else"), "other"},
	}
	for _, tc := range cases {
		if got := classifySyncError(tc.err); got != tc.want {
			t.Errorf("classify(%v)=%q want %q", tc.err, got, tc.want)
		}
	}
}
