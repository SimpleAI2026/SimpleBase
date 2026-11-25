// 文件说明：agent_crud_handler.go — 云助手 agent 的 CRUD、模型列表与归一化
//
// 用法
//
//	ListModules、ListAgents、CreateAgent（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_crud_handler.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, godatabase, internal/agentbridge, internal/auth 等
//	[同包] adapter.go, api_key_policy.go, api_keys_handler.go, auth_handler.go 等 59 个
//
// 主要符号
//
//	func (h *cloudAgentHandler) ensureAgents(ctx context.Context, projectID string) error
//	func (h *cloudAgentHandler) ListModules(c echo.Context) error
//	func (h *cloudAgentHandler) ListAgents(c echo.Context) error
//	type upsertAgentBody struct {…}
//	func (h *cloudAgentHandler) denyReadonly(c echo.Context) error
//	func (h *cloudAgentHandler) CreateAgent(c echo.Context) error
//	func (h *cloudAgentHandler) GetAgent(c echo.Context) error
//	func (h *cloudAgentHandler) PatchAgent(c echo.Context) error
//	... 另有 4 个顶层符号

package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

func (h *cloudAgentHandler) ensureAgents(ctx context.Context, projectID string) error {
	if h.store == nil {
		return systemdb.ErrUnavailable
	}
	return h.store.SeedDefaultCloudAgentsWithCapabilities(ctx, projectID, h.sandboxAvailable(), h.skillsCLI())
}

func (h *cloudAgentHandler) ListModules(c echo.Context) error {
	modules := agentbridge.Modules(h.sandboxAvailable())
	if !h.skillsCLI() {
		available := modules[:0]
		for _, module := range modules {
			switch module.ID {
			case agentbridge.ModuleKV, agentbridge.ModuleGoFunction, agentbridge.ModuleCron:
				continue
			}
			available = append(available, module)
		}
		modules = available
	}
	return c.JSON(http.StatusOK, map[string]any{"modules": modules})
}

func (h *cloudAgentHandler) ListAgents(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	if h.writable {
		if err := h.ensureAgents(c.Request().Context(), pc.ID); err != nil {
			return WriteError(c, err)
		}
	}
	list, err := h.store.ListCloudAgents(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]agentDTO, 0, len(list))
	for _, a := range list {
		out = append(out, toAgentDTO(a))
	}
	return c.JSON(http.StatusOK, map[string]any{"agents": out})
}

type upsertAgentBody struct {
	Name          string   `json:"name"`
	Module        string   `json:"module"`
	Description   string   `json:"description"`
	SystemPrompt  string   `json:"system_prompt"`
	ToolIDs       []string `json:"tool_ids"`
	ModelOverride *string  `json:"model_override"`
	TeamEnabled   *bool    `json:"team_enabled"`
}

func (h *cloudAgentHandler) denyReadonly(c echo.Context) error {
	if !h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
	return nil
}

func (h *cloudAgentHandler) CreateAgent(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	var body upsertAgentBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	principal := principalOf(c)
	a, err := h.validateAgentCapabilities(pc.ID, body, systemdb.CloudAgent{}, principal)
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, err.Error()))
	}
	created, err := h.store.CreateCloudAgent(c.Request().Context(), a)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, toAgentDTO(created))
}

func (h *cloudAgentHandler) GetAgent(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	a, err := h.store.GetCloudAgent(c.Request().Context(), pc.ID, c.Param("agentID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toAgentDTO(a))
}

func (h *cloudAgentHandler) PatchAgent(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	cur, err := h.store.GetCloudAgent(c.Request().Context(), pc.ID, c.Param("agentID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	var body upsertAgentBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	principal := principalOf(c)
	next, err := h.validateAgentCapabilities(pc.ID, body, cur, principal)
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, err.Error()))
	}
	next.ID = cur.ID
	updated, err := h.store.UpdateCloudAgent(c.Request().Context(), next)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toAgentDTO(updated))
}

