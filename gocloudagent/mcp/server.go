package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MaxServerRequestBytes 是内置 Server 单个请求体上限（§3.2）。
const MaxServerRequestBytes = 1 << 20

// ServerTool 是一个服务端工具（不含 SDK 类型）。
type ServerTool struct {
	Name        string
	Description string
	// InputSchema 必须是 type=object 的 JSON Schema。
	InputSchema json.RawMessage
	// ReadOnly 写入 annotations.readOnlyHint。
	ReadOnly bool
	// Destructive 写入 annotations.destructiveHint（仅非只读工具有意义）。
	Destructive bool
	// Handler 执行工具。arguments 是客户端给的原始 JSON，校验权在 Handler。
	// 返回 error 会被转成 IsError 结果，不会变成协议错误。
	Handler func(ctx context.Context, arguments []byte) (CallResult, error)
}

// ServerSpec 是一个 MCP Server 的完整定义。
type ServerSpec struct {
	Name, Version, Instructions string
	Tools                       []ServerTool
}

// validate 在构造期检查限额与 schema，让问题在测试期暴露而不是运行时。
func (s *ServerSpec) validate() error {
	if s == nil {
		return errors.New("mcp: server spec is nil")
	}
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("mcp: server name is required")
	}
	if len(s.Tools) > MaxToolsPerServer {
		return fmt.Errorf("mcp: %d tools exceeds limit %d", len(s.Tools), MaxToolsPerServer)
	}
	seen := make(map[string]struct{}, len(s.Tools))
	for _, t := range s.Tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			return errors.New("mcp: tool name is required")
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("mcp: duplicate tool %q", name)
		}
		seen[name] = struct{}{}
		if t.Handler == nil {
			return fmt.Errorf("mcp: tool %q has no handler", name)
		}
		if len(t.InputSchema) == 0 {
			return fmt.Errorf("mcp: tool %q has no input schema", name)
		}
		if len(t.InputSchema) > MaxSchemaBytes {
			return fmt.Errorf("mcp: tool %q schema exceeds %d bytes", name, MaxSchemaBytes)
		}
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(t.InputSchema, &probe); err != nil {
			return fmt.Errorf("mcp: tool %q schema is not valid JSON: %w", name, err)
		}
		if probe.Type != "object" {
			return fmt.Errorf("mcp: tool %q schema must have type \"object\"", name)
		}
	}
	return nil
}

// buildServer 把 spec 转成 SDK Server。调用前必须已通过 validate。
func buildServer(spec *ServerSpec) (srv *sdkmcp.Server, err error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	// AddTool 对非法输入会 panic；validate 已拦截，这里兜底避免带崩进程。
	defer func() {
		if r := recover(); r != nil {
			srv, err = nil, fmt.Errorf("mcp: build server: %v", r)
		}
	}()
	version := spec.Version
	if version == "" {
		version = "1"
	}
	srv = sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: spec.Name, Version: version},
		&sdkmcp.ServerOptions{Instructions: spec.Instructions},
	)
	for _, t := range spec.Tools {
		srv.AddTool(&sdkmcp.Tool{
			Name:        strings.TrimSpace(t.Name),
			Description: t.Description,
			InputSchema: t.InputSchema,
			Annotations: toolAnnotations(t),
		}, toolHandler(t))
	}
	return srv, nil
}

func toolAnnotations(t ServerTool) *sdkmcp.ToolAnnotations {
	a := &sdkmcp.ToolAnnotations{ReadOnlyHint: t.ReadOnly}
	if !t.ReadOnly {
		// MCP 规范里 destructiveHint 缺省为 true，写工具必须显式给出。
		destructive := t.Destructive
		a.DestructiveHint = &destructive
	}
	return a
}

func toolHandler(t ServerTool) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args []byte
		if req != nil && req.Params != nil {
			args = req.Params.Arguments
		}
		res, err := t.Handler(ctx, args)
		if err != nil {
			return textResult(err.Error(), true), nil
		}
		return textResult(res.Text, res.IsError), nil
	}
}

func textResult(text string, isError bool) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
		IsError: isError,
	}
}

type serverCtxKey struct{}

// NewHTTPHandler 返回 stateless + JSONResponse 的 Streamable HTTP handler。
// resolve 按请求返回 spec（宿主在这里绑定项目与主体）；返回 nil 时回 404。
// spec 不合法（限额、schema）时回 500，错误细节只写入响应的 message，不含请求内容。
func NewHTTPHandler(resolve func(*http.Request) *ServerSpec) http.Handler {
	inner := sdkmcp.NewStreamableHTTPHandler(func(r *http.Request) *sdkmcp.Server {
		srv, _ := r.Context().Value(serverCtxKey{}).(*sdkmcp.Server)
		return srv
	}, &sdkmcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spec := resolve(r)
		if spec == nil {
			writeJSONError(w, http.StatusNotFound, "mcp server not found")
			return
		}
		srv, err := buildServer(spec)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxServerRequestBytes)
		inner.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), serverCtxKey{}, srv)))
	})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "message": message})
}

// memSession 在关闭客户端会话的同时回收服务端会话。
type memSession struct {
	Session
	server *sdkmcp.ServerSession
}

func (m *memSession) Close() error {
	err := m.Session.Close()
	// 客户端关闭后管道已断，服务端会话的关闭错误没有信息量。
	_ = m.server.Close()
	return err
}

// ConnectInMemory 用 InMemoryTransport 直连 spec，返回与 ConnectHTTP 同接口的 Session。
// 供宿主在进程内把内置 Server 交给云助手，不经网络、不受 SSRF 守卫影响。
func ConnectInMemory(ctx context.Context, serverID string, spec *ServerSpec) (Session, error) {
	srv, err := buildServer(spec)
	if err != nil {
		return nil, err
	}
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	// 服务端会话的生命周期跟随会话本身，不应随调用方（可能是一次请求）的 ctx 取消。
	ss, err := srv.Connect(context.WithoutCancel(ctx), serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: start in-memory server %s: %w", serverID, err)
	}
	sess, err := connect(ctx, serverID, clientTransport, 0)
	if err != nil {
		_ = ss.Close()
		return nil, err
	}
	return &memSession{Session: sess, server: ss}, nil
}
