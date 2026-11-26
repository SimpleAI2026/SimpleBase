// 文件说明：mcp.go — internal/agentbridge 中 MCPOptions、BuiltinMCP 等符号的实现
//
// 用法
//
//	type MCPOptions struct{…}  // MCPOptions 是连接器的构造参数
//	type BuiltinMCP interface{…}  // BuiltinMCP 由宿主 api 层实现：按主体与项目构造进程内内置 Server（planv6.7 §3.6）
//	type MCPConnector struct{…}  // MCPConnector 从系统库读项目配置并建立会话
//	NewMCPConnector(store *systemdb.Store, opts MCPOptions) *MCPConnector  // NewMCPConnector 构造连接器
//	TestMCPServer(ctx context.Context, srv systemdb.MCPServer, opts MCPOptions) ([]string, error)  // TestMCPServer 建立一次临时连接并列出工具名（API 的连通性测试用）
//	type MCPPolicy struct{…}  // MCPPolicy 从系统库的工具策略表判定只读/破坏性
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/mcp.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//	[关键引用] MCPOptions ← mcp_test.go；NewMCPConnector ← mcp_test.go；NewMCPPolicy ← mcp_test.go
//
// 主要符号
//
//	type MCPOptions struct{…}
//	type BuiltinMCP interface{…}
//	type MCPConnector struct{…}
//	NewMCPConnector(store *systemdb.Store, opts MCPOptions) *MCPConnector
//	(c *MCPConnector) SetBuiltin(b BuiltinMCP)
//	(c *MCPConnector) Connect(ctx context.Context, req cloudagent.RunRequest, serverIDs []string) (mcp.SessionSet,...
//	(c *MCPConnector) httpClient(srv systemdb.MCPServer) (*http.Client, error)
//	TestMCPServer(ctx context.Context, srv systemdb.MCPServer, opts MCPOptions) ([]string, error)
//	... 另有 4 个顶层符号

// mcp.go — 宿主侧 MCP 连接器实现（planv6.2 §4.2-5、§6-3）。
//
// 分工：模块（gocloudagent）只认 MCPConnector/MCPPolicy 两个接口，
// 凭据、网络出口策略、SSRF 防护全部留在宿主这一层。
//
// SSRF 是这里最关键的约束：SDK 的 StreamableClientTransport 不做任何防护，
// 也不能依赖它的默认 http.Client（会直连任意地址）。所以：
//   - 只允许 https（http 需显式开启）；
//   - 在 DialContext 的 Control 里校验**实际连接 IP**，拒绝回环/私网/链路本地/
//     元数据地址，同时挡住 DNS rebinding（解析结果与连接 IP 不一致也能拦）；
//   - 禁止跟随跨域重定向；
//   - 响应体大小受限。
package agentbridge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/gocloudagent/tools"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// MCPOptions 是连接器的构造参数。
type MCPOptions struct {
	// AllowHTTP 允许 http:// 端点；默认只允许 https。
	AllowHTTP bool
	// ConnectTimeout 为零时用 mcp.DefaultConnectTimeout。
	ConnectTimeout time.Duration
	// DisableExternal 为真时不连接用户配置的外部 Server，只服务内置 Server
	// （agent.mcp.enabled=false 且 agent.mcp.builtin=true 的场景）。
	DisableExternal bool
}

// BuiltinMCP 由宿主 api 层实现：按主体与项目构造进程内内置 Server（planv6.7 §3.6）。
// 接口定义在这里，agentbridge 因此不需要 import api。
type BuiltinMCP interface {
	// Spec 返回 name 对应的内置 Server。名称未注册、实例或项目开关关闭时 ok=false。
	// 返回的工具会以 p 的身份对宿主路由树发起子请求。
	Spec(ctx context.Context, p auth.Principal, projectID, name string) (spec *mcp.ServerSpec, ok bool)
	// Policy 按工具表判定只读/破坏性。ok=false 表示 name 不是内置 Server。
	// name 是内置 Server 但 tool 未登记时，返回 ok=true 且按写处理（fail-closed）。
	Policy(name, tool string) (decision cloudagent.ToolDecision, ok bool)
}

