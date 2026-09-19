// gofunction_handler.go 实现云函数（Go Function）的管理与调用 handler。
//
// 管理面（挂在 /v1/projects/:projectID/gofunctions 下）：
//
//	GET    /gofunctions          列出（列表不带 source）
//	POST   /gofunctions          创建（保存前 ValidateHTTPFuncs 校验）
//	GET    /gofunctions/:name    详情（含 source）
//	PUT    /gofunctions/:name    更新源码
//	DELETE /gofunctions/:name    软删
//
// 调用面（挂在 /go/:projectID 下）：
//
//	POST /go/:projectID/:name/:functionName   执行（响应体为返回值 JSON，无 envelope）
//	GET  /go/:projectID/:name/:functionName   405 JSON（防 SPA fallback 吞掉）
//
// metrics（ui-gofunction-plan §10.1）：
//
//	gofunction_invokes / gofunction_invoke_duration_ms / gofunction_invoke_errors
//	gofunction_compile_ms
package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// gofunctionNameRe 云函数名规则（与集合名一致）：^[A-Za-z][A-Za-z0-9_]{0,62}$
var gofunctionNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

// gofunctionExportNameRe 导出函数名：大写开头的 Go 标识符
var gofunctionExportNameRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*$`)

const (
	gofunctionMaxSourceBytes = 256 << 10 // 256 KiB
	gofunctionMaxPerProject  = 100
)

// GoFunctionHandler 依赖系统库；writable=false 时写操作返回 503。
// audit 可为 nil（不写审计）；写操作成功时记 kind=gofunction（detail 不含源码，§10）。
type GoFunctionHandler struct {
	store    *systemdb.Store
	writable bool
	audit    AuditService
}

// NewGoFunctionHandler 构造 handler。
func NewGoFunctionHandler(store *systemdb.Store, writable bool, audit AuditService) *GoFunctionHandler {
	return &GoFunctionHandler{store: store, writable: writable, audit: audit}
}

// goFunctionDTO 是对外 JSON（snake_case，ui-gofunction-plan §7.1）。
type goFunctionDTO struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	File      string   `json:"file"`
	Source    string   `json:"source,omitempty"`
	Exports   []string `json:"exports"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

func toGoFunctionDTO(g systemdb.GoFunction, withSource bool) goFunctionDTO {
	exports := g.Exports
	if exports == nil {
		exports = []string{}
	}
	dto := goFunctionDTO{
		ID:        g.ID,
		Name:      g.Name,
		File:      g.Name + ".go",
		Exports:   exports,
		CreatedAt: g.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: g.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if withSource {
		dto.Source = g.Source
	}
	return dto
}

// List 列出当前项目云函数（列表不带 source）。
func (h *GoFunctionHandler) List(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	rows, err := h.store.ListGoFunctions(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	// admin 系统项目不提供云函数：返回空列表（与系统库只读策略一致）
	if systemdb.IsAdminProject(pc.ID) {
		rows = nil
	}
	dtos := make([]goFunctionDTO, 0, len(rows))
	for _, g := range rows {
		dtos = append(dtos, toGoFunctionDTO(g, false))
	}
	return c.JSON(http.StatusOK, map[string]any{"functions": dtos})
}

// goFunctionCreateRequest 创建请求体。
type goFunctionCreateRequest struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// Create 创建云函数。保存前 ValidateHTTPFuncs 编译校验。
func (h *GoFunctionHandler) Create(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if !h.writable {
		return WriteError(c, errWriterUnavailable())
	}
	if systemdb.IsAdminProject(pc.ID) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support go functions", RequestIDFromContext(c.Request().Context())))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	var req goFunctionCreateRequest
	if err := bindGoFunctionBody(c, &req); err != nil {
		return WriteError(c, err)
	}
	rid := RequestIDFromContext(c.Request().Context())
	if !gofunctionNameRe.MatchString(req.Name) {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_gofunction_name", "name must match ^[A-Za-z][A-Za-z0-9_]{0,62}$", rid))
	}
	if err := validateGoFunctionSource(req.Source); err != nil {
		return WriteError(c, err)
	}
	infos, err := h.validateSource(c, req.Source)
	if err != nil {
		return WriteError(c, mapGoFunctionValidationError(err, rid))
	}
	// 查重 + 配额
	if existing, err := h.store.GetGoFunction(c.Request().Context(), pc.ID, req.Name); err == nil && existing.ID != "" {
		return WriteError(c, NewAPIError(http.StatusConflict, "gofunction_already_exists", "go function already exists", rid))
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, err)
	}
	if n, err := h.store.CountGoFunctions(c.Request().Context(), pc.ID); err != nil {
		return WriteError(c, err)
	} else if n >= gofunctionMaxPerProject {
		return WriteError(c, NewAPIError(422, "gofunction_limit_exceeded", "go function limit exceeded (100 per project)", rid))
	}
	exports := make([]string, 0, len(infos))
	for _, fi := range infos {
		exports = append(exports, fi.Name)
	}
	created, err := h.store.CreateGoFunction(c.Request().Context(), systemdb.GoFunction{
		ProjectID: pc.ID, Name: req.Name, Source: req.Source, Exports: exports,
	})
	if err != nil {
		return WriteError(c, err)
	}
	h.recordAudit(c, pc.ID, "create "+req.Name+".go")
	return c.JSON(http.StatusCreated, toGoFunctionDTO(created, true))
}

// Get 详情（含 source）。
func (h *GoFunctionHandler) Get(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	name := c.Param("name")
	if !gofunctionNameRe.MatchString(name) {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_gofunction_name", "invalid go function name", RequestIDFromContext(c.Request().Context())))
	}
	g, err := h.store.GetGoFunction(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", RequestIDFromContext(c.Request().Context())))
	}
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toGoFunctionDTO(g, true))
}

