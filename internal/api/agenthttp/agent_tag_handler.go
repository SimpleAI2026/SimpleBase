package agenthttp

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

// agentTagPromptDTO 是标签提示词的 DTO 形状。
type agentTagPromptDTO struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type agentTagDTO struct {
	ID         string              `json:"id"`
	BuiltinKey string              `json:"builtin_key,omitempty"`
	Name       string              `json:"name"`
	AgentIDs   []string            `json:"agent_ids"`
	Prompts    []agentTagPromptDTO `json:"prompts"`
	// RoutingPrompt 是群路由提示词（v2 §4.1）：只决定分工，不授予工具权限。
	RoutingPrompt string `json:"routing_prompt,omitempty"`
	// FallbackAgentID 是路由失败时的唯一兜底助手（必须是成员）；空表示没有兜底。
	FallbackAgentID string `json:"fallback_agent_id,omitempty"`
	// HumanPolicy 为空表示允许把任务交给当前人；"disabled" 表示禁止人工任务。
	HumanPolicy   string `json:"human_policy,omitempty"`
	ConfirmPolicy string `json:"confirm_policy,omitempty"`
	// ThreadKinds 是该群允许的会话类型（v2 §5.3），前端据此渲染「新建会话」菜单。
	ThreadKinds []string `json:"thread_kinds,omitempty"`
	// UnavailableAgentIDs 是配置里已归档/不存在的助手，供编辑器标「助手已不可用」。
	UnavailableAgentIDs []string   `json:"unavailable_agent_ids"`
	Version             int64      `json:"version"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
}

func toAgentTagPromptDTOs(prompts []systemdb.AgentTagPrompt) []agentTagPromptDTO {
	out := make([]agentTagPromptDTO, 0, len(prompts))
	for _, p := range prompts {
		out = append(out, agentTagPromptDTO{ID: p.ID, Title: p.Title, Content: p.Content})
	}
	return out
}

// builtinAgentTagDefaults 返回预置群的内置默认配置；规则归 agentrun（预置群解析与入口共用一份实现）。
func builtinAgentTagDefaults() map[string]systemdb.AgentTagConfig {
	return agentrun.BuiltinAgentTagDefaults()
}

// builtinAgentTagKeys 是预置群的固定顺序，通用群始终位于首位。
func builtinAgentTagKeys() []string {
	return []string{systemdb.AgentTagGeneral, systemdb.AgentTagAlerts, systemdb.AgentTagStats, systemdb.AgentTagScheduled}
}

// threadKindsForTag 返回该群允许创建的会话类型（v2 §5.3）。
func threadKindsForTag(tagID string) []string {
	switch tagID {
	case systemdb.AgentTagAlerts:
		return []string{systemdb.ThreadKindChat, systemdb.ThreadKindAlert}
	case systemdb.AgentTagStats:
		return []string{systemdb.ThreadKindChat, systemdb.ThreadKindStatistics}
	case systemdb.AgentTagScheduled:
		return []string{systemdb.ThreadKindScheduled}
	default:
		return []string{systemdb.ThreadKindChat, systemdb.ThreadKindAlert,
			systemdb.ThreadKindStatistics, systemdb.ThreadKindScheduled}
	}
}

// builtinAgentTagName 返回预置群的固定显示名（实际本地化由前端完成）。
func builtinAgentTagName(key string) string {
	return agentrun.BuiltinAgentTagName(key)
}

// agentTagDTOFrom 把仓储行转 DTO，并标记不可用助手。
func agentTagDTOFrom(t systemdb.AgentTag, available map[string]bool) agentTagDTO {
	unavailable := make([]string, 0)
	for _, id := range t.Config.AgentIDs {
		if !available[id] {
			unavailable = append(unavailable, id)
		}
	}
	dto := agentTagDTO{
		ID:                  t.ID,
		BuiltinKey:          t.BuiltinKey,
		Name:                t.Name,
		AgentIDs:            t.Config.AgentIDs,
		Prompts:             toAgentTagPromptDTOs(t.Config.Prompts),
		RoutingPrompt:       t.Config.RoutingPrompt,
		FallbackAgentID:     t.Config.FallbackAgentID,
		HumanPolicy:         t.Config.HumanPolicy,
		ConfirmPolicy:       t.Config.ConfirmPolicy,
		ThreadKinds:         threadKindsForTag(t.ID),
		UnavailableAgentIDs: unavailable,
		Version:             t.Version,
	}
	if !t.UpdatedAt.IsZero() {
		u := t.UpdatedAt
		dto.UpdatedAt = &u
	}
	return dto
}

// ListAgentTags 返回预置与自定义标签：内置默认 + 覆盖行 + 自定义，
// 按「预置（general、alerts、stats、scheduled）→ 自定义（created_at 升序）」排序。
func (h *CloudAgentHandler) ListAgentTags(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	ctx := c.Request().Context()
	agents, err := h.store.ListCloudAgents(ctx, pc.ID)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	// available 按助手 ID 判定可用；keyToID 把 builtin_key 解析为项目助手 ID。
	available := make(map[string]bool, len(agents))
	keyToID := make(map[string]string, len(agents))
	for _, a := range agents {
		available[a.ID] = true
		if a.BuiltinKey != "" {
			keyToID[a.BuiltinKey] = a.ID
		}
	}
	rows, err := h.store.ListAgentTagRows(ctx, pc.ID)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	// 覆盖行按 builtin_key 索引（预置标签 ID 固定为 builtin_key）。
	overrides := make(map[string]systemdb.AgentTag, len(rows))
	custom := make([]systemdb.AgentTag, 0, len(rows))
	for _, t := range rows {
		if t.BuiltinKey != "" {
			overrides[t.BuiltinKey] = t
		} else {
			custom = append(custom, t)
		}
	}
	out := make([]agentTagDTO, 0, len(rows)+len(builtinAgentTagKeys()))
	for _, key := range builtinAgentTagKeys() {
		if t, ok := overrides[key]; ok {
			out = append(out, agentTagDTOFrom(t, available))
			continue
		}
		if key == systemdb.AgentTagGeneral {
			tag, err := h.resolveAgentTag(ctx, pc.ID, systemdb.AgentTagGeneral)
			if err != nil {
				return httpkit.WriteError(c, err)
			}
			out = append(out, agentTagDTOFrom(tag, available))
			continue
		}
		// 内置默认：builtin_key 引用解析为本项目助手 ID，缺失的跳过。
		cfg := builtinAgentTagDefaults()[key]
		resolved := make([]string, 0, len(cfg.AgentIDs))
		for _, id := range cfg.AgentIDs {
			if available[id] {
				resolved = append(resolved, id)
				continue
			}
			if realID, ok := keyToID[id]; ok {
				resolved = append(resolved, realID)
			}
		}
		out = append(out, agentTagDTO{
			ID:            key,
			BuiltinKey:    key,
			Name:          builtinAgentTagName(key),
			AgentIDs:      resolved,
			Prompts:       toAgentTagPromptDTOs(cfg.Prompts),
			RoutingPrompt: cfg.RoutingPrompt,
			HumanPolicy:   cfg.HumanPolicy,
			ConfirmPolicy: cfg.ConfirmPolicy,
			ThreadKinds:   threadKindsForTag(key),
			Version:       0,
		})
	}
	for _, t := range custom {
		out = append(out, agentTagDTOFrom(t, available))
	}
	return c.JSON(http.StatusOK, map[string]any{"tags": out})
}

// agentTagBody 是创建/更新的请求体。
type agentTagBody struct {
	Name            *string             `json:"name"`
	AgentIDs        []string            `json:"agent_ids"`
	Prompts         []agentTagPromptDTO `json:"prompts"`
	ExpectedVersion int64               `json:"expected_version"`
	// v2 §4.1：群路由提示词、兜底助手、人工策略。
	RoutingPrompt   *string `json:"routing_prompt"`
	FallbackAgentID *string `json:"fallback_agent_id"`
	HumanPolicy     *string `json:"human_policy"`
	ConfirmPolicy   *string `json:"confirm_policy"`
}

// normalizeTagBody 校验长度/数量/去重/助手归属；提示词 ID 缺省生成 UUID。
// 返回名称（TrimSpace 后）与配置。内置标签的名称与 ID 均为保留字。
func (h *CloudAgentHandler) normalizeTagBody(ctx context.Context, projectID string, body agentTagBody, requireName bool) (string, systemdb.AgentTagConfig, error) {
	name := ""
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	}
	if requireName {
		if n := len([]rune(name)); n < 1 || n > 32 {
			return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "tag name must be 1-32 characters")
		}
		normalizedName := strings.ToLower(name)
		switch normalizedName {
		case "通用", "告警", "统计", "定时":
			return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "tag name is reserved")
		}
		if systemdb.IsReservedAgentTagID(normalizedName) {
			return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "tag name is reserved")
		}
	}
	if len(body.AgentIDs) > 20 {
		return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "too many agents (max 20)")
	}
	seenAgent := map[string]bool{}
	agentIDs := make([]string, 0, len(body.AgentIDs))
	for _, id := range body.AgentIDs {
		id = strings.TrimSpace(id)
		if id == "" || seenAgent[id] {
			continue
		}
		if _, err := h.store.GetCloudAgent(ctx, projectID, id); err != nil {
			return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "agent not found: "+id)
		}
		seenAgent[id] = true
		agentIDs = append(agentIDs, id)
	}
	if len(body.Prompts) > 20 {
		return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "too many prompts (max 20)")
	}
	prompts := make([]systemdb.AgentTagPrompt, 0, len(body.Prompts))
	for _, p := range body.Prompts {
		title := strings.TrimSpace(p.Title)
		if n := len([]rune(title)); n < 1 || n > 40 {
			return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "prompt title must be 1-40 characters")
		}
		if n := len([]rune(p.Content)); n < 1 || n > 4000 {
			return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "prompt content must be 1-4000 characters")
		}
		id := strings.TrimSpace(p.ID)
		if id == "" {
			id = uuid.NewString()
		}
		prompts = append(prompts, systemdb.AgentTagPrompt{ID: id, Title: title, Content: p.Content})
	}
	// v2：路由提示词限 4000 字符（与提示词正文同量级，避免把长文档塞进路由器）。
	routingPrompt := ""
	if body.RoutingPrompt != nil {
		routingPrompt = strings.TrimSpace(*body.RoutingPrompt)
		if n := len([]rune(routingPrompt)); n > 4000 {
			return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "routing prompt must be at most 4000 characters")
		}
	}
	// 兜底助手必须是成员：否则「兜底」会越出群候选范围，等于绕过成员控制。
	fallback := ""
	if body.FallbackAgentID != nil {
		fallback = strings.TrimSpace(*body.FallbackAgentID)
		if fallback != "" && !seenAgent[fallback] {
			return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "fallback agent must be a member of this tag")
		}
	}
	humanPolicy := ""
	if body.HumanPolicy != nil {
		humanPolicy = strings.TrimSpace(*body.HumanPolicy)
		if humanPolicy != "" && humanPolicy != systemdb.HumanPolicyDisabled {
			return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "human_policy must be empty or disabled")
		}
	}
	confirmPolicy := "auto"
	if body.ConfirmPolicy != nil {
		confirmPolicy = strings.ToLower(strings.TrimSpace(*body.ConfirmPolicy))
		if confirmPolicy == "" {
			confirmPolicy = "auto"
		}
		if confirmPolicy != "auto" && confirmPolicy != "always" && confirmPolicy != "never" {
			return "", systemdb.AgentTagConfig{}, echo.NewHTTPError(http.StatusBadRequest, "confirm_policy must be auto, always or never")
		}
	}
	return name, systemdb.AgentTagConfig{
		AgentIDs:        agentIDs,
		Prompts:         prompts,
		RoutingPrompt:   routingPrompt,
		FallbackAgentID: fallback,
		HumanPolicy:     humanPolicy,
		ConfirmPolicy:   confirmPolicy,
		SchemaVersion:   2,
	}, nil
}

func (h *CloudAgentHandler) CreateAgentTag(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var body agentTagBody
	if err := c.Bind(&body); err != nil {
		return httpkit.WriteError(c, err)
	}
	name, cfg, err := h.normalizeTagBody(c.Request().Context(), pc.ID, body, true)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	createdBy := ""
	if principal, ok := httpkit.PrincipalFromContext(c.Request().Context()); ok {
		createdBy = principal.Actor()
	}
	tag, err := h.store.CreateAgentTag(c.Request().Context(), systemdb.AgentTag{
		ProjectID: pc.ID, Name: name, CreatedBy: createdBy, Config: cfg,
	})
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	agents, err := h.store.ListCloudAgents(c.Request().Context(), pc.ID)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	available := map[string]bool{}
	for _, a := range agents {
		available[a.ID] = true
	}
	return c.JSON(http.StatusCreated, agentTagDTOFrom(tag, available))
}

// ArchiveAgentTag 归档群（v2 §9.2）：保留历史会话的群归属，暂停该群的定时绑定。
func (h *CloudAgentHandler) DeleteAgentTag(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	tagID := strings.TrimSpace(c.Param("tagID"))
	if err := h.store.ArchiveAgentTag(c.Request().Context(), pc.ID, tagID); err != nil {
		return httpkit.WriteError(c, err)
	}
	// 群归档后停止调度：绑定整体归档（失败不回滚归档，只记日志）。
	if err := h.store.ArchiveAgentGroupCronsByTag(c.Request().Context(), pc.ID, tagID); err != nil {
		c.Logger().Warnf("archive group crons for tag %s failed: %v", tagID, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *CloudAgentHandler) PatchAgentTag(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	tagID := strings.TrimSpace(c.Param("tagID"))
	ctx := c.Request().Context()
	current, err := h.resolveAgentTag(ctx, pc.ID, tagID)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var body agentTagBody
	if err := c.Bind(&body); err != nil {
		return httpkit.WriteError(c, err)
	}
	requireName := current.BuiltinKey == ""
	name, cfg, err := h.normalizeTagBody(ctx, pc.ID, body, requireName)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if current.BuiltinKey != "" {
		// 预置标签：名称固定（传了且不同则 400），忽略 body.name。
		if name != "" && name != current.Name {
			return httpkit.WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "builtin tag name cannot be changed"))
		}
		tag, err := h.store.UpsertBuiltinAgentTag(ctx, pc.ID, current.BuiltinKey, current.Name, cfg, body.ExpectedVersion)
		if err != nil {
			return httpkit.WriteError(c, err)
		}
		return c.JSON(http.StatusOK, h.tagDTOWithAvailability(ctx, pc.ID, tag))
	}
	// 自定义标签：未传 name 时保持原名。
	if name == "" {
		name = current.Name
	}
	tag, err := h.store.UpdateAgentTag(ctx, pc.ID, tagID, name, cfg, body.ExpectedVersion)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, h.tagDTOWithAvailability(ctx, pc.ID, tag))
}

func (h *CloudAgentHandler) tagDTOWithAvailability(ctx context.Context, projectID string, tag systemdb.AgentTag) agentTagDTO {
	agents, err := h.store.ListCloudAgents(ctx, projectID)
	if err != nil {
		// 列表失败不阻塞响应；只丢 unavailable 标记。
		return agentTagDTOFrom(tag, map[string]bool{})
	}
	available := map[string]bool{}
	for _, a := range agents {
		available[a.ID] = true
	}
	return agentTagDTOFrom(tag, available)
}