// MCPConnector 从系统库读项目配置并建立会话。
type MCPConnector struct {
	store   *systemdb.Store
	opts    MCPOptions
	builtin BuiltinMCP
}

// NewMCPConnector 构造连接器。store 为 nil 时 Connect 不连外部 Server。
func NewMCPConnector(store *systemdb.Store, opts MCPOptions) *MCPConnector {
	return &MCPConnector{store: store, opts: opts}
}

// SetBuiltin 注入内置 MCP 实现。api 在 app 里晚于 runtime 创建，所以用 setter，
// 必须在服务开始接收请求之前调用。
func (c *MCPConnector) SetBuiltin(b BuiltinMCP) {
	if c != nil {
		c.builtin = b
	}
}

// Connect 按 serverIDs（Server 名称）建立本轮可用的会话。
//
// 内置名优先：命中内置注册表时走进程内会话，不经过网络与 SSRF 守卫。
// 单个 Server 失败不终止 run：跳过它并继续，由调用方把提示交给模型。
// 全部失败时返回空集合（不是错误），只有配置读取失败才返回 error。
func (c *MCPConnector) Connect(ctx context.Context, req cloudagent.RunRequest, serverIDs []string) (mcp.SessionSet, error) {
	if c == nil || len(serverIDs) == 0 {
		return mcp.NewSessionSet(nil), nil
	}
	rc, err := hostContext(req.Context)
	if err != nil {
		return mcp.NewSessionSet(nil), err
	}
	principal := rc.Principal
	if !req.Agent.Writable {
		principal = stripWritePermissions(principal)
	}
	sessions := make([]mcp.Session, 0, len(serverIDs))
	for _, name := range serverIDs {
		if c.builtin != nil {
			// 进程内子请求必须带委托身份，委托边界（delegable）才会生效。
			principal.Delegated = true
			if spec, ok := c.builtin.Spec(ctx, principal, rc.ProjectID, name); ok {
				if sess, err := mcp.ConnectInMemory(ctx, name, spec); err == nil {
					sessions = append(sessions, sess)
				}
				continue
			}
		}
		if c.store == nil || c.opts.DisableExternal {
			continue
		}
		srv, err := c.store.GetMCPServerByName(ctx, rc.ProjectID, name)
		if err != nil || !srv.Enabled {
			// 未配置或已停用：跳过，不阻断 run。
			continue
		}
		client, err := c.httpClient(srv)
		if err != nil {
			continue
		}
		sess, err := mcp.ConnectHTTP(ctx, srv.Name, mcp.HTTPOptions{
			Endpoint:       strings.TrimSpace(srv.URL),
			HTTPClient:     client,
			ConnectTimeout: c.opts.ConnectTimeout,
		})
		if err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return mcp.NewSessionSet(sessions), nil
}

// httpClient 构造已加固的 *http.Client（Header 注入 + SSRF 防护都在这里）。
func (c *MCPConnector) httpClient(srv systemdb.MCPServer) (*http.Client, error) {
	raw := strings.TrimSpace(srv.URL)
	if raw == "" {
		return nil, errors.New("mcp: server url is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
	case "http":
		if !c.opts.AllowHTTP {
			return nil, errors.New("mcp: http endpoints are disabled")
		}
	default:
		return nil, fmt.Errorf("mcp: unsupported scheme %q", u.Scheme)
	}
	headers := http.Header{}
	for k, v := range systemdb.ParseMCPHeaders(srv.HeadersJSON) {
		headers.Set(k, v)
	}
	// 加固 Transport 已下沉到 gocloudagent/tools（planv6.8 §13.7）：
	// 不复用连接、TLS ≥ 1.2、拨号时校验实际 IP（挡 DNS rebinding）。
	transport := (tools.NetPolicy{}).NewGuardedTransport()
	return &http.Client{
		Transport: &mcp.HeaderTransport{Base: transport, Headers: headers, MaxResponseBytes: mcp.MaxEventBytes},
		Timeout:   time.Duration(srv.TimeoutS) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 禁止跟随重定向：SSRF 最常用的绕过手段。
			return http.ErrUseLastResponse
		},
	}, nil
}

// TestMCPServer 建立一次临时连接并列出工具名（API 的连通性测试用）。
//
// 调用方负责超时；这里只做「连上 + 列工具」，不缓存会话、不执行工具。
func TestMCPServer(ctx context.Context, srv systemdb.MCPServer, opts MCPOptions) ([]string, error) {
	c := &MCPConnector{opts: opts}
	client, err := c.httpClient(srv)
	if err != nil {
		return nil, err
	}
	sess, err := mcp.ConnectHTTP(ctx, srv.Name, mcp.HTTPOptions{
		Endpoint:       strings.TrimSpace(srv.URL),
		HTTPClient:     client,
		ConnectTimeout: opts.ConnectTimeout,
	})
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	infos, err := sess.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(infos))
	for _, info := range infos {
		out = append(out, info.Name)
	}
	return out, nil
}

