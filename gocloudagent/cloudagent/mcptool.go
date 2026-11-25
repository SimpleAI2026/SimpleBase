// 文件说明：mcptool.go — 云端 MCP 工具：分类、确认与调用

package cloudagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/gocloudagent/tools"
)

// mcpTool 把一个 MCP 工具包装成 eino InvokableTool。
//
// 权限顺序与 execSimplebase 一致（planv6.2 §4.2）：
//  1. 策略缺失 → mcp_policy_unavailable（不执行）
//  2. 不可写助手 + 非只读 → write_not_allowed（在确认之前拦截）
//  3. 破坏性 + Headless → confirmation_unavailable
//  4. 破坏性 → confirmation_required / Wait / confirmation_resolved
type mcpTool struct {
	serverID string
	// rawName 是 Server 侧的原始工具名（未加 mcp__ 前缀）。
	rawName string
	info    *schema.ToolInfo
	session mcp.Session
	policy  MCPPolicy
	req     RunRequest
	rc      RunContext
	confirm ConfirmationWaiter
	timeout time.Duration
	// confirmTimeout 是破坏性调用的确认窗口。
	confirmTimeout time.Duration
	trace          Tracer
}

func (t *mcpTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }

func (t *mcpTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	callID := t.callID(ctx)
	emit := t.rc.Emit
	name := t.info.Name

	decision, err := t.classify(ctx)
	if err != nil {
		return toolFailure(mcp.CodePolicyUnavailable, "MCP 工具策略不可用"), nil
	}
	if !t.req.Agent.Writable && !decision.ReadOnly {
		return toolFailure(mcp.CodeWriteNotAllowed, "该助手未开启写权限，只能调用只读 MCP 工具"), nil
	}
	if decision.Destructive {
		argv := mcpConfirmArgv(t.serverID, t.rawName, argumentsInJSON)
		outcome, err := AwaitConfirmation(ctx, ConfirmationRequest{
			RunID: t.req.RunID, CallID: callID, Subject: t.req.Subject, Name: name,
			Argv: argv, Message: "将调用破坏性 MCP 工具：" + strings.Join(argv, " "),
			Headless: t.req.Headless, Timeout: t.confirmTimeout,
			Waiter: t.confirm, Emit: emit,
		})
		if err != nil {
			return "", err
		}
		switch outcome {
		case ConfirmationUnavailable:
			return toolFailure(mcp.CodeConfirmationUnavailable, "定时任务不能确认破坏性操作"), nil
		case ConfirmationDenied:
			return toolFailure(mcp.CodeConfirmationDenied, "已取消"), nil
		}
	}

	if emit != nil {
		emit(Event{Type: "tool_start", CallID: callID, Name: name})
	}
	callCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	started := time.Now()
	res, err := t.session.CallTool(callCtx, t.rawName, []byte(argumentsInJSON))
	if err != nil {
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return toolFailure(mcp.CodeTimeout, "MCP 工具调用超时"), nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return toolFailure(mcp.CodeUnavailable, "MCP Server 不可用"), nil
	}
	// 只记元数据：server、工具名、耗时、is_error、结果 hash（§3.4）。
	if t.trace.enabled() {
		t.trace.Meta("gocloudagent: mcp tool called",
			zap.String("run_id", t.req.RunID),
			zap.String("server", t.serverID),
			zap.String("tool", t.rawName),
			zap.Int64("duration_ms", time.Since(started).Milliseconds()),
			zap.Bool("is_error", res.IsError),
			zap.String("result", HashSummary(res.Text)),
		)
	}
	if res.IsError {
		return toolFailure(mcp.CodeToolError, truncate(res.Text, 2000)), nil
	}
	out := agentcore.RedactSecrets(res.Text)
	if res.Truncated {
		out += "\n[truncated]"
	}
	return out, nil
}

// callID 取 eino 注入的 tool call id；测试或非 ToolsNode 路径下为空时
// 用工具名兜底，保证确认流程仍能登记（否则破坏性调用会被静默拒绝）。
func (t *mcpTool) callID(ctx context.Context) string {
	if id := compose.GetToolCallID(ctx); id != "" {
		return id
	}
	return "mcp:" + t.serverID + "/" + t.rawName
}

