// 文件说明：plan_handler.go — 云助手计划模式（planv6.7 cloud-agent-plan-mode）：计划 API 与阶段协调器
//
// 用法
//
//	POST /agent-threads/:threadID/plans        ← {goal, agent_id, idempotency_key, stages?}
//	GET  /agent-threads/:threadID/plans        → {plans:[...]}
//	GET  /agent-threads/:threadID/plans/:planID → {plan, stages}
//	POST /agent-threads/:threadID/plans/:planID/actions ← {action: retry|skip|cancel|takeover, stage?}
//
// 协调器在宿主侧：run 行、消息、审批与 SSE 事件缓存都在 internal/api，
// 阶段推进复用既有 run 管道，不把运行生命周期搬进 gocloudagent。
// 单写实例：进程内单例，version 只做乐观并发。

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// ErrPlanInvalid 表示计划请求不合法（阶段顺序、目标、幂等键等）。
var ErrPlanInvalid = errors.New("agent plan is invalid")

// ErrPlanBusy 表示该线程已有活动计划或运行。
var ErrPlanBusy = errors.New("agent plan is busy")

type planStageBody struct {
	ID         string   `json:"id"`
	Goal       string   `json:"goal"`
	SkipReason string   `json:"skip_reason"`
	DependsOn  []string `json:"depends_on"`
}

type createPlanBody struct {
	Goal           string          `json:"goal"`
	AgentID        string          `json:"agent_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Stages         []planStageBody `json:"stages"`
}

type planStageDTO struct {
	Stage      string `json:"stage"`
	Status     string `json:"status"`
	Attempt    int    `json:"attempt"`
	RunID      string `json:"run_id,omitempty"`
	Goal       string `json:"goal,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type planDTO struct {
	ID           string         `json:"id"`
	ThreadID     string         `json:"thread_id"`
	AgentID      string         `json:"agent_id"`
	Goal         string         `json:"goal"`
	Status       string         `json:"status"`
	CurrentStage string         `json:"current_stage,omitempty"`
	Version      int64          `json:"version"`
	Stages       []planStageDTO `json:"stages"`
}

func toPlanDTO(p systemdb.AgentPlan, stages []systemdb.AgentPlanStage) planDTO {
	out := planDTO{ID: p.ID, ThreadID: p.ThreadID, AgentID: p.AgentID, Goal: p.Goal,
		Status: p.Status, CurrentStage: p.CurrentStage, Version: p.Version}
	out.Stages = make([]planStageDTO, 0, len(stages))
	for _, st := range stages {
		out.Stages = append(out.Stages, planStageDTO{Stage: st.Stage, Status: st.Status, Attempt: st.Attempt,
			RunID: st.RunID, Goal: st.Goal, SkipReason: st.SkipReason, Detail: st.Detail})
	}
	return out
}

// planStagesFromBody 把请求里的阶段换算成模块契约并校验；缺省时六阶段全部执行。
func planStagesFromBody(goal string, body []planStageBody) ([]agentbridge.PlanStageSpec, error) {
	order := agentbridge.PlanStageOrder()
	if len(body) == 0 {
		out := make([]agentbridge.PlanStageSpec, 0, len(order))
		for _, id := range order {
			out = append(out, agentbridge.PlanStageSpec{ID: id, Goal: goal})
		}
		return out, nil
	}
	if len(body) != len(order) {
		return nil, ErrPlanInvalid
	}
	out := make([]agentbridge.PlanStageSpec, 0, len(order))
	for i, raw := range body {
		spec := agentbridge.PlanStageSpec{ID: agentbridge.PlanStageID(strings.TrimSpace(raw.ID)),
			Goal: raw.Goal, SkipReason: raw.SkipReason}
		for _, dep := range raw.DependsOn {
			spec.DependsOn = append(spec.DependsOn, agentbridge.PlanStageID(strings.TrimSpace(dep)))
		}
		if spec.ID != order[i] {
			return nil, ErrPlanInvalid
		}
		out = append(out, spec)
	}
	if err := agentbridge.ValidatePlanStages(out); err != nil {
		return nil, ErrPlanInvalid
	}
	return out, nil
}

