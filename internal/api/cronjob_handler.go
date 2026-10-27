// 文件说明：cronjob_handler.go — 定时任务（Cron Jobs）的管理与手动触发 handler（ui-cronjob-plan §6）
//
// 用法
//
//	NewCronJobHandler(store *systemdb.Store, writable bool, scheduler ...
//	(h *CronJobHandler) WithUsage(u UsageService) *CronJobHandler  // WithUsage 挂接配额服务，返回自身便于链式装配
//	(h *CronJobHandler) List(c echo.Context) error  // List 列出项目内全部任务，附 target_missing 标记
//	同包调用：Get() → collection_meta_handler.go, collection_transfer.go 等；List() → api_keys_handler.go, backup_handler.go 等；Delete() → api_keys_handler.go, backup_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/cronjob_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/agentbridge, internal/cronjob, internal/systemdb, internal/utils/crontab
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Get() ← collection_meta_handler.go, collection_transfer.go, data_handler.go
//
// 主要符号
//
//	type CronJobHandler struct {…}
//	NewCronJobHandler(store *systemdb.Store, writable bool, scheduler ...
//	(h *CronJobHandler) WithUsage(u UsageService) *CronJobHandler
//	(h *CronJobHandler) List(c echo.Context) error
//	(h *CronJobHandler) Get(c echo.Context) error
//	(h *CronJobHandler) Create(c echo.Context) error
//	... 另有 23 个顶层符号

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/utils/crontab"
)

