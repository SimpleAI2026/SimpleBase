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
	sum, err := h.store.MetricsSummary(c.Request().Context(), pc.ID)
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
	points, err := h.store.MetricsTrend(c.Request().Context(), pc.ID, days)
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

type llmSessionDTO struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toSessionDTO(s systemdb.LLMSession) llmSessionDTO {
	return llmSessionDTO{ID: s.ID, Title: s.Title, Provider: s.Provider, Model: s.Model, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
}

func (h *llmSessionHandler) List(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	list, err := h.store.ListLLMSessions(c.Request().Context(), pc.ID, 50)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]llmSessionDTO, 0, len(list))
	for _, s := range list {
		out = append(out, toSessionDTO(s))
	}
	return c.JSON(http.StatusOK, map[string]any{"sessions": out})
}

func (h *llmSessionHandler) Create(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	var body struct {
		Title    string `json:"title"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	_ = c.Bind(&body)
	createdBy := ""
	if p, ok := PrincipalFromContext(c.Request().Context()); ok {
		createdBy = p.APIKeyID
	}
	sess, err := h.store.CreateLLMSession(c.Request().Context(), systemdb.LLMSession{
		ProjectID: pc.ID, Title: body.Title, Provider: body.Provider, Model: body.Model, CreatedBy: createdBy,
	})
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, toSessionDTO(sess))
}

func (h *llmSessionHandler) Get(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	sess, err := h.store.GetLLMSession(c.Request().Context(), pc.ID, c.Param("sessionID"))
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toSessionDTO(sess))
}

func (h *llmSessionHandler) Delete(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	if err := h.store.ArchiveLLMSession(c.Request().Context(), pc.ID, c.Param("sessionID")); err != nil {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *llmSessionHandler) ListMessages(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	msgs, err := h.store.ListLLMMessages(c.Request().Context(), pc.ID, c.Param("sessionID"), 200)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"messages": msgs})
}

func (h *llmSessionHandler) PostMessage(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	var body struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if body.Role == "" || body.Content == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "role and content required"))
	}
	msg, err := h.store.AppendLLMMessage(c.Request().Context(), systemdb.LLMMessage{
		SessionID: c.Param("sessionID"), ProjectID: pc.ID, Role: body.Role, Content: body.Content,
		RequestID: RequestIDFromContext(c.Request().Context()),
	})
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, msg)
}

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
