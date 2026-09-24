// cronjob_handler.go 实现定时任务（Cron Jobs）的管理与手动触发 handler（ui-cronjob-plan §6）。
//
// 管理面（挂在 /v1/projects/:projectID/cron-jobs 下）：
//
//	GET    /cron-jobs            列出
//	POST   /cron-jobs            创建（校验调度与目标云函数）
//	GET    /cron-jobs/:jobID     详情
//	PATCH  /cron-jobs/:jobID     更新（name 不可改）
//	DELETE /cron-jobs/:jobID     软删
//	GET    /cron-jobs/:jobID/runs    运行记录
//	POST   /cron-jobs/:jobID/trigger 手动立即执行（异步，202）
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/crontab"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// cronJobNameRe 任务名：字母开头，字母数字_-，1~63（§6.2）。
var cronJobNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,62}$`)

// cronJobInputLimit 入参 JSON 上限（§6.2）。
const cronJobInputLimit = 8 * 1024

// cronJobInterval 域：60s ~ 30 天（§3）。
const (
	cronJobIntervalMinSeconds = 60
	cronJobIntervalMaxSeconds = 30 * 86400
)

// CronJobHandler 依赖系统库；writable=false 时写操作返回 503。
// scheduler 用于手动触发；audit 可为 nil。
type CronJobHandler struct {
	store     *systemdb.Store
	writable  bool
	scheduler *cronjob.Scheduler
	audit     AuditService
}

// NewCronJobHandler 构造 handler。
func NewCronJobHandler(store *systemdb.Store, writable bool, scheduler *cronjob.Scheduler, audit AuditService) *CronJobHandler {
	return &CronJobHandler{store: store, writable: writable, scheduler: scheduler, audit: audit}
}

type cronJobDTO struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	ScheduleKind    string     `json:"schedule_kind"`
	CronExpr        string     `json:"cron_expr,omitempty"`
	IntervalSeconds *int64     `json:"interval_seconds,omitempty"`
	RunAt           *time.Time `json:"run_at,omitempty"`
	FuncFile        string     `json:"func_file"`
	FuncExport      string     `json:"func_export"`
	InputJSON       string     `json:"input_json"`
	Enabled         bool       `json:"enabled"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	LastStatus      string     `json:"last_status"`
	LastError       string     `json:"last_error"`
	RunCount        int64      `json:"run_count"`
	TargetMissing   bool       `json:"target_missing"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type cronJobRunDTO struct {
	ID           string     `json:"id"`
	JobID        string     `json:"job_id"`
	Trigger      string     `json:"trigger"`
	Status       string     `json:"status"`
	Error        string     `json:"error,omitempty"`
	DurationMs   int64      `json:"duration_ms"`
	ResponseJSON string     `json:"response_json,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func toCronJobDTO(j systemdb.CronJob, targetMissing bool) cronJobDTO {
	dto := cronJobDTO{
		ID: j.ID, Name: j.Name, Description: j.Description,
		ScheduleKind: j.ScheduleKind, CronExpr: j.CronExpr,
		FuncFile: j.FuncFile, FuncExport: j.FuncExport, InputJSON: j.InputJSON,
		Enabled: j.Enabled, LastStatus: j.LastStatus, LastError: j.LastError,
		RunCount: j.RunCount, TargetMissing: targetMissing,
		CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt,
	}
	if j.IntervalSeconds > 0 {
		v := j.IntervalSeconds
		dto.IntervalSeconds = &v
	}
	if !j.RunAt.IsZero() {
		t := j.RunAt
		dto.RunAt = &t
	}
	if !j.LastRunAt.IsZero() {
		t := j.LastRunAt
		dto.LastRunAt = &t
	}
	if !j.NextRunAt.IsZero() {
		t := j.NextRunAt
		dto.NextRunAt = &t
	}
	return dto
}

func toCronJobRunDTO(r systemdb.CronJobRun) cronJobRunDTO {
	dto := cronJobRunDTO{
		ID: r.ID, JobID: r.JobID, Trigger: r.Trigger, Status: r.Status,
		Error: r.Error, DurationMs: r.DurationMs, ResponseJSON: r.ResponseJSON,
		CreatedAt: r.CreatedAt,
	}
	if !r.StartedAt.IsZero() {
		t := r.StartedAt
		dto.StartedAt = &t
	}
	if !r.FinishedAt.IsZero() {
		t := r.FinishedAt
		dto.FinishedAt = &t
	}
	return dto
}

