// 文件说明：backup.go — 备份 / 恢复的 HTTP 入站适配（internal-ddd-hexagonal-implementation-plan §6.2 backuphttp）
//
// 用法
//
//	h := backuphttp.New(service, audit, writable)   // app 构造；路由仍由 api 的 mountBackupRealtimeRoutes 注册
//	(h *Handler) List / Create / Delete / Restore / Download
//
// 调用架构
//
//	[上游] internal/app（构造）、internal/api（路由注册与守卫）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/backuphttp/backup.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/httpkit（错误协议 / 审计 / 项目上下文）, internal/backup, internal/catalog, godatabase
//
// 主要符号
//
//	type Service interface {…}
//	type Handler struct {…}
//	New(service Service, audit httpkit.AuditService, writable bool) *Handler
//	(h *Handler) List(c echo.Context) error
//	(h *Handler) Create(c echo.Context) error
//	(h *Handler) Delete(c echo.Context) error
//	(h *Handler) Restore(c echo.Context) error
//	(h *Handler) Download(c echo.Context) error
//
// 首个按业务域拆出的子包（机制验证）：权限 / 委托 / 写门禁在路由守卫，handler 内先判系统项目再判实例可写；
// 不 import api、app 或同级子包。

package backuphttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/backup"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// Service 是备份用例需要的最小契约（backup.Service 满足）；Item 原样透传，不复制 DTO。
type Service interface {
	Create(context.Context, string, string) (backup.Item, error)
	List(context.Context, string) ([]backup.Item, error)
	Delete(context.Context, string, string) error
	Restore(context.Context, string, string, string) error
	OpenDownload(context.Context, string, string) (io.ReadCloser, backup.Item, error)
}

// Handler 是备份 / 恢复的 HTTP 入站适配：五条 /v1/projects/:projectID/backups… 路由。
type Handler struct {
	service  Service
	audit    httpkit.AuditService
	writable bool
}

// New 构造 handler；audit 可为 nil（不记审计），writable=false 时写操作返回 503。
func New(service Service, audit httpkit.AuditService, writable bool) *Handler {
	return &Handler{service: service, audit: audit, writable: writable}
}

func (h *Handler) List(c echo.Context) error {
	if err := h.guard(c, false); err != nil {
		return httpkit.WriteError(c, err)
	}
	items, err := h.service.List(c.Request().Context(), c.Param("projectID"))
	if err != nil {
		return apiError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Create(c echo.Context) error {
	if err := h.guard(c, true); err != nil {
		return httpkit.WriteError(c, err)
	}
	item, err := h.service.Create(c.Request().Context(), tenantID(c), c.Param("projectID"))
	h.record(c, "backup_create", item.ID, item.SHA256, err)
	if err != nil {
		return apiError(c, err)
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *Handler) Delete(c echo.Context) error {
	if err := h.guard(c, true); err != nil {
		return httpkit.WriteError(c, err)
	}
	err := h.service.Delete(c.Request().Context(), c.Param("projectID"), c.Param("id"))
	h.record(c, "backup_delete", c.Param("id"), "", err)
	if err != nil {
		return apiError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) Restore(c echo.Context) error {
	if err := h.guard(c, true); err != nil {
		return httpkit.WriteError(c, err)
	}
	err := h.service.Restore(c.Request().Context(), tenantID(c), c.Param("projectID"), c.Param("id"))
	h.record(c, "backup_restore", c.Param("id"), "", err)
	if err != nil {
		return apiError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) Download(c echo.Context) error {
	if err := h.guard(c, false); err != nil {
		return httpkit.WriteError(c, err)
	}
	reader, item, err := h.service.OpenDownload(c.Request().Context(), c.Param("projectID"), c.Param("id"))
	if err != nil {
		return apiError(c, err)
	}
	defer reader.Close()
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEOctetStream)
	c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(item.SizeBytes, 10))
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+item.ID+"."+item.Engine+"\"")
	return c.Stream(http.StatusOK, echo.MIMEOctetStream, reader)
}

func (h *Handler) guard(c echo.Context, write bool) error {
	if catalog.IsSystemProject(c.Param("projectID")) {
		return catalog.ErrSystemProtected
	}
	if write && !h.writable {
		return godatabase.ErrWriterUnavailable
	}
	return nil
}

func (h *Handler) record(c echo.Context, kind, id, hash string, actionErr error) {
	if h.audit == nil {
		return
	}
	status := "success"
	if actionErr != nil {
		status = "error"
	}
	httpkit.RecordAudit(c, h.audit, kind, c.Param("projectID"), status, id+":"+hash)
}

// apiError 把备份领域错误映射为协议载荷；未知错误交给 WriteError 兜底（不新建第二张错误表）。
func apiError(c echo.Context, err error) error {
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
		return httpkit.WriteError(c, httpkit.NewAPIError(status, code, code, httpkit.RequestID(c)))
	}
	return httpkit.WriteError(c, err)
}

func tenantID(c echo.Context) string {
	return httpkit.ProjectOf(c).TenantID
}