// CreatePlan 创建计划并启动第一阶段。
func (h *cloudAgentHandler) CreatePlan(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	threadID := c.Param("threadID")
	thread, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, threadID)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	var body createPlanBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	goal := strings.TrimSpace(body.Goal)
	if goal == "" {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "goal is required", RequestIDFromContext(c.Request().Context())))
	}
	if err := h.ensureAgents(c.Request().Context(), pc.ID); err != nil {
		return WriteError(c, err)
	}
	agentID := strings.TrimSpace(body.AgentID)
	if agentID == "" {
		agentID = thread.AgentID
	}
	// planv6.7 v2 §9.4：Plan 与协作轮次互斥。
	//
	// 协作会话（含读时统一的旧 auto 会话）一轮可能产生多个任务与人工待办，
	// 而 Plan 是「单助手六阶段串行」，共用会话占用会互相打断。
	// 定位到单个助手的旧固定会话仍可用 Plan。
	ws, wsErr := h.store.GetAgentThreadWorkspace(c.Request().Context(), pc.ID, threadID)
	if wsErr != nil {
		return WriteError(c, wsErr)
	}
	if isGroupDispatch(ws.DispatchMode) {
		return WriteError(c, NewAPIError(http.StatusConflict, "plan_requires_single_agent_thread",
			"plan mode is not available in group threads; start a new thread for planning",
			RequestIDFromContext(c.Request().Context())))
	}
	agent, err := h.store.GetCloudAgent(c.Request().Context(), pc.ID, agentID)
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "agent not found"))
	}
	if thread.AgentID != "" && thread.AgentID != agent.ID {
		return WriteError(c, NewAPIError(http.StatusConflict, "agent_switch_required", "switch to the mentioned agent before planning", RequestIDFromContext(c.Request().Context())))
	}
	if !h.skillsCLI() {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "cli_skills_unavailable", "plan mode requires CLI skills", RequestIDFromContext(c.Request().Context())))
	}
	specs, err := planStagesFromBody(goal, body.Stages)
	if err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "plan_invalid", "invalid plan stages", RequestIDFromContext(c.Request().Context())))
	}
	// 幂等优先：同线程同幂等键已存在时直接返回既有计划，不受「已有活动计划」限制。
	if key := strings.TrimSpace(body.IdempotencyKey); key != "" {
		if existing, err := h.store.FindAgentPlanByKey(c.Request().Context(), threadID, key); err == nil {
			stages, err := h.store.ListAgentPlanStages(c.Request().Context(), existing.ID)
			if err != nil {
				return WriteError(c, err)
			}
			return c.JSON(http.StatusOK, toPlanDTO(existing, stages))
		}
	}
	// 同一线程只允许一个活动计划：与普通 run 共用线程锁语义。
	if active, err := h.activePlan(c.Request().Context(), pc.ID, threadID); err != nil {
		return WriteError(c, err)
	} else if active != nil {
		return WriteError(c, NewAPIError(http.StatusConflict, "plan_busy", "this thread already has an active plan", RequestIDFromContext(c.Request().Context())))
	}
	stages := make([]systemdb.AgentPlanStage, 0, len(specs))
	for _, spec := range specs {
		status := systemdb.AgentPlanStagePending
		if strings.TrimSpace(spec.SkipReason) != "" {
			status = systemdb.AgentPlanStageSkipped
		}
		stages = append(stages, systemdb.AgentPlanStage{Stage: string(spec.ID), Status: status,
			Goal: spec.Goal, SkipReason: spec.SkipReason})
	}
	plan, err := h.store.CreateAgentPlan(c.Request().Context(), systemdb.AgentPlan{
		ProjectID: pc.ID, ThreadID: threadID, AgentID: agent.ID, Goal: goal,
		Status: systemdb.AgentPlanQueued, IdempotencyKey: strings.TrimSpace(body.IdempotencyKey),
		CreatedBy: principalIdentity(principal),
	}, stages)
	if err != nil {
		return WriteError(c, err)
	}
	saved, err := h.store.ListAgentPlanStages(c.Request().Context(), plan.ID)
	if err != nil {
		return WriteError(c, err)
	}
	// 幂等命中：已有计划直接返回，不重复启动。
	if plan.Status != systemdb.AgentPlanQueued {
		return c.JSON(http.StatusOK, toPlanDTO(plan, saved))
	}
	go h.runPlan(plan.ID, pc.ID, threadID, agent.ID, principal)
	return c.JSON(http.StatusAccepted, toPlanDTO(plan, saved))
}

