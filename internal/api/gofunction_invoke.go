// 文件说明：gofunction_invoke.go — Go 云函数的执行面：/go 调用、调试台试跑、全局并发槽位与调用指标
//
// 用法
//
//	路由：Invoke() ← routes_function.go（/go/:projectID/:name/:functionName）；TestVersion() ← routes_function.go
//	同包调用：acquireRunSlot() / releaseRunSlot() 仅本文件
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/gofunction_invoke.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction, internal/functionrun, internal/systemdb
//	[同包] gofunction_handler.go（handler 定义与 CRUD）、gofunction_errors.go（错误映射）
//	[关键引用] Invoke ← routes_function.go
//
// 主要符号
//
//	(*GoFunctionHandler).acquireRunSlot / releaseRunSlot
//	(*GoFunctionHandler).TestVersion / Invoke(c echo.Context) error
//	(*GoFunctionHandler).loadFunc / pickSource / recordInvokeMetrics

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/functionrun"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// acquireRunSlot 非阻塞获取执行槽位；满载返回 false（GF-RUN-04 → 503）。
func (h *GoFunctionHandler) acquireRunSlot() bool {
	if h.runner != nil {
		return h.runner.Acquire()
	}
	if h.runSem == nil {
		return true
	}
	select {
	case h.runSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (h *GoFunctionHandler) releaseRunSlot() {
	if h.runner != nil {
		h.runner.Release()
		return
	}
	if h.runSem != nil {
		<-h.runSem
	}
}

// TestVersion: POST /gofunctions/:name/versions/:ver/test（plan §5.3）
func (h *GoFunctionHandler) TestVersion(c echo.Context) error {
	pc, rid, err := h.requireWriteStore(c, true)
	if err != nil {
		return WriteError(c, err)
	}
	name := c.Param("name")
	ver, err := parseVersionParam(c, rid)
	if err != nil {
		return WriteError(c, err)
	}
	var req goFuncTestRequest
	if err := bindGoFunctionBody(c, &req); err != nil {
		return WriteError(c, err)
	}
	if !gofunctionExportNameRe.MatchString(req.FunctionName) {
		return WriteError(c, badRequest(c, "invalid function_name"))
	}
	f, err := h.store.GetGoFunc(c.Request().Context(), pc.ID, name)
	if err != nil {
		return WriteError(c, mapNotFound(err))
	}
	v, err := h.store.GetGoFuncVersion(c.Request().Context(), pc.ID, name, ver)
	if err != nil {
		return WriteError(c, mapNotFound(err))
	}
	body := req.Body
	if len(body) == 0 {
		body = json.RawMessage(`{}`)
	}
	// GF-RUN-04：与 /go 调用共享全局并发上限，满载 503。
	if !h.acquireRunSlot() {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "too_many_go_runs", "go function run concurrency limit reached", rid))
	}
	defer h.releaseRunSlot()
	start := time.Now()
	out, runErr := gofunction.RunJSON(c.Request().Context(), rid, name, v.Source, req.FunctionName, body)
	dur := time.Since(start)
	resp := goFuncTestResponse{
		OK:            runErr == nil,
		StatusCode:    http.StatusOK,
		DurationMs:    dur.Milliseconds(),
		Version:       ver,
		ActiveVersion: f.ActiveVersion,
		FunctionName:  req.FunctionName,
	}
	if runErr != nil {
		ae := mapGoFunctionRunError(runErr, rid)
		var he *APIError
		if errors.As(ae, &he) {
			resp.StatusCode = he.HTTPStatus
			resp.Error = he.Body.Error.Message
		} else {
			resp.StatusCode = http.StatusInternalServerError
			resp.Error = runErr.Error()
		}
	} else {
		resp.Data = out
	}
	h.store.RecordGoFuncInvoke(c.Request().Context(), pc.ID, name, req.FunctionName, ver, "test",
		resp.StatusCode, dur.Milliseconds(), rid, principalOf(c).Actor())
	h.recordInvokeMetrics(pc.ID, dur, runErr)
	h.recordAudit(c, pc.ID, "test "+name+".go v"+strconv.FormatInt(ver, 10))
	return c.JSON(http.StatusOK, resp)
}

