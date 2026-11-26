package functionhttp

import (
	"database/sql"
	"errors"
	"github.com/linkxzhou/SimpleBase/internal/collection"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/trigger"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

type FunctionTriggerHandler struct {
	store      *systemdb.Store
	dispatcher *trigger.Dispatcher
	writable   bool
}

func NewFunctionTriggerHandler(store *systemdb.Store, dispatcher *trigger.Dispatcher, writable bool) *FunctionTriggerHandler {
	return &FunctionTriggerHandler{store: store, dispatcher: dispatcher, writable: writable}
}

func (h *FunctionTriggerHandler) List(c echo.Context) error {
	if err := h.guard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	items, err := h.store.ListFunctionTriggers(c.Request().Context(), httpkit.ProjectID(c))
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items, "dropped": h.dispatcher.Dropped()})
}

func (h *FunctionTriggerHandler) Get(c echo.Context) error {
	if err := h.guard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	item, err := h.store.GetFunctionTrigger(c.Request().Context(), httpkit.ProjectID(c), c.Param("id"))
	if err != nil {
		return httpkit.WriteError(c, mapNotFound(err))
	}
	return c.JSON(http.StatusOK, item)
}

func (h *FunctionTriggerHandler) Create(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	var item systemdb.FunctionTrigger
	if err := httpkit.DecodeJSONBody(c, &item, false); err != nil {
		return httpkit.WriteError(c, err)
	}
	item.ID = ""
	item.ProjectID = httpkit.ProjectID(c)
	item.CreatedBy = httpkit.PrincipalOf(c).Actor()
	if err := h.validate(c, &item); err != nil {
		return httpkit.WriteError(c, err)
	}
	created, err := h.store.CreateFunctionTrigger(c.Request().Context(), item)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, created)
}

func (h *FunctionTriggerHandler) Update(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	item, err := h.store.GetFunctionTrigger(c.Request().Context(), httpkit.ProjectID(c), c.Param("id"))
	if err != nil {
		return httpkit.WriteError(c, mapNotFound(err))
	}
	var patch struct {
		Kind       *string   `json:"kind"`
		Target     *string   `json:"target"`
		Events     *[]string `json:"events"`
		FuncFile   *string   `json:"func_file"`
		FuncExport *string   `json:"func_export"`
		Filter     *string   `json:"filter"`
		Enabled    *bool     `json:"enabled"`
	}
	if err := httpkit.DecodeJSONBody(c, &patch, false); err != nil {
		return httpkit.WriteError(c, err)
	}
	if patch.Kind != nil {
		item.Kind = *patch.Kind
	}
	if patch.Target != nil {
		item.Target = *patch.Target
	}
	if patch.Events != nil {
		item.Events = *patch.Events
	}
	if patch.FuncFile != nil {
		item.FuncFile = *patch.FuncFile
	}
	if patch.FuncExport != nil {
		item.FuncExport = *patch.FuncExport
	}
	if patch.Filter != nil {
		item.Filter = *patch.Filter
	}
	if patch.Enabled != nil {
		item.Enabled = *patch.Enabled
	}
	if err := h.validate(c, &item); err != nil {
		return httpkit.WriteError(c, err)
	}
	updated, err := h.store.UpdateFunctionTrigger(c.Request().Context(), item)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, updated)
}

func (h *FunctionTriggerHandler) Delete(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := h.store.ArchiveFunctionTrigger(c.Request().Context(), httpkit.ProjectID(c), c.Param("id")); err != nil {
		return httpkit.WriteError(c, mapNotFound(err))
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *FunctionTriggerHandler) Invoke(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	item, err := h.store.GetFunctionTrigger(c.Request().Context(), httpkit.ProjectID(c), c.Param("id"))
	if err != nil {
		return httpkit.WriteError(c, mapNotFound(err))
	}
	if item.Kind != "http" || !item.Enabled {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusConflict, "trigger_not_invokable", "only enabled HTTP triggers can be invoked", httpkit.RequestID(c)))
	}
	var body map[string]any
	if err := httpkit.DecodeJSONBody(c, &body, true); err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := h.dispatcher.Invoke(item, body); err != nil {
		if errors.Is(err, trigger.ErrQueueFull) {
			return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusTooManyRequests, "trigger_queue_full", "trigger queue is full", httpkit.RequestID(c)))
		}
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]any{"queued": true})
}

func (h *FunctionTriggerHandler) validate(c echo.Context, item *systemdb.FunctionTrigger) error {
	item.FuncFile = strings.TrimSuffix(strings.TrimSpace(item.FuncFile), ".go")
	if item.Kind != "record_change" && item.Kind != "object_event" && item.Kind != "http" || item.Target == "" && item.Kind != "http" || !gofunctionNameRe.MatchString(item.FuncFile) || !gofunctionExportNameRe.MatchString(item.FuncExport) || len(item.Filter) > 2048 || !trigger.ValidateFilter(item.Filter) {
		return httpkit.NewAPIError(http.StatusBadRequest, "invalid_trigger", "invalid trigger target, function or filter", httpkit.RequestID(c))
	}
	if item.Kind == "record_change" && item.Target != "*" && !collection.NamePattern.MatchString(item.Target) {
		return httpkit.NewAPIError(http.StatusBadRequest, "invalid_trigger", "invalid record collection", httpkit.RequestID(c))
	}
	if item.Kind == "object_event" && item.Target != "*" {
		if err := objectstore.ValidateFileKey(item.Target); err != nil {
			return httpkit.NewAPIError(http.StatusBadRequest, "invalid_trigger", "invalid object prefix", httpkit.RequestID(c))
		}
	}
	if _, err := h.store.ResolveActiveSource(c.Request().Context(), item.ProjectID, item.FuncFile); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return httpkit.NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", httpkit.RequestID(c))
		}
		return err
	}
	return nil
}

func (h *FunctionTriggerHandler) guard(c echo.Context) error {
	if h.store == nil || h.dispatcher == nil {
		return systemdb.ErrUnavailable
	}
	if systemdb.IsAdminProject(httpkit.ProjectID(c)) {
		return httpkit.NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support function triggers", httpkit.RequestID(c))
	}
	return nil
}

func (h *FunctionTriggerHandler) writeGuard(c echo.Context) error {
	if !h.writable {
		return godatabase.ErrWriterUnavailable
	}
	return h.guard(c)
}
