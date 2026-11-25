// 文件说明：collab_coordinator.go — 协作群「发起一轮」的入口（POST /agent-threads/:threadID/turns）
//
// 用法
//
//	(h *cloudAgentHandler) CreateTurn(c echo.Context) error   // routes_agent.go；解析 → 入口校验 → agentrun.SubmitTurn → JSON 或 SSE
//	type mentionTargetDTO / createTurnBody                     // 请求体
//
// 调用架构
//
//	[上游] routes_agent.go
//	         │ 同包调用
//	         ▼
//	[本文件] internal/api/collab_coordinator.go
//	         │ 依赖
//	         ▼
//	[下游] internal/agentrun（群轮次用例）, internal/systemdb（读端口）
//	[同包] collab_handler.go, collab_query_handler.go, collab_group_stream.go
//
// 主要符号
//
//	type mentionTargetDTO struct {…}
//	type createTurnBody struct {…}
//	(h *cloudAgentHandler) CreateTurn(c echo.Context) error
//
// 群消息统一经过 Plan 栅栏；执行由 agentrun 持有的 group.Engine 后台推进，SSE 仅订阅持久化事件。

package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// mentionTargetDTO 是结构化 @ 目标：{kind,id}（kind = agent / human）。
type mentionTargetDTO struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// createTurnBody 是 POST /agent-threads/:threadID/turns 的请求体。
type createTurnBody struct {
	Content        string             `json:"content"`
	Mentions       []mentionTargetDTO `json:"mentions"`
	IdempotencyKey string             `json:"idempotency_key"`
	Stream         bool               `json:"stream"`
}

// turnContext 是一次轮次提交的上下文快照。
type turnContext struct {
	pc        ProjectContext
	principal auth.Principal
	thread    systemdb.AgentThread
	ws        systemdb.AgentThreadWorkspace
	collab    systemdb.AgentThreadCollab
	tag       systemdb.AgentTag
	// humanID 是当前可指派的人（API Key 主体为空，表示不能指派）。
	humanID string
}

// CreateTurn 处理 POST /agent-threads/:threadID/turns：解析与入口校验在这里，
// 会话元数据 / 标签 / 表就绪 / 模型 / 配额前置与引擎提交在 agentrun.SubmitTurn。
func (h *cloudAgentHandler) CreateTurn(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	principal := principalOf(c)
	threadID := c.Param("threadID")
	thread, err := h.svc.LookupThread(c.Request().Context(), pc.ID, threadID)
	if err != nil {
		return WriteError(c, err)
	}
	var body createTurnBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	content := strings.TrimSpace(body.Content)
	if content == "" {
		return WriteError(c, agentrun.ErrRunContentRequired)
	}
	mentions := make([]agentrun.MentionTarget, 0, len(body.Mentions))
	for _, mention := range body.Mentions {
		mentions = append(mentions, agentrun.MentionTarget{Kind: mention.Kind, ID: mention.ID})
	}
	turnID, err := h.svc.SubmitTurn(c.Request().Context(), agentrun.TurnInput{
		ProjectID: pc.ID, Thread: thread, Principal: principal,
		Content: content, Mentions: mentions, IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		return WriteError(c, err)
	}
	if body.Stream {
		return h.streamGroupTurn(c, pc.ID, turnID, 0)
	}
	turn, err := h.store.GetAgentTurn(c.Request().Context(), pc.ID, turnID)
	if err != nil {
		return WriteError(c, err)
	}
	dto, err := h.buildTurnDTO(c.Request().Context(), pc.ID, turn)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusAccepted, dto)
}
