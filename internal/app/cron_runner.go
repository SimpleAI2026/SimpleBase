// 文件说明：cron_runner.go — internal/app 的内部实现（systemDBRunner、RunFunction 等）
//
// 用法
//
//	type systemDBRunner（包内使用）
//	同包调用：systemDBRunner → app.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/cron_runner.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction, internal/functionrun, internal/systemdb
//	[同包] app.go, catalog_engine.go, identity.go, project_purger.go, reset.go 等 6 个
//	[关键引用] systemDBRunner ← app.go
//
// 主要符号
//
//	type systemDBRunner struct {…}
//	(r *systemDBRunner) RunFunction(ctx context.Context, projectID, funcFile ...

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/functionrun"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// systemDBRunner 实现 cronjob.Runner：按任务目标执行一次云函数调用
// （ui-cronjob-plan §5.3）。放 app 层是因为需要同时访问系统库与内核。
type systemDBRunner struct {
	store  *systemdb.Store
	runner *functionrun.Runner
}

// RunFunction 取目标云函数**生效版**源码并调 gofunction.RunJSON（plan §2.1）。
// 错误保留哨兵语义（ErrTimeout/ErrCompile/ErrBind），由调度器统一落 failed 记录。
func (r *systemDBRunner) RunFunction(ctx context.Context, projectID, funcFile, funcExport string, input json.RawMessage) ([]byte, error) {
	if r.runner != nil {
		result, err := r.runner.Run(ctx, functionrun.Request{ProjectID: projectID, FuncFile: funcFile, FuncExport: funcExport, Input: input, Channel: "cron", RequestID: "cron-" + funcFile, Actor: "cron"})
		return result.Output, err
	}
	if r.store == nil {
		return nil, errors.New("system store unavailable")
	}
	v, err := r.store.ResolveActiveSource(ctx, projectID, funcFile)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("gofunction not found: %s", funcFile)
	}
	if errors.Is(err, systemdb.ErrNoActiveVersion) {
		return nil, fmt.Errorf("no_active_version: %s", funcFile)
	}
	if err != nil {
		return nil, err
	}
	return gofunction.RunJSON(ctx, fmt.Sprintf("cron-%s", funcFile), funcFile, v.Source, funcExport, input)
}
