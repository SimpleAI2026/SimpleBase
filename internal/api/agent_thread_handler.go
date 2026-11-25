// 文件说明：agent_thread_handler.go — 云助手会话（thread）与消息的 HTTP handler
//
// 用法
//
//	ListThreads、PatchThread、ListThreadRuns（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_thread_handler.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, godatabase, internal/agentbridge, internal/auth 等
//	[同包] adapter.go, api_key_policy.go, api_keys_handler.go, auth_handler.go 等 59 个
//
// 主要符号
//
//	func (h *cloudAgentHandler) ListThreads(c echo.Context) error
//	func (h *cloudAgentHandler) PatchThread(c echo.Context) error
//	func (h *cloudAgentHandler) ListThreadRuns(c echo.Context) error
//	func (h *cloudAgentHandler) ListActiveRuns(c echo.Context) error
//	func (h *cloudAgentHandler) CreateThread(c echo.Context) error
//	func threadKindForTag(tagID string) string
//	func (h *cloudAgentHandler) GetThread(c echo.Context) error
//	... 另有 2 个顶层符号

package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func (h *cloudAgentHandler) ListThreads(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	limit := 50
	if raw := c.QueryParam("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid limit"))
		}
		limit = n
	}
	// agent_id 与 tag_id 同传时取交集；不带时返回全部会话。
	agentID := strings.TrimSpace(c.QueryParam("agent_id"))
	tagID := strings.TrimSpace(c.QueryParam("tag_id"))
	if tagID != "" {
		if _, err := h.resolveAgentTag(c.Request().Context(), pc.ID, tagID); err != nil {
			return WriteError(c, err)
		}
	}
	list, next, err := h.store.PageAgentThreadsFiltered(c.Request().Context(), pc.ID, systemdb.AgentThreadFilter{
		AgentID:    agentID,
		TagID:      tagID,
		ThreadKind: strings.TrimSpace(c.QueryParam("thread_kind")),
	}, c.QueryParam("cursor"), limit)
	if errors.Is(err, systemdb.ErrAgentThreadCursor) {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid cursor"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	// 批量补工作台元数据，避免 N+1（planv6.7 AGC-NAV §7.8）。
	threadIDs := make([]string, 0, len(list))
	for _, t := range list {
		threadIDs = append(threadIDs, t.ID)
	}
	workspaces, err := h.store.ListAgentThreadWorkspaces(c.Request().Context(), pc.ID, threadIDs)
	if err != nil {
		return WriteError(c, err)
	}
	// v2：批量补会话类型（旧会话无行按 chat 处理），同样避免 N+1。
	collabs, err := h.store.ListAgentThreadCollabs(c.Request().Context(), pc.ID, threadIDs)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]threadDTO, 0, len(list))
	for _, t := range list {
		row := toThreadDTOWithWorkspace(t, workspaces[t.ID], collabs[t.ID].ThreadKind)
		row.LastMessagePreview, err = h.store.AgentThreadPreview(c.Request().Context(), pc.ID, t.ID)
		if err != nil {
			return WriteError(c, err)
		}
		out = append(out, row)
	}
	return c.JSON(http.StatusOK, map[string]any{"threads": out, "next_cursor": next})
}

func (h *cloudAgentHandler) PatchThread(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	if !h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	body.Title = strings.TrimSpace(body.Title)
	if n := len([]rune(body.Title)); n < 1 || n > 80 {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "title must be 1-80 characters"))
	}
	th, err := h.store.RenameAgentThread(c.Request().Context(), pc.ID, c.Param("threadID"), body.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	ws, err := h.store.GetAgentThreadWorkspace(c.Request().Context(), pc.ID, th.ID)
	if err != nil {
		return WriteError(c, err)
	}
	collab, err := h.store.GetAgentThreadCollab(c.Request().Context(), pc.ID, th.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toThreadDTOWithWorkspace(th, ws, collab.ThreadKind))
}

func (h *cloudAgentHandler) ListThreadRuns(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	threadID := c.Param("threadID")
	if _, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, threadID); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	limit := 20
	if raw := c.QueryParam("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid limit"))
		}
		limit = n
	}
	list, err := h.store.ListAgentRuns(c.Request().Context(), pc.ID, threadID, limit)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]runDTO, 0, len(list))
	for _, r := range list {
		out = append(out, toRunDTO(r))
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": out})
}

