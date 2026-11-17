// mcp_adapter.go — CLI 侧的 MCP 工具包装（planv6.2 §5.2）。
//
// 权限模型与云端不同：CLI 没有 Agent.Writable，破坏性判定来自本地配置的
// read_only_tools 白名单。不在白名单内的工具首次调用需确认，Yolo 跳过确认
// （与 shell 工具同口径，§5.2）。
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
)

// mcpLocalTool 把一个 MCP 工具包装成 eino InvokableTool（本地版）。
type mcpLocalTool struct {
	serverID string
	// rawName 是 Server 侧原始工具名。
	rawName string
	info    *schema.ToolInfo
	session mcp.Session
	cfg     MCPServerConfig
	env     mcpServerEnv
	timeout time.Duration
}

func newMCPLocalTool(serverID string, cfg MCPServerConfig, sess mcp.Session, info mcp.ToolInfo, env mcpServerEnv) *mcpLocalTool {
	name := mcp.ToolName(serverID, info.Name)
	return &mcpLocalTool{
		serverID: serverID,
		rawName:  info.Name,
		info:     localToolInfo(name, info),
		session:  sess,
		cfg:      cfg,
		env:      env,
		timeout:  mcpCallTimeout(cfg),
	}
}

func (t *mcpLocalTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }

// InvokableRun 执行一次工具调用。
//
// 确认顺序（fail-closed）：非只读白名单 → 需确认；确认缺失或拒绝则不执行。
func (t *mcpLocalTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	// 非交互（没有 Workspace）时 fail-closed：不执行非只读工具。
	yolo := t.env.Workspace != nil && t.env.Workspace.Yolo
	if !t.cfg.ReadOnly(t.rawName) && !yolo {
		if !t.confirm() {
			return toolsErrEnvelope(mcp.CodeConfirmationDenied, "已取消"), nil
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	res, err := t.session.CallTool(callCtx, t.rawName, []byte(argumentsInJSON))
	if err != nil {
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return toolsErrEnvelope(mcp.CodeTimeout, "MCP 工具调用超时"), nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return toolsErrEnvelope(mcp.CodeUnavailable, "MCP Server 不可用"), nil
	}
	if res.IsError {
		return toolsErrEnvelope(mcp.CodeToolError, truncateMCPText(res.Text, 2000)), nil
	}
	out := agentcore.RedactSecrets(res.Text)
	if res.Truncated {
		out += "\n[truncated]"
	}
	t.record(out)
	return out, nil
}

// confirm 走 Workspace 的统一确认入口；没有确认函数时一律拒绝（非交互）。
func (t *mcpLocalTool) confirm() bool {
	if t.env.Workspace == nil || t.env.Workspace.Confirm == nil {
		return false
	}
	return t.env.Workspace.Confirm("调用 MCP 工具 " + t.serverID + " · " + t.rawName + "？")
}

// record 把调用结果交给 REPL 展示（与内置工具同一条通道）。
func (t *mcpLocalTool) record(result string) {
	if t.env.Workspace == nil || t.env.Workspace.Record == nil {
		return
	}
	t.env.Workspace.Record(t.info.Name, "", result, false)
}

// mcpCallTimeout 返回单次调用超时；0 用默认 60s，上限 300s。
func mcpCallTimeout(cfg MCPServerConfig) time.Duration {
	if cfg.TimeoutSeconds <= 0 {
		return mcp.DefaultCallTimeout
	}
	d := time.Duration(cfg.TimeoutSeconds) * time.Second
	if d > mcp.MaxCallTimeout {
		return mcp.MaxCallTimeout
	}
	return d
}

// localToolInfo 把自有 ToolInfo 转成 eino 的 schema.ToolInfo。
//
// 与云端同一策略：schema 走 JSON 往返，两套 jsonschema 库不交换类型。
func localToolInfo(name string, info mcp.ToolInfo) *schema.ToolInfo {
	ti := &schema.ToolInfo{
		Name:  name,
		Desc:  info.Description,
		Extra: map[string]any{"mcp_tool": info.Name},
	}
	var js jsonschema.Schema
	if err := json.Unmarshal(info.InputSchema, &js); err == nil && (js.Type == "" || js.Type == "object") {
		ti.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(&js)
	}
	return ti
}

// toolsErrEnvelope 构造与云端 skillFail 同形的失败信封。
func toolsErrEnvelope(code, msg string) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": map[string]any{"code": code, "message": msg}})
	return string(b)
}

func truncateMCPText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n…[truncated]"
}

// mcpToolNames 返回已装配的 MCP 工具名（/mcp 命令与测试用）。
func mcpToolNames(tools []tool.BaseTool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil || info == nil {
			continue
		}
		if strings.HasPrefix(info.Name, "mcp__") {
			out = append(out, info.Name)
		}
	}
	return out
}

// mcpDisplay 把 mcp__server__tool 展示成「server · tool」。
func mcpDisplay(name string) string { return mcp.DisplayName(name) }
