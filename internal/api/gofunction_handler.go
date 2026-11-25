// 文件说明：gofunction_handler.go — internal/api 中 GoFunctionHandler、NewGoFunctionHandler 等符号的实现
//
// 用法
//
//	type GoFunctionHandler struct{…}  // GoFunctionHandler 依赖系统库；writable=false 时写/测试返回 503
//	NewGoFunctionHandler(store *systemdb.Store, writable bool, audit AuditService, maxConcurrentRuns int) *GoFunct...  // NewGoFunctionHandler 构造 handler
//	同包调用：parseVersionParam() → gofunction_invoke.go；goFuncTestResponse → gofunction_invoke.go；bindGoFunctionBody() → gofunction_invoke.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/gofunction_handler.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] GoFunctionHandler ← gofunction_errors.go, gofunction_invoke.go；NewGoFunctionHandler ← gofunction_gate_test.go, gofunction_handler_test.go；parseVersionParam ← gofunction_invoke.go
//
// 主要符号
//
//	type GoFunctionHandler struct{…}
//	var gofunctionNameRe / gofunctionExportNameRe
//	const gofunctionMaxSourceBytes / gofunctionMaxPerProject
//	NewGoFunctionHandler(store *systemdb.Store, writable bool, audit AuditService, maxConcurrentRuns int) *GoFunct...
//	(h *GoFunctionHandler) WithRunner(runner *functionrun.Runner) *GoFunctionHandler
//	type goFuncVersionSummary struct{…}
//	type goFuncDTO struct{…}
//	toVersionSummary(v systemdb.GoFuncVersion, activeVersion int64) goFuncVersionSummary
//	... 另有 24 个顶层符号

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/functionrun"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// GoFunctionHandler 依赖系统库；writable=false 时写/测试返回 503。
type GoFunctionHandler struct {
	store    *systemdb.Store
	writable bool
	audit    AuditService
	// runSem 是 /go 调用与调试台试跑的全局并发信号量（GF-RUN-04）：
	// 每次调用都重新编译执行源码，恶意或高频调用会耗尽 CPU/内存。
	// nil 表示不限流（旧测试路径）。
	runSem chan struct{}
	runner *functionrun.Runner
}

