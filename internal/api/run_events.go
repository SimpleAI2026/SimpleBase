// 文件说明：run_events.go — run 的确认放行与事件续传入口（SSE 编码）
//
// 用法
//
//	(h *cloudAgentHandler) ConfirmRun(c echo.Context) error     // POST /agent-runs/:runID/confirmations：放行 + 持久审批收敛
//	(h *cloudAgentHandler) ReplayEvents(c echo.Context) error   // GET /agent-runs/:runID/events?after=：回放 + 实时续传
//
// 调用架构
//
//	[上游] routes_agent.go
//	         │ 同包调用
//	         ▼
//	[本文件] internal/api/run_events.go
//	         │ 依赖
//	         ▼
//	[下游] internal/agentrun（事件环 / 确认表）, gocloudagent/cloudagent（事件类型）
//	[同包] sse.go, agent_approval_handler.go
//
// 主要符号
//
//	(h *cloudAgentHandler) ConfirmRun(c echo.Context) error
//	(h *cloudAgentHandler) ReplayEvents(c echo.Context) error
//
// 事件环与确认表在 internal/agentrun；这里只做主体校验与 SSE 帧编码。

package api

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
)

func (h *cloudAgentHandler) ConfirmRun(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	// SEC-04：确认放行的是工具执行（可能写库），属于破坏性操作；
	// 只读主体拒绝，与 CancelRun 同一判定。
	p, hasPrincipal := PrincipalFromContext(c.Request().Context())
	if hasPrincipal && !h.canWritePrincipal(p) {
		return WriteError(c, echo.NewHTTPError(http.StatusForbidden, "write permission required"))
	}
	runID := c.Param("runID")
	callID := c.Param("callID")
	run, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, runID)
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	// planv6.0 D5：确认与取消同一发起人判定；非发起人的同项目写主体拒绝，
	// 防止「项目内互相放行破坏性工具」。
	if hasPrincipal && !canCancelConfirm(p, run.InitiatorID) {
		return WriteError(c, echo.NewHTTPError(http.StatusForbidden, "only the initiator or a project admin can confirm this run"))
	}
	var body struct {
		Approve bool `json:"approve"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	// 等待表放行 + 持久审批收敛在 agentrun.ResolveConfirm（放行失败 → 404 confirmation not found）。
	if err := h.svc.ResolveConfirm(c.Request().Context(), pc.ID, runID, callID, body.Approve, p.Actor()); err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "approve": body.Approve})
}

func (h *cloudAgentHandler) ReplayEvents(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	runID := c.Param("runID")
	if _, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, runID); err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	after, _ := strconv.ParseInt(c.QueryParam("after"), 10, 64)
	if h.svc == nil {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	frames, gap, live, sub := h.svc.ReplayRunEvents(runID, after)
	defer sub.Close()
	// planv6.0 v2 §11：续传同样是长连接，必须解除 WriteTimeout。
	beginSSE(c)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	write := func(id int64, ev cloudagent.Event) {
		writeSSEFrame(c.Response().Writer, flusher, id, ev)
	}
	if gap {
		write(0, cloudagent.Event{Type: "error", Code: "stream_gap", Message: "事件已过期，请刷新消息", RunID: runID})
		return nil
	}
	for _, f := range frames {
		write(f.ID, f.Event)
	}
	if !live || sub == nil {
		return nil
	}
	// BUG-B2（planv6.0 v2 §2.2）：sub 容量 16，append 满时静默丢帧。
	// 帧 id 由 hub 单调分配，跳号即代表丢帧，必须让客户端知道要去刷新消息，
	// 否则前端会拿一段有缺口的事件序列当完整流。
	// 期望值：有补发帧时是末帧 id，否则是客户端声明的 after。
	last := after
	if len(frames) > 0 {
		last = frames[len(frames)-1].ID
	}
	ctx := c.Request().Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case f := <-sub.Chan():
			if f.ID != last+1 {
				write(f.ID, cloudagent.Event{Type: "error", Code: "stream_gap", Message: "事件已过期，请刷新消息", RunID: runID})
				return nil
			}
			last = f.ID
			write(f.ID, f.Event)
			if f.Event.Type == "end" {
				return nil
			}
		}
	}
}