// cronJobNameRe 任务名：字母开头，字母数字_-，1~63（§6.2）。
var cronJobNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,62}$`)

// cronJobInputLimit 入参 JSON 上限（§6.2）。
const cronJobInputLimit = 8 * 1024

// cronJobAgentPromptLimit agent 任务提示词上限：16KiB（字节口径，与 cronJobInputLimit 的 len() 一致）。
// 单一来源在 agentbridge，执行侧按同一值截断。
const cronJobAgentPromptLimit = agentbridge.CronAgentPromptLimit

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
	usage     UsageService // 可为 nil；agent 任务手动触发前做 llm 配额检查
}

// NewCronJobHandler 构造 handler。
func NewCronJobHandler(store *systemdb.Store, writable bool, scheduler *cronjob.Scheduler, audit AuditService) *CronJobHandler {
	return &CronJobHandler{store: store, writable: writable, scheduler: scheduler, audit: audit}
}

// WithUsage 挂接配额服务，返回自身便于链式装配。
func (h *CronJobHandler) WithUsage(u UsageService) *CronJobHandler {
	if h != nil {
		h.usage = u
	}
	return h
}

type cronJobDTO struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	ScheduleKind    string     `json:"schedule_kind"`
	CronExpr        string     `json:"cron_expr,omitempty"`
	IntervalSeconds *int64     `json:"interval_seconds,omitempty"`
	RunAt           *time.Time `json:"run_at,omitempty"`
	TargetKind      string     `json:"target_kind"` // "function" | "agent"
	FuncFile        string     `json:"func_file"`
	FuncExport      string     `json:"func_export"`
	InputJSON       string     `json:"input_json"`
	AgentID         string     `json:"agent_id,omitempty"`
	AgentThreadID   string     `json:"agent_thread_id,omitempty"`
	AgentPrompt     string     `json:"agent_prompt,omitempty"`
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
		TargetKind: j.EffectiveTargetKind(),
		FuncFile:   j.FuncFile, FuncExport: j.FuncExport, InputJSON: j.InputJSON,
		AgentID: j.AgentID, AgentThreadID: j.AgentThreadID, AgentPrompt: j.AgentPrompt,
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
	TargetKind      string `json:"target_kind"` // 空串视同 function（向后兼容）
	FuncFile        string `json:"func_file"`
	FuncExport      string `json:"func_export"`
	InputJSON       string `json:"input_json"`
	AgentID         string `json:"agent_id"`
	AgentPrompt     string `json:"agent_prompt"`
	Enabled         *bool  `json:"enabled"`

	// enabledValue 是校验后的 resolved 值（缺省 true），供写库使用。
	enabledValue bool
	// enabledSet 表示请求显式携带了 enabled；false 时 Update 保留原状态。
	enabledSet bool
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
	if len(jobs) == 0 {
		return c.JSON(http.StatusOK, map[string]any{"jobs": []cronJobDTO{}})
	}
	// 两类目标索引按需构建：没有对应类型任务时不做多余查询。
	var targets, agents map[string]bool
	for _, j := range jobs {
		if j.EffectiveTargetKind() == systemdb.CronJobTargetAgent {
			if agents == nil {
				if agents, err = h.buildAgentIndex(c.Request().Context(), pc.ID); err != nil {
					return WriteError(c, err)
				}
			}
		} else if targets == nil {
			if targets, err = h.buildTargetIndex(c.Request().Context(), pc.ID); err != nil {
				return WriteError(c, err)
			}
		}
	}
	out := make([]cronJobDTO, 0, len(jobs))
	for _, j := range jobs {
		missing := false
		if j.EffectiveTargetKind() == systemdb.CronJobTargetAgent {
			missing = !agents[j.AgentID]
		} else {
			missing = !targets[j.FuncFile+"\x00"+j.FuncExport]
		}
		out = append(out, toCronJobDTO(j, missing))
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
	var missing bool
	if j.EffectiveTargetKind() == systemdb.CronJobTargetAgent {
		agents, aerr := h.buildAgentIndex(c.Request().Context(), pc.ID)
		if aerr != nil {
			return WriteError(c, aerr)
		}
		missing = !agents[j.AgentID]
	} else if missing, err = h.isTargetMissing(c.Request().Context(), pc.ID, j.FuncFile, j.FuncExport); err != nil {
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
	if err := h.validateTargetFor(c.Request().Context(), pc.ID, req); err != nil {
		return WriteError(c, err)
	}
	next, err := computeNextRun(req.ScheduleKind, req.CronExpr, req.IntervalSeconds, req.runAtTime)
	if err != nil {
		return WriteError(c, err)
	}
	created, err := h.store.CreateCronJob(c.Request().Context(), systemdb.CronJob{
		ProjectID: pc.ID, Name: req.Name, Description: req.Description,
		ScheduleKind: req.ScheduleKind, CronExpr: req.CronExpr, IntervalSeconds: req.IntervalSeconds,
		RunAt:      req.runAtTime,
		TargetKind: req.TargetKind,
		FuncFile:   req.FuncFile, FuncExport: req.FuncExport, InputJSON: req.InputJSON,
		AgentID: req.AgentID, AgentPrompt: req.AgentPrompt,
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
	// 纯开关切换（body 只带 enabled，无调度字段）：保留原调度配置，
	// 仅翻转启停并重算排期——禁用清空 next_run_at（零值=未排期语义），
	// 启用按原 schedule 从当下重算。避免前端开关 PATCH 因缺 schedule_kind 报 400。
	if req.ScheduleKind == "" && req.Enabled != nil && req.Name == "" && req.FuncFile == "" && req.AgentID == "" {
		j.Enabled = *req.Enabled
		j.NextRunAt = time.Time{}
		if j.Enabled {
			next, err := computeNextRun(j.ScheduleKind, j.CronExpr, j.IntervalSeconds, j.RunAt)
			if err != nil {
				return WriteError(c, err)
			}
			j.NextRunAt = next
		}
		updated, err := h.store.UpdateCronJob(c.Request().Context(), j)
		if err != nil {
			return WriteError(c, err)
		}
		h.recordAudit(c, pc.ID, "toggle cron job "+j.Name)
		return c.JSON(http.StatusOK, toCronJobDTO(updated, false))
	}
	req, err = validateCronJobBody(req, rid, false)
	if err != nil {
		return WriteError(c, err)
	}
	if err := h.validateTargetFor(c.Request().Context(), pc.ID, req); err != nil {
		return WriteError(c, err)
	}
	// 保留不可变字段与快照字段；调度字段更新时重算排期。
	j.Description = req.Description
	j.ScheduleKind = req.ScheduleKind
	j.CronExpr = req.CronExpr
	j.IntervalSeconds = req.IntervalSeconds
	j.RunAt = req.runAtTime
	// 换了助手或切换了目标类型后，旧专属会话不再适用：清空，由执行器首次运行时重建。
	if req.TargetKind != j.EffectiveTargetKind() || req.AgentID != j.AgentID {
		j.AgentThreadID = ""
	}
	j.TargetKind = req.TargetKind
	j.FuncFile = req.FuncFile
	j.FuncExport = req.FuncExport
	j.InputJSON = req.InputJSON
	j.AgentID = req.AgentID
	j.AgentPrompt = req.AgentPrompt
	// enabled 未显式携带时保留原状态（不隐式重新启用已暂停任务）。
	if req.enabledSet {
		j.Enabled = req.enabledValue
	}
	if j.NextRunAt, err = computeNextRun(req.ScheduleKind, req.CronExpr, req.IntervalSeconds, req.runAtTime); err != nil {
		return WriteError(c, err)
	}
	// 禁用时清空排期（NextRunAt 零值=未排期），启用时保留上面重算的值。
	if !j.Enabled {
		j.NextRunAt = time.Time{}
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
	limit := queryLimit(c, 20, 0)
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
	// agent 任务会消耗 LLM 额度：手动触发前先检查配额（原 TriggerScheduleRun 的语义）。
	if j.EffectiveTargetKind() == systemdb.CronJobTargetAgent && h.usage != nil {
		if err := h.usage.CheckQuota(c.Request().Context(), pc.ID, "llm"); err != nil {
			return WriteError(c, err)
		}
	}
	// 异步执行用的是调度器自己的 ctx（LIFE-01 修复后 Trigger 忽略调用方 ctx），
	// 任务与请求生命周期解耦、但受调度器停止控制。
	if err := h.scheduler.Trigger(context.WithoutCancel(c.Request().Context()), j); err != nil {
		rid := RequestIDFromContext(c.Request().Context())
		if errors.Is(err, cronjob.ErrSchedulerStopped) {
			return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "cron_scheduler_stopped", "cron scheduler is not running", rid))
		}
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
	switch req.TargetKind {
	case "", systemdb.CronJobTargetFunction:
		req.TargetKind = systemdb.CronJobTargetFunction
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
		// 清掉另一侧字段，避免脏数据残留。
		req.AgentID, req.AgentPrompt = "", ""
	case systemdb.CronJobTargetAgent:
		req.AgentID = strings.TrimSpace(req.AgentID)
		if req.AgentID == "" {
			return req, NewAPIError(http.StatusBadRequest, "invalid_request", "agent_id is required for target_kind=agent", rid)
		}
		if strings.TrimSpace(req.AgentPrompt) == "" {
			return req, NewAPIError(http.StatusBadRequest, "agent_prompt_required", "agent_prompt is required for target_kind=agent", rid)
		}
		if len(req.AgentPrompt) > cronJobAgentPromptLimit {
			return req, NewAPIError(http.StatusBadRequest, "agent_prompt_too_long", "agent_prompt exceeds 16KB limit", rid)
		}
		req.FuncFile, req.FuncExport = "", ""
		req.InputJSON = "{}"
	default:
		return req, NewAPIError(http.StatusBadRequest, "invalid_target_kind", "target_kind must be function or agent", rid)
	}
	// enabled 缺省语义：未显式指定时不隐式翻转（保留原状态）。
	// enabledValue=false + enabledSet=false 表示“请求未带 enabled”，由调用方保留原值。
	req.enabledValue = true
	req.enabledSet = false
	if req.Enabled != nil {
		req.enabledValue = *req.Enabled
		req.enabledSet = true
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

// validateTargetFor 按 target_kind 分支校验目标：function 走生效版校验；
// agent 只校验云助手存在（不复用 gofunction 校验）。
func (h *CronJobHandler) validateTargetFor(ctx context.Context, projectID string, req upsertCronJobBody) error {
	if req.TargetKind != systemdb.CronJobTargetAgent {
		return h.validateTarget(ctx, projectID, req.FuncFile, req.FuncExport)
	}
	_, err := h.store.GetCloudAgent(ctx, projectID, req.AgentID)
	if errors.Is(err, sql.ErrNoRows) {
		return NewAPIError(http.StatusBadRequest, "agent_not_found", "agent does not exist in this project", RequestIDFromContext(ctx))
	}
	return err
}

// buildAgentIndex 构造项目内存在的云助手 ID 集合，用于 agent 任务的 target_missing 标记。
func (h *CronJobHandler) buildAgentIndex(ctx context.Context, projectID string) (map[string]bool, error) {
	agents, err := h.store.ListCloudAgents(ctx, projectID)
	if err != nil {
		return nil, err
	}
	index := make(map[string]bool, len(agents))
	for _, a := range agents {
		index[a.ID] = true
	}
	return index, nil
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