// ListPlans 列出线程的计划。
func (h *cloudAgentHandler) ListPlans(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	plans, err := h.store.ListAgentPlans(c.Request().Context(), pc.ID, c.Param("threadID"), 20)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]planDTO, 0, len(plans))
	for _, p := range plans {
		stages, err := h.store.ListAgentPlanStages(c.Request().Context(), p.ID)
		if err != nil {
			return WriteError(c, err)
		}
		out = append(out, toPlanDTO(p, stages))
	}
	return c.JSON(http.StatusOK, map[string]any{"plans": out})
}

// GetPlan 返回计划与阶段快照。
func (h *cloudAgentHandler) GetPlan(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	plan, err := h.store.GetAgentPlan(c.Request().Context(), pc.ID, c.Param("planID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "plan not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	if plan.ThreadID != c.Param("threadID") {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "plan not found"))
	}
	stages, err := h.store.ListAgentPlanStages(c.Request().Context(), plan.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toPlanDTO(plan, stages))
}

// PlanEvents 推送完整快照，重连时始终先发送当前状态。
func (h *cloudAgentHandler) PlanEvents(c echo.Context) error {
	ctx := c.Request().Context()
	pc, ok := ProjectFromContext(ctx)
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	load := func() (planDTO, error) {
		p, err := h.store.GetAgentPlan(ctx, pc.ID, c.Param("planID"))
		if err != nil {
			return planDTO{}, err
		}
		if p.ThreadID != c.Param("threadID") {
			return planDTO{}, sql.ErrNoRows
		}
		stages, err := h.store.ListAgentPlanStages(ctx, p.ID)
		return toPlanDTO(p, stages), err
	}
	p, err := load()
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "plan not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	r := c.Response()
	// 与 ReplayEvents 一致：长连接必须解除 http.Server.WriteTimeout（默认 30s 绝对写 deadline）。
	// 计划通常运行数分钟，不解除的话连接会在 30s 到点被服务端切断，前端订阅静默结束，
	// 后续阶段不再推送快照（对话区既无流式输出也无 loading）。
	beginSSE(c)
	r.Header().Set(echo.HeaderContentType, "text/event-stream")
	r.Header().Set(echo.HeaderCacheControl, "no-cache")
	r.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last string
	for {
		raw, err := json.Marshal(p)
		if err != nil {
			return err
		}
		if string(raw) != last {
			last = string(raw)
			_, err = fmt.Fprintf(r, "event: snapshot\ndata: %s\n\n", raw)
		} else {
			_, err = fmt.Fprint(r, ": heartbeat\n\n")
		}
		if err != nil {
			return nil
		}
		r.Flush()
		switch p.Status {
		case systemdb.AgentPlanCompleted, systemdb.AgentPlanCanceled:
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			p, err = load()
			if err != nil {
				return nil
			}
		}
	}
}