// ListActiveRuns 返回项目内正在执行的 run（交互 + 定时任务），供助手列表 / 会话切换器显示转圈。
// 库里 running 的行还要与运行时内存态核对，过滤掉进程重启后残留的僵尸记录。
func (h *cloudAgentHandler) ListActiveRuns(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]activeRunDTO, 0)
	if h.svc == nil {
		return c.JSON(http.StatusOK, map[string]any{"runs": out})
	}
	list, err := h.store.ListActiveAgentRuns(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	for _, r := range list {
		if !h.svc.IsRunActive(r.ID) {
			continue
		}
		dto := activeRunDTO{RunID: r.ID, AgentID: r.AgentID, ThreadID: r.ThreadID}
		if !r.StartedAt.IsZero() {
			t := r.StartedAt
			dto.StartedAt = &t
		}
		out = append(out, dto)
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": out})
}

func (h *cloudAgentHandler) CreateThread(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	var body struct {
		Title   string `json:"title"`
		AgentID string `json:"agent_id"`
		// planv6.7 AGC-NAV：工作台元数据（三者都不传时不写元数据行，旧请求完全不变）。
		TagID        string `json:"tag_id"`
		DispatchMode string `json:"dispatch_mode"`
		Source       string `json:"source"`
		// planv6.7 v2：会话类型（chat/alert/statistics/scheduled），缺省 chat。
		ThreadKind string `json:"thread_kind"`
	}
	_ = c.Bind(&body)
	// createdBy 统一取 Principal.Actor()（user:<id> / key:<id>）；无主体时为空串。
	createdBy := ""
	principal, hasPrincipal := PrincipalFromContext(c.Request().Context())
	if hasPrincipal {
		createdBy = principal.Actor()
	}
	// agent_id 可选：带上即把会话归属到该助手，后续列表按它过滤。
	agentID := strings.TrimSpace(body.AgentID)
	if agentID != "" {
		if _, err := h.store.GetCloudAgent(c.Request().Context(), pc.ID, agentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
			}
			return WriteError(c, err)
		}
	}
	// planv6.7 AGC-NAV §2.2 + v2 §5：tag_id / dispatch_mode / source / thread_kind 校验。
	tagID := strings.TrimSpace(body.TagID)
	dispatchMode := strings.TrimSpace(body.DispatchMode)
	source := strings.TrimSpace(body.Source)
	threadKind := strings.TrimSpace(body.ThreadKind)
	switch dispatchMode {
	case "", systemdb.DispatchCollaborative:
	default:
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "dispatch_mode must be collaborative"))
	}
	switch source {
	case "", systemdb.ThreadSourceWorkspace, systemdb.ThreadSourceStarter:
	default:
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid source"))
	}
	switch threadKind {
	case "", systemdb.ThreadKindChat, systemdb.ThreadKindAlert,
		systemdb.ThreadKindStatistics, systemdb.ThreadKindScheduled:
	default:
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid thread_kind"))
	}
	// 群会话可以带 tag_id，也可以落在 #对话（空 tag 即通用群）。
	if tagID != "" {
		if dispatchMode == "" {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "tag_id requires dispatch_mode"))
		}
		if _, err := h.resolveAgentTag(c.Request().Context(), pc.ID, tagID); err != nil {
			return WriteError(c, err)
		}
	}
	// 群会话不允许绑定单个助手：成员由群配置决定。
	if dispatchMode == systemdb.DispatchCollaborative && agentID != "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "group thread cannot set agent_id"))
	}
	if dispatchMode != "" && source == "" {
		source = systemdb.ThreadSourceWorkspace
	}
	if dispatchMode == "" && threadKind != "" && threadKind != systemdb.ThreadKindChat {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "thread_kind requires a group dispatch mode"))
	}
	var ws systemdb.AgentThreadWorkspace
	if tagID != "" || dispatchMode != "" || source != "" {
		ws = systemdb.AgentThreadWorkspace{TagID: tagID, DispatchMode: dispatchMode, Source: source}
	}
	th, err := h.store.CreateAgentThreadWithWorkspace(c.Request().Context(), systemdb.AgentThread{
		ProjectID: pc.ID, Title: body.Title, AgentID: agentID, CreatedBy: createdBy,
	}, ws)
	if err != nil {
		return WriteError(c, err)
	}
	// 协作会话写会话级元数据（类型 + 活动轮次占用位）；失败时回滚会话，避免半成品。
	var collab systemdb.AgentThreadCollab
	if dispatchMode != "" {
		if threadKind == "" {
			threadKind = threadKindForTag(tagID)
		}
		collab, err = h.store.CreateAgentThreadCollab(c.Request().Context(), systemdb.AgentThreadCollab{
			ProjectID: pc.ID, ThreadID: th.ID, TagID: tagID, ThreadKind: threadKind,
		})
		if err != nil {
			_ = h.store.ArchiveAgentThread(c.Request().Context(), pc.ID, th.ID)
			return WriteError(c, err)
		}
	}
	return c.JSON(http.StatusCreated, toThreadDTOWithWorkspace(th, ws, collab.ThreadKind))
}

// threadKindForTag 按群推出默认会话类型：告警群=alert、统计群=statistics、
// 定时群=scheduled，其余群=chat。
func threadKindForTag(tagID string) string {
	switch tagID {
	case "":
		return systemdb.ThreadKindChat
	case systemdb.AgentTagAlerts:
		return systemdb.ThreadKindAlert
	case systemdb.AgentTagStats:
		return systemdb.ThreadKindStatistics
	case systemdb.AgentTagScheduled:
		return systemdb.ThreadKindScheduled
	default:
		return systemdb.ThreadKindChat
	}
}

func (h *cloudAgentHandler) GetThread(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	th, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, c.Param("threadID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	ws, err := h.store.GetAgentThreadWorkspace(c.Request().Context(), pc.ID, th.ID)
	if err != nil {
		return WriteError(c, err)
	}
	collab, err := h.store.GetAgentThreadCollab(c.Request().Context(), pc.ID, th.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toThreadDTOWithWorkspace(th, ws, collab.ThreadKind))
}

func (h *cloudAgentHandler) DeleteThread(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	threadID := c.Param("threadID")
	if err := h.store.ArchiveAgentThread(c.Request().Context(), pc.ID, threadID); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	// 释放该 thread 的云沙盒；失败只记日志，不阻塞软删（cloud-agent-sandbox-plan §3.2）。
	if err := h.svc.ReleaseThreadSandbox(c.Request().Context(), pc.ID, threadID); err != nil {
		c.Logger().Warnf("release sandbox for thread %s failed: %v", threadID, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *cloudAgentHandler) ListMessages(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	if _, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, c.Param("threadID")); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	msgs, err := h.store.ListAgentMessages(c.Request().Context(), pc.ID, c.Param("threadID"), 200)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]messageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toMessageDTO(m))
	}
	return c.JSON(http.StatusOK, map[string]any{"messages": out})
}
