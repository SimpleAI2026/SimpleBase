// 文件说明：e2b_terminal.go — internal/api 中 TerminalBridge、TerminalTicket 等符号的实现
//
// 用法
//
//	NewSSHAuthenticator(h *SandboxHandler) sshd.Authenticator
//	(h *SandboxHandler) TerminalTicket(c echo.Context) error  // TerminalTicket 对应 POST /e2b/ext/sandboxes/{id}/terminal-tickets
//	(h *SandboxHandler) Terminal(c echo.Context) error  // Terminal 对应 GET /e2b/ext/sandboxes/{id}/terminal?ticket=...：WebSocket 升级
//	同包调用：Terminal() → router.go, sandbox_handler.go；TerminalBridge → router.go；TerminalTicket() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/e2b_terminal.go
//	         │ 依赖
//	         ▼
//	[下游] internal/auth, internal/sandbox/sshd
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Terminal() ← router.go, sandbox_handler.go
//
// 主要符号
//
//	type TerminalBridge struct {…}
//	(h *SandboxHandler) TerminalTicket(c echo.Context) error
//	(h *SandboxHandler) Terminal(c echo.Context) error
//	NewSSHAuthenticator(h *SandboxHandler) sshd.Authenticator
//	... 另有 16 个顶层符号

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/sandbox/sshd"
)

// TerminalBridge 把浏览器的 WebSocket 接到进程内 SSH 服务端：
// WebSocket ⇄ 内存连接 ⇄ ssh.Client ⇄（NewPipe）⇄ sshd.Server.ServeConn。
//
// 它由 app 在装配 SSH 服务端后注入；为 nil 时终端相关的 ext 路由不挂载，
// capabilities.terminal 随之为 false。
type TerminalBridge struct {
	Server *sshd.Server
	// creds 是浏览器终端的进程内 SSH 客户端用的短时凭据。它挂在桥上而不是 handler 上，
	// 因为 SSH 服务端的认证器与签发凭据的 WebSocket handler 是两个 SandboxHandler 实例，
	// 必须共享同一份存储。
	// Fingerprint 是主机密钥指纹，信息页展示。
	Fingerprint string
	// SSHListen 是对外 SSH 端口的监听地址（空表示未开放）。
	SSHListen string

	credsOnce sync.Once
	creds     *ticketStore
}

// credStore 懒创建共享的桥接凭据存储。
func (b *TerminalBridge) credStore() *ticketStore {
	b.credsOnce.Do(func() { b.creds = newTicketStore() })
	return b.creds
}

// terminalTicketSecret 是进程内 SSH 客户端用来登录的「密码」前缀，
// 与真实 API Key 区分：认证器据此走票据校验分支，而不是把票据当 API Key 查库。
const ticketPasswordPrefix = "ticket:"

// ---- ext 路由：签发票据 ----

