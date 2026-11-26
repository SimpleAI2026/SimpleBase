package agenthttp

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

func (h *CloudAgentHandler) ConfirmRun(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	// SEC-04：确认放行的是工具执行（可能写库），属于破坏性操作；
	// 只读主体拒绝，与 CancelRun 同一判定。
	p, hasPrincipal := httpkit.PrincipalFromContext(c.Request().Context())
	if hasPrincipal && !h.canWritePrincipal(p) {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusForbidden, "write permission required"))
	}
	runID := c.Param("runID")
	callID := c.Param("callID")
	run, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, runID)
	if err != nil {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	// planv6.0 D5：确认与取消同一发起人判定；非发起人的同项目写主体拒绝，
	// 防止「项目内互相放行破坏性工具」。
	if hasPrincipal && !canCancelConfirm(p, run.InitiatorID) {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusForbidden, "only the initiator or a project admin can confirm this run"))
	}
	var body struct {
		Approve bool `json:"approve"`
	}
	if err := c.Bind(&body); err != nil {
		return httpkit.WriteError(c, err)
	}
	// 等待表放行 + 持久审批收敛在 agentrun.ResolveConfirm（放行失败 → 404 confirmation not found）。
	if err := h.svc.ResolveConfirm(c.Request().Context(), pc.ID, runID, callID, body.Approve, p.Actor()); err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "approve": body.Approve})
}

func (h *CloudAgentHandler) ReplayEvents(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	runID := c.Param("runID")
	if _, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, runID); err != nil {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	after, _ := strconv.ParseInt(c.QueryParam("after"), 10, 64)
	if h.svc == nil {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	frames, gap, live, sub := h.svc.ReplayRunEvents(runID, after)
	defer sub.Close()
	// planv6.0 v2 §11：续传同样是长连接，必须解除 WriteTimeout。
	httpkit.BeginSSE(c)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	write := func(id int64, ev cloudagent.Event) {
		httpkit.WriteSSEFrame(c.Response().Writer, flusher, id, ev)
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
