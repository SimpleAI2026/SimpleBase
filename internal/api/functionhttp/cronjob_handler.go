package functionhttp

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
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/utils/crontab"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
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
	audit     httpkit.AuditService
	usage     httpkit.UsageService // 可为 nil；agent 任务手动触发前做 llm 配额检查
}

// NewCronJobHandler 构造 handler。
func NewCronJobHandler(store *systemdb.Store, writable bool, scheduler *cronjob.Scheduler, audit httpkit.AuditService) *CronJobHandler {
	return &CronJobHandler{store: store, writable: writable, scheduler: scheduler, audit: audit}
}

// WithUsage 挂接配额服务，返回自身便于链式装配。
func (h *CronJobHandler) WithUsage(u httpkit.UsageService) *CronJobHandler {
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
	// planv6.7 v2 §11：群定时绑定（可选）。TagID 非空表示这条任务按「协作群」执行，
	// AgentID 仍必填（用于群成员失效时的目标存在性校验与旧客户端兼容）。
	TagID        string `json:"tag_id"`
	ThreadPolicy string `json:"thread_policy"` // reuse_thread | new_thread_per_fire
	ThreadKind   string `json:"thread_kind"`
	OwnerUserID  string `json:"owner_user_id"`

	// enabledValue 是校验后的 resolved 值（缺省 true），供写库使用。
	enabledValue bool
	// enabledSet 表示请求显式携带了 enabled；false 时 Update 保留原状态。
	enabledSet bool
	// runAtTime 是校验后的 run_at（kind=once）。
	runAtTime time.Time
}

// List 列出项目内全部任务，附 target_missing 标记。
func (h *CronJobHandler) List(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.store == nil {
		return httpkit.WriteError(c, systemdb.ErrUnavailable)
	}
	jobs, err := h.store.ListCronJobs(c.Request().Context(), pc.ID)
	if err != nil {
		return httpkit.WriteError(c, err)
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
					return httpkit.WriteError(c, err)
				}
			}
		} else if targets == nil {
			if targets, err = h.buildTargetIndex(c.Request().Context(), pc.ID); err != nil {
				return httpkit.WriteError(c, err)
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
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.store == nil {
		return httpkit.WriteError(c, systemdb.ErrUnavailable)
	}
	j, err := h.store.GetCronJob(c.Request().Context(), pc.ID, c.Param("jobID"))
	if errors.Is(err, sql.ErrNoRows) {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusNotFound, "cron_job_not_found", "cron job not found", httpkit.RequestID(c)))
	}
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var missing bool
	if j.EffectiveTargetKind() == systemdb.CronJobTargetAgent {
		agents, aerr := h.buildAgentIndex(c.Request().Context(), pc.ID)
		if aerr != nil {
			return httpkit.WriteError(c, aerr)
		}
		missing = !agents[j.AgentID]
	} else if missing, err = h.isTargetMissing(c.Request().Context(), pc.ID, j.FuncFile, j.FuncExport); err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toCronJobDTO(j, missing))
}

// Create 创建任务（§6.2 校验顺序：name → schedule → 目标 → input → 写库）。
func (h *CronJobHandler) Create(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.store == nil {
		return httpkit.WriteError(c, systemdb.ErrUnavailable)
	}
	if !h.writable {
		return httpkit.WriteError(c, godatabase.ErrWriterUnavailable)
	}
	rid := httpkit.RequestID(c)
	var req upsertCronJobBody
	if err := bindCronJobBody(c, &req); err != nil {
		return httpkit.WriteError(c, err)
	}
	err = validateCronJobBody(&req, rid, true)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	// 同名唯一（项目惯例：先查后写映射 409）。
	if _, err := h.store.GetCronJobByName(c.Request().Context(), pc.ID, req.Name); err == nil {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusConflict, "cron_job_exists", "cron job with this name already exists", rid))
	} else if !errors.Is(err, sql.ErrNoRows) {
		return httpkit.WriteError(c, err)
	}
	if err := h.validateTargetFor(c.Request().Context(), pc.ID, req); err != nil {
		return httpkit.WriteError(c, err)
	}
	next, err := computeNextRun(req.ScheduleKind, req.CronExpr, req.IntervalSeconds, req.runAtTime)
	if err != nil {
		return httpkit.WriteError(c, err)
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
		return httpkit.WriteError(c, err)
	}
	// v2：写入群绑定。失败时回滚任务，避免「任务已建但绑定丢失」的半成品。
	if err := h.saveGroupBinding(c.Request().Context(), pc.ID, created.ID, req); err != nil {
		_ = h.store.ArchiveCronJob(c.Request().Context(), pc.ID, created.ID)
		return httpkit.WriteError(c, err)
	}
	h.recordAudit(c, pc.ID, "create cron job "+req.Name)
	return c.JSON(http.StatusCreated, toCronJobDTO(created, false))
}

