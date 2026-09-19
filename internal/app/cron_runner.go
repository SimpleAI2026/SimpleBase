package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// systemDBRunner 实现 cronjob.Runner：按任务目标执行一次云函数调用
// （ui-cronjob-plan §5.3）。放 app 层是因为需要同时访问系统库与内核。
type systemDBRunner struct {
	store *systemdb.Store
}

// RunFunction 取目标云函数源码并调 gofunction.RunJSON。
// 错误保留哨兵语义（ErrTimeout/ErrCompile/ErrBind），由调度器统一落 failed 记录。
func (r *systemDBRunner) RunFunction(ctx context.Context, projectID, funcFile, funcExport string, input json.RawMessage) ([]byte, error) {
	if r.store == nil {
		return nil, errors.New("system store unavailable")
	}
	g, err := r.store.GetGoFunction(ctx, projectID, funcFile)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("gofunction not found: %s", funcFile)
	}
	if err != nil {
		return nil, err
	}
	return gofunction.RunJSON(ctx, fmt.Sprintf("cron-%s", funcFile), funcFile, g.Source, funcExport, input)
}
