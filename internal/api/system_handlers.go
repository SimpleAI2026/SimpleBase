package api

import (
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
		Temperature     *float64 `json:"temperature"`
		MaxTokens       *int     `json:"max_tokens"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	st := systemdb.LLMSettings{ProjectID: pc.ID, DefaultProvider: body.DefaultProvider, DefaultModel: body.DefaultModel, Temperature: 0.7, MaxTokens: 1024}
	if body.Temperature != nil {
		st.Temperature = *body.Temperature
	}
	if body.MaxTokens != nil {
		st.MaxTokens = *body.MaxTokens
	}
	if err := h.store.PutLLMSettings(c.Request().Context(), st); err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"default_provider": st.DefaultProvider,
		"default_model":    st.DefaultModel,
		"temperature":      st.Temperature,
		"max_tokens":       st.MaxTokens,
	})
}

type llmProviderCredHandler struct{ store *systemdb.Store }

// ListProviderCreds 返回项目全部厂商凭证（脱敏：secret 字段仅掩码）。
func (h *llmProviderCredHandler) ListProviderCreds(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	rows, err := h.store.ListLLMProviderCreds(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"providers": systemdb.MaskedLLMProviderCreds(rows)})
}

// PutProviderCred 保存单个厂商凭证。secret 字段留空表示沿用既有值。
func (h *llmProviderCredHandler) PutProviderCred(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	provider := c.Param("provider")
	if provider == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "provider is required"))
	}
	var body struct {
		Credentials map[string]string `json:"credentials"`
		DefaultModel string           `json:"default_model"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if err := h.store.UpsertLLMProviderCred(c.Request().Context(), pc.ID, provider, body.DefaultModel, body.Credentials); err != nil {
		return WriteError(c, err)
	}
	row, err := h.store.GetLLMProviderCred(c.Request().Context(), pc.ID, provider)
	if err != nil {
		return WriteError(c, err)
	}
	masked := systemdb.MaskedLLMProviderCreds([]systemdb.LLMProviderCred{row})
	return c.JSON(http.StatusOK, masked[0])
}

// DeleteProviderCred 清除单个厂商凭证。
func (h *llmProviderCredHandler) DeleteProviderCred(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	provider := c.Param("provider")
	if provider == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "provider is required"))
	}
	if err := h.store.DeleteLLMProviderCred(c.Request().Context(), pc.ID, provider); err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}