// Update 更新任务（name 不可改；调度变更重算 next_run_at）。
func (h *CronJobHandler) Update(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.store == nil {
		return httpkit.WriteError(c, systemdb.ErrUnavailable)
	}
	if !h.writable {
		return httpkit.WriteError(c, godatabase.ErrWriterUnavailable)
	}
	rid := httpkit.RequestID(c)
	j, err := h.store.GetCronJob(c.Request().Context(), pc.ID, c.Param("jobID"))
	if errors.Is(err, sql.ErrNoRows) {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusNotFound, "cron_job_not_found", "cron job not found", rid))
	}
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var req upsertCronJobBody
	if err := bindCronJobBody(c, &req); err != nil {
		return httpkit.WriteError(c, err)
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
				return httpkit.WriteError(c, err)
			}
			j.NextRunAt = next
		}
		updated, err := h.store.UpdateCronJob(c.Request().Context(), j)
		if err != nil {
			return httpkit.WriteError(c, err)
		}
		h.recordAudit(c, pc.ID, "toggle cron job "+j.Name)
		return c.JSON(http.StatusOK, toCronJobDTO(updated, false))
	}
	err = validateCronJobBody(&req, rid, false)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := h.validateTargetFor(c.Request().Context(), pc.ID, req); err != nil {
		return httpkit.WriteError(c, err)
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
		return httpkit.WriteError(c, err)
	}
	// 禁用时清空排期（NextRunAt 零值=未排期），启用时保留上面重算的值。
	if !j.Enabled {
		j.NextRunAt = time.Time{}
	}
	updated, err := h.store.UpdateCronJob(c.Request().Context(), j)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	// v2：同步群绑定（TagID 为空表示解除绑定，回到单助手语义）。
	if err := h.saveGroupBinding(c.Request().Context(), pc.ID, updated.ID, req); err != nil {
		return httpkit.WriteError(c, err)
	}
	h.recordAudit(c, pc.ID, "update cron job "+j.Name)
	return c.JSON(http.StatusOK, toCronJobDTO(updated, false))
}

// Delete 软删任务。
func (h *CronJobHandler) Delete(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.store == nil {
		return httpkit.WriteError(c, systemdb.ErrUnavailable)
	}
	if !h.writable {
		return httpkit.WriteError(c, godatabase.ErrWriterUnavailable)
	}
	rid := httpkit.RequestID(c)
	j, err := h.store.GetCronJob(c.Request().Context(), pc.ID, c.Param("jobID"))
	if errors.Is(err, sql.ErrNoRows) {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusNotFound, "cron_job_not_found", "cron job not found", rid))
	}
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := h.store.ArchiveCronJob(c.Request().Context(), pc.ID, j.ID); err != nil {
		return httpkit.WriteError(c, err)
	}
	// v2：任务删除后群绑定一并作废（失败只记日志，不阻塞删除）。
	if err := h.store.ArchiveAgentGroupCron(c.Request().Context(), pc.ID, j.ID); err != nil {
		c.Logger().Warnf("archive group binding for job %s failed: %v", j.ID, err)
	}
	h.recordAudit(c, pc.ID, "delete cron job "+j.Name)
	return c.NoContent(http.StatusNoContent)
}