// classify 取宿主策略；缺失或出错一律 fail-closed。
func (t *mcpTool) classify(ctx context.Context) (ToolDecision, error) {
	if t.policy == nil {
		return ToolDecision{}, errors.New("mcp policy is not configured")
	}
	return t.policy.Classify(ctx, t.req, t.serverID, t.rawName)
}

// mcpConfirmArgv 生成脱敏后的确认参数摘要：工具名 + 参数 JSON 截断。
func mcpConfirmArgv(serverID, tool, arguments string) []string {
	summary := truncate(agentcore.RedactSecrets(strings.TrimSpace(arguments)), 200)
	if summary == "" {
		return []string{serverID, tool}
	}
	return []string{serverID, tool, summary}
}

// mcpBuildParams 是 buildMCPTools 的入参（避免长参数列表）。
type mcpBuildParams struct {
	Req            RunRequest
	RC             RunContext
	Connector      MCPConnector
	Policy         MCPPolicy
	Confirm        ConfirmationWaiter
	ConfirmTimeout time.Duration
	CallTimeout    time.Duration
	Trace          Tracer
	Reserved       map[string]bool
}

// buildMCPTools 按 Agent 的 mcp: 引用装配工具（planv6.2 §4.3）。
//
// 返回的 SessionSet 由调用方 defer Close()。单个 Server 失败不终止 run：
// 跳过该 Server 并在 notes 里留一行提示，让模型如实告知用户。
func buildMCPTools(ctx context.Context, p mcpBuildParams) ([]tool.BaseTool, mcp.SessionSet, []string, error) {
	refs := mcpToolRefs(p.Req.Agent.ToolIDs)
	if len(refs) == 0 {
		return nil, nil, nil, nil
	}
	if p.Connector == nil {
		// Agent 声明了 MCP 能力但宿主没装配：明确报错，不静默忽略（§4.1）。
		return nil, nil, nil, errors.New("mcp is not configured; remove mcp: tool ids from this agent")
	}
	serverIDs := mcpServerIDs(refs)
	set, err := p.Connector.Connect(ctx, p.Req, serverIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	if set == nil {
		return nil, nil, nil, nil
	}
	callTimeout := p.CallTimeout
	if callTimeout <= 0 {
		callTimeout = mcp.DefaultCallTimeout
	}
	confirmTimeout := p.ConfirmTimeout
	if confirmTimeout <= 0 {
		confirmTimeout = 2 * time.Minute
	}
	var out []tool.BaseTool
	var notes []string
	for _, sess := range set.Sessions() {
		serverID := sess.ServerID()
		all, only := mcpToolFilter(refs, serverID)
		infos, err := sess.ListTools(ctx)
		if err != nil {
			notes = append(notes, fmt.Sprintf("mcp: %s unavailable", serverID))
			continue
		}
		for _, info := range infos {
			if !all && !only[info.Name] {
				continue
			}
			if len(out) >= mcp.MaxToolsPerRun {
				notes = append(notes, "mcp: tool limit reached, remaining tools skipped")
				break
			}
			name := mcp.ToolName(serverID, info.Name)
			if p.Reserved[name] {
				notes = append(notes, fmt.Sprintf("mcp: %s skipped (name conflicts with a builtin tool)", name))
				continue
			}
			ti, err := mcpToolInfo(name, info)
			if err != nil {
				notes = append(notes, fmt.Sprintf("mcp: %s skipped (invalid schema)", name))
				continue
			}
			out = append(out, &mcpTool{
				serverID:       serverID,
				rawName:        info.Name,
				info:           ti,
				session:        sess,
				policy:         p.Policy,
				req:            p.Req,
				rc:             p.RC,
				confirm:        p.Confirm,
				timeout:        callTimeout,
				confirmTimeout: confirmTimeout,
				trace:          p.Trace,
			})
		}
	}
	// 一个工具都没装配成功时，会话没有意义，直接回收。
	if len(out) == 0 {
		_ = set.Close()
		return nil, nil, notes, nil
	}
	return out, set, notes, nil
}

// mcpToolInfo 把自有 ToolInfo 转成 eino 的 schema.ToolInfo（严格模式：非法 schema 报错跳过）。
func mcpToolInfo(name string, info mcp.ToolInfo) (*schema.ToolInfo, error) {
	return tools.MCPToolInfo(name, info, true)
}