// PlanAction 处理 retry / skip / cancel / takeover。
func (h *cloudAgentHandler) PlanAction(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	plan, err := h.store.GetAgentPlan(c.Request().Context(), pc.ID, c.Param("planID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "plan not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	if plan.ThreadID != c.Param("threadID") {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "plan not found"))
	}
	var body struct {
		Action string `json:"action"`
		Stage  string `json:"stage"`
		// ExpectedVersion 非 0 时按客户端看到的版本做 CAS，避免覆盖他人推进。
		ExpectedVersion int64 `json:"expected_version"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid action body", RequestIDFromContext(c.Request().Context())))
	}
	rid := RequestIDFromContext(c.Request().Context())
	version := plan.Version
	if body.ExpectedVersion != 0 && body.ExpectedVersion != version {
		return WriteError(c, planVersionError(systemdb.ErrAgentPlanVersion, rid))
	}
	if !canCancelConfirm(principal, plan.CreatedBy) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "forbidden", "cannot control this plan", rid))
	}
	switch strings.TrimSpace(body.Action) {
	case "cancel", "takeover":
		stages, err := h.store.ListAgentPlanStages(c.Request().Context(), plan.ID)
		if err != nil {
			return WriteError(c, err)
		}
		if err := h.store.UpdateAgentPlanStatus(c.Request().Context(), pc.ID, plan.ID, systemdb.AgentPlanCanceled, plan.CurrentStage, version); err != nil {
			return WriteError(c, planVersionError(err, rid))
		}
		for _, stage := range stages {
			if stage.Status == systemdb.AgentPlanStageRunning {
				h.runtime.CancelRun(stage.RunID)
			}
		}
	case "retry", "skip":
		stage := strings.TrimSpace(body.Stage)
		if stage == "" {
			stage = plan.CurrentStage
		}
		stages, err := h.store.ListAgentPlanStages(c.Request().Context(), plan.ID)
		if err != nil {
			return WriteError(c, err)
		}
		var target *systemdb.AgentPlanStage
		for i := range stages {
			if stages[i].Stage == stage {
				target = &stages[i]
				break
			}
		}
		if target == nil {
			return WriteError(c, NewAPIError(http.StatusBadRequest, "plan_invalid", "unknown stage", rid))
		}
		if body.Action == "skip" {
			target.Status = systemdb.AgentPlanStageSkipped
			target.Detail = "skipped by user"
			if err := h.store.UpdateAgentPlanStage(c.Request().Context(), *target); err != nil {
				return WriteError(c, err)
			}
		} else {
			target.Status = systemdb.AgentPlanStagePending
			target.Detail = ""
			if err := h.store.UpdateAgentPlanStage(c.Request().Context(), *target); err != nil {
				return WriteError(c, err)
			}
		}
		if err := h.store.UpdateAgentPlanStatus(c.Request().Context(), pc.ID, plan.ID, systemdb.AgentPlanRunning, stage, version); err != nil {
			return WriteError(c, planVersionError(err, rid))
		}
		go h.runPlan(plan.ID, pc.ID, plan.ThreadID, plan.AgentID, principal)
	default:
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "action must be retry, skip, cancel or takeover", rid))
	}
	updated, err := h.store.GetAgentPlan(c.Request().Context(), pc.ID, plan.ID)
	if err != nil {
		return WriteError(c, err)
	}
	stages, err := h.store.ListAgentPlanStages(c.Request().Context(), plan.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toPlanDTO(updated, stages))
}

func planVersionError(err error, rid string) error {
	if errors.Is(err, systemdb.ErrAgentPlanVersion) {
		return NewAPIError(http.StatusConflict, "plan_version_conflict", "plan changed, reload it", rid)
	}
	return err
}

// activePlan 返回线程当前活动计划（queued / running / paused）。
func (h *cloudAgentHandler) activePlan(ctx context.Context, projectID, threadID string) (*systemdb.AgentPlan, error) {
	plans, err := h.store.ListAgentPlans(ctx, projectID, threadID, 20)
	if err != nil {
		return nil, err
	}
	for i := range plans {
		switch plans[i].Status {
		case systemdb.AgentPlanQueued, systemdb.AgentPlanRunning, systemdb.AgentPlanPaused:
			return &plans[i], nil
		}
	}
	return nil, nil
}

// planMu 串行化同一计划的推进，避免 retry 与自动推进并发。
var planMu sync.Map

func planLock(planID string) func() {
	v, _ := planMu.LoadOrStore(planID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// runPlan 从当前阶段起推进计划，直到完成、暂停或取消。
func (h *cloudAgentHandler) runPlan(planID, projectID, threadID, agentID string, principal auth.Principal) {
	unlock := planLock(planID)
	defer unlock()
	ctx := context.Background()
	for {
		plan, err := h.store.GetAgentPlan(ctx, projectID, planID)
		if err != nil || plan.Status == systemdb.AgentPlanCanceled || plan.Status == systemdb.AgentPlanInterrupted {
			return
		}
		stages, err := h.store.ListAgentPlanStages(ctx, planID)
		if err != nil {
			return
		}
		next := nextPlanStage(stages)
		if next == nil {
			_ = h.store.UpdateAgentPlanStatus(ctx, projectID, planID, systemdb.AgentPlanCompleted, "", plan.Version)
			return
		}
		if next.Status == systemdb.AgentPlanStageSkipped {
			continue
		}
		if err := h.store.UpdateAgentPlanStatus(ctx, projectID, planID, systemdb.AgentPlanRunning, next.Stage, plan.Version); err != nil {
			return
		}
		if next.Stage != string(agentbridge.PlanStagePlanning) && stages[0].Status == systemdb.AgentPlanStageDone {
			var proposal []agentbridge.PlanStageSpec
			if json.Unmarshal([]byte(stages[0].Detail), &proposal) != nil || agentbridge.ValidatePlanStages(proposal) != nil {
				_ = h.store.UpdateAgentPlanStatus(ctx, projectID, planID, systemdb.AgentPlanPaused, next.Stage, plan.Version+1)
				return
			}
			for _, spec := range proposal {
				if string(spec.ID) != next.Stage {
					continue
				}
				next.Goal, next.SkipReason = spec.Goal, spec.SkipReason
				if spec.SkipReason != "" {
					next.Status = systemdb.AgentPlanStageSkipped
				} else {
					for _, dep := range spec.DependsOn {
						for _, previous := range stages {
							if previous.Stage == string(dep) && previous.Status != systemdb.AgentPlanStageDone {
								next.Status = systemdb.AgentPlanStageBlocked
								next.Detail = "dependency_not_completed: " + string(dep)
							}
						}
					}
				}
			}
			if next.Status == systemdb.AgentPlanStageSkipped || next.Status == systemdb.AgentPlanStageBlocked {
				if h.store.UpdateAgentPlanStage(ctx, *next) != nil || next.Status == systemdb.AgentPlanStageBlocked {
					_ = h.store.UpdateAgentPlanStatus(ctx, projectID, planID, systemdb.AgentPlanPaused, next.Stage, plan.Version+1)
					return
				}
				continue
			}
		}
		ok := h.runPlanStage(ctx, projectID, threadID, agentID, planID, *next, principal)
		if !ok {
			latest, err := h.store.GetAgentPlan(ctx, projectID, planID)
			if err == nil {
				if latest.Status == systemdb.AgentPlanRunning {
					_ = h.store.UpdateAgentPlanStatus(ctx, projectID, planID, systemdb.AgentPlanPaused, next.Stage, latest.Version)
				}
			}
			return
		}
	}
}

// nextPlanStage 返回第一个未完成阶段；全部完成返回 nil。
func nextPlanStage(stages []systemdb.AgentPlanStage) *systemdb.AgentPlanStage {
	for i := range stages {
		switch stages[i].Status {
		case systemdb.AgentPlanStageDone, systemdb.AgentPlanStageSkipped:
			continue
		default:
			return &stages[i]
		}
	}
	return nil
}

// runPlanStage 执行一个阶段 run，返回是否通过。
func (h *cloudAgentHandler) runPlanStage(ctx context.Context, projectID, threadID, agentID, planID string, stage systemdb.AgentPlanStage, principal auth.Principal) bool {
	agent, err := h.store.GetCloudAgent(ctx, projectID, agentID)
	if err != nil {
		return false
	}
	stageID := agentbridge.PlanStageID(stage.Stage)
	skills, readonly, ok := agentbridge.PlanStageSkills(stageID)
	if !ok {
		return false
	}
	claimID := uuid.NewString()
	if err := h.runtime.ClaimThread(threadID, claimID); err != nil {
		return false
	}
	defer h.runtime.ReleaseThread(threadID, claimID)

	stage.Attempt++
	run, err := h.store.CreateAgentRun(ctx, systemdb.AgentRun{
		ID: claimID, ThreadID: threadID, ProjectID: projectID, AgentID: agent.ID,
		Status: systemdb.AgentRunRunning, StartedAt: time.Now().UTC(), InitiatorID: principalIdentity(principal),
	})
	if err != nil {
		return false
	}
	text := planStageMessage(stageID, stage.Goal)
	mentions, _ := json.Marshal([]mentionDTO{{AgentID: agent.ID}})
	userMsg, err := h.store.AppendAgentMessage(ctx, systemdb.AgentMessage{
		ThreadID: threadID, ProjectID: projectID, Role: "user", Content: text,
		AgentID: agent.ID, MentionsJSON: string(mentions), RunID: run.ID,
	})
	if err != nil {
		return false
	}
	// run 行与阶段消息落库后再标记阶段运行中：前端看到 running 时即可续接这条 run 的事件流。
	stage.Status = systemdb.AgentPlanStageRunning
	stage.RunID = run.ID
	if err := h.store.UpdateAgentPlanStage(ctx, stage); err != nil {
		return false
	}
	hist, err := h.store.ListAgentMessages(ctx, projectID, threadID, 40)
	if err != nil {
		return false
	}
	filtered := hist[:0]
	for _, m := range hist {
		if m.ID == userMsg.ID {
			continue
		}
		filtered = append(filtered, m)
	}
	// 写能力恒为可写（v2 §3.4），无需逐次提升。
	runAgent := agentbridge.Agent(agent)
	req := cloudagent.RunRequest{
		Agent: runAgent, ThreadID: threadID, RunID: run.ID, UserText: text,
		History: agentbridge.Messages(filtered), Stream: true, Subject: principalIdentity(principal),
		Context: agentbridge.RunContext{
			ProjectID: projectID, Principal: principal, Skills: skills,
			ReadOnlySkills: readonly, PlanStage: stageID,
		},
	}
	runCtx := llmgateway.WithTraceMeta(context.WithoutCancel(ctx), run.ID, "agent")
	res, runErr := h.runtime.StartRun(runCtx, req, func(ev cloudagent.Event) {
		h.recordApprovalEvent(runCtx, projectID, run, agent, ev)
		if h.events != nil {
			if ev.RunID == "" {
				ev.RunID = run.ID
			}
			h.events.append(run.ID, ev)
		}
	})
	if err := h.finishAgentRun(context.Background(), projectID, run, agent, res, runErr); err != nil {
		return false
	}
	if runErr != nil {
		stage.Status = systemdb.AgentPlanStageFailed
		stage.Detail = agentbridge.ClassifyError(runErr).Code
		_ = h.store.UpdateAgentPlanStage(ctx, stage)
		return false
	}
	passed := agentbridge.PlanStageDone(stageID, res.ToolCallsJSON)
	if stageID == agentbridge.PlanStagePlanning {
		var proposal []agentbridge.PlanStageSpec
		passed = json.Unmarshal([]byte(res.Content), &proposal) == nil && agentbridge.ValidatePlanStages(proposal) == nil
		if passed {
			stage.Status = systemdb.AgentPlanStageDone
			stage.Detail = res.Content
			return h.store.UpdateAgentPlanStage(ctx, stage) == nil
		}
	}
	if !passed {
		stage.Status = systemdb.AgentPlanStageFailed
		stage.Detail = "stage_evidence_missing"
		_ = h.store.UpdateAgentPlanStage(ctx, stage)
		return false
	}
	stage.Status = systemdb.AgentPlanStageDone
	stage.Detail = ""
	_ = h.store.UpdateAgentPlanStage(ctx, stage)
	return true
}

// planStageMessage 是阶段 run 的用户消息：只说明「现在做哪一步」。
func planStageMessage(stage agentbridge.PlanStageID, goal string) string {
	label := map[agentbridge.PlanStageID]string{
		agentbridge.PlanStagePlanning:  "规划",
		agentbridge.PlanStageDatabase:  "数据库",
		agentbridge.PlanStageKV:        "KV 存储",
		agentbridge.PlanStageFunctions: "云函数",
		agentbridge.PlanStageCron:      "定时任务",
		agentbridge.PlanStageSite:      "前端站点",
	}[stage]
	msg := "开始「" + label + "」阶段。"
	if strings.TrimSpace(goal) != "" {
		msg += "本阶段目标：" + strings.TrimSpace(goal)
	}
	return msg
}
