package llmhttp

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

// TracesHandler 查询 sys_llm_traces（planv6.5 model-settings-and-trace-plan §3.3）。
type TracesHandler struct{ store *systemdb.Store }

// NewTracesHandler 构造 LLM trace 查询 handler（只读）。
func NewTracesHandler(store *systemdb.Store) *TracesHandler { return &TracesHandler{store: store} }

// List 处理 GET /v1/projects/:id/llm/traces。
func (h *TracesHandler) List(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "system store unavailable")
	}
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	q := systemdb.LLMTraceQuery{ProjectID: pc.ID}
	q.Status = c.QueryParam("status")
	if q.Status != "" && q.Status != "ok" && q.Status != "error" {
		q.Status = ""
	}
	q.Model = c.QueryParam("model")
	q.RequestID = c.QueryParam("request_id")
	if raw := c.QueryParam("from"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			q.From = t
		}
	}
	if raw := c.QueryParam("to"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			q.To = t
		}
	}
	if raw := c.QueryParam("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			q.Limit = n
		}
	}
	if raw := c.QueryParam("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			q.Offset = n
		}
	}
	traces, err := h.store.QueryLLMTraces(c.Request().Context(), q)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	total, err := h.store.CountLLMTraces(c.Request().Context(), q)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"traces": traces, "total": total})
}

// —— 二期（planv6.5 §9.3）：分组列表 + span 详情 ——

// parseTraceTimeRange 解析 from/to 查询参数（RFC3339）。
func parseTraceTimeRange(c echo.Context) (time.Time, time.Time) {
	var from, to time.Time
	if raw := c.QueryParam("from"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			from = t
		}
	}
	if raw := c.QueryParam("to"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			to = t
		}
	}
	return from, to
}

// Groups 处理 GET /v1/projects/:id/llm/trace-groups：按 trace_id 聚合的顶层调用列表。
// 参数：caller（api|agent|cron）/ status / model（模糊）/ from / to / limit / offset。
func (h *TracesHandler) Groups(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "system store unavailable")
	}
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	q := systemdb.LLMTraceGroupQuery{ProjectID: pc.ID}
	q.Caller = c.QueryParam("caller")
	if q.Caller != "" && q.Caller != "api" && q.Caller != "agent" && q.Caller != "cron" {
		q.Caller = ""
	}
	q.Status = c.QueryParam("status")
	if q.Status != "" && q.Status != "ok" && q.Status != "error" {
		q.Status = ""
	}
	q.Model = c.QueryParam("model")
	q.From, q.To = parseTraceTimeRange(c)
	if raw := c.QueryParam("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			q.Limit = n
		}
	}
	if raw := c.QueryParam("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			q.Offset = n
		}
	}
	groups, err := h.store.QueryLLMTraceGroups(c.Request().Context(), q)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	total, err := h.store.CountLLMTraceGroups(c.Request().Context(), q)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"groups": groups, "total": total})
}

// Detail 处理 GET /v1/projects/:id/llm/trace-groups/:traceId：一个 trace 的全部 span。
func (h *TracesHandler) Detail(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "system store unavailable")
	}
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	traceID := c.Param("traceId")
	if traceID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "trace id required")
	}
	spans, err := h.store.QueryTraceSpans(c.Request().Context(), pc.ID, traceID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"spans": spans})
}
