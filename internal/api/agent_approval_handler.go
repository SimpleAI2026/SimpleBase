// 文件说明：agent_approval_handler.go — 云助手审批列表与运行取消
//
// 用法
//
//	ListApprovals、CancelRun（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_approval_handler.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, godatabase, internal/agentbridge, internal/auth 等
//	[同包] adapter.go, api_key_policy.go, api_keys_handler.go, auth_handler.go 等 59 个
//
// 主要符号
//
//	func (h *cloudAgentHandler) confirmTimeout() time.Duration
//	func (h *cloudAgentHandler) ListApprovals(c echo.Context) error
//	func (h *cloudAgentHandler) CancelRun(c echo.Context) error
//	func canCancelConfirm(p auth.Principal, initiatorID string) bool

package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// ListApprovals 列出项目审批队列。status 为空表示全部；查询前先收敛过期项。
func (h *cloudAgentHandler) ListApprovals(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	ctx := c.Request().Context()
	// 惰性收敛：不引入后台 goroutine，列表查询顺带把超时项标成 expired。
	_, _ = h.store.ExpireAgentApprovals(ctx, pc.ID)
	status := strings.TrimSpace(c.QueryParam("status"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	rows, err := h.store.ListAgentApprovals(ctx, pc.ID, status, limit)
	if err != nil {
		return WriteError(c, err)
	}
	pending, err := h.store.CountPendingAgentApprovals(ctx, pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]approvalDTO, 0, len(rows))
	for _, a := range rows {
		out = append(out, toApprovalDTO(a))
	}
	return c.JSON(http.StatusOK, map[string]any{"approvals": out, "pending": pending})
}

func (h *cloudAgentHandler) CancelRun(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	// SEC-04：取消终止的是他人的运行，属于破坏性操作；只读主体（登录态
	// 只读角色或仅持 DatabaseRead 的 Key）拒绝，写主体允许。
	p, hasPrincipal := PrincipalFromContext(c.Request().Context())
	if hasPrincipal && !h.canWritePrincipal(p) {
		return WriteError(c, echo.NewHTTPError(http.StatusForbidden, "write permission required"))
	}
	runID := c.Param("runID")
	run, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	// planv6.0 D5：发起人校验。仅发起人本人、project:admin、或历史数据
	//（initiator_id 为空）可取消；同项目其他写主体拒绝。
	if hasPrincipal && !canCancelConfirm(p, run.InitiatorID) {
		return WriteError(c, echo.NewHTTPError(http.StatusForbidden, "only the initiator or a project admin can cancel this run"))
	}
	run, err = h.svc.CancelRun(c.Request().Context(), pc.ID, runID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toRunDTO(run))
}

// canCancelConfirm 判断主体能否确认/取消 run（planv6.0 D5）：
// 发起人本人、project:admin，或历史数据（initiator_id 为空，仍要求写权限）。
func canCancelConfirm(p auth.Principal, initiatorID string) bool {
	if initiatorID == "" {
		return true
	}
	if initiatorID == p.Actor() {
		return true
	}
	return p.HasPermission(auth.ProjectAdmin)
}