// goFunctionUpdateRequest 更新请求体（name 不可改）。
type goFunctionUpdateRequest struct {
	Source string `json:"source"`
}

// Update 更新源码，重算 exports。
func (h *GoFunctionHandler) Update(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if !h.writable {
		return WriteError(c, errWriterUnavailable())
	}
	if systemdb.IsAdminProject(pc.ID) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support go functions", RequestIDFromContext(c.Request().Context())))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	name := c.Param("name")
	rid := RequestIDFromContext(c.Request().Context())
	if !gofunctionNameRe.MatchString(name) {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_gofunction_name", "invalid go function name", rid))
	}
	var req goFunctionUpdateRequest
	if err := bindGoFunctionBody(c, &req); err != nil {
		return WriteError(c, err)
	}
	if err := validateGoFunctionSource(req.Source); err != nil {
		return WriteError(c, err)
	}
	infos, err := h.validateSource(c, req.Source)
	if err != nil {
		return WriteError(c, mapGoFunctionValidationError(err, rid))
	}
	// 必须已存在
	if _, err := h.store.GetGoFunction(c.Request().Context(), pc.ID, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WriteError(c, NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", rid))
		}
		return WriteError(c, err)
	}
	exports := make([]string, 0, len(infos))
	for _, fi := range infos {
		exports = append(exports, fi.Name)
	}
	updated, err := h.store.UpdateGoFunction(c.Request().Context(), systemdb.GoFunction{
		ProjectID: pc.ID, Name: name, Source: req.Source, Exports: exports,
	})
	if err != nil {
		return WriteError(c, err)
	}
	h.recordAudit(c, pc.ID, "update "+name+".go")
	return c.JSON(http.StatusOK, toGoFunctionDTO(updated, true))
}

// Delete 软删云函数。
func (h *GoFunctionHandler) Delete(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if !h.writable {
		return WriteError(c, errWriterUnavailable())
	}
	if systemdb.IsAdminProject(pc.ID) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support go functions", RequestIDFromContext(c.Request().Context())))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	name := c.Param("name")
	if !gofunctionNameRe.MatchString(name) {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_gofunction_name", "invalid go function name", RequestIDFromContext(c.Request().Context())))
	}
	err := h.store.ArchiveGoFunction(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", RequestIDFromContext(c.Request().Context())))
	}
	if err != nil {
		return WriteError(c, err)
	}
	h.recordAudit(c, pc.ID, "delete "+name+".go")
	return c.NoContent(http.StatusNoContent)
}

