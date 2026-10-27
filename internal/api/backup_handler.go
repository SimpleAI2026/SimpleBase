// 文件说明：backup_handler.go — internal/api 中 BackupService、BackupHandler 等符号的实现
//
// 用法
//
//	NewBackupHandler(service BackupService, audit AuditService, writable bool) ...
//	(h *BackupHandler) List(c echo.Context) error
//	(h *BackupHandler) Create(c echo.Context) error
//	同包调用：List() → api_keys_handler.go, cronjob_handler.go 等；Delete() → api_keys_handler.go, collection_meta_handler.go 等；Create() → api_keys_handler.go, cronjob_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/backup_handler.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, internal/backup, internal/catalog
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] List() ← api_keys_handler.go, cronjob_handler.go, data_handler.go
//
// 主要符号
//
//	type BackupService interface {…}
//	type BackupHandler struct {…}
//	NewBackupHandler(service BackupService, audit AuditService, writable bool) ...
//	(h *BackupHandler) List(c echo.Context) error
//	(h *BackupHandler) Create(c echo.Context) error
//	(h *BackupHandler) Delete(c echo.Context) error
//	... 另有 6 个顶层符号

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/backup"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

type BackupService interface {
	Create(context.Context, string, string) (backup.Item, error)
	List(context.Context, string) ([]backup.Item, error)
	Delete(context.Context, string, string) error
	Restore(context.Context, string, string, string) error
	OpenDownload(context.Context, string, string) (io.ReadCloser, backup.Item, error)
}

type BackupHandler struct {
	service  BackupService
	audit    AuditService
	writable bool
}

func NewBackupHandler(service BackupService, audit AuditService, writable bool) *BackupHandler {
	return &BackupHandler{service: service, audit: audit, writable: writable}
}

func (h *BackupHandler) List(c echo.Context) error {
	if err := h.guard(c, false); err != nil {
		return WriteError(c, err)
	}
	items, err := h.service.List(c.Request().Context(), c.Param("projectID"))
	if err != nil {
		return backupAPIError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items})
}

func (h *BackupHandler) Create(c echo.Context) error {
	if err := h.guard(c, true); err != nil {
		return WriteError(c, err)
	}
	item, err := h.service.Create(c.Request().Context(), tenantID(c), c.Param("projectID"))
	h.record(c, "backup_create", item.ID, item.SHA256, err)
	if err != nil {
		return backupAPIError(c, err)
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *BackupHandler) Delete(c echo.Context) error {
	if err := h.guard(c, true); err != nil {
		return WriteError(c, err)
	}
	err := h.service.Delete(c.Request().Context(), c.Param("projectID"), c.Param("id"))
	h.record(c, "backup_delete", c.Param("id"), "", err)
	if err != nil {
		return backupAPIError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *BackupHandler) Restore(c echo.Context) error {
	if err := h.guard(c, true); err != nil {
		return WriteError(c, err)
	}
	err := h.service.Restore(c.Request().Context(), tenantID(c), c.Param("projectID"), c.Param("id"))
	h.record(c, "backup_restore", c.Param("id"), "", err)
	if err != nil {
		return backupAPIError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *BackupHandler) Download(c echo.Context) error {
	if err := h.guard(c, false); err != nil {
		return WriteError(c, err)
	}
	reader, item, err := h.service.OpenDownload(c.Request().Context(), c.Param("projectID"), c.Param("id"))
	if err != nil {
		return backupAPIError(c, err)
	}
	defer reader.Close()
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEOctetStream)
	c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(item.SizeBytes, 10))
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+item.ID+"."+item.Engine+"\"")
	return c.Stream(http.StatusOK, echo.MIMEOctetStream, reader)
}

func (h *BackupHandler) guard(c echo.Context, write bool) error {
	if catalog.IsSystemProject(c.Param("projectID")) {
		return catalog.ErrSystemProtected
	}
	if write && !h.writable {
		return godatabase.ErrWriterUnavailable
	}
	return nil
}

func (h *BackupHandler) record(c echo.Context, kind, id, hash string, actionErr error) {
	if h.audit == nil {
		return
	}
	status := "success"
	if actionErr != nil {
		status = "error"
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	_ = h.audit.Record(c.Request().Context(), AuditEvent{ProjectID: c.Param("projectID"), PrincipalID: principal.APIKeyID, Kind: kind, RequestID: RequestIDFromContext(c.Request().Context()), Status: status, Detail: id + ":" + hash})
}

func backupAPIError(c echo.Context, err error) error {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, backup.ErrDisabled):
		status, code = http.StatusServiceUnavailable, "backup_disabled"
	case errors.Is(err, backup.ErrNotFound):
		status, code = http.StatusNotFound, "backup_not_found"
	case errors.Is(err, backup.ErrProjectBusy):
		status, code = http.StatusConflict, "project_busy"
	case errors.Is(err, backup.ErrSnapshotMismatch):
		status, code = http.StatusConflict, "snapshot_mismatch"
	case errors.Is(err, backup.ErrUnavailable):
		status, code = http.StatusServiceUnavailable, "backup_unavailable"
	}
	if code != "internal_error" {
		return WriteError(c, NewAPIError(status, code, code, RequestIDFromContext(c.Request().Context())))
	}
	return WriteError(c, err)
}

func tenantID(c echo.Context) string {
	project, _ := ProjectFromContext(c.Request().Context())
	return project.TenantID
}
