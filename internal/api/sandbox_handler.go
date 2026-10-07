package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// SandboxService 由 app 装配的 Manager 实现；handler 不直接依赖 SDK。
type SandboxService interface {
	Available() bool
	Capabilities() sandbox.Capabilities
	Create(context.Context, string, sandbox.CreateInput, string) (sandbox.Sandbox, error)
	List(context.Context, string, string, string, string, int) ([]sandbox.Sandbox, string, error)
	Get(context.Context, string, string, bool) (sandbox.Sandbox, error)
	Update(context.Context, string, string, sandbox.UpdateInput) (sandbox.Sandbox, error)
	Start(context.Context, string, string) (sandbox.Sandbox, error)
	Stop(context.Context, string, string) (sandbox.Sandbox, error)
	Delete(context.Context, string, string) error
	Exec(context.Context, string, string, sandbox.ExecInput) (sandbox.ExecOutput, error)
	ReadFile(context.Context, string, string, string) (sandbox.FileContent, error)
	WriteFile(context.Context, string, string, string, []byte) error
	RemoveFile(context.Context, string, string, string) error
	ListDir(context.Context, string, string, string) ([]sandbox.FileEntry, error)
	RunOnce(context.Context, string, sandbox.RunInput, string) (sandbox.ExecOutput, error)
}

type SandboxHandler struct {
	svc      SandboxService
	writable bool
	audit    AuditService
	store    *systemdb.Store
	usage    interface {
		RecordSandbox(context.Context, string, string, int64) error
	}
}