func (h *cloudAgentHandler) DeleteAgent(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	if err := h.store.ArchiveCloudAgent(c.Request().Context(), pc.ID, c.Param("agentID")); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// validateAgentCapabilities 在 normalizeAgent 之上叠加宿主能力校验：云沙盒未启用时拒绝 sandbox 模块
// 与沙盒工具（cloud-agent-sandbox-plan §7）。
func (h *cloudAgentHandler) validateAgentCapabilities(projectID string, body upsertAgentBody, cur systemdb.CloudAgent, p auth.Principal) (systemdb.CloudAgent, error) {
	a, err := normalizeAgent(projectID, body, cur)
	if err != nil {
		return systemdb.CloudAgent{}, err
	}
	if (a.Module == agentbridge.ModuleKV || a.Module == agentbridge.ModuleGoFunction || a.Module == agentbridge.ModuleCron) && (!h.skillsCLI()) {
		return systemdb.CloudAgent{}, errors.New("this module requires CLI skills")
	}
	if !h.sandboxAvailable() {
		if strings.EqualFold(a.Module, agentbridge.ModuleSandbox) {
			return systemdb.CloudAgent{}, errors.New("sandbox is not enabled; sandbox module is unavailable")
		}
		for _, id := range a.ToolIDs {
			if agentbridge.IsSandboxTool(id) {
				return systemdb.CloudAgent{}, errors.New("sandbox is not enabled; sandbox tools are unavailable")
			}
		}
	}
	// MCP 引用需要宿主装配了连接器，否则保存后 run 会直接报错（planv6.2 §4.1）。
	if cloudagent.HasMCPTools(a.ToolIDs) && !h.caps().MCPConfigured {
		return systemdb.CloudAgent{}, errors.New("mcp is not enabled; remove mcp: tool ids from this agent")
	}
	return a, nil
}

func normalizeAgent(projectID string, body upsertAgentBody, cur systemdb.CloudAgent) (systemdb.CloudAgent, error) {
	a := cur
	a.ProjectID = projectID
	if body.Name != "" {
		a.Name = strings.TrimSpace(body.Name)
	}
	if body.Module != "" {
		a.Module = strutil.Normalize(body.Module)
	}
	if body.Description != "" || cur.ID == "" {
		a.Description = body.Description
	}
	if body.SystemPrompt != "" || cur.ID == "" {
		a.SystemPrompt = body.SystemPrompt
	}
	if body.ToolIDs != nil {
		a.ToolIDs = body.ToolIDs
	}
	if body.ModelOverride != nil {
		a.ModelOverride = strings.TrimSpace(*body.ModelOverride)
	}
	if body.TeamEnabled != nil {
		a.TeamEnabled = *body.TeamEnabled
	}
	if strings.TrimSpace(a.Name) == "" {
		return systemdb.CloudAgent{}, errors.New("name is required")
	}
	if a.Module == "" {
		a.Module = agentbridge.ModuleGeneral
	}
	if !agentbridge.KnownModule(a.Module) {
		return systemdb.CloudAgent{}, errors.New("unknown module")
	}
	if a.ToolIDs == nil {
		a.ToolIDs = agentbridge.DefaultToolsForModule(a.Module)
	}
	filtered := make([]string, 0, len(a.ToolIDs))
	for _, id := range a.ToolIDs {
		if agentbridge.KnownTool(id) {
			filtered = append(filtered, id)
			continue
		}
		// mcp: 引用（planv6.2 §3.2）：格式非法直接 400，不静默丢弃——
		// 静默丢弃会让用户以为能力已启用，实际没有。
		if cloudagent.HasMCPTools([]string{id}) {
			if _, _, ok := mcp.ParseToolID(id); !ok {
				return systemdb.CloudAgent{}, fmt.Errorf("invalid mcp tool id: %s", id)
			}
			filtered = append(filtered, id)
		}
	}
	a.ToolIDs = filtered
	return a, nil
}

// ListAgentModels 返回项目可用模型列表（BUG-07）。
// 来源为 provider allowed_models；只返回名字，不暴露 base_url / api_key。
// LLM 未启用时返回空列表（200）。
func (h *cloudAgentHandler) ListAgentModels(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	out := struct {
		DefaultModel string          `json:"default_model"`
		Models       []agentModelDTO `json:"models"`
	}{Models: []agentModelDTO{}}
	if h.llm == nil {
		return c.JSON(http.StatusOK, out)
	}
	models, err := h.llm.ProviderModels(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	defaultModel := ""
	if model := h.svc.DefaultModel(c.Request().Context(), pc.ID); model != "" {
		defaultModel = model
	}
	providers, _ := h.llm.ListProviders(c.Request().Context(), pc.ID)
	for _, p := range providers {
		for _, m := range models[p] {
			if defaultModel == "" {
				defaultModel = m
			}
			out.Models = append(out.Models, agentModelDTO{Provider: p, Name: m})
		}
	}
	out.DefaultModel = defaultModel
	return c.JSON(http.StatusOK, out)
}
