// 文件说明：agent_group_handler.go — 群成员管理与告警/统计输入入口（planv6.7 v2 §4.3 / §11.5）。
//
// 路由（router.go，mountAgentRoutes）
//
//	POST   /agent-tags/:tagID/members             加入已存在助手，或「新建助手并加入」
//	DELETE /agent-tags/:tagID/members/:agentID    移出成员（不删除助手）
//	POST   /agent-tags/:tagID/events              告警 / 统计输入入口（按 external_id 幂等）
//
// 成员关系的唯一权威是 sys_agent_workspace_tags.config_json.agent_ids；
// 助手是项目资源，可同时属于多个群，移出群不影响助手的其他关系。
package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// parseInt64Query 解析可选的 int64 query 参数（非法值按 0 处理）。
func parseInt64Query(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}

// groupMembersBody 是 POST /agent-tags/:tagID/members 的请求体。
//
// 两种用法（互斥，同时传以 new_agent 优先并只建一个助手）：
//
//	{"agent_ids":["a1","a2"]}                       加入已有助手
//	{"new_agent":{"name":"告警分析","module":"logs"}} 新建助手并加入
type groupMembersBody struct {
	AgentIDs        []string       `json:"agent_ids"`
	NewAgent        *groupNewAgent `json:"new_agent"`
	ExpectedVersion int64          `json:"expected_version"`
}

// groupNewAgent 是「新建助手并加入」的入参（字段与 /agents 的创建体保持同构）。
type groupNewAgent struct {
	Name          string   `json:"name"`
	Module        string   `json:"module"`
	Description   string   `json:"description"`
	SystemPrompt  string   `json:"system_prompt"`
	ToolIDs       []string `json:"tool_ids"`
	ModelOverride *string  `json:"model_override"`
}

// AddGroupMembers 加入成员 / 新建助手并加入（POST /agent-tags/:tagID/members）。
//
// 组合事务语义：新建助手与写成员关系要么都成功，要么都不留孤儿助手。
// DuckLake 无跨表事务保证，因此失败时显式归档刚创建的助手作为补偿。
func (h *cloudAgentHandler) AddGroupMembers(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	ctx := c.Request().Context()
	tagID := strings.TrimSpace(c.Param("tagID"))
	current, err := h.resolveAgentTag(ctx, pc.ID, tagID)
	if err != nil {
		return WriteError(c, err)
	}
	var body groupMembersBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	// 待加入的助手 ID（已有助手）。
	addIDs := make([]string, 0, len(body.AgentIDs)+1)
	for _, id := range body.AgentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := h.store.GetCloudAgent(ctx, pc.ID, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "agent not found: "+id))
			}
			return WriteError(c, err)
		}
		addIDs = append(addIDs, id)
	}
	// 新建助手并加入：与成员写入配对补偿。
	var createdAgentID string
	if body.NewAgent != nil {
		principal, _ := PrincipalFromContext(ctx)
		created, err := h.validateAgentCapabilities(pc.ID, upsertAgentBody{
			Name: body.NewAgent.Name, Module: body.NewAgent.Module,
			Description: body.NewAgent.Description, SystemPrompt: body.NewAgent.SystemPrompt,
			ToolIDs: body.NewAgent.ToolIDs, ModelOverride: body.NewAgent.ModelOverride,
		}, systemdb.CloudAgent{}, principal)
		if err != nil {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, err.Error()))
		}
		agent, err := h.store.CreateCloudAgent(ctx, created)
		if err != nil {
			return WriteError(c, err)
		}
		createdAgentID = agent.ID
		addIDs = append(addIDs, agent.ID)
	}
	if len(addIDs) == 0 {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "agent_ids or new_agent is required"))
	}
	// 合并成员（去重、保序），沿用标签 CAS 版本。
	merged := make([]string, 0, len(current.Config.AgentIDs)+len(addIDs))
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, current.Config.AgentIDs...), addIDs...) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		merged = append(merged, id)
	}
	if len(merged) > 20 {
		h.compensateAgent(ctx, pc.ID, createdAgentID)
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "too many agents (max 20)"))
	}
	cfg := current.Config
	cfg.AgentIDs = merged
	saved, err := h.saveTagConfig(ctx, pc.ID, current, cfg, body.ExpectedVersion)
	if err != nil {
		// 补偿：成员写入失败时归档刚新建的助手，避免留下孤儿。
		h.compensateAgent(ctx, pc.ID, createdAgentID)
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, h.tagDTOWithAvailability(ctx, pc.ID, saved))
}

// compensateAgent 归档刚创建但成员关系写入失败的助手（尽力而为）。
func (h *cloudAgentHandler) compensateAgent(ctx context.Context, projectID, agentID string) {
	if agentID == "" {
		return
	}
	_ = h.store.ArchiveCloudAgent(ctx, projectID, agentID)
}

// RemoveGroupMember 移出成员（DELETE /agent-tags/:tagID/members/:agentID）。
// 只移除本群关系，不删除助手本身。
func (h *cloudAgentHandler) RemoveGroupMember(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	ctx := c.Request().Context()
	tagID := strings.TrimSpace(c.Param("tagID"))
	agentID := strings.TrimSpace(c.Param("agentID"))
	current, err := h.resolveAgentTag(ctx, pc.ID, tagID)
	if err != nil {
		return WriteError(c, err)
	}
	kept := make([]string, 0, len(current.Config.AgentIDs))
	found := false
	for _, id := range current.Config.AgentIDs {
		if id == agentID {
			found = true
			continue
		}
		kept = append(kept, id)
	}
	if !found {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent is not a member of this tag"))
	}
	cfg := current.Config
	cfg.AgentIDs = kept
	// 兜底助手被移出时同步清空，避免「兜底指向非成员」。
	if cfg.FallbackAgentID == agentID {
		cfg.FallbackAgentID = ""
	}
	expected, _ := parseInt64Query(c.QueryParam("expected_version"))
	saved, err := h.saveTagConfig(ctx, pc.ID, current, cfg, expected)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, h.tagDTOWithAvailability(ctx, pc.ID, saved))
}