// Invoke 执行云函数：POST /go/:projectID/:name/:functionName。
// 响应体就是返回值的 JSON（无 envelope）。
// metrics（§10.1）：每次 invoke（含 4xx/5xx 失败）都计数，用 defer 保证覆盖全部返回路径。
func (h *GoFunctionHandler) Invoke(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	start := time.Now()
	var invokeErr error
	defer func() { h.recordInvokeMetrics(pc.ID, time.Since(start), invokeErr) }()
	fail := func(err error) error {
		invokeErr = err
		return WriteError(c, err)
	}

	name := c.Param("name")
	fnName := c.Param("functionName")
	rid := RequestIDFromContext(c.Request().Context())
	if !gofunctionNameRe.MatchString(name) {
		return fail(NewAPIError(http.StatusBadRequest, "invalid_gofunction_name", "invalid go function name", rid))
	}
	if !gofunctionExportNameRe.MatchString(fnName) {
		return fail(NewAPIError(http.StatusNotFound, "function_not_found", "function not found", rid))
	}
	g, err := h.store.GetGoFunction(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return fail(NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", rid))
	}
	if err != nil {
		return fail(err)
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return fail(NewAPIError(http.StatusBadRequest, "invalid_request", "cannot read request body", rid))
	}
	out, runErr := gofunction.RunJSON(c.Request().Context(), rid, name, g.Source, fnName, body)
	if runErr != nil {
		return fail(mapGoFunctionRunError(runErr, rid))
	}
	return c.Blob(http.StatusOK, "application/json", out)
}

// MethodNotAllowed 返回 405 JSON，防止 SPA fallback 把 GET /go 吞成 index.html。
func (h *GoFunctionHandler) MethodNotAllowed(c echo.Context) error {
	return c.JSON(http.StatusMethodNotAllowed, APIErrorBody{Error: APIErrorDetail{
		Code: "method_not_allowed", Message: "use POST with JSON body",
		RequestID: RequestIDFromContext(c.Request().Context()),
	}})
}

// validateSource 调用 ValidateHTTPFuncs 并上报 gofunction_compile_ms。
func (h *GoFunctionHandler) validateSource(c echo.Context, source string) ([]gofunction.FuncInfo, error) {
	start := time.Now()
	infos, err := gofunction.ValidateHTTPFuncs(source)
	if h.store != nil {
		pc, _ := ProjectFromContext(c.Request().Context())
		if pc.ID != "" && !systemdb.IsAdminProject(pc.ID) {
			h.store.RecordMetric(systemdb.MetricSample{
				ProjectID: pc.ID, Name: "gofunction_compile_ms", Value: float64(time.Since(start).Milliseconds()),
			})
		}
	}
	return infos, err
}

// recordInvokeMetrics 上报 §10.1 四指标中的三个 invoke 指标（失败静默）。
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

// recordAudit 写操作审计（§10）：kind=gofunction，detail 只含动作与文件名，不含源码。
func (h *GoFunctionHandler) recordAudit(c echo.Context, projectID, detail string) {
	if h.audit == nil {
		return
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	pID := ""
	if ok {
		pID = string(principal.APIKeyID)
	}
	_ = h.audit.Record(context.Background(), AuditEvent{
		ProjectID:   projectID,
		PrincipalID: pID,
		Kind:        "gofunction",
		RequestID:   RequestIDFromContext(c.Request().Context()),
		Status:      "ok",
		Detail:      detail,
	})
}

// bindGoFunctionBody 解析 JSON body（不使用 DisallowUnknownFields，宽容前端扩展字段）。
func bindGoFunctionBody(c echo.Context, req any) error {
	if err := c.Bind(req); err != nil {
		return NewAPIError(http.StatusBadRequest, "invalid_request", "malformed JSON body", RequestIDFromContext(c.Request().Context()))
	}
	return nil
}

// validateGoFunctionSource 校验源码非空与体积上限。
func validateGoFunctionSource(source string) error {
	if strings.TrimSpace(source) == "" {
		return NewAPIError(400, "gofunction_signature_invalid", "source is empty", "")
	}
	if len(source) > gofunctionMaxSourceBytes {
		return NewAPIError(http.StatusBadRequest, "gofunction_source_too_large", "source exceeds 256KiB", "")
	}
	return nil
}

// mapGoFunctionValidationError 把保存时 ValidateHTTPFuncs 的失败映射为
// 400 compile/signature 错误；message 原样透传（含每个不合规函数名与原因）。
// 判定顺序：能 Parse 但签名/类型不合 → signature_invalid；Parse/编译失败 → compile_error。
func mapGoFunctionValidationError(err error, rid string) error {
	if err == nil {
		return nil
	}
	msg := truncateMessage(err.Error(), 1024)
	if strings.Contains(err.Error(), "不符合约定") || strings.Contains(err.Error(), "至少导出一个") {
		return NewAPIError(http.StatusBadRequest, "gofunction_signature_invalid", msg, rid)
	}
	return NewAPIError(http.StatusBadRequest, "gofunction_compile_error", msg, rid)
}

// mapGoFunctionRunError 把 gofunction 包错误映射为统一错误协议（§7.2/§13）。
func mapGoFunctionRunError(err error, rid string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gofunction.ErrFunctionNotFound) {
		return NewAPIError(http.StatusNotFound, "function_not_found", "function not found", rid)
	}
	if errors.Is(err, gofunction.ErrTimeout) {
		return NewAPIError(http.StatusGatewayTimeout, "request_timeout", "go function execution timed out", rid)
	}
	if errors.Is(err, gofunction.ErrBind) {
		return NewAPIError(http.StatusBadRequest, "invalid_request", truncateMessage(err.Error(), 512), rid)
	}
	// invoke 时 BuildProgram 失败（保存后环境变化）→ 500 gofunction_compile_error（§7.2/§13）
	if errors.Is(err, gofunction.ErrCompile) {
		return NewAPIError(http.StatusInternalServerError, "gofunction_compile_error", truncateMessage(err.Error(), 512), rid)
	}
	return NewAPIError(http.StatusInternalServerError, "gofunction_runtime_error", truncateMessage(err.Error(), 512), rid)
}

// truncateMessage 错误 message 截断脱敏。
func truncateMessage(msg string, max int) string {
	if len(msg) <= max {
		return msg
	}
	return msg[:max] + "…"
}

// errWriterUnavailable 复用统一只读实例错误。
func errWriterUnavailable() error {
	return database.ErrWriterUnavailable
}