var (
	gofunctionNameRe       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)
	gofunctionExportNameRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*$`)
)

const (
	gofunctionMaxSourceBytes = 256 << 10
	gofunctionMaxPerProject  = 100
)

// NewGoFunctionHandler 构造 handler。maxConcurrentRuns<=0 时不限流。
func NewGoFunctionHandler(store *systemdb.Store, writable bool, audit AuditService, maxConcurrentRuns int) *GoFunctionHandler {
	h := &GoFunctionHandler{store: store, writable: writable, audit: audit}
	if maxConcurrentRuns > 0 {
		h.runSem = make(chan struct{}, maxConcurrentRuns)
	}
	return h
}

func (h *GoFunctionHandler) WithRunner(runner *functionrun.Runner) *GoFunctionHandler {
	h.runner = runner
	return h
}

// ---------- DTO ----------

type goFuncVersionSummary struct {
	Version   int64    `json:"version"`
	Exports   []string `json:"exports"`
	Note      string   `json:"note"`
	CreatedAt string   `json:"created_at"`
	Active    bool     `json:"active"`
	Source    string   `json:"source,omitempty"`
}

type goFuncDTO struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	File          string                 `json:"file"`
	Description   string                 `json:"description"`
	ActiveVersion int64                  `json:"active_version"`
	LatestVersion int64                  `json:"latest_version"`
	Published     bool                   `json:"published"`
	Exports       []string               `json:"exports"`
	Versions      []goFuncVersionSummary `json:"versions,omitempty"`
	Source        string                 `json:"source,omitempty"`
	CreatedAt     string                 `json:"created_at"`
	UpdatedAt     string                 `json:"updated_at"`
}

func toVersionSummary(v systemdb.GoFuncVersion, activeVersion int64) goFuncVersionSummary {
	exports := v.Exports
	if exports == nil {
		exports = []string{}
	}
	return goFuncVersionSummary{
		Version:   v.Version,
		Exports:   exports,
		Note:      v.Note,
		CreatedAt: v.CreatedAt.UTC().Format(time.RFC3339),
		Active:    v.Version == activeVersion,
		Source:    v.Source,
	}
}

func (h *GoFunctionHandler) toDTO(f systemdb.GoFunc, vers []systemdb.GoFuncVersion, withVersions bool) (goFuncDTO, error) {
	latest := int64(0)
	if len(vers) > 0 {
		latest = vers[0].Version // 降序
	}
	// D12：exports 取 active，否则 latest
	pick := f.ActiveVersion
	if pick <= 0 {
		pick = latest
	}
	var exports []string
	source := ""
	for _, v := range vers {
		if v.Version == pick {
			exports = v.Exports
			break
		}
	}
	if exports == nil {
		exports = []string{}
	}
	// 需要 source 时单独取（列表 vers.Source 已被清空）
	dto := goFuncDTO{
		ID:            f.ID,
		Name:          f.Name,
		File:          f.Name + ".go",
		Description:   f.Description,
		ActiveVersion: f.ActiveVersion,
		LatestVersion: latest,
		Published:     f.ActiveVersion > 0,
		Exports:       exports,
		CreatedAt:     f.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     f.UpdatedAt.UTC().Format(time.RFC3339),
		Source:        source,
	}
	if withVersions {
		dto.Versions = make([]goFuncVersionSummary, 0, len(vers))
		for _, v := range vers {
			dto.Versions = append(dto.Versions, toVersionSummary(v, f.ActiveVersion))
		}
	}
	return dto, nil
}

// ---------- 管理面 ----------

func (h *GoFunctionHandler) requireReadStore(c echo.Context) (ProjectContext, error) {
	pc, err := requireProject(c)
	if err != nil {
		return ProjectContext{}, err
	}
	if h.store == nil {
		return ProjectContext{}, systemdb.ErrUnavailable
	}
	return pc, nil
}

func (h *GoFunctionHandler) requireWriteStore(c echo.Context, forbidAdmin bool) (ProjectContext, string, error) {
	pc, err := requireProject(c)
	if err != nil {
		return ProjectContext{}, "", err
	}
	rid := requestID(c)
	if !h.writable {
		return ProjectContext{}, "", godatabase.ErrWriterUnavailable
	}
	if forbidAdmin && systemdb.IsAdminProject(pc.ID) {
		return ProjectContext{}, "", NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support go functions", rid)
	}
	if h.store == nil {
		return ProjectContext{}, "", systemdb.ErrUnavailable
	}
	return pc, rid, nil
}

func (h *GoFunctionHandler) compileExports(c echo.Context, source, rid string) ([]string, error) {
	if err := validateGoFunctionSource(source); err != nil {
		return nil, err
	}
	infos, err := h.validateSource(c, source)
	if err != nil {
		return nil, mapGoFunctionValidationError(err, rid)
	}
	return funcNames(infos), nil
}

func shouldActivate(flag *bool) bool {
	return flag == nil || *flag
}

func parseVersionParam(c echo.Context, rid string) (int64, error) {
	ver, err := strconv.ParseInt(c.Param("ver"), 10, 64)
	if err != nil || ver <= 0 {
		return 0, badRequest(c, "invalid version")
	}
	return ver, nil
}

// List: GET /gofunctions
func (h *GoFunctionHandler) List(c echo.Context) error {
	pc, err := h.requireReadStore(c)
	if err != nil {
		return WriteError(c, err)
	}
	if systemdb.IsAdminProject(pc.ID) {
		return c.JSON(http.StatusOK, map[string]any{"functions": []goFuncDTO{}})
	}
	rows, err := h.store.ListGoFuncs(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	dtos := make([]goFuncDTO, 0, len(rows))
	for _, f := range rows {
		vers, err := h.store.ListGoFuncVersions(c.Request().Context(), pc.ID, f.Name)
		if err != nil {
			return WriteError(c, err)
		}
		d, err := h.toDTO(f, vers, false)
		if err != nil {
			return WriteError(c, err)
		}
		dtos = append(dtos, d)
	}
	return c.JSON(http.StatusOK, map[string]any{"functions": dtos})
}

type goFuncCreateRequest struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
	Note        string `json:"note"`
	Activate    *bool  `json:"activate"`
}

// Create: POST /gofunctions —— 实体 + v1。
func (h *GoFunctionHandler) Create(c echo.Context) error {
	pc, rid, err := h.requireWriteStore(c, true)
	if err != nil {
		return WriteError(c, err)
	}
	var req goFuncCreateRequest
	if err := bindGoFunctionBody(c, &req); err != nil {
		return WriteError(c, err)
	}
	if !gofunctionNameRe.MatchString(req.Name) {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_gofunction_name", "name must match ^[A-Za-z][A-Za-z0-9_]{0,62}$", rid))
	}
	exports, err := h.compileExports(c, req.Source, rid)
	if err != nil {
		return WriteError(c, err)
	}
	if existing, err := h.store.GetGoFunc(c.Request().Context(), pc.ID, req.Name); err == nil && existing.ID != "" {
		return WriteError(c, NewAPIError(http.StatusConflict, "gofunction_already_exists", "go function already exists", rid))
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, err)
	}
	if n, err := h.store.CountGoFuncs(c.Request().Context(), pc.ID); err != nil {
		return WriteError(c, err)
	} else if n >= gofunctionMaxPerProject {
		return WriteError(c, NewAPIError(422, "gofunction_limit_exceeded", "go function limit exceeded (100 per project)", rid))
	}

	f, err := h.store.CreateGoFunc(c.Request().Context(), systemdb.GoFunc{
		ProjectID: pc.ID, Name: req.Name, Description: req.Description,
		CreatedBy: principalOf(c).Actor(),
	})
	if err != nil {
		return WriteError(c, err)
	}
	v, err := h.store.AppendGoFuncVersion(c.Request().Context(), systemdb.GoFuncVersion{
		FuncID: f.ID, ProjectID: pc.ID, Name: req.Name,
		Source: req.Source, Exports: exports, Note: req.Note, CreatedBy: principalOf(c).Actor(),
	})
	if err != nil {
		return WriteError(c, err)
	}
	if shouldActivate(req.Activate) {
		if err := h.store.ActivateGoFuncVersion(c.Request().Context(), pc.ID, req.Name, v.Version); err != nil {
			return WriteError(c, err)
		}
		f.ActiveVersion = v.Version
	}
	h.recordAudit(c, pc.ID, "create "+req.Name+".go v"+strconv.FormatInt(v.Version, 10))
	dto, err := h.toDTO(f, []systemdb.GoFuncVersion{v}, true)
	if err != nil {
		return WriteError(c, err)
	}
	dto.Source = req.Source
	return c.JSON(http.StatusCreated, dto)
}

// Get: GET /gofunctions/:name
func (h *GoFunctionHandler) Get(c echo.Context) error {
	pc, err := h.requireReadStore(c)
	if err != nil {
		return WriteError(c, err)
	}
	name := c.Param("name")
	f, vers, err := h.loadFunc(c, pc.ID, name)
	if err != nil {
		return WriteError(c, err)
	}
	dto, err := h.toDTO(f, vers, true)
	if err != nil {
		return WriteError(c, err)
	}
	// 附 source：active 优先，否则 latest
	src, err := h.pickSource(c, pc.ID, name, f)
	if err != nil {
		return WriteError(c, err)
	}
	dto.Source = src
	return c.JSON(http.StatusOK, dto)
}

type goFuncPatchRequest struct {
	Description string `json:"description"`
}

// Patch: PATCH /gofunctions/:name —— 仅 description。
func (h *GoFunctionHandler) Patch(c echo.Context) error {
	pc, rid, err := h.requireWriteStore(c, false)
	if err != nil {
		return WriteError(c, err)
	}
	name := c.Param("name")
	f, err := h.store.GetGoFunc(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", rid))
	}
	if err != nil {
		return WriteError(c, err)
	}
	var req goFuncPatchRequest
	if err := bindGoFunctionBody(c, &req); err != nil {
		return WriteError(c, err)
	}
	f.Description = req.Description
	if err := h.store.UpdateGoFuncMeta(c.Request().Context(), f); err != nil {
		return WriteError(c, err)
	}
	vers, err := h.store.ListGoFuncVersions(c.Request().Context(), pc.ID, name)
	if err != nil {
		return WriteError(c, err)
	}
	dto, err := h.toDTO(f, vers, true)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, dto)
}

// Delete: DELETE /gofunctions/:name
func (h *GoFunctionHandler) Delete(c echo.Context) error {
	pc, rid, err := h.requireWriteStore(c, true)
	if err != nil {
		return WriteError(c, err)
	}
	name := c.Param("name")
	if err := h.store.ArchiveGoFunc(c.Request().Context(), pc.ID, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WriteError(c, NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", rid))
		}
		return WriteError(c, err)
	}
	h.recordAudit(c, pc.ID, "delete "+name+".go")
	return c.NoContent(http.StatusNoContent)
}

type goFuncVersionCreateRequest struct {
	Source   string `json:"source"`
	Note     string `json:"note"`
	Activate *bool  `json:"activate"`
}

// ListVersions: GET /gofunctions/:name/versions
func (h *GoFunctionHandler) ListVersions(c echo.Context) error {
	pc, err := h.requireReadStore(c)
	if err != nil {
		return WriteError(c, err)
	}
	name := c.Param("name")
	f, err := h.store.GetGoFunc(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", requestID(c)))
	}
	if err != nil {
		return WriteError(c, err)
	}
	vers, err := h.store.ListGoFuncVersions(c.Request().Context(), pc.ID, name)
	if err != nil {
		return WriteError(c, err)
	}
	sums := make([]goFuncVersionSummary, 0, len(vers))
	for _, v := range vers {
		sums = append(sums, toVersionSummary(v, f.ActiveVersion))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"active_version": f.ActiveVersion,
		"versions":       sums,
	})
}

// CreateVersion: POST /gofunctions/:name/versions
func (h *GoFunctionHandler) CreateVersion(c echo.Context) error {
	pc, rid, err := h.requireWriteStore(c, true)
	if err != nil {
		return WriteError(c, err)
	}
	name := c.Param("name")
	f, err := h.store.GetGoFunc(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", rid))
	}
	if err != nil {
		return WriteError(c, err)
	}
	var req goFuncVersionCreateRequest
	if err := bindGoFunctionBody(c, &req); err != nil {
		return WriteError(c, err)
	}
	exports, err := h.compileExports(c, req.Source, rid)
	if err != nil {
		return WriteError(c, err)
	}
	v, err := h.store.AppendGoFuncVersion(c.Request().Context(), systemdb.GoFuncVersion{
		FuncID: f.ID, ProjectID: pc.ID, Name: name,
		Source: req.Source, Exports: exports, Note: req.Note, CreatedBy: principalOf(c).Actor(),
	})
	if err != nil {
		if errors.Is(err, systemdb.ErrVersionLimit) {
			return WriteError(c, NewAPIError(422, "version_limit_exceeded", "version limit exceeded (50 per function)", rid))
		}
		return WriteError(c, err)
	}
	if shouldActivate(req.Activate) {
		if err := h.store.ActivateGoFuncVersion(c.Request().Context(), pc.ID, name, v.Version); err != nil {
			return WriteError(c, err)
		}
		f.ActiveVersion = v.Version
	}
	vers := []systemdb.GoFuncVersion{v}
	all, _ := h.store.ListGoFuncVersions(c.Request().Context(), pc.ID, name)
	if len(all) > 0 {
		vers = all
	}
	h.recordAudit(c, pc.ID, "save "+name+".go v"+strconv.FormatInt(v.Version, 10))
	dto, err := h.toDTO(f, vers, true)
	if err != nil {
		return WriteError(c, err)
	}
	dto.Source = req.Source
	// PUT 兼容路径期望 200；新 API 返回 201
	if c.Request().Method == http.MethodPut {
		return c.JSON(http.StatusOK, dto)
	}
	return c.JSON(http.StatusCreated, dto)
}

// GetVersion: GET /gofunctions/:name/versions/:ver
func (h *GoFunctionHandler) GetVersion(c echo.Context) error {
	pc, err := h.requireReadStore(c)
	if err != nil {
		return WriteError(c, err)
	}
	name := c.Param("name")
	ver, err := parseVersionParam(c, requestID(c))
	if err != nil {
		return WriteError(c, err)
	}
	f, err := h.store.GetGoFunc(c.Request().Context(), pc.ID, name)
	if err != nil {
		return WriteError(c, mapNotFound(err))
	}
	v, err := h.store.GetGoFuncVersion(c.Request().Context(), pc.ID, name, ver)
	if err != nil {
		return WriteError(c, mapNotFound(err))
	}
	sum := toVersionSummary(v, f.ActiveVersion)
	return c.JSON(http.StatusOK, sum)
}

// ActivateVersion: POST /gofunctions/:name/versions/:ver/activate
func (h *GoFunctionHandler) ActivateVersion(c echo.Context) error {
	pc, rid, err := h.requireWriteStore(c, false)
	if err != nil {
		return WriteError(c, err)
	}
	name := c.Param("name")
	ver, err := parseVersionParam(c, rid)
	if err != nil {
		return WriteError(c, err)
	}
	if err := h.store.ActivateGoFuncVersion(c.Request().Context(), pc.ID, name, ver); err != nil {
		return WriteError(c, mapNotFound(err))
	}
	h.recordAudit(c, pc.ID, "activate "+name+".go v"+strconv.FormatInt(ver, 10))
	return c.JSON(http.StatusOK, map[string]any{"active_version": ver})
}

// ---------- 调试台 ----------

type goFuncTestRequest struct {
	FunctionName string          `json:"function_name"`
	Body         json.RawMessage `json:"body"`
}

type goFuncTestResponse struct {
	OK            bool            `json:"ok"`
	StatusCode    int             `json:"status_code"`
	DurationMs    int64           `json:"duration_ms"`
	Version       int64           `json:"version"`
	ActiveVersion int64           `json:"active_version"`
	FunctionName  string          `json:"function_name"`
	Data          json.RawMessage `json:"data,omitempty"`
	Error         string          `json:"error,omitempty"`
}

// ---------- /go 调用面（只跑 active） ----------

// MethodNotAllowed 返回 405 JSON。
func (h *GoFunctionHandler) MethodNotAllowed(c echo.Context) error {
	return c.JSON(http.StatusMethodNotAllowed, APIErrorBody{Error: APIErrorDetail{
		Code: "method_not_allowed", Message: "use POST with JSON body",
		RequestID: requestID(c),
	}})
}

// ---------- helpers ----------

func funcNames(infos []gofunction.FuncInfo) []string {
	out := make([]string, 0, len(infos))
	for _, fi := range infos {
		out = append(out, fi.Name)
	}
	return out
}

func (h *GoFunctionHandler) recordAudit(c echo.Context, projectID, detail string) {
	recordAudit(c, h.audit, "gofunction", projectID, "ok", detail)
}

// bindGoFunctionBody 解析 JSON body。
func bindGoFunctionBody(c echo.Context, req any) error {
	if err := c.Bind(req); err != nil {
		return badRequest(c, "malformed JSON body")
	}
	return nil
}