// ListRuns 运行记录（新→旧）。
func (h *CronJobHandler) ListRuns(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.store == nil {
		return httpkit.WriteError(c, systemdb.ErrUnavailable)
	}
	limit := httpkit.QueryLimit(c, 20, 0)
	runs, err := h.store.ListCronJobRuns(c.Request().Context(), pc.ID, c.Param("jobID"), limit)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	out := make([]cronJobRunDTO, 0, len(runs))
	for _, r := range runs {
		out = append(out, toCronJobRunDTO(r))
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": out})
}

// Trigger 手动立即执行（异步，不改排期）。
func (h *CronJobHandler) Trigger(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.store == nil {
		return httpkit.WriteError(c, systemdb.ErrUnavailable)
	}
	rid := httpkit.RequestID(c)
	j, err := h.store.GetCronJob(c.Request().Context(), pc.ID, c.Param("jobID"))
	if errors.Is(err, sql.ErrNoRows) {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusNotFound, "cron_job_not_found", "cron job not found", rid))
	}
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.scheduler == nil {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusServiceUnavailable, "cron_scheduler_unavailable", "cron scheduler is not running", rid))
	}
	// agent 任务会消耗 LLM 额度：手动触发前先检查配额（原 TriggerScheduleRun 的语义）。
	if j.EffectiveTargetKind() == systemdb.CronJobTargetAgent && h.usage != nil {
		if err := h.usage.CheckQuota(c.Request().Context(), pc.ID, "llm"); err != nil {
			return httpkit.WriteError(c, err)
		}
	}
	// 异步执行用的是调度器自己的 ctx（LIFE-01 修复后 Trigger 忽略调用方 ctx），
	// 任务与请求生命周期解耦、但受调度器停止控制。
	if err := h.scheduler.Trigger(context.WithoutCancel(c.Request().Context()), j); err != nil {
		rid := httpkit.RequestID(c)
		if errors.Is(err, cronjob.ErrSchedulerStopped) {
			return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusServiceUnavailable, "cron_scheduler_stopped", "cron scheduler is not running", rid))
		}
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusConflict, "cron_job_running", "cron job is already executing", rid))
	}
	h.recordAudit(c, pc.ID, "trigger cron job "+j.Name)
	return c.JSON(http.StatusAccepted, map[string]any{"triggered": true})
}

// ---- 内部辅助 ----

// bindCronJobBody 解析 body 并做 input_json 长度检查。
func bindCronJobBody(c echo.Context, req *upsertCronJobBody) error {
	if err := c.Bind(req); err != nil {
		return httpkit.BadRequest(c, "cannot parse request body")
	}
	return nil
}

// validateCronJobBody 字段级校验与归一化；create=true 时 name 必填（更新时忽略 name）。
// 依次校验名称、调度、目标（含群绑定选项），并补 enabled 的缺省语义。
func validateCronJobBody(req *upsertCronJobBody, rid string, create bool) error {
	if create && !cronJobNameRe.MatchString(req.Name) {
		return httpkit.NewAPIError(http.StatusBadRequest, "invalid_cron_job_name", "name must start with a letter and contain only letters, digits, _ or -", rid)
	}
	if err := validateCronSchedule(req, rid); err != nil {
		return err
	}
	return validateCronTarget(req, rid)
}

// validateCronSchedule 按 schedule_kind 校验并清掉另外两种调度方式的字段。
func validateCronSchedule(req *upsertCronJobBody, rid string) error {
	switch req.ScheduleKind {
	case systemdb.CronJobKindCron:
		if req.CronExpr == "" {
			return httpkit.NewAPIError(http.StatusBadRequest, "invalid_cron_expression", "cron_expr is required for schedule_kind=cron", rid)
		}
		if _, err := crontab.ParseCron(req.CronExpr); err != nil {
			return httpkit.NewAPIError(http.StatusBadRequest, "invalid_cron_expression", err.Error(), rid)
		}
		req.IntervalSeconds = 0
		req.RunAt = ""
	case systemdb.CronJobKindInterval:
		if req.IntervalSeconds < cronJobIntervalMinSeconds || req.IntervalSeconds > cronJobIntervalMaxSeconds {
			return httpkit.NewAPIError(http.StatusBadRequest, "invalid_interval", "interval_seconds must be between 60 and 2592000", rid)
		}
		req.CronExpr = ""
		req.RunAt = ""
	case systemdb.CronJobKindOnce:
		if req.RunAt == "" {
			return httpkit.NewAPIError(http.StatusBadRequest, "invalid_run_at", "run_at is required for schedule_kind=once", rid)
		}
		t, err := time.Parse(time.RFC3339, req.RunAt)
		if err != nil {
			return httpkit.NewAPIError(http.StatusBadRequest, "invalid_run_at", "run_at must be RFC3339 timestamp", rid)
		}
		req.runAtTime = t.UTC()
		req.CronExpr = ""
		req.IntervalSeconds = 0
	default:
		return httpkit.BadRequestWithID("schedule_kind must be cron, interval or once", rid)
	}
	return nil
}