// Invoke: POST /go/:projectID/:name/:functionName
func (h *GoFunctionHandler) Invoke(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	start := time.Now()
	var invokeErr error
	defer func() { h.recordInvokeMetrics(pc.ID, time.Since(start), invokeErr) }()
	fail := func(err error) error {
		invokeErr = err
		return WriteError(c, err)
	}

	name := c.Param("name")
	fnName := c.Param("functionName")
	rid := requestID(c)
	if !gofunctionNameRe.MatchString(name) {
		return fail(NewAPIError(http.StatusBadRequest, "invalid_gofunction_name", "invalid go function name", rid))
	}
	if !gofunctionExportNameRe.MatchString(fnName) {
		return fail(NewAPIError(http.StatusNotFound, "function_not_found", "function not found", rid))
	}
	if h.runner != nil {
		body, readErr := io.ReadAll(c.Request().Body)
		if readErr != nil {
			return fail(badRequest(c, "cannot read request body"))
		}
		result, runErr := h.runner.Run(c.Request().Context(), functionrun.Request{ProjectID: pc.ID, FuncFile: name, FuncExport: fnName, Input: body, Channel: "go", RequestID: rid, Actor: principalOf(c).Actor()})
		if runErr != nil {
			if errors.Is(runErr, functionrun.ErrTooManyRuns) {
				return fail(NewAPIError(http.StatusServiceUnavailable, "too_many_go_runs", "go function run concurrency limit reached", rid))
			}
			if errors.Is(runErr, sql.ErrNoRows) {
				return fail(NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", rid))
			}
			if errors.Is(runErr, systemdb.ErrNoActiveVersion) {
				return fail(NewAPIError(http.StatusConflict, "no_active_version", "go function has no active version", rid))
			}
			return fail(mapGoFunctionRunError(runErr, rid))
		}
		return c.Blob(http.StatusOK, "application/json", result.Output)
	}
	// D8：只跑 active_version
	v, err := h.store.ResolveActiveSource(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return fail(NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", rid))
	}
	if errors.Is(err, systemdb.ErrNoActiveVersion) {
		return fail(NewAPIError(http.StatusConflict, "no_active_version", "go function has no active version", rid))
	}
	if err != nil {
		return fail(err)
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return fail(badRequest(c, "cannot read request body"))
	}
	// GF-RUN-04：编译执行有全局并发上限，满载立即 503（不排队，
	// 避免高频调用把请求队列也打满）。
	if !h.acquireRunSlot() {
		return fail(NewAPIError(http.StatusServiceUnavailable, "too_many_go_runs", "go function run concurrency limit reached", rid))
	}
	defer h.releaseRunSlot()
	out, runErr := gofunction.RunJSON(c.Request().Context(), rid, name, v.Source, fnName, body)
	dur := time.Since(start)
	status := http.StatusOK
	if runErr != nil {
		ae := mapGoFunctionRunError(runErr, rid)
		var he *APIError
		if errors.As(ae, &he) {
			status = he.HTTPStatus
		} else {
			status = http.StatusInternalServerError
		}
		h.store.RecordGoFuncInvoke(c.Request().Context(), pc.ID, name, fnName, v.Version, "go",
			status, dur.Milliseconds(), rid, principalOf(c).Actor())
		return fail(ae)
	}
	h.store.RecordGoFuncInvoke(c.Request().Context(), pc.ID, name, fnName, v.Version, "go",
		status, dur.Milliseconds(), rid, principalOf(c).Actor())
	return c.Blob(http.StatusOK, "application/json", out)
}

func (h *GoFunctionHandler) loadFunc(c echo.Context, projectID, name string) (systemdb.GoFunc, []systemdb.GoFuncVersion, error) {
	rid := requestID(c)
	f, err := h.store.GetGoFunc(c.Request().Context(), projectID, name)
	if err != nil {
		return systemdb.GoFunc{}, nil, mapNotFound(err)
	}
	vers, err := h.store.ListGoFuncVersions(c.Request().Context(), projectID, name)
	if err != nil {
		_ = rid
		return systemdb.GoFunc{}, nil, err
	}
	return f, vers, nil
}

func (h *GoFunctionHandler) pickSource(c echo.Context, projectID, name string, f systemdb.GoFunc) (string, error) {
	if f.ActiveVersion > 0 {
		v, err := h.store.GetGoFuncVersion(c.Request().Context(), projectID, name, f.ActiveVersion)
		if err == nil {
			return v.Source, nil
		}
	}
	v, err := h.store.LatestGoFuncVersion(c.Request().Context(), projectID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v.Source, nil
}

func (h *GoFunctionHandler) recordInvokeMetrics(projectID string, dur time.Duration, runErr error) {
	if h.store == nil || projectID == "" || systemdb.IsAdminProject(projectID) {
		return
	}
	h.store.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "gofunction_invokes", Value: 1})
	h.store.RecordMetric(systemdb.MetricSample{
		ProjectID: projectID, Name: "gofunction_invoke_duration_ms", Value: float64(dur.Milliseconds()),
	})
	if runErr != nil {
		h.store.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "gofunction_invoke_errors", Value: 1})
	}
}