// TerminalTicket 对应 POST /e2b/ext/sandboxes/{id}/terminal-tickets。
// 走普通鉴权（写权限、实例可写、非系统项目、沙盒归属），换取一张 30 秒、单次使用的票据。
func (h *SandboxHandler) TerminalTicket(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if h.deps.Terminal == nil || !h.svc.Interactive() {
		return writeE2BErr(c, errTerminalUnsupported)
	}
	id := c.Param("sandboxID")
	row, err := h.svc.Get(c.Request().Context(), p, id, false)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if row.Source == "agent" {
		return writeE2BErr(c, NewAPIError(http.StatusConflict, "sandbox_busy", "agent sandboxes are read-only", ""))
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	owner := principal.UserID
	if owner == "" {
		owner = principal.APIKeyID
	}
	tok, ok := h.tickets.Issue(terminalTicket{PrincipalID: owner, ProjectID: p, SandboxID: id}, h.now())
	if !ok {
		c.Response().Header().Set("Retry-After", "5")
		return writeE2BErr(c, NewAPIError(http.StatusTooManyRequests, "too_many_tickets", "too many pending terminal tickets", ""))
	}
	h.record(c, p, id, "terminal.ticket", "")
	return c.JSON(http.StatusOK, map[string]any{"ticket": tok, "expires_in": int(ticketTTL.Seconds())})
}

var errTerminalUnsupported = NewAPIError(http.StatusNotImplemented, "unsupported_feature", "terminal is not available on this backend", "")

// wsFrame 是读到的一帧，保留 WebSocket 帧类型：文本帧是 JSON 控制，二进制帧是键盘输入。
// 不能按内容首字节判断：用户键入的第一个字符可能就是 "{"。
type wsFrame struct {
	Text bool
	Data []byte
}

var wsFrameCodec = websocket.Codec{
	Marshal: func(v interface{}) ([]byte, byte, error) {
		f := v.(wsFrame)
		if f.Text {
			return f.Data, websocket.TextFrame, nil
		}
		return f.Data, websocket.BinaryFrame, nil
	},
	Unmarshal: func(data []byte, payloadType byte, v interface{}) error {
		f := v.(*wsFrame)
		f.Text = payloadType == websocket.TextFrame
		f.Data = append([]byte(nil), data...)
		return nil
	},
}

// ---- ext 路由：WebSocket 升级 ----

// 控制帧（文本帧，JSON）。二进制帧是终端数据。
type termControl struct {
	Type string `json:"type"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Code int    `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
}

const (
	maxWSFrame      = 64 << 10
	termDialTimeout = 10 * time.Second
)

// Terminal 对应 GET /e2b/ext/sandboxes/{id}/terminal?ticket=...：WebSocket 升级。
//
// 不走 Authorization：浏览器的 WebSocket 不能自定义头，身份由一次性票据证明。
// 因此本路由 *不* 经过 authMW，必须自己完成：同源校验、票据校验（立即作废）、项目 / 沙盒绑定。
func (h *SandboxHandler) Terminal(c echo.Context) error {
	if h.deps.Terminal == nil || !h.svc.Interactive() {
		return writeE2BErr(c, errTerminalUnsupported)
	}
	req := c.Request()
	// 同源：不同源直接 403，防跨站 WebSocket 劫持。缺失 Origin（非浏览器）同样拒绝：
	// 非浏览器客户端应使用真实 SSH 端口，而不是绕过票据与同源。
	if !sameOrigin(req) {
		return writeE2BErr(c, auth.ErrForbidden)
	}
	id := c.Param("sandboxID")
	tok := req.URL.Query().Get("ticket")
	// 票据只在 query 里出现这一次；立即从 URL 上抹掉，避免后续日志 / 错误处理读到它。
	req.URL.RawQuery = ""
	t, ok := h.tickets.RedeemFor(tok, id, h.now())
	if !ok {
		return writeE2BErr(c, auth.ErrInvalidCredentials)
	}
	// 重新用票据里的项目确认沙盒仍然存在且可用。
	row, err := h.svc.Get(req.Context(), t.ProjectID, id, false)
	if err != nil || row.Source == "agent" {
		return writeE2BErr(c, auth.ErrInvalidCredentials)
	}

	beginSSE(c) // 解除 WriteTimeout：Hijack 之后 http.Server 的 deadline 不会自动清除。
	srv := websocket.Server{
		Handshake: func(*websocket.Config, *http.Request) error { return nil }, // 同源已在上面校验
		Handler: func(ws *websocket.Conn) {
			ws.MaxPayloadBytes = maxWSFrame
			h.bridge(req.Context(), ws, t, c.RealIP())
		},
	}
	srv.ServeHTTP(c.Response(), req)
	return nil
}

// sameOrigin 校验 Origin 与请求 Host 一致。
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return false
	}
	u, err := url.Parse(o)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// bridge 在一条已升级的 WebSocket 上建立 SSH 会话并双向转发。