type upsertCronJobBody struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	ScheduleKind    string `json:"schedule_kind"`
	CronExpr        string `json:"cron_expr"`
	IntervalSeconds int64  `json:"interval_seconds"`
	RunAt           string `json:"run_at"`
	FuncFile        string `json:"func_file"`
	FuncExport      string `json:"func_export"`
	InputJSON       string `json:"input_json"`
	Enabled         *bool  `json:"enabled"`

	// enabledValue 是校验后的 resolved 值（缺省 true），供写库使用。
	enabledValue bool
	// runAtTime 是校验后的 run_at（kind=once）。
	runAtTime time.Time
}

// List 列出项目内全部任务，附 target_missing 标记。
func (h *CronJobHandler) List(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	jobs, err := h.store.ListCronJobs(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	targets, err := h.buildTargetIndex(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]cronJobDTO, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, toCronJobDTO(j, !targets[j.FuncFile+"\x00"+j.FuncExport]))
	}
	return c.JSON(http.StatusOK, map[string]any{"jobs": out})
}

// Get 任务详情。
func (h *CronJobHandler) Get(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	j, err := h.store.GetCronJob(c.Request().Context(), pc.ID, c.Param("jobID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "cron_job_not_found", "cron job not found", RequestIDFromContext(c.Request().Context())))
	}
	if err != nil {
		return WriteError(c, err)
	}
	missing, err := h.isTargetMissing(c.Request().Context(), pc.ID, j.FuncFile, j.FuncExport)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toCronJobDTO(j, missing))
}

// Create 创建任务（§6.2 校验顺序：name → schedule → 目标 → input → 写库）。
func (h *CronJobHandler) Create(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	if !h.writable {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "readonly_mode", "server is in readonly mode", RequestIDFromContext(c.Request().Context())))
	}
	rid := RequestIDFromContext(c.Request().Context())
	var req upsertCronJobBody
	if err := bindCronJobBody(c, &req); err != nil {
		return WriteError(c, err)
	}
	req, err := validateCronJobBody(req, rid, true)
	if err != nil {
		return WriteError(c, err)
	}
	// 同名唯一（项目惯例：先查后写映射 409）。
	if _, err := h.store.GetCronJobByName(c.Request().Context(), pc.ID, req.Name); err == nil {
		return WriteError(c, NewAPIError(http.StatusConflict, "cron_job_exists", "cron job with this name already exists", rid))
	} else if !errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, err)
	}
	if err := h.validateTarget(c.Request().Context(), pc.ID, req.FuncFile, req.FuncExport); err != nil {
		return WriteError(c, err)
	}
	next, err := computeNextRun(req.ScheduleKind, req.CronExpr, req.IntervalSeconds, req.runAtTime)
	if err != nil {
		return WriteError(c, err)
	}
	created, err := h.store.CreateCronJob(c.Request().Context(), systemdb.CronJob{
		ProjectID: pc.ID, Name: req.Name, Description: req.Description,
		ScheduleKind: req.ScheduleKind, CronExpr: req.CronExpr, IntervalSeconds: req.IntervalSeconds,
		RunAt: req.runAtTime,
		FuncFile: req.FuncFile, FuncExport: req.FuncExport, InputJSON: req.InputJSON,
		Enabled: req.enabledValue, NextRunAt: next,
	})
	if err != nil {
		return WriteError(c, err)
	}
	h.recordAudit(c, pc.ID, "create cron job "+req.Name)
	return c.JSON(http.StatusCreated, toCronJobDTO(created, false))
}

// Update 更新任务（name 不可改；调度变更重算 next_run_at）。
func (h *CronJobHandler) Update(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	if !h.writable {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "readonly_mode", "server is in readonly mode", RequestIDFromContext(c.Request().Context())))
	}
	rid := RequestIDFromContext(c.Request().Context())
	j, err := h.store.GetCronJob(c.Request().Context(), pc.ID, c.Param("jobID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "cron_job_not_found", "cron job not found", rid))
	}
	if err != nil {
		return WriteError(c, err)
	}
	var req upsertCronJobBody
	if err := bindCronJobBody(c, &req); err != nil {
		return WriteError(c, err)
	}
	req, err = validateCronJobBody(req, rid, false)
	if err != nil {
		return WriteError(c, err)
	}
	if err := h.validateTarget(c.Request().Context(), pc.ID, req.FuncFile, req.FuncExport); err != nil {
		return WriteError(c, err)
	}
	// 保留不可变字段与快照字段；调度字段更新时重算排期。
	j.Description = req.Description
	j.ScheduleKind = req.ScheduleKind
	j.CronExpr = req.CronExpr
	j.IntervalSeconds = req.IntervalSeconds
	j.RunAt = req.runAtTime
	j.FuncFile = req.FuncFile
	j.FuncExport = req.FuncExport
	j.InputJSON = req.InputJSON
	j.Enabled = req.enabledValue
	j.NextRunAt, err = computeNextRun(req.ScheduleKind, req.CronExpr, req.IntervalSeconds, req.runAtTime)
	if err != nil {
		return WriteError(c, err)
	}
	updated, err := h.store.UpdateCronJob(c.Request().Context(), j)
	if err != nil {
		return WriteError(c, err)
	}
	h.recordAudit(c, pc.ID, "update cron job "+j.Name)
	return c.JSON(http.StatusOK, toCronJobDTO(updated, false))
}

