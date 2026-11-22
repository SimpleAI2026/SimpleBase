// mcp_builtin_admin.go — 内置 MCP 的展示与管理接口（planv6.7 §3.5）
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// builtinToolDTO 是内置 MCP 工具的展示信息。
type builtinToolDTO struct {
	Name        string `json:"name"`
	ReadOnly    bool   `json:"read_only"`
	Destructive bool   `json:"destructive"`
}

// builtinServerDTO 是 GET /mcp/servers 的 builtin 数组元素。
type builtinServerDTO struct {
	Name      string           `json:"name"`
	Builtin   bool             `json:"builtin"`
	Enabled   bool             `json:"enabled"`
	Transport string           `json:"transport"`
	URL       string           `json:"url"`
	Tools     []builtinToolDTO `json:"tools"`
}

// builtinList 返回当前项目可见的内置 MCP。实例开关关闭、系统项目时返回空数组。
func (h *mcpServerHandler) builtinList(c echo.Context, pc ProjectContext) []builtinServerDTO {
	out := []builtinServerDTO{}
	if h.builtin == nil || catalog.IsSystemProject(pc.ID) {
		return out
	}
	base := publicBaseURL(c, h.publicURL, h.publicOverride)
	for _, entry := range builtinMCPs {
		if entry.Available != nil && !entry.Available(h.builtin.deps) {
			continue
		}
		spec, ok := h.builtin.resolveSpec(entry.Name, pc)
		if !ok {
			continue
		}
		enabled, err := h.builtin.checkProjectSetting(c.Request().Context(), pc.ID, entry.Name)
		if err != nil {
			enabled = true
		}
		url := base + "/v1/projects/" + pc.ID + "/mcp"
		if entry.Name != "simplebase" {
			url = base + "/v1/projects/" + pc.ID + "/mcp/builtin/" + entry.Name
		}
		out = append(out, builtinServerDTO{
			Name:      entry.Name,
			Builtin:   true,
			Enabled:   enabled,
			Transport: "http",
			URL:       url,
			Tools:     toolDTOs(spec),
		})
	}
	return out
}

func toolDTOs(spec *mcp.ServerSpec) []builtinToolDTO {
	tools := make([]builtinToolDTO, 0, len(spec.Tools))
	for _, t := range spec.Tools {
		tools = append(tools, builtinToolDTO{Name: t.Name, ReadOnly: t.ReadOnly, Destructive: t.Destructive})
	}
	return tools
}

type builtinStateBody struct {
	Enabled *bool `json:"enabled"`
}

// SetState 是 PUT /mcp/builtin/:name/state：写项目级开关并记审计。
func (h *mcpServerHandler) SetState(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	name := c.Param("name")
	if h.builtin == nil || catalog.IsSystemProject(pc.ID) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "builtin mcp not available"))
	}
	if _, found := h.builtin.resolveSpec(name, pc); !found {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "builtin mcp not found"))
	}
	if h.writableGuard(c) != nil {
		return nil
	}
	var body builtinStateBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if body.Enabled == nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "enabled is required"))
	}
	raw, err := json.Marshal(*body.Enabled)
	if err != nil {
		return WriteError(c, err)
	}
	if err := h.store.PutProjectSetting(c.Request().Context(), pc.ID, "mcp.builtin."+name+".enabled", string(raw)); err != nil {
		return WriteError(c, err)
	}
	h.recordBuiltinAudit(c, pc.ID, name, *body.Enabled)
	return c.JSON(http.StatusOK, map[string]any{"name": name, "enabled": *body.Enabled})
}

func (h *mcpServerHandler) recordBuiltinAudit(c echo.Context, projectID, name string, enabled bool) {
	if h.audit == nil {
		return
	}
	kind := "mcp.builtin.disable"
	if enabled {
		kind = "mcp.builtin.enable"
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	_ = h.audit.Record(c.Request().Context(), AuditEvent{
		ProjectID:   projectID,
		PrincipalID: principal.APIKeyID,
		Kind:        kind,
		RequestID:   RequestIDFromContext(c.Request().Context()),
		Status:      "ok",
		Detail:      "name:" + name + ",enabled:" + strconv.FormatBool(enabled),
	})
}

// TestBuiltin 是 POST /mcp/builtin/:name/test：进程内连一次并列工具，不依赖 agent.mcp.enabled。
func (h *mcpServerHandler) TestBuiltin(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	if h.builtin == nil || catalog.IsSystemProject(pc.ID) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "builtin mcp not available"))
	}
	name := c.Param("name")
	spec, found := h.builtin.resolveSpec(name, pc)
	if !found {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "builtin mcp not found"))
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*mcp.DefaultConnectTimeout)
	defer cancel()
	tools, err := listInMemory(ctx, name, spec)
	if err != nil {
		return c.JSON(http.StatusOK, mcpTestResult{OK: false, Error: "connect_failed"})
	}
	return c.JSON(http.StatusOK, mcpTestResult{OK: true, Tools: tools})
}

func listInMemory(ctx context.Context, name string, spec *mcp.ServerSpec) ([]string, error) {
	sess, err := mcp.ConnectInMemory(ctx, name, spec)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sess.Close() }()
	infos, err := sess.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(infos))
	for _, t := range infos {
		names = append(names, t.Name)
	}
	return names, nil
}