func (h *SandboxHandler) bridge(parent context.Context, ws *websocket.Conn, t terminalTicket, remoteIP string) {
	defer ws.Close()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	cc, sc := sshd.NewPipe()
	// 进程内连接用真实客户端 IP 作为来源，限速才有意义。
	go h.deps.Terminal.Server.ServeConn(sshd.WithRemote(sc, remoteIP+":0"))
	// 一次性票据兑换成短时登录凭据：SSH 认证器只认 ticket: 前缀 + 绑定的 (项目, 沙盒)。
	pw := ticketPasswordPrefix + h.mintBridgeCredential(t)
	conn, chans, reqs, err := ssh.NewClientConn(cc, "sandbox", &ssh.ClientConfig{
		User: t.SandboxID, Auth: []ssh.AuthMethod{ssh.Password(pw)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 进程内连接，没有网络可被中间人
		Timeout:         termDialTimeout,
	})
	if err != nil {
		if h.deps.Logger != nil {
			h.deps.Logger.Warn("sandbox terminal handshake failed", fieldString("err", err.Error()))
		}
		sendControl(ws, termControl{Type: "error", Msg: "cannot open a session"})
		return
	}
	cl := ssh.NewClient(conn, chans, reqs)
	defer cl.Close()
	sess, err := cl.NewSession()
	if err != nil {
		sendControl(ws, termControl{Type: "error", Msg: "cannot open a session"})
		return
	}
	defer sess.Close()

	cols, rows := 80, 24
	if err := sess.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{}); err != nil {
		sendControl(ws, termControl{Type: "error", Msg: "cannot allocate a terminal"})
		return
	}
	stdin, _ := sess.StdinPipe()
	stdout, _ := sess.StdoutPipe()
	if err := sess.Shell(); err != nil {
		sendControl(ws, termControl{Type: "error", Msg: "cannot start a shell"})
		return
	}

	var wmu sync.Mutex // 终端输出与控制帧会并发写同一个 WebSocket
	writeBin := func(b []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		return websocket.Message.Send(ws, b)
	}

	// 进程输出 → 浏览器（二进制帧）。
	outDone := make(chan struct{})
	go func() {
		defer close(outDone)
		buf := make([]byte, 16<<10)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				if werr := writeBin(buf[:n]); werr != nil {
					cancel()
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	// 会话结束 → 通知浏览器并断开。
	go func() {
		err := sess.Wait()
		<-outDone
		code := 0
		var ee *ssh.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitStatus()
		}
		wmu.Lock()
		_ = websocket.JSON.Send(ws, termControl{Type: "exit", Code: code})
		wmu.Unlock()
		cancel()
		ws.Close()
	}()
	// 连接断开（含 ctx 取消）→ 关闭 SSH 会话。
	go func() { <-ctx.Done(); sess.Close(); cl.Close() }()

	// 浏览器 → 进程：二进制帧是键盘输入，文本帧是 JSON 控制。
	for {
		var f wsFrame
		if err := wsFrameCodec.Receive(ws, &f); err != nil {
			return
		}
		if len(f.Data) == 0 {
			continue
		}
		if f.Text {
			var ctl termControl
			if json.Unmarshal(f.Data, &ctl) == nil && ctl.Type == "resize" &&
				ctl.Cols > 0 && ctl.Rows > 0 && ctl.Cols <= 1000 && ctl.Rows <= 1000 {
				_ = sess.WindowChange(ctl.Rows, ctl.Cols)
			}
			continue
		}
		if _, err := stdin.Write(f.Data); err != nil {
			return
		}
	}
}

func sendControl(ws *websocket.Conn, ctl termControl) { _ = websocket.JSON.Send(ws, ctl) }

// ---- SSH 认证器 ----

// sshAuthenticator 实现 sshd.Authenticator。两种凭据：
//
//   - 用户名 = 沙盒 ID，密码 = SimpleBase API Key（真实 ssh 客户端）；
//   - 用户名 = 沙盒 ID，密码 = "ticket:" + 桥接凭据（浏览器终端的进程内客户端）。
//
// 沙盒不存在、无权限、密钥错误对外是同一种失败。
type sshAuthenticator struct {
	h *SandboxHandler
}

var errSSHDenied = errors.New("denied")

// NewSSHAuthenticator 构造 SSH 认证器。
func NewSSHAuthenticator(h *SandboxHandler) sshd.Authenticator { return &sshAuthenticator{h: h} }

func (a *sshAuthenticator) Authenticate(ctx context.Context, username, secret string, _ net.Addr) (sshd.Identity, error) {
	h := a.h
	if strings.HasPrefix(secret, ticketPasswordPrefix) {
		t, ok := h.redeemBridgeCredential(strings.TrimPrefix(secret, ticketPasswordPrefix), username)
		if !ok {
			return sshd.Identity{}, errSSHDenied
		}
		return sshd.Identity{ProjectID: t.ProjectID, SandboxID: t.SandboxID, Actor: t.PrincipalID}, nil
	}
	row, err := h.svc.GetByID(ctx, username)
	if err != nil {
		// 不存在：仍然走一遍密钥校验，让响应时间与「存在但密钥错」接近，避免计时侧信道。
		_, _ = h.deps.Auth.Authenticate(ctx, secret)
		return sshd.Identity{}, errSSHDenied
	}
	p, err := h.deps.Auth.Authenticate(ctx, secret)
	if err != nil {
		return sshd.Identity{}, errSSHDenied
	}
	if !p.HasPermission(auth.DatabaseWrite) || !h.writable || row.Source == "agent" {
		return sshd.Identity{}, errSSHDenied
	}
	tenantID, err := h.deps.Catalog.ResolveProjectTenant(ctx, row.ProjectID)
	if err != nil || !auth.CheckProjectAccess(p, row.ProjectID, tenantID, false) {
		return sshd.Identity{}, errSSHDenied
	}
	return sshd.Identity{ProjectID: row.ProjectID, SandboxID: row.ID, Actor: p.APIKeyID}, nil
}

// ---- 桥接凭据 ----
//
// 浏览器终端的进程内 SSH 客户端用它登录。它不是 API Key，也不是原始票据（原始票据已被兑换并作废）：
// 是同一进程里生成的、单次使用的随机串，只在这条内存连接上存在，不经过网络。

func (h *SandboxHandler) mintBridgeCredential(t terminalTicket) string {
	tok, _ := h.deps.Terminal.credStore().Issue(t, h.now())
	return tok
}

func (h *SandboxHandler) redeemBridgeCredential(tok, sandboxID string) (terminalTicket, bool) {
	if h.deps.Terminal == nil {
		return terminalTicket{}, false
	}
	return h.deps.Terminal.credStore().RedeemFor(tok, sandboxID, h.now())
}