// saveTagConfig 按标签类型走 CAS 写入（预置群用 Upsert，自定义用 Update）。
func (h *cloudAgentHandler) saveTagConfig(ctx context.Context, projectID string, current systemdb.AgentTag, cfg systemdb.AgentTagConfig, expectedVersion int64) (systemdb.AgentTag, error) {
	if expectedVersion <= 0 {
		// 未显式给版本时用当前版本推进，减少前端在成员增删上的版本摩擦。
		expectedVersion = current.Version
	}
	if current.BuiltinKey != "" {
		return h.store.UpsertBuiltinAgentTag(ctx, projectID, current.BuiltinKey, current.Name, cfg, expectedVersion)
	}
	return h.store.UpdateAgentTag(ctx, projectID, current.ID, current.Name, cfg, expectedVersion)
}

// groupEventBody 是 POST /agent-tags/:tagID/events 的请求体（告警/统计输入入口）。
type groupEventBody struct {
	// ExternalID 是外部事件 ID（告警指纹 / 报表批次），用于幂等去重。
	ExternalID string `json:"external_id"`
	// Title 是会话标题（缺省用群名 + 时间）。
	Title string `json:"title"`
	// Prompt 是交给群处理的内容（纯文本）。
	Prompt string `json:"prompt"`
	// ThreadKind 缺省按群类型推断（alerts=alert、stats=statistics）。
	ThreadKind string `json:"thread_kind"`
}

// GroupEvent 把外部事件转成一条群会话并立即发起一轮（POST /agent-tags/:tagID/events）。
//
// 该入口面向机器调用（告警系统 / 报表任务），因此：
//   - 不创建人工任务（没有「当前人」）；
//   - 幂等键 = external_id，重复投递只产生一条会话；
//   - 正文长度受限，避免把整份日志塞进提示词。
func (h *cloudAgentHandler) GroupEvent(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	ctx := c.Request().Context()
	tagID := strings.TrimSpace(c.Param("tagID"))
	tag, err := h.resolveAgentTag(ctx, pc.ID, tagID)
	if err != nil {
		return WriteError(c, err)
	}
	var body groupEventBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "prompt is required"))
	}
	if n := len([]rune(prompt)); n > 8000 {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "prompt is too long (max 8000 characters)"))
	}
	if err := h.store.CollabSchemaReady(ctx); err != nil {
		return WriteError(c, agentrun.ErrWorkspaceUpgradeRequired)
	}
	if !h.caps().LLMConfigured {
		return WriteError(c, &cloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"})
	}
	kind := strings.TrimSpace(body.ThreadKind)
	if kind == "" {
		kind = threadKindForTag(tag.ID)
	}
	switch kind {
	case systemdb.ThreadKindChat, systemdb.ThreadKindAlert,
		systemdb.ThreadKindStatistics, systemdb.ThreadKindScheduled:
	default:
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid thread_kind"))
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = tag.Name
	}
	title = strutil.TruncateRunes(title, 80)
	// 每事件一条独立会话：告警/统计需要可追溯的独立上下文。
	principal, _ := PrincipalFromContext(ctx)
	th, err := h.store.CreateAgentThreadWithWorkspace(ctx, systemdb.AgentThread{
		ProjectID: pc.ID, Title: title, CreatedBy: principal.Actor(),
	}, systemdb.AgentThreadWorkspace{
		TagID: tag.ID, DispatchMode: systemdb.DispatchCollaborative, Source: systemdb.ThreadSourceWorkspace,
	})
	if err != nil {
		return WriteError(c, err)
	}
	if _, err := h.store.CreateAgentThreadCollab(ctx, systemdb.AgentThreadCollab{
		ProjectID: pc.ID, ThreadID: th.ID, TagID: tag.ID, ThreadKind: kind,
	}); err != nil {
		_ = h.store.ArchiveAgentThread(ctx, pc.ID, th.ID)
		return WriteError(c, err)
	}
	thread, err := h.store.GetAgentThread(ctx, pc.ID, th.ID)
	if err != nil {
		return WriteError(c, err)
	}
	idempotency := strings.TrimSpace(body.ExternalID)
	if idempotency != "" {
		idempotency = "event:" + tag.ID + ":" + idempotency
	}
	turnID, err := h.svc.RunHeadlessTurn(ctx, agentrun.HeadlessTurnInput{
		ProjectID: pc.ID, Thread: thread, Principal: principal,
		Workspace: systemdb.AgentThreadWorkspace{
			TagID: tag.ID, DispatchMode: systemdb.DispatchCollaborative, Source: systemdb.ThreadSourceWorkspace,
		},
		Collab: systemdb.AgentThreadCollab{
			ProjectID: pc.ID, ThreadID: th.ID, TagID: tag.ID, ThreadKind: kind,
		},
		Tag:            tag,
		Content:        prompt,
		IdempotencyKey: idempotency,
	})
	if err != nil {
		return WriteError(c, err)
	}
	turn, err := h.store.GetAgentTurn(ctx, pc.ID, turnID)
	if err != nil {
		return WriteError(c, err)
	}
	dto, err := h.buildTurnDTO(ctx, pc.ID, turn)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusAccepted, dto)
}
