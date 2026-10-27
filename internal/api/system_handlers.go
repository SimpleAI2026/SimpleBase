// 文件说明：system_handlers.go — internal/api 的内部实现（metricsHandler、Summary 等）
//
// 用法
//
//	type metricsHandler（包内使用）
//	type logsHTTPHandler（包内使用）
//	同包调用：List() → api_keys_handler.go, backup_handler.go 等；Export() → collection_transfer.go, router.go；Stats() → router.go, stage_timing.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/system_handlers.go
//	         │ 依赖
//	         ▼
//	[下游] internal/auth, internal/systemdb
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] List() ← api_keys_handler.go, backup_handler.go, cronjob_handler.go
//
// 主要符号
//
//	type metricsHandler struct {…}
//	(h *metricsHandler) Summary(c echo.Context) error
//	(h *metricsHandler) Trend(c echo.Context) error
//	type logsHTTPHandler struct {…}
//	(h *logsHTTPHandler) List(c echo.Context) error
//	(h *logsHTTPHandler) Stats(c echo.Context) error
//	... 另有 12 个顶层符号

package api

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type metricsHandler struct{ store *systemdb.Store }

func (h *metricsHandler) Summary(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	sum, err := h.store.MetricsSummaryCached(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, sum)
}

func (h *metricsHandler) Trend(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	days := 7
	if s := c.QueryParam("days"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			days = n
		}
	}
	points, err := h.store.MetricsTrendCached(c.Request().Context(), pc.ID, days)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"points": points})
}

type logsHTTPHandler struct{ store *systemdb.Store }

func (h *logsHTTPHandler) List(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	q := systemdb.LogQuery{ProjectID: pc.ID, Level: c.QueryParam("level"), Q: c.QueryParam("q")}
	if s := c.QueryParam("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			q.Limit = n
		}
	}
	if s := c.QueryParam("from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			q.From = t
		}
	}
	if s := c.QueryParam("to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			q.To = t
		}
	}
	events, err := h.store.QueryLogs(c.Request().Context(), q)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"events": events})
}

func (h *logsHTTPHandler) Stats(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	from, to, err := logTimeWindow(c)
	if err != nil {
		return WriteError(c, err)
	}
	items, err := h.store.LogStats(c.Request().Context(), pc.ID, c.QueryParam("group_by"), from, to)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items})
}

func (h *logsHTTPHandler) Export(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	from, to, err := logTimeWindow(c)
	if err != nil {
		return WriteError(c, err)
	}
	events, err := h.store.ExportLogs(c.Request().Context(), systemdb.LogQuery{ProjectID: pc.ID, Level: c.QueryParam("level"), Q: c.QueryParam("q"), From: from, To: to})
	if err != nil {
		return WriteError(c, err)
	}
	c.Response().Header().Set(echo.HeaderContentType, "text/csv; charset=utf-8")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="logs.csv"`)
	c.Response().WriteHeader(http.StatusOK)
	writer := csv.NewWriter(c.Response())
	_ = writer.Write([]string{"id", "project_id", "level", "logger", "message", "fields_json", "request_id", "occurred_at"})
	for _, event := range events {
		_ = writer.Write([]string{event.ID, event.ProjectID, event.Level, event.Logger, event.Message, event.FieldsJSON, event.RequestID, event.OccurredAt.UTC().Format(time.RFC3339Nano)})
	}
	writer.Flush()
	return writer.Error()
}

func logTimeWindow(c echo.Context) (time.Time, time.Time, error) {
	to := time.Now().UTC()
	from := to.Add(-24 * time.Hour)
	var err error
	if value := c.QueryParam("from"); value != "" {
		from, err = time.Parse(time.RFC3339, value)
	}
	if err == nil && c.QueryParam("to") != "" {
		to, err = time.Parse(time.RFC3339, c.QueryParam("to"))
	}
	if err != nil || from.After(to) || to.Sub(from) > 30*24*time.Hour {
		return time.Time{}, time.Time{}, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid time window; maximum is 30 days", RequestIDFromContext(c.Request().Context()))
	}
	return from, to, nil
}

func (h *logsHTTPHandler) GetRetention(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	r, err := h.store.GetRetention(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"scope": r.Scope, "keep_days": r.KeepDays, "updated_at": r.UpdatedAt})
}

func (h *logsHTTPHandler) PutRetention(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	var body struct {
		KeepDays int `json:"keep_days"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if err := h.store.PutRetention(c.Request().Context(), pc.ID, body.KeepDays); err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"keep_days": body.KeepDays})
}

type settingsHandler struct{ store *systemdb.Store }

func (h *settingsHandler) GetProject(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	rows, err := h.store.ListProjectSettings(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"settings": rows})
}

func (h *settingsHandler) PutProject(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	var body struct {
		Key       string `json:"key"`
		ValueJSON string `json:"value_json"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if body.Key == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "key is required"))
	}
	if err := h.store.PutProjectSetting(c.Request().Context(), pc.ID, body.Key, body.ValueJSON); err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func (h *settingsHandler) GetGlobal(c echo.Context) error {
	rows, err := h.store.ListGlobalSettings(c.Request().Context())
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"settings": rows})
}

func (h *settingsHandler) PutGlobal(c echo.Context) error {
	if _, ok := PrincipalFromContext(c.Request().Context()); !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	var body struct {
		Key       string `json:"key"`
		ValueJSON string `json:"value_json"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if body.Key == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "key is required"))
	}
	if err := h.store.PutGlobalSetting(c.Request().Context(), body.Key, body.ValueJSON); err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

type llmSessionHandler struct{ store *systemdb.Store }

// GetSettings 返回项目模型配置。api_key 只回掩码，原文不出网。
func (h *llmSessionHandler) GetSettings(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	st, err := h.store.GetLLMSettings(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"default_provider": st.DefaultProvider,
		"default_model":    st.DefaultModel,
		"base_url":         st.BaseURL,
		"api_key_masked":   systemdb.MaskSecretValue(st.APIKey),
		"has_api_key":      st.APIKey != "",
		"temperature":      st.Temperature,
		"max_tokens":       st.MaxTokens,
	})
}

func (h *llmSessionHandler) PutSettings(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	var body struct {
		DefaultProvider string   `json:"default_provider"`
		DefaultModel    string   `json:"default_model"`
		APIKey          string   `json:"api_key"`
		BaseURL         string   `json:"base_url"`
		Temperature     *float64 `json:"temperature"`
		MaxTokens       *int     `json:"max_tokens"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	st := systemdb.LLMSettings{
		ProjectID:       pc.ID,
		DefaultProvider: body.DefaultProvider,
		DefaultModel:    body.DefaultModel,
		APIKey:          body.APIKey,
		BaseURL:         body.BaseURL,
		Temperature:     0.7,
		MaxTokens:       1024,
	}
	if body.Temperature != nil {
		st.Temperature = *body.Temperature
	}
	if body.MaxTokens != nil {
		st.MaxTokens = *body.MaxTokens
	}
	if err := h.store.PutLLMSettings(c.Request().Context(), st); err != nil {
		return WriteError(c, err)
	}
	saved, err := h.store.GetLLMSettings(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"default_provider": saved.DefaultProvider,
		"default_model":    saved.DefaultModel,
		"base_url":         saved.BaseURL,
		"api_key_masked":   systemdb.MaskSecretValue(saved.APIKey),
		"has_api_key":      saved.APIKey != "",
		"temperature":      saved.Temperature,
		"max_tokens":       saved.MaxTokens,
	})
}
