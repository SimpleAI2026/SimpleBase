// 文件说明：mcp_dispatch.go — internal/api 的内部实现（mcpToolResultLimit、mcpCaller 等）
//
// 用法
//
//	同包调用：mcpCaller → mcp_builtin.go；withMCPCaller() → mcp_builtin.go；dispatchBuiltinTool() → mcp_builtin.go,
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/mcp_dispatch.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backup_handler.go, data_compat.go, error.go, file_compat.go, function_compat.go, health.go, mcp_builtin.go, mcp_builtin_admin.go 等 28 个
//	[关键引用] mcpCaller ← mcp_builtin.go；withMCPCaller ← mcp_builtin.go；dispatchBuiltinTool ← mcp_builtin.go,
//
// 主要符号
//
//	const mcpToolResultLimit
//	type mcpCaller struct{…}
//	type mcpCallerKey struct{…}
//	withMCPCaller(ctx context.Context, c mcpCaller) context.Context
//	mcpCallerFrom(ctx context.Context) (mcpCaller, bool)
//	var errMCPNoCaller
//	dispatchBuiltinTool(ctx context.Context, e *echo.Echo, deps Dependencies, pc ProjectContext, t builtinAPITool,...
//	subRequest(ctx context.Context, e *echo.Echo, caller mcpCaller, method, path string, body io.Reader) (int, str...
//	... 另有 2 个顶层符号

// mcp_dispatch.go — 内置 MCP 工具调用：转成对完整路由树的子请求（planv6.7 §3.4）
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/linkxzhou/SimpleBase/internal/api/datahttp"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
)

// mcpToolResultLimit 是单次工具结果文本的上限，防止大表结果撑爆模型上下文。
const mcpToolResultLimit = 256 << 10

// mcpCaller 是发起 MCP 调用的外部请求身份，由 Serve 写入 ctx。
// 子请求带同一个 Authorization 头，由统一认证中间件重新鉴权，权限、
// 委托边界、policyGate、writeGate 与审计全部沿用路由原有逻辑，不在这里重复实现。
type mcpCaller struct {
	Authorization string
	RemoteAddr    string
	RequestID     string
}

type mcpCallerKey struct{}

func withMCPCaller(ctx context.Context, c mcpCaller) context.Context {
	return context.WithValue(ctx, mcpCallerKey{}, c)
}

func mcpCallerFrom(ctx context.Context) (mcpCaller, bool) {
	if c, ok := ctx.Value(mcpCallerKey{}).(mcpCaller); ok && c.Authorization != "" {
		return c, true
	}
	// 进程内会话（云助手）没有 Authorization 头，主体由 context 携带。
	if _, ok := internalPrincipalFrom(ctx); ok {
		return mcpCaller{}, true
	}
	return mcpCaller{}, false
}

var errMCPNoCaller = errors.New("mcp caller identity missing")

// dispatchBuiltinTool 把一次 tools/call 转成 `/v1` 子请求。
// HTTP 状态 >= 400 时返回 IsError 结果（正文原样给模型，便于自我纠正），不转协议错误。
func dispatchBuiltinTool(ctx context.Context, e *echo.Echo, deps Dependencies, pc httpkit.ProjectContext, t builtinAPITool, arguments []byte) (mcp.CallResult, error) {
	caller, ok := mcpCallerFrom(ctx)
	if !ok {
		return mcp.CallResult{}, errMCPNoCaller
	}
	args := map[string]json.RawMessage{}
	if trimmed := bytes.TrimSpace(arguments); len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &args); err != nil {
			return mcp.CallResult{}, fmt.Errorf("arguments must be a JSON object")
		}
	}

	path := strings.ReplaceAll(t.Path, "{projectID}", url.PathEscape(pc.ID))
	used := map[string]bool{}
	for _, name := range t.PathArgs {
		value, err := mcpScalarArg(args, name)
		if err != nil {
			return mcp.CallResult{}, err
		}
		if value == "" && name == "databaseID" {
			// 单项目单库：省略时取项目唯一的库。
			id, derr := defaultDatabaseID(ctx, e, caller, pc)
			if derr != nil {
				return mcp.CallResult{}, derr
			}
			value = id
		}
		if value == "" {
			return mcp.CallResult{}, fmt.Errorf("missing required argument %q", name)
		}
		used[name] = true
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(value))
	}
	if strings.Contains(path, "{") {
		return mcp.CallResult{}, fmt.Errorf("unresolved path parameter in tool %q", t.Name)
	}

	query := url.Values{}
	for _, name := range t.QueryArgs {
		value, err := mcpScalarArg(args, name)
		if err != nil {
			return mcp.CallResult{}, err
		}
		used[name] = true
		if value != "" {
			query.Set(name, value)
		}
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var body io.Reader
	switch t.BodyArg {
	case "-":
	case "":
		rest := make(map[string]json.RawMessage, len(args))
		for k, v := range args {
			if !used[k] {
				rest[k] = v
			}
		}
		raw, err := json.Marshal(rest)
		if err != nil {
			return mcp.CallResult{}, err
		}
		body = bytes.NewReader(raw)
	default:
		raw, ok := args[t.BodyArg]
		if !ok {
			return mcp.CallResult{}, fmt.Errorf("missing required argument %q", t.BodyArg)
		}
		body = bytes.NewReader(raw)
	}

	status, text, err := subRequest(ctx, e, caller, t.Method, path, body)
	if err != nil {
		return mcp.CallResult{}, err
	}
	res := mcp.CallResult{Text: text, IsError: status >= http.StatusBadRequest}
	if len(res.Text) > mcpToolResultLimit {
		res.Text = res.Text[:mcpToolResultLimit]
		res.Truncated = true
	}
	if res.Text == "" {
		res.Text = fmt.Sprintf("HTTP %d", status)
	}
	return res, nil
}

// subRequest 用调用方的 Authorization 头对完整路由树发起一次内部请求。
func subRequest(ctx context.Context, e *echo.Echo, caller mcpCaller, method, path string, body io.Reader) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return 0, "", err
	}
	if caller.Authorization != "" {
		req.Header.Set(echo.HeaderAuthorization, caller.Authorization)
	}
	if body != nil {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if caller.RequestID != "" {
		req.Header.Set(echo.HeaderXRequestID, caller.RequestID)
	}
	req.RemoteAddr = caller.RemoteAddr
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), nil
}

// defaultDatabaseID 返回项目唯一的数据库 ID；0 个或多个库时报错并提示显式传 databaseID。
func defaultDatabaseID(ctx context.Context, e *echo.Echo, caller mcpCaller, pc httpkit.ProjectContext) (string, error) {
	path := "/v1/projects/" + url.PathEscape(pc.ID) + "/databases"
	status, text, err := subRequest(ctx, e, caller, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	if status >= http.StatusBadRequest {
		return "", fmt.Errorf("list databases failed: HTTP %d: %s", status, text)
	}
	var resp datahttp.DatabaseListResponse
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return "", fmt.Errorf("list databases: invalid response")
	}
	if len(resp.Databases) != 1 {
		return "", fmt.Errorf("project has %d databases; pass databaseID explicitly", len(resp.Databases))
	}
	return resp.Databases[0].ID, nil
}

// mcpScalarArg 取字符串或数字参数的文本；缺省返回空串。
func mcpScalarArg(args map[string]json.RawMessage, name string) (string, error) {
	raw, ok := args[name]
	if !ok {
		return "", nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return "", fmt.Errorf("argument %q must be a string", name)
		}
		return s, nil
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return "", fmt.Errorf("argument %q must be a scalar", name)
	}
	return string(trimmed), nil
}