// Delete 软删任务。
func (h *CronJobHandler) Delete(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	if !h.writable {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "readonly_mode", "server is in readonly mode", RequestIDFromContext(c.Request().Context())))
	}
	rid := RequestIDFromContext(c.Request().Context())
	j, err := h.store.GetCronJob(c.Request().Context(), pc.ID, c.Param("jobID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "cron_job_not_found", "cron job not found", rid))
	}
	if err != nil {
		return WriteError(c, err)
	}
	if err := h.store.ArchiveCronJob(c.Request().Context(), pc.ID, j.ID); err != nil {
		return WriteError(c, err)
	}
	h.recordAudit(c, pc.ID, "delete cron job "+j.Name)
	return c.NoContent(http.StatusNoContent)
}

// ListRuns 运行记录（新→旧）。
func (h *CronJobHandler) ListRuns(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	limit := 20
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	runs, err := h.store.ListCronJobRuns(c.Request().Context(), pc.ID, c.Param("jobID"), limit)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]cronJobRunDTO, 0, len(runs))
	for _, r := range runs {
		out = append(out, toCronJobRunDTO(r))
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": out})
}

// Trigger 手动立即执行（异步，不改排期）。
func (h *CronJobHandler) Trigger(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	rid := RequestIDFromContext(c.Request().Context())
	j, err := h.store.GetCronJob(c.Request().Context(), pc.ID, c.Param("jobID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, NewAPIError(http.StatusNotFound, "cron_job_not_found", "cron job not found", rid))
	}
	if err != nil {
		return WriteError(c, err)
	}
	if h.scheduler == nil {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "cron_scheduler_unavailable", "cron scheduler is not running", rid))
	}
	if err := h.scheduler.Trigger(c.Request().Context(), j); err != nil {
		return WriteError(c, NewAPIError(http.StatusConflict, "cron_job_running", "cron job is already executing", rid))
	}
	h.recordAudit(c, pc.ID, "trigger cron job "+j.Name)
	return c.JSON(http.StatusAccepted, map[string]any{"triggered": true})
}

// ---- 内部辅助 ----

// bindCronJobBody 解析 body 并做 input_json 长度检查。
func bindCronJobBody(c echo.Context, req *upsertCronJobBody) error {
	if err := c.Bind(req); err != nil {
		return NewAPIError(http.StatusBadRequest, "invalid_request", "cannot parse request body", RequestIDFromContext(c.Request().Context()))
	}
	return nil
}

// validateCronJobBody 字段级校验；create=true 时 name 必填（更新时忽略 name）。
func validateCronJobBody(req upsertCronJobBody, rid string, create bool) (upsertCronJobBody, error) {
	if create && !cronJobNameRe.MatchString(req.Name) {
		return req, NewAPIError(http.StatusBadRequest, "invalid_cron_job_name", "name must start with a letter and contain only letters, digits, _ or -", rid)
	}
	switch req.ScheduleKind {
	case systemdb.CronJobKindCron:
		if req.CronExpr == "" {
			return req, NewAPIError(http.StatusBadRequest, "invalid_cron_expression", "cron_expr is required for schedule_kind=cron", rid)
		}
		if _, err := crontab.ParseCron(req.CronExpr); err != nil {
			return req, NewAPIError(http.StatusBadRequest, "invalid_cron_expression", err.Error(), rid)
		}
		req.IntervalSeconds = 0
		req.RunAt = ""
	case systemdb.CronJobKindInterval:
		if req.IntervalSeconds < cronJobIntervalMinSeconds || req.IntervalSeconds > cronJobIntervalMaxSeconds {
			return req, NewAPIError(http.StatusBadRequest, "invalid_interval", "interval_seconds must be between 60 and 2592000", rid)
		}
		req.CronExpr = ""
		req.RunAt = ""
	case systemdb.CronJobKindOnce:
		if req.RunAt == "" {
			return req, NewAPIError(http.StatusBadRequest, "invalid_run_at", "run_at is required for schedule_kind=once", rid)
		}
		t, err := time.Parse(time.RFC3339, req.RunAt)
		if err != nil {
			return req, NewAPIError(http.StatusBadRequest, "invalid_run_at", "run_at must be RFC3339 timestamp", rid)
		}
		req.runAtTime = t.UTC()
		req.CronExpr = ""
		req.IntervalSeconds = 0
	default:
		return req, NewAPIError(http.StatusBadRequest, "invalid_request", "schedule_kind must be cron, interval or once", rid)
	}
	if req.FuncFile == "" || req.FuncExport == "" {
		return req, NewAPIError(http.StatusBadRequest, "invalid_request", "func_file and func_export are required", rid)
	}
	if req.InputJSON == "" {
		req.InputJSON = "{}"
	}
	if len(req.InputJSON) > cronJobInputLimit {
		return req, NewAPIError(http.StatusBadRequest, "invalid_request", "input_json exceeds 8KB limit", rid)
	}
	var probe json.RawMessage
	if err := json.Unmarshal([]byte(req.InputJSON), &probe); err != nil {
		return req, NewAPIError(http.StatusBadRequest, "invalid_request", "input_json is not valid JSON: "+err.Error(), rid)
	}
	req.enabledValue = true
	if req.Enabled != nil {
		req.enabledValue = *req.Enabled
	}
	return req, nil
}

