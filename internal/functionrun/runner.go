// 文件说明：runner.go — internal/functionrun 中 ErrTooManyRuns、Store 等符号的实现
//
// 用法
//
//	NewRunner(store Store, maxConcurrent int) *Runner
//	(r *Runner) Acquire() bool
//	(r *Runner) Release()
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/trigger
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/functionrun/runner.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction, internal/systemdb
//
// 主要符号
//
//	var ErrTooManyRuns
//	type Store interface {…}
//	type Request struct {…}
//	type Result struct {…}
//	type Runner struct {…}
//	NewRunner(store Store, maxConcurrent int) *Runner
//	... 另有 4 个顶层符号

package functionrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

var ErrTooManyRuns = errors.New("too_many_go_runs")

type Store interface {
	ResolveActiveSource(ctx context.Context, projectID, name string) (systemdb.GoFuncVersion, error)
	RecordGoFuncInvoke(ctx context.Context, projectID, funcName, functionName string, version int64, channel string, statusCode int, durationMs int64, requestID, actor string)
}

type Request struct {
	ProjectID  string
	FuncFile   string
	FuncExport string
	Input      json.RawMessage
	Channel    string
	RequestID  string
	Actor      string
}

type Result struct {
	Output     json.RawMessage
	Version    int64
	StatusCode int
	Duration   time.Duration
}

type Runner struct {
	store Store
	sem   chan struct{}
}

func NewRunner(store Store, maxConcurrent int) *Runner {
	r := &Runner{store: store}
	if maxConcurrent > 0 {
		r.sem = make(chan struct{}, maxConcurrent)
	}
	return r
}

func (r *Runner) Acquire() bool {
	if r == nil || r.sem == nil {
		return true
	}
	select {
	case r.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (r *Runner) Release() {
	if r != nil && r.sem != nil {
		<-r.sem
	}
}

func (r *Runner) Run(ctx context.Context, request Request) (Result, error) {
	result := Result{StatusCode: http.StatusOK}
	if r == nil || r.store == nil {
		return result, systemdb.ErrUnavailable
	}
	start := time.Now()
	version, err := r.store.ResolveActiveSource(ctx, request.ProjectID, request.FuncFile)
	if err != nil {
		result.StatusCode = http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			result.StatusCode = http.StatusNotFound
		}
		if errors.Is(err, systemdb.ErrNoActiveVersion) {
			result.StatusCode = http.StatusConflict
		}
		return result, err
	}
	result.Version = version.Version
	if r.sem != nil {
		select {
		case r.sem <- struct{}{}:
			defer func() { <-r.sem }()
		default:
			result.StatusCode = http.StatusServiceUnavailable
			return result, ErrTooManyRuns
		}
	}
	input := request.Input
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	output, runErr := gofunction.RunJSON(ctx, request.RequestID, request.FuncFile, version.Source, request.FuncExport, input)
	result.Output = output
	result.Duration = time.Since(start)
	if runErr != nil {
		result.StatusCode = statusCode(runErr)
	}
	channel := request.Channel
	if channel == "" {
		channel = "go"
	}
	r.store.RecordGoFuncInvoke(ctx, request.ProjectID, request.FuncFile, request.FuncExport, version.Version, channel, result.StatusCode, result.Duration.Milliseconds(), request.RequestID, request.Actor)
	return result, runErr
}

func statusCode(err error) int {
	switch {
	case errors.Is(err, gofunction.ErrFunctionNotFound):
		return http.StatusNotFound
	case errors.Is(err, gofunction.ErrTimeout):
		return http.StatusGatewayTimeout
	case errors.Is(err, gofunction.ErrBind):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