// isBlockedIP 判定已下沉到 gocloudagent/tools.IsBlockedIP（planv6.8 §13.7），
// 宿主不再保留副本；表驱动断言见 gocloudagent/tools 包的 netguard 测试。

// MCPPolicy 从系统库的工具策略表判定只读/破坏性。
//
// 未标记的工具一律按「写」处理（fail-closed）：Server 自报的
// annotations 不可信，只有宿主存的标记生效。
type MCPPolicy struct {
	store   *systemdb.Store
	builtin BuiltinMCP
}

// NewMCPPolicy 构造策略实现。
func NewMCPPolicy(store *systemdb.Store) *MCPPolicy {
	return &MCPPolicy{store: store}
}

// SetBuiltin 注入内置 MCP 实现，用于判定内置工具的读写属性。
func (p *MCPPolicy) SetBuiltin(b BuiltinMCP) {
	if p != nil {
		p.builtin = b
	}
}

// Classify 实现 cloudagent.MCPPolicy。
//
// 事实来源是 sys_mcp_servers.tools_policy_json：Server 自报的
// annotations 只作配置向导的默认建议。未标记的工具一律按「写」处理。
func (p *MCPPolicy) Classify(ctx context.Context, req cloudagent.RunRequest, serverID, tool string) (cloudagent.ToolDecision, error) {
	rc, err := hostContext(req.Context)
	if err != nil {
		return cloudagent.ToolDecision{}, err
	}
	// 内置 Server 的事实来源是工具表，不读 tools_policy_json。
	if p != nil && p.builtin != nil {
		if d, ok := p.builtin.Policy(serverID, tool); ok {
			return d, nil
		}
	}
	if p == nil || p.store == nil {
		return cloudagent.ToolDecision{}, errors.New("mcp: policy store is not configured")
	}
	srv, err := p.store.GetMCPServerByName(ctx, rc.ProjectID, serverID)
	if err != nil {
		// Server 未登记 = 策略缺失，调用方必须拒绝执行（fail-closed）。
		return cloudagent.ToolDecision{}, fmt.Errorf("mcp: server %q is not configured", serverID)
	}
	policies := systemdb.ParseMCPToolPolicies(srv.ToolsPolicyJSON)
	pol, ok := policies[tool]
	if !ok {
		// 未标记：按写处理，且不认定为「破坏性」（走普通写权限校验即可）。
		return cloudagent.ToolDecision{ReadOnly: false, Destructive: false}, nil
	}
	return cloudagent.ToolDecision{ReadOnly: pol.ReadOnly, Destructive: pol.Destructive}, nil
}
