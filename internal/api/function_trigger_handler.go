// 文件说明：function_trigger_handler.go — internal/api 中 FunctionTriggerHandler、NewFunctionTriggerHandler 等符号的…
//
// 用法
//
//	NewFunctionTriggerHandler(store *systemdb.Store, dispatcher *trigger.Dispatcher ...
//	(h *FunctionTriggerHandler) List(c echo.Context) error
//	(h *FunctionTriggerHandler) Get(c echo.Context) error
//	同包调用：Get() → collection_meta_handler.go, collection_transfer.go 等；List() → api_keys_handler.go, backup_handler.go 等；Delete() → api_keys_handler.go, backup_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/function_trigger_handler.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, internal/objectstore, internal/systemdb, internal/trigger
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Get() ← collection_meta_handler.go, collection_transfer.go, cronjob_handler.go
//
// 主要符号
//
//	type FunctionTriggerHandler struct {…}
//	NewFunctionTriggerHandler(store *systemdb.Store, dispatcher *trigger.Dispatcher ...
//	(h *FunctionTriggerHandler) List(c echo.Context) error
//	(h *FunctionTriggerHandler) Get(c echo.Context) error
//	(h *FunctionTriggerHandler) Create(c echo.Context) error
//	(h *FunctionTriggerHandler) Update(c echo.Context) error
//	... 另有 5 个顶层符号

package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/trigger"
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
		return WriteError(c, err)
	}
	items, err := h.store.ListFunctionTriggers(c.Request().Context(), projectID(c))
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items, "dropped": h.dispatcher.Dropped()})
}

func (h *FunctionTriggerHandler) Get(c echo.Context) error {
	if err := h.guard(c); err != nil {
		return WriteError(c, err)
	}
	item, err := h.store.GetFunctionTrigger(c.Request().Context(), projectID(c), c.Param("id"))
	if err != nil {
		return WriteError(c, mapNotFound(err))
	}
	return c.JSON(http.StatusOK, item)
}

func (h *FunctionTriggerHandler) Create(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return WriteError(c, err)
	}
	var item systemdb.FunctionTrigger
	if err := decodeJSONBody(c, &item, false); err != nil {
		return WriteError(c, err)
	}
	item.ID = ""
	item.ProjectID = projectID(c)
	item.CreatedBy = actorID(c)
	if err := h.validate(c, &item); err != nil {
		return WriteError(c, err)
	}
	created, err := h.store.CreateFunctionTrigger(c.Request().Context(), item)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, created)
}

func (h *FunctionTriggerHandler) Update(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return WriteError(c, err)
	}
	item, err := h.store.GetFunctionTrigger(c.Request().Context(), projectID(c), c.Param("id"))
	if err != nil {
		return WriteError(c, mapNotFound(err))
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
	if err := decodeJSONBody(c, &patch, false); err != nil {
		return WriteError(c, err)
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
		return WriteError(c, err)
	}
	updated, err := h.store.UpdateFunctionTrigger(c.Request().Context(), item)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, updated)
}

func (h *FunctionTriggerHandler) Delete(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return WriteError(c, err)
	}
	if err := h.store.ArchiveFunctionTrigger(c.Request().Context(), projectID(c), c.Param("id")); err != nil {
		return WriteError(c, mapNotFound(err))
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *FunctionTriggerHandler) Invoke(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return WriteError(c, err)
	}
	item, err := h.store.GetFunctionTrigger(c.Request().Context(), projectID(c), c.Param("id"))
	if err != nil {
		return WriteError(c, mapNotFound(err))
	}
	if item.Kind != "http" || !item.Enabled {
		return WriteError(c, NewAPIError(http.StatusConflict, "trigger_not_invokable", "only enabled HTTP triggers can be invoked", RequestIDFromContext(c.Request().Context())))
	}
	var body map[string]any
	if err := decodeJSONBody(c, &body, true); err != nil {
		return WriteError(c, err)
	}
	if err := h.dispatcher.Invoke(item, body); err != nil {
		if errors.Is(err, trigger.ErrQueueFull) {
			return WriteError(c, NewAPIError(http.StatusTooManyRequests, "trigger_queue_full", "trigger queue is full", RequestIDFromContext(c.Request().Context())))
		}
		return WriteError(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]any{"queued": true})
}

func (h *FunctionTriggerHandler) validate(c echo.Context, item *systemdb.FunctionTrigger) error {
	item.FuncFile = strings.TrimSuffix(strings.TrimSpace(item.FuncFile), ".go")
	if item.Kind != "record_change" && item.Kind != "object_event" && item.Kind != "http" || item.Target == "" && item.Kind != "http" || !gofunctionNameRe.MatchString(item.FuncFile) || !gofunctionExportNameRe.MatchString(item.FuncExport) || len(item.Filter) > 2048 || !trigger.ValidateFilter(item.Filter) {
		return NewAPIError(http.StatusBadRequest, "invalid_trigger", "invalid trigger target, function or filter", RequestIDFromContext(c.Request().Context()))
	}
	if item.Kind == "record_change" && item.Target != "*" && !collectionNamePattern.MatchString(item.Target) {
		return NewAPIError(http.StatusBadRequest, "invalid_trigger", "invalid record collection", RequestIDFromContext(c.Request().Context()))
	}
	if item.Kind == "object_event" && item.Target != "*" {
		if err := objectstore.ValidateFileKey(item.Target); err != nil {
			return NewAPIError(http.StatusBadRequest, "invalid_trigger", "invalid object prefix", RequestIDFromContext(c.Request().Context()))
		}
	}
	if _, err := h.store.ResolveActiveSource(c.Request().Context(), item.ProjectID, item.FuncFile); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", RequestIDFromContext(c.Request().Context()))
		}
		return err
	}
	return nil
}

func (h *FunctionTriggerHandler) guard(c echo.Context) error {
	if h.store == nil || h.dispatcher == nil {
		return systemdb.ErrUnavailable
	}
	if systemdb.IsAdminProject(projectID(c)) {
		return NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support function triggers", RequestIDFromContext(c.Request().Context()))
	}
	return nil
}

func (h *FunctionTriggerHandler) writeGuard(c echo.Context) error {
	if !h.writable {
		return godatabase.ErrWriterUnavailable
	}
	return h.guard(c)
}
