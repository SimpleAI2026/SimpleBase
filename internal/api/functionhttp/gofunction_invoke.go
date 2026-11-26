package functionhttp

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

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
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
		return httpkit.WriteError(c, err)
	}
	name := c.Param("name")
	ver, err := parseVersionParam(c, rid)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var req goFuncTestRequest
	if err := bindGoFunctionBody(c, &req); err != nil {
		return httpkit.WriteError(c, err)
	}
	if !gofunctionExportNameRe.MatchString(req.FunctionName) {
		return httpkit.WriteError(c, httpkit.BadRequest(c, "invalid function_name"))
	}
	f, err := h.store.GetGoFunc(c.Request().Context(), pc.ID, name)
	if err != nil {
		return httpkit.WriteError(c, mapNotFound(err))
	}
	v, err := h.store.GetGoFuncVersion(c.Request().Context(), pc.ID, name, ver)
	if err != nil {
		return httpkit.WriteError(c, mapNotFound(err))
	}
	body := req.Body
	if len(body) == 0 {
		body = json.RawMessage(`{}`)
	}
	// GF-RUN-04：与 /go 调用共享全局并发上限，满载 503。
	if !h.acquireRunSlot() {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusServiceUnavailable, "too_many_go_runs", "go function run concurrency limit reached", rid))
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
		var he *httpkit.APIError
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
		resp.StatusCode, dur.Milliseconds(), rid, httpkit.PrincipalOf(c).Actor())
	h.recordInvokeMetrics(pc.ID, dur, runErr)
	h.recordAudit(c, pc.ID, "test "+name+".go v"+strconv.FormatInt(ver, 10))
	return c.JSON(http.StatusOK, resp)
}

// Invoke: POST /go/:projectID/:name/:functionName
func (h *GoFunctionHandler) Invoke(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.store == nil {
		return httpkit.WriteError(c, systemdb.ErrUnavailable)
	}
	start := time.Now()
	var invokeErr error
	defer func() { h.recordInvokeMetrics(pc.ID, time.Since(start), invokeErr) }()
	fail := func(err error) error {
		invokeErr = err
		return httpkit.WriteError(c, err)
	}

	name := c.Param("name")
	fnName := c.Param("functionName")
	rid := httpkit.RequestID(c)
	if !gofunctionNameRe.MatchString(name) {
		return fail(httpkit.NewAPIError(http.StatusBadRequest, "invalid_gofunction_name", "invalid go function name", rid))
	}
	if !gofunctionExportNameRe.MatchString(fnName) {
		return fail(httpkit.NewAPIError(http.StatusNotFound, "function_not_found", "function not found", rid))
	}
	if h.runner != nil {
		body, readErr := io.ReadAll(c.Request().Body)
		if readErr != nil {
			return fail(httpkit.BadRequest(c, "cannot read request body"))
		}
		result, runErr := h.runner.Run(c.Request().Context(), functionrun.Request{ProjectID: pc.ID, FuncFile: name, FuncExport: fnName, Input: body, Channel: "go", RequestID: rid, Actor: httpkit.PrincipalOf(c).Actor()})
		if runErr != nil {
			if errors.Is(runErr, functionrun.ErrTooManyRuns) {
				return fail(httpkit.NewAPIError(http.StatusServiceUnavailable, "too_many_go_runs", "go function run concurrency limit reached", rid))
			}
			if errors.Is(runErr, sql.ErrNoRows) {
				return fail(httpkit.NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", rid))
			}
			if errors.Is(runErr, systemdb.ErrNoActiveVersion) {
				return fail(httpkit.NewAPIError(http.StatusConflict, "no_active_version", "go function has no active version", rid))
			}
			return fail(mapGoFunctionRunError(runErr, rid))
		}
		return c.Blob(http.StatusOK, "application/json", result.Output)
	}
	// D8：只跑 active_version
	v, err := h.store.ResolveActiveSource(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return fail(httpkit.NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", rid))
	}
	if errors.Is(err, systemdb.ErrNoActiveVersion) {
		return fail(httpkit.NewAPIError(http.StatusConflict, "no_active_version", "go function has no active version", rid))
	}
	if err != nil {
		return fail(err)
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return fail(httpkit.BadRequest(c, "cannot read request body"))
	}
	// GF-RUN-04：编译执行有全局并发上限，满载立即 503（不排队，
	// 避免高频调用把请求队列也打满）。
	if !h.acquireRunSlot() {
		return fail(httpkit.NewAPIError(http.StatusServiceUnavailable, "too_many_go_runs", "go function run concurrency limit reached", rid))
	}
	defer h.releaseRunSlot()
	out, runErr := gofunction.RunJSON(c.Request().Context(), rid, name, v.Source, fnName, body)
	dur := time.Since(start)
	status := http.StatusOK
	if runErr != nil {
		ae := mapGoFunctionRunError(runErr, rid)
		var he *httpkit.APIError
		if errors.As(ae, &he) {
			status = he.HTTPStatus
		} else {
			status = http.StatusInternalServerError
		}
		h.store.RecordGoFuncInvoke(c.Request().Context(), pc.ID, name, fnName, v.Version, "go",
			status, dur.Milliseconds(), rid, httpkit.PrincipalOf(c).Actor())
		return fail(ae)
	}
	h.store.RecordGoFuncInvoke(c.Request().Context(), pc.ID, name, fnName, v.Version, "go",
		status, dur.Milliseconds(), rid, httpkit.PrincipalOf(c).Actor())
	return c.Blob(http.StatusOK, "application/json", out)
}

func (h *GoFunctionHandler) loadFunc(c echo.Context, projectID, name string) (systemdb.GoFunc, []systemdb.GoFuncVersion, error) {
	rid := httpkit.RequestID(c)
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