// validateTarget 校验目标云函数存在生效版且函数在生效版 exports 内（plan §2.1）。
func (h *CronJobHandler) validateTarget(ctx context.Context, projectID, file, export string) error {
	v, err := h.store.ResolveActiveSource(ctx, projectID, file)
	if errors.Is(err, sql.ErrNoRows) {
		return NewAPIError(http.StatusBadRequest, "gofunction_not_found", "target go function file does not exist", RequestIDFromContext(ctx))
	}
	if errors.Is(err, systemdb.ErrNoActiveVersion) {
		return NewAPIError(http.StatusBadRequest, "no_active_version", "go function has no active version", RequestIDFromContext(ctx))
	}
	if err != nil {
		return err
	}
	for _, e := range v.Exports {
		if e == export {
			return nil
		}
	}
	return NewAPIError(http.StatusBadRequest, "function_not_exported", "target function is not exported in this file", RequestIDFromContext(ctx))
}

// buildTargetIndex 构造项目内可用目标集合（仅生效版 exports）：file\x00export → true。
func (h *CronJobHandler) buildTargetIndex(ctx context.Context, projectID string) (map[string]bool, error) {
	funcs, err := h.store.ListGoFuncs(ctx, projectID)
	if err != nil {
		return nil, err
	}
	index := map[string]bool{}
	for _, f := range funcs {
		if f.ActiveVersion <= 0 {
			continue
		}
		v, err := h.store.GetGoFuncVersion(ctx, projectID, f.Name, f.ActiveVersion)
		if err != nil {
			continue
		}
		for _, e := range v.Exports {
			index[f.Name+"\x00"+e] = true
		}
	}
	return index, nil
}

// isTargetMissing 单个目标可用性检查。
func (h *CronJobHandler) isTargetMissing(ctx context.Context, projectID, file, export string) (bool, error) {
	index, err := h.buildTargetIndex(ctx, projectID)
	if err != nil {
		return false, err
	}
	return !index[file+"\x00"+export], nil
}

// computeNextRun 按调度模式计算下一次触发时刻。
func computeNextRun(kind, cronExpr string, intervalSeconds int64, runAt time.Time) (time.Time, error) {
	now := time.Now().UTC()
	switch kind {
	case systemdb.CronJobKindCron:
		spec, err := crontab.ParseCron(cronExpr)
		if err != nil {
			return time.Time{}, NewAPIError(http.StatusBadRequest, "invalid_cron_expression", err.Error(), "")
		}
		next, err := spec.NextAfter(now)
		if err != nil {
			return time.Time{}, NewAPIError(http.StatusBadRequest, "invalid_cron_expression", err.Error(), "")
		}
		return next, nil
	case systemdb.CronJobKindOnce:
		return runAt, nil
	default:
		return now.Add(time.Duration(intervalSeconds) * time.Second), nil
	}
}

// recordAudit 写操作审计（§8）：kind=cronjob，detail 只含动作与任务名。
func (h *CronJobHandler) recordAudit(c echo.Context, projectID, detail string) {
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
		Kind:        "cronjob",
		RequestID:   RequestIDFromContext(c.Request().Context()),
		Status:      "ok",
		Detail:      detail,
	})
}