func NewSandboxHandler(svc SandboxService, writable bool, audit AuditService, store *systemdb.Store, usage interface {
	RecordSandbox(context.Context, string, string, int64) error
}) *SandboxHandler {
	return &SandboxHandler{svc: svc, writable: writable, audit: audit, store: store, usage: usage}
}
func (h *SandboxHandler) project(c echo.Context) (string, error) {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return "", echo.NewHTTPError(http.StatusBadRequest, "project context missing")
	}
	return pc.ID, nil
}
func (h *SandboxHandler) guardWrite(c echo.Context) error {
	if !h.writable {
		return errWriterUnavailable()
	}
	p, err := h.project(c)
	if err != nil {
		return err
	}
	if systemdb.IsAdminProject(p) {
		return NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support sandboxes", RequestIDFromContext(c.Request().Context()))
	}
	return nil
}
func (h *SandboxHandler) Capabilities(c echo.Context) error {
	if h.svc == nil || !h.svc.Available() {
		return c.JSON(http.StatusOK, map[string]any{"available": false, "backend": "cloud", "images": []string{}})
	}
	return c.JSON(http.StatusOK, h.svc.Capabilities())
}
func bindSandboxJSON(c echo.Context, dst any) error {
	decoder := json.NewDecoder(c.Request().Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return NewAPIError(http.StatusBadRequest, "sandbox_invalid_spec", "invalid JSON body", RequestIDFromContext(c.Request().Context()))
	}
	var rest any
	if err := decoder.Decode(&rest); !errors.Is(err, io.EOF) {
		return NewAPIError(http.StatusBadRequest, "sandbox_invalid_spec", "extra JSON input", RequestIDFromContext(c.Request().Context()))
	}
	return nil
}
func (h *SandboxHandler) record(c echo.Context, p, id, action, detail string) {
	if h.audit == nil {
		return
	}
	_ = h.audit.Record(c.Request().Context(), AuditEvent{ProjectID: p, PrincipalID: actorID(c), Kind: "sandbox." + action,
		RequestID: RequestIDFromContext(c.Request().Context()), Status: "ok", Detail: id + " " + detail})
}
func (h *SandboxHandler) List(c echo.Context) error {
	p, err := h.project(c)
	if err != nil {
		return WriteError(c, err)
	}
	limit := queryLimit(c, 0, 0)
	if c.QueryParam("limit") != "" && limit == 0 {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "sandbox_invalid_spec", "invalid limit parameter", RequestIDFromContext(c.Request().Context())))
	}
	rows, next, err := h.svc.List(c.Request().Context(), p, c.QueryParam("status"), c.QueryParam("source"), c.QueryParam("cursor"), limit)
	if err != nil {
		return WriteError(c, err)
	}
	out := map[string]any{"sandboxes": rows}
	if next != "" {
		out["next_cursor"] = next
	}
	return c.JSON(http.StatusOK, out)
}
func (h *SandboxHandler) Create(c echo.Context) error {
	if err := h.guardWrite(c); err != nil {
		return WriteError(c, err)
	}
	p, _ := h.project(c)
	var in sandbox.CreateInput
	if err := bindSandboxJSON(c, &in); err != nil {
		return WriteError(c, err)
	}
	in.IdempotencyKey = c.Request().Header.Get("Idempotency-Key")
	if principal, ok := PrincipalFromContext(c.Request().Context()); ok && principal.UserID != "" {
		in.Source = "console"
	}
	row, err := h.svc.Create(c.Request().Context(), p, in, actorID(c))
	if err != nil {
		return WriteError(c, err)
	}
	if row.Reused {
		return c.JSON(http.StatusOK, row)
	}
	h.record(c, p, row.ID, "create", "")
	return c.JSON(http.StatusCreated, row)
}
func (h *SandboxHandler) Get(c echo.Context) error {
	p, err := h.project(c)
	if err != nil {
		return WriteError(c, err)
	}
	row, err := h.svc.Get(c.Request().Context(), p, c.Param("sandboxID"), c.QueryParam("refresh") == "1")
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, row)
}
func (h *SandboxHandler) Update(c echo.Context) error {
	if err := h.guardWrite(c); err != nil {
		return WriteError(c, err)
	}
	p, _ := h.project(c)
	var in sandbox.UpdateInput
	if err := bindSandboxJSON(c, &in); err != nil {
		return WriteError(c, err)
	}
	row, err := h.svc.Update(c.Request().Context(), p, c.Param("sandboxID"), in)
	if err != nil {
		return WriteError(c, err)
	}
	h.record(c, p, row.ID, "update", "")
	return c.JSON(http.StatusOK, row)
}
func (h *SandboxHandler) Delete(c echo.Context) error {
	if err := h.guardWrite(c); err != nil {
		return WriteError(c, err)
	}
	p, _ := h.project(c)
	id := c.Param("sandboxID")
	if err := h.svc.Delete(c.Request().Context(), p, id); err != nil {
		if errors.Is(err, sandbox.ErrBackend) {
			// 用户删除请求仍成功；未删除 VM 留在资源表供 reaper 重试。
			h.record(c, p, id, "delete", "backend_removal_pending")
			return c.NoContent(http.StatusNoContent)
		}
		return WriteError(c, err)
	}
	h.record(c, p, id, "delete", "")
	return c.NoContent(http.StatusNoContent)
}
func (h *SandboxHandler) Start(c echo.Context) error {
	if err := h.guardWrite(c); err != nil {
		return WriteError(c, err)
	}
	p, _ := h.project(c)
	row, err := h.svc.Start(c.Request().Context(), p, c.Param("sandboxID"))
	if err != nil {
		return WriteError(c, err)
	}
	h.record(c, p, row.ID, "start", "")
	return c.JSON(http.StatusOK, row)
}
func (h *SandboxHandler) Stop(c echo.Context) error {
	if err := h.guardWrite(c); err != nil {
		return WriteError(c, err)
	}
	p, _ := h.project(c)
	row, err := h.svc.Stop(c.Request().Context(), p, c.Param("sandboxID"))
	if err != nil {
		return WriteError(c, err)
	}
	h.record(c, p, row.ID, "stop", "")
	return c.JSON(http.StatusOK, row)
}
func (h *SandboxHandler) recordExec(c echo.Context, p, id, action, command string, out sandbox.ExecOutput) {
	digest := sha256.Sum256([]byte(command))
	detail := fmt.Sprintf("sha256=%s exit=%d duration_ms=%d timed_out=%t truncated=%t", hex.EncodeToString(digest[:]), out.ExitCode, out.DurationMs, out.TimedOut, out.StdoutTruncated || out.StderrTruncated)
	h.record(c, p, id, action, detail)
	if h.usage != nil && !systemdb.IsAdminProject(p) {
		_ = h.usage.RecordSandbox(c.Request().Context(), p, RequestIDFromContext(c.Request().Context()), out.DurationMs)
	}
	if h.store != nil && !systemdb.IsAdminProject(p) {
		h.store.RecordMetric(systemdb.MetricSample{ProjectID: p, Name: "sandbox_exec_count", Value: 1})
		h.store.RecordMetric(systemdb.MetricSample{ProjectID: p, Name: "sandbox_exec_duration_ms", Value: float64(out.DurationMs)})
	}
}
func (h *SandboxHandler) Exec(c echo.Context) error {
	if err := h.guardWrite(c); err != nil {
		return WriteError(c, err)
	}
	p, _ := h.project(c)
	var in sandbox.ExecInput
	if err := bindSandboxJSON(c, &in); err != nil {
		return WriteError(c, err)
	}
	out, err := h.svc.Exec(c.Request().Context(), p, c.Param("sandboxID"), in)
	if err != nil {
		return WriteError(c, err)
	}
	command := in.Command
	if command == "" {
		command = in.Cmd
	}
	h.recordExec(c, p, c.Param("sandboxID"), "exec", command, out)
	return c.JSON(http.StatusOK, out)
}
func (h *SandboxHandler) ListDir(c echo.Context) error {
	p, err := h.project(c)
	if err != nil {
		return WriteError(c, err)
	}
	entries, err := h.svc.ListDir(c.Request().Context(), p, c.Param("sandboxID"), c.QueryParam("path"))
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"entries": entries})
}
func (h *SandboxHandler) ReadFile(c echo.Context) error {
	p, err := h.project(c)
	if err != nil {
		return WriteError(c, err)
	}
	f, err := h.svc.ReadFile(c.Request().Context(), p, c.Param("sandboxID"), c.QueryParam("path"))
	if err != nil {
		return WriteError(c, err)
	}
	if strings.Contains(c.Request().Header.Get("Accept"), "application/octet-stream") {
		return c.Blob(http.StatusOK, "application/octet-stream", f.Content)
	}
	if utf8.Valid(f.Content) {
		return c.JSON(http.StatusOK, map[string]any{"content": string(f.Content), "encoding": "utf8", "truncated": f.Truncated})
	}
	return c.JSON(http.StatusOK, map[string]any{"content": base64.StdEncoding.EncodeToString(f.Content), "encoding": "base64", "truncated": f.Truncated})
}
func (h *SandboxHandler) WriteFile(c echo.Context) error {
	if err := h.guardWrite(c); err != nil {
		return WriteError(c, err)
	}
	p, _ := h.project(c)
	const maxBody = 1<<20 + 64<<10
	data, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBody+1))
	if err != nil {
		return WriteError(c, err)
	}
	if len(data) > maxBody {
		return WriteError(c, sandbox.ErrFileTooLarge)
	}
	if strings.Contains(c.Request().Header.Get("Content-Type"), "application/json") {
		var in struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		if err = json.Unmarshal(data, &in); err != nil {
			return WriteError(c, sandbox.ErrInvalidSpec)
		}
		data = []byte(in.Content)
		if in.Encoding == "base64" {
			data, err = base64.StdEncoding.DecodeString(in.Content)
			if err != nil {
				return WriteError(c, sandbox.ErrInvalidSpec)
			}
		}
	}
	id, path := c.Param("sandboxID"), c.QueryParam("path")
	if err = h.svc.WriteFile(c.Request().Context(), p, id, path, data); err != nil {
		return WriteError(c, err)
	}
	h.record(c, p, id, "file.write", fmt.Sprintf("bytes=%d", len(data)))
	return c.NoContent(http.StatusNoContent)
}
func (h *SandboxHandler) RemoveFile(c echo.Context) error {
	if err := h.guardWrite(c); err != nil {
		return WriteError(c, err)
	}
	p, _ := h.project(c)
	id, path := c.Param("sandboxID"), c.QueryParam("path")
	if err := h.svc.RemoveFile(c.Request().Context(), p, id, path); err != nil {
		return WriteError(c, err)
	}
	h.record(c, p, id, "file.delete", "")
	return c.NoContent(http.StatusNoContent)
}
func (h *SandboxHandler) RunOnce(c echo.Context) error {
	if err := h.guardWrite(c); err != nil {
		return WriteError(c, err)
	}
	p, _ := h.project(c)
	var in sandbox.RunInput
	if err := bindSandboxJSON(c, &in); err != nil {
		return WriteError(c, err)
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 6*time.Minute)
	defer cancel()
	out, err := h.svc.RunOnce(ctx, p, in, actorID(c))
	if err != nil {
		return WriteError(c, err)
	}
	command := in.Command
	if command == "" {
		command = in.Cmd
	}
	h.recordExec(c, p, out.SandboxID, "run", command, out)
	return c.JSON(http.StatusOK, out)
}