// validateCronTarget 按 target_kind 校验目标字段、清掉另一侧字段，并补 enabled 的缺省语义：
// enabledValue=false + enabledSet=false 表示「请求未带 enabled」，由调用方保留原值，不隐式翻转。
func validateCronTarget(req *upsertCronJobBody, rid string) error {
	switch req.TargetKind {
	case "", systemdb.CronJobTargetFunction:
		req.TargetKind = systemdb.CronJobTargetFunction
		if req.FuncFile == "" || req.FuncExport == "" {
			return httpkit.BadRequestWithID("func_file and func_export are required", rid)
		}
		if req.InputJSON == "" {
			req.InputJSON = "{}"
		}
		if len(req.InputJSON) > cronJobInputLimit {
			return httpkit.BadRequestWithID("input_json exceeds 8KB limit", rid)
		}
		var probe json.RawMessage
		if err := json.Unmarshal([]byte(req.InputJSON), &probe); err != nil {
			return httpkit.BadRequestWithID("input_json is not valid JSON: "+err.Error(), rid)
		}
		// 清掉另一侧字段，避免脏数据残留。
		req.AgentID, req.AgentPrompt = "", ""
	case systemdb.CronJobTargetAgent:
		if err := validateGroupScheduleOptions(req, rid); err != nil {
			return err
		}
	default:
		return httpkit.NewAPIError(http.StatusBadRequest, "invalid_target_kind", "target_kind must be function or agent", rid)
	}
	req.enabledValue = true
	req.enabledSet = false
	if req.Enabled != nil {
		req.enabledValue = *req.Enabled
		req.enabledSet = true
	}
	return nil
}

// validateGroupScheduleOptions 校验 target_kind=agent 的助手字段与可选的群绑定（tag_id 及其线程选项）。
func validateGroupScheduleOptions(req *upsertCronJobBody, rid string) error {
	req.AgentID = strings.TrimSpace(req.AgentID)
	if req.AgentID == "" {
		return httpkit.BadRequestWithID("agent_id is required for target_kind=agent", rid)
	}
	if strings.TrimSpace(req.AgentPrompt) == "" {
		return httpkit.NewAPIError(http.StatusBadRequest, "agent_prompt_required", "agent_prompt is required for target_kind=agent", rid)
	}
	if len(req.AgentPrompt) > cronJobAgentPromptLimit {
		return httpkit.NewAPIError(http.StatusBadRequest, "agent_prompt_too_long", "agent_prompt exceeds 16KB limit", rid)
	}
	req.FuncFile, req.FuncExport = "", ""
	req.InputJSON = "{}"
	// v2：可选的群绑定字段。
	req.TagID = strings.TrimSpace(req.TagID)
	req.ThreadPolicy = strings.TrimSpace(req.ThreadPolicy)
	req.ThreadKind = strings.TrimSpace(req.ThreadKind)
	req.OwnerUserID = strings.TrimSpace(req.OwnerUserID)
	if req.TagID == "" && (req.ThreadPolicy != "" || req.OwnerUserID != "" || req.ThreadKind != "") {
		return httpkit.BadRequestWithID("tag_id is required for group schedule options", rid)
	}
	if req.TagID == "" {
		return nil
	}
	switch req.ThreadPolicy {
	case "", systemdb.GroupCronReuseThread, systemdb.GroupCronNewThreadPerFire:
	default:
		return httpkit.BadRequestWithID(
			"thread_policy must be reuse_thread or new_thread_per_fire", rid)

	}
	switch req.ThreadKind {
	case "", systemdb.ThreadKindScheduled, systemdb.ThreadKindChat,
		systemdb.ThreadKindAlert, systemdb.ThreadKindStatistics:
	default:
		return httpkit.BadRequestWithID("invalid thread_kind", rid)
	}
	return nil
}

// computeNextRun 按调度模式计算下一次触发时刻。
func computeNextRun(kind, cronExpr string, intervalSeconds int64, runAt time.Time) (time.Time, error) {
	now := time.Now().UTC()
	switch kind {
	case systemdb.CronJobKindCron:
		spec, err := crontab.ParseCron(cronExpr)
		if err != nil {
			return time.Time{}, httpkit.NewAPIError(http.StatusBadRequest, "invalid_cron_expression", err.Error(), "")
		}
		next, err := spec.NextAfter(now)
		if err != nil {
			return time.Time{}, httpkit.NewAPIError(http.StatusBadRequest, "invalid_cron_expression", err.Error(), "")
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
	httpkit.RecordAudit(c, h.audit, "cronjob", projectID, "ok", detail)
}
