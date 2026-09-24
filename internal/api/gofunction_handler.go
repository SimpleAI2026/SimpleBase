// gofunction_handler.go 云函数管理 + 版本 + 调试台 + /go 调用
// （gofunction-versions-testplan §5）。
//
// 管理面（/v1/projects/:projectID/gofunctions）：
//
//	GET    /gofunctions                         列表（含 active/latest）
//	POST   /gofunctions                         创建实体 + v1
//	GET    /gofunctions/:name                   详情 + 版本摘要
//	PATCH  /gofunctions/:name                   改 description
//	DELETE /gofunctions/:name                   软删
//	GET    /gofunctions/:name/versions          版本列表
//	POST   /gofunctions/:name/versions          新建版本
//	GET    /gofunctions/:name/versions/:ver     版本详情（含 source）
//	POST   /gofunctions/:name/versions/:ver/activate  发布/回滚
//	POST   /gofunctions/:name/versions/:ver/test      调试台试跑
//
// 调用面：
//
//	POST /go/:projectID/:name/:functionName     只跑 active_version
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// GoFunctionHandler 依赖系统库；writable=false 时写/测试返回 503。
type GoFunctionHandler struct {
	store    *systemdb.Store
	writable bool
	audit    AuditService
}

var (
	gofunctionNameRe       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)
	gofunctionExportNameRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*$`)
)

const (
	gofunctionMaxSourceBytes = 256 << 10
	gofunctionMaxPerProject  = 100
)

// NewGoFunctionHandler 构造 handler。
func NewGoFunctionHandler(store *systemdb.Store, writable bool, audit AuditService) *GoFunctionHandler {
	return &GoFunctionHandler{store: store, writable: writable, audit: audit}
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

// goFunctionDTO / toGoFunctionDTO：兼容旧测试与 PUT 路径。
type goFunctionDTO = goFuncDTO

func toGoFunctionDTO(g systemdb.GoFunction, withSource bool) goFuncDTO {
	exports := g.Exports
	if exports == nil {
		exports = []string{}
	}
	dto := goFuncDTO{
		ID:            g.ID,
		Name:          g.Name,
		File:          g.Name + ".go",
		ActiveVersion: 1,
		LatestVersion: 1,
		Published:     true,
		Exports:       exports,
		CreatedAt:     g.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     g.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if withSource {
		dto.Source = g.Source
	}
	return dto
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

// List: GET /gofunctions
func (h *GoFunctionHandler) List(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
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
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	rid := RequestIDFromContext(c.Request().Context())
	if !h.writable {
		return WriteError(c, errWriterUnavailable())
	}
	if systemdb.IsAdminProject(pc.ID) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support go functions", rid))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	var req goFuncCreateRequest
	if err := bindGoFunctionBody(c, &req); err != nil {
		return WriteError(c, err)
	}
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
	exports := funcNames(infos)

	f, err := h.store.CreateGoFunc(c.Request().Context(), systemdb.GoFunc{
		ProjectID: pc.ID, Name: req.Name, Description: req.Description,
		CreatedBy: actorID(c),
	})
	if err != nil {
		return WriteError(c, err)
	}
	v, err := h.store.AppendGoFuncVersion(c.Request().Context(), systemdb.GoFuncVersion{
		FuncID: f.ID, ProjectID: pc.ID, Name: req.Name,
		Source: req.Source, Exports: exports, Note: req.Note, CreatedBy: actorID(c),
	})
	if err != nil {
		return WriteError(c, err)
	}
	activate := true
	if req.Activate != nil {
		activate = *req.Activate
	}
	if activate {
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
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
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
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if !h.writable {
		return WriteError(c, errWriterUnavailable())
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	name := c.Param("name")
	f, err := h.store.GetGoFunc(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", RequestIDFromContext(c.Request().Context())))
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
	if err := h.store.ArchiveGoFunc(c.Request().Context(), pc.ID, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WriteError(c, NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", RequestIDFromContext(c.Request().Context())))
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

// Update 兼容旧 PUT：保存为新版本并生效，响应 200。
func (h *GoFunctionHandler) Update(c echo.Context) error {
	err := h.CreateVersion(c)
	return err
}

// ListVersions: GET /gofunctions/:name/versions
func (h *GoFunctionHandler) ListVersions(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	name := c.Param("name")
	f, err := h.store.GetGoFunc(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", RequestIDFromContext(c.Request().Context())))
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
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	rid := RequestIDFromContext(c.Request().Context())
	if !h.writable {
		return WriteError(c, errWriterUnavailable())
	}
	if systemdb.IsAdminProject(pc.ID) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support go functions", rid))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
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
	if err := validateGoFunctionSource(req.Source); err != nil {
		return WriteError(c, err)
	}
	infos, err := h.validateSource(c, req.Source)
	if err != nil {
		return WriteError(c, mapGoFunctionValidationError(err, rid))
	}
	v, err := h.store.AppendGoFuncVersion(c.Request().Context(), systemdb.GoFuncVersion{
		FuncID: f.ID, ProjectID: pc.ID, Name: name,
		Source: req.Source, Exports: funcNames(infos), Note: req.Note, CreatedBy: actorID(c),
	})
	if err != nil {
		if errors.Is(err, systemdb.ErrVersionLimit) {
			return WriteError(c, NewAPIError(422, "version_limit_exceeded", "version limit exceeded (50 per function)", rid))
		}
		return WriteError(c, err)
	}
	activate := true
	if req.Activate != nil {
		activate = *req.Activate
	}
	if activate {
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
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	name := c.Param("name")
	ver, err := strconv.ParseInt(c.Param("ver"), 10, 64)
	if err != nil || ver <= 0 {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid version", RequestIDFromContext(c.Request().Context())))
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
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	rid := RequestIDFromContext(c.Request().Context())
	if !h.writable {
		return WriteError(c, errWriterUnavailable())
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	name := c.Param("name")
	ver, err := strconv.ParseInt(c.Param("ver"), 10, 64)
	if err != nil || ver <= 0 {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid version", rid))
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

// TestVersion: POST /gofunctions/:name/versions/:ver/test（plan §5.3）
func (h *GoFunctionHandler) TestVersion(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	rid := RequestIDFromContext(c.Request().Context())
	if !h.writable {
		return WriteError(c, errWriterUnavailable())
	}
	if systemdb.IsAdminProject(pc.ID) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support go functions", rid))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	name := c.Param("name")
	ver, err := strconv.ParseInt(c.Param("ver"), 10, 64)
	if err != nil || ver <= 0 {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid version", rid))
	}
	var req goFuncTestRequest
	if err := bindGoFunctionBody(c, &req); err != nil {
		return WriteError(c, err)
	}
	if !gofunctionExportNameRe.MatchString(req.FunctionName) {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid function_name", rid))
	}
	f, err := h.store.GetGoFunc(c.Request().Context(), pc.ID, name)
	if err != nil {
		return WriteError(c, mapNotFound(err))
	}
	v, err := h.store.GetGoFuncVersion(c.Request().Context(), pc.ID, name, ver)
	if err != nil {
		return WriteError(c, mapNotFound(err))
	}
	body := req.Body
	if len(body) == 0 {
		body = json.RawMessage(`{}`)
	}
	start := time.Now()
	out, runErr := gofunction.RunJSON(c.Request().Context(), rid, name, v.Source, req.FunctionName, body)
	dur := time.Since(start)
	resp := goFuncTestResponse{
		OK:           runErr == nil,
		StatusCode:   http.StatusOK,
		DurationMs:   dur.Milliseconds(),
		Version:      ver,
		ActiveVersion: f.ActiveVersion,
		FunctionName: req.FunctionName,
	}
	if runErr != nil {
		ae := mapGoFunctionRunError(runErr, rid)
		var he *APIError
		if errors.As(ae, &he) {
			resp.StatusCode = he.HTTPStatus
			resp.Error = he.Body.Error.Message
		} else {
			resp.StatusCode = http.StatusInternalServerError
			resp.Error = runErr.Error()
		}
	} else {
		resp.Data = out
	}
	h.store.RecordGoFuncInvoke(c.Request().Context(), pc.ID, name, req.FunctionName, ver, "test",
		resp.StatusCode, dur.Milliseconds(), rid, actorID(c))
	h.recordInvokeMetrics(pc.ID, dur, runErr)
	h.recordAudit(c, pc.ID, "test "+name+".go v"+strconv.FormatInt(ver, 10))
	return c.JSON(http.StatusOK, resp)
}

// ---------- /go 调用面（只跑 active） ----------

// Invoke: POST /go/:projectID/:name/:functionName
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
	// D8：只跑 active_version
	v, err := h.store.ResolveActiveSource(c.Request().Context(), pc.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return fail(NewAPIError(http.StatusNotFound, "gofunction_not_found", "go function not found", rid))
	}
	if errors.Is(err, systemdb.ErrNoActiveVersion) {
		return fail(NewAPIError(http.StatusConflict, "no_active_version", "go function has no active version", rid))
	}
	if err != nil {
		return fail(err)
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return fail(NewAPIError(http.StatusBadRequest, "invalid_request", "cannot read request body", rid))
	}
	out, runErr := gofunction.RunJSON(c.Request().Context(), rid, name, v.Source, fnName, body)
	dur := time.Since(start)
	status := http.StatusOK
	if runErr != nil {
		ae := mapGoFunctionRunError(runErr, rid)
		var he *APIError
		if errors.As(ae, &he) {
			status = he.HTTPStatus
		} else {
			status = http.StatusInternalServerError
		}
		h.store.RecordGoFuncInvoke(c.Request().Context(), pc.ID, name, fnName, v.Version, "go",
			status, dur.Milliseconds(), rid, actorID(c))
		return fail(ae)
	}
	h.store.RecordGoFuncInvoke(c.Request().Context(), pc.ID, name, fnName, v.Version, "go",
		status, dur.Milliseconds(), rid, actorID(c))
	return c.Blob(http.StatusOK, "application/json", out)
}

// MethodNotAllowed 返回 405 JSON。
func (h *GoFunctionHandler) MethodNotAllowed(c echo.Context) error {
	return c.JSON(http.StatusMethodNotAllowed, APIErrorBody{Error: APIErrorDetail{
		Code: "method_not_allowed", Message: "use POST with JSON body",
		RequestID: RequestIDFromContext(c.Request().Context()),
	}})
}

// ---------- helpers ----------

func (h *GoFunctionHandler) loadFunc(c echo.Context, projectID, name string) (systemdb.GoFunc, []systemdb.GoFuncVersion, error) {
	rid := RequestIDFromContext(c.Request().Context())
	f, err := h.store.GetGoFunc(c.Request().Context(), projectID, name)
	if err != nil {
		return systemdb.GoFunc{}, nil, mapNotFound(err)
	}
	vers, err := h.store.ListGoFuncVersions(c.Request().Context(), projectID, name)
	if err != nil {
		_ = rid
		return systemdb.GoFunc{}, nil, err
	}
	return f, vers, nil
}

func (h *GoFunctionHandler) pickSource(c echo.Context, projectID, name string, f systemdb.GoFunc) (string, error) {
	if f.ActiveVersion > 0 {
		v, err := h.store.GetGoFuncVersion(c.Request().Context(), projectID, name, f.ActiveVersion)
		if err == nil {
			return v.Source, nil
		}
	}
	v, err := h.store.LatestGoFuncVersion(c.Request().Context(), projectID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v.Source, nil
}

func funcNames(infos []gofunction.FuncInfo) []string {
	out := make([]string, 0, len(infos))
	for _, fi := range infos {
		out = append(out, fi.Name)
	}
	return out
}

func mapNotFound(err error) error {
	rid := "" // filled by WriteError via Error()
	if errors.Is(err, sql.ErrNoRows) {
		return NewAPIError(http.StatusNotFound, "not_found", "resource not found", rid)
	}
	return err
}

func actorID(c echo.Context) string {
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return ""
	}
	if p.UserID != "" {
		return p.UserID
	}
	return p.APIKeyID
}

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

func (h *GoFunctionHandler) recordAudit(c echo.Context, projectID, detail string) {
	if h.audit == nil {
		return
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	pID := ""
	if ok {
		pID = string(principal.APIKeyID)
		if principal.UserID != "" {
			pID = principal.UserID
		}
	}
	_ = h.audit.Record(c.Request().Context(), AuditEvent{
		ProjectID:   projectID,
		PrincipalID: pID,
		Kind:        "gofunction",
		RequestID:   RequestIDFromContext(c.Request().Context()),
		Status:      "ok",
		Detail:      detail,
	})
}

// bindGoFunctionBody 解析 JSON body。
func bindGoFunctionBody(c echo.Context, req any) error {
	if err := c.Bind(req); err != nil {
		return NewAPIError(http.StatusBadRequest, "invalid_request", "malformed JSON body", RequestIDFromContext(c.Request().Context()))
	}
	return nil
}

// validateGoFunctionSource 校验源码非空与体积上限。
func validateGoFunctionSource(source string) error {
	rid := ""
	if source == "" {
		return NewAPIError(http.StatusBadRequest, "invalid_request", "source is required", rid)
	}
	if len(source) > gofunctionMaxSourceBytes {
		return NewAPIError(http.StatusRequestEntityTooLarge, "request_too_large", "source exceeds 256KB limit", rid)
	}
	return nil
}

// mapGoFunctionValidationError 映射 ValidateHTTPFuncs 错误。
// 约定/签名问题 → gofunction_signature_invalid；编译错误 → gofunction_compile_error。
func mapGoFunctionValidationError(err error, rid string) error {
	if err == nil {
		return nil
	}
	msg := truncateMessage(err.Error(), 512)
	// 编译/语法错误才用 compile_error；签名与导出约定问题统一 signature_invalid
	if strings.Contains(err.Error(), "syntax error") ||
		strings.Contains(err.Error(), "expected") ||
		strings.Contains(err.Error(), "解析失败") {
		return NewAPIError(http.StatusBadRequest, "gofunction_compile_error", msg, rid)
	}
	return NewAPIError(http.StatusBadRequest, "gofunction_signature_invalid", msg, rid)
}

// mapGoFunctionRunError 映射 RunJSON 错误。
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
