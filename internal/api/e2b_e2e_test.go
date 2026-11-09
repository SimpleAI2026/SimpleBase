package api

// e2b_e2e_test.go：用真实的 sandbox.Manager（fake 驱动）走完整链路，覆盖 fakeRouterSandbox 够不到的部分：
//   - process.Start 的 Connect 流式输出、Connect、SendInput、SendSignal、List；
//   - 浏览器终端：票据签发 → WebSocket 升级 → 进程内 SSH → 沙盒 PTY；
//   - 票据的单次使用、过期、同源校验、不能跨沙盒。

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm/fake"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/e2bproto"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/sandbox/sshd"
)

// e2eStore 是带真实读写语义的内存 Store。
type e2eStore struct {
	mu   sync.Mutex
	rows map[string]sandbox.Sandbox
}

func (s *e2eStore) CreateSandbox(_ context.Context, sb sandbox.Sandbox) (sandbox.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[sb.ID] = sb
	return sb, nil
}
func (s *e2eStore) GetSandbox(_ context.Context, p, id string) (sandbox.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rows[id]; ok && r.ProjectID == p && r.DeletedAt == nil {
		return r, nil
	}
	return sandbox.Sandbox{}, sql.ErrNoRows
}
func (s *e2eStore) GetSandboxByID(_ context.Context, id string) (sandbox.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rows[id]; ok && r.DeletedAt == nil {
		return r, nil
	}
	return sandbox.Sandbox{}, sql.ErrNoRows
}
func (s *e2eStore) GetSandboxByName(_ context.Context, p, n string) (sandbox.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rows {
		if r.ProjectID == p && r.Name == n && r.DeletedAt == nil {
			return r, nil
		}
	}
	return sandbox.Sandbox{}, sql.ErrNoRows
}
func (s *e2eStore) GetSandboxByThread(context.Context, string, string) (sandbox.Sandbox, error) {
	return sandbox.Sandbox{}, sql.ErrNoRows
}
func (s *e2eStore) ListSandboxes(_ context.Context, p, _, _, _ string, _ int) ([]sandbox.Sandbox, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []sandbox.Sandbox
	for _, r := range s.rows {
		if r.ProjectID == p && r.DeletedAt == nil {
			out = append(out, r)
		}
	}
	return out, "", nil
}
func (s *e2eStore) CountSandboxes(ctx context.Context, p string) (int, error) {
	l, _, _ := s.ListSandboxes(ctx, p, "", "", "", 0)
	return len(l), nil
}
func (s *e2eStore) UpdateSandbox(_ context.Context, sb sandbox.Sandbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[sb.ID] = sb
	return nil
}
func (s *e2eStore) SoftDeleteSandbox(_ context.Context, _, id, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rows[id]
	now := time.Now()
	r.DeletedAt = &now
	s.rows[id] = r
	return nil
}
func (s *e2eStore) ListExpiredSandboxes(context.Context, time.Time, int) ([]sandbox.Sandbox, error) {
	return nil, nil
}
func (s *e2eStore) ListStaleRunSandboxes(context.Context, time.Time, int) ([]sandbox.Sandbox, error) {
	return nil, nil
}
func (s *e2eStore) ListFailedSandboxRemovals(context.Context, int) ([]sandbox.Sandbox, error) {
	return nil, nil
}
func (s *e2eStore) PurgeDeletedSandboxes(context.Context, time.Time) (int64, error) { return 0, nil }

type e2eEnv struct {
	srv  *httptest.Server
	key  string
	mgr  *sandbox.Manager
	deps Dependencies
	sb   sandbox.Sandbox
}

func newE2E(t *testing.T) *e2eEnv {
	t.Helper()
	deps, key := fullRouterDeps(t)
	cfg := config.SandboxConfig{Enabled: true, Workdir: "/workspace", Image: "host", CPUs: 1, MemoryMiB: 256,
		Network: "none", IdleTimeout: 5 * time.Minute, MaxDuration: time.Hour, MaxPerProject: 5,
		ExecTimeout: time.Second, ExecTimeoutMax: time.Minute, MaxFileBytes: 4096, MaxOutputBytes: 4096,
		Terminal: config.SandboxTerminalConfig{Enabled: true, MaxSessionsPerSandbox: 3, IdleTimeout: time.Minute}}
	deps.Config.Sandbox = cfg
	mgr := sandbox.NewManager(cfg, &e2eStore{rows: map[string]sandbox.Sandbox{}}, fake.New())
	deps.Sandbox = mgr
	// 终端桥：与 app 里的装配顺序一致——先建 handler 需要的认证器，再建 SSH 服务端。
	hostKey, err := sshd.LoadOrCreateHostKey(t.TempDir() + "/hk")
	if err != nil {
		t.Fatal(err)
	}
	bridge := &TerminalBridge{Fingerprint: sshd.Fingerprint(hostKey)}
	deps.Terminal = bridge
	h := NewSandboxHandler(mgr, true, deps.Audit, deps.System, nil, deps)
	srv, err := sshd.New(sshd.Options{HostKey: hostKey, Authenticator: NewSSHAuthenticator(h), Sessions: mgr})
	if err != nil {
		t.Fatal(err)
	}
	bridge.Server = srv
	e := NewRouter(deps)
	ts := httptest.NewServer(e)
	t.Cleanup(func() { ts.Close(); _ = srv.Close(); _ = mgr.Close() })
	sb, err := mgr.Create(context.Background(), "proj-1", sandbox.CreateInput{Name: "e2e", Start: true}, "u")
	if err != nil {
		t.Fatal(err)
	}
	return &e2eEnv{srv: ts, key: key, mgr: mgr, deps: deps, sb: sb}
}

func (v *e2eEnv) post(t *testing.T, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", v.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", v.key)
	req.Header.Set("E2b-Sandbox-Id", v.sb.ID)
	for k, val := range hdr {
		req.Header.Set(k, val)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// readEvents 读 Connect 流，直到 end-stream 或超时，返回所有事件。
func readEvents(t *testing.T, r io.Reader) ([]e2bproto.ProcessEvent, *e2bproto.Error) {
	t.Helper()
	type result struct {
		evs []e2bproto.ProcessEvent
		end *e2bproto.Error
		err error
	}
	done := make(chan result, 1)
	go func() {
		var res result
		for {
			f, err := e2bproto.ReadFrame(r)
			if err != nil {
				res.err = err
				break
			}
			if f.EndStream {
				var end struct {
					Error *e2bproto.Error `json:"error"`
				}
				_ = json.Unmarshal(f.Body, &end)
				res.end = end.Error
				break
			}
			var ev e2bproto.StartResponse
			_ = json.Unmarshal(f.Body, &ev)
			res.evs = append(res.evs, ev.Event)
		}
		done <- res
	}()
	select {
	case res := <-done:
		if res.err != nil && res.err != io.EOF {
			t.Fatalf("stream error: %v", res.err)
		}
		return res.evs, res.end
	case <-time.After(5 * time.Second):
		t.Fatal("event stream did not finish")
		return nil, nil
	}
}

func envelope(t *testing.T, v any) string {
	t.Helper()
	var b strings.Builder
	if err := e2bproto.WriteFrame(&b, v); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// ---- process.Start 流式 ----

func TestProcessStartStreamsOutputAndExit(t *testing.T) {
	v := newE2E(t)
	body := envelope(t, e2bproto.StartRequest{Process: e2bproto.ProcessConfig{Cmd: "echo", Args: []string{"hello", "world"}}})
	resp := v.post(t, "/process.Process/Start", body, map[string]string{"Content-Type": e2bproto.ContentTypeStream, "Connect-Protocol-Version": "1"})
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), e2bproto.ContentTypeStream) {
		t.Fatalf("status %d content-type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	evs, end := readEvents(t, resp.Body)
	if end != nil {
		t.Fatalf("stream ended with an error: %+v", end)
	}
	if len(evs) < 3 || evs[0].Start == nil || evs[0].Start.PID == 0 {
		t.Fatalf("first event must be start{pid}: %+v", evs)
	}
	var out strings.Builder
	for _, ev := range evs {
		if ev.Data != nil {
			out.Write(ev.Data.Stdout)
		}
	}
	if strings.TrimSpace(out.String()) != "hello world" {
		t.Fatalf("stdout = %q", out.String())
	}
	last := evs[len(evs)-1]
	if last.End == nil || !last.End.Exited || last.End.ExitCode != 0 || last.End.Status == "" {
		t.Fatalf("last event must be end{exitCode:0}: %+v", last)
	}
}

func TestProcessStartExitCodeAndJSONRequestForm(t *testing.T) {
	v := newE2E(t)
	// 服务端流的请求体也可以是裸 JSON（content-type: application/json）。
	resp := v.post(t, "/process.Process/Start", `{"process":{"cmd":"exit","args":["7"]}}`, nil)
	defer resp.Body.Close()
	evs, _ := readEvents(t, resp.Body)
	last := evs[len(evs)-1]
	if last.End == nil || last.End.ExitCode != 7 {
		t.Fatalf("exit code: %+v", last)
	}
}

func TestProcessBackgroundTagConnectInputSignalList(t *testing.T) {
	v := newE2E(t)
	// 带 tag 的 PTY 进程：交互 shell。
	resp := v.post(t, "/process.Process/Start", `{"process":{"cmd":"/bin/sh"},"pty":{"size":{"cols":80,"rows":24}},"tag":"bg1"}`, nil)
	first := make([]byte, 64)
	n, _ := resp.Body.Read(first) // 先读到 start 事件，确认进程已起
	if n == 0 {
		t.Fatal("no start event")
	}
	resp.Body.Close() // 客户端断开：带 tag 的进程必须继续运行

	time.Sleep(100 * time.Millisecond)
	if got := v.mgr.Procs(v.sb.ID); len(got) != 1 || got[0].Tag != "bg1" {
		t.Fatalf("a tagged process must outlive its Start stream: %+v", got)
	}
	r := v.post(t, "/process.Process/List", `{}`, nil)
	var list e2bproto.ListResponse
	_ = json.NewDecoder(r.Body).Decode(&list)
	r.Body.Close()
	if len(list.Processes) != 1 || list.Processes[0].Tag == nil || *list.Processes[0].Tag != "bg1" {
		t.Fatalf("List = %+v", list)
	}

	// 重新 Connect，并经 SendInput 驱动它。
	conn := v.post(t, "/process.Process/Connect", `{"process":{"tag":"bg1"}}`, nil)
	defer conn.Body.Close()
	input := base64.StdEncoding.EncodeToString([]byte("echo from-input\n"))
	si := v.post(t, "/process.Process/SendInput", `{"process":{"tag":"bg1"},"input":{"pty":"`+input+`"}}`, nil)
	if si.StatusCode != 200 {
		t.Fatalf("SendInput: %d", si.StatusCode)
	}
	si.Body.Close()
	// 逐帧解码 Connect 流，把 data 事件里的 base64 还原后再找输出。
	var got strings.Builder
	found := make(chan struct{})
	go func() {
		defer close(found)
		for {
			f, err := e2bproto.ReadFrame(conn.Body)
			if err != nil || f.EndStream {
				return
			}
			var ev e2bproto.StartResponse
			if json.Unmarshal(f.Body, &ev) == nil && ev.Event.Data != nil {
				got.Write(ev.Event.Data.Stdout)
				got.Write(ev.Event.Data.PTY)
				if strings.Contains(got.String(), "from-input") {
					return
				}
			}
		}
	}()
	select {
	case <-found:
	case <-time.After(4 * time.Second):
	}
	if !strings.Contains(got.String(), "from-input") {
		t.Fatalf("Connect did not stream the new output: %q", got.String())
	}

	// SIGHUP 被拒；SIGTERM 终止，流以 end 事件收尾。
	bad := v.post(t, "/process.Process/SendSignal", `{"process":{"tag":"bg1"},"signal":"SIGNAL_UNSPECIFIED"}`, nil)
	if bad.StatusCode != 400 {
		t.Fatalf("unspecified signal: %d", bad.StatusCode)
	}
	bad.Body.Close()
	sg := v.post(t, "/process.Process/SendSignal", `{"process":{"tag":"bg1"},"signal":"SIGNAL_SIGTERM"}`, nil)
	if sg.StatusCode != 200 {
		t.Fatalf("SendSignal: %d", sg.StatusCode)
	}
	sg.Body.Close()
	deadline := time.Now().Add(4 * time.Second)
	for v.mgr.ProcCount(v.sb.ID) != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if v.mgr.ProcCount(v.sb.ID) != 0 {
		t.Fatal("process survived SIGTERM")
	}
}

// 没有 tag 的前台进程：客户端断开就终止，不留孤儿。
func TestUntaggedProcessKilledWhenClientDisconnects(t *testing.T) {
	v := newE2E(t)
	resp := v.post(t, "/process.Process/Start", `{"process":{"cmd":"/bin/sh"},"pty":{"size":{"cols":80,"rows":24}}}`, nil)
	buf := make([]byte, 64)
	_, _ = resp.Body.Read(buf)
	if v.mgr.ProcCount(v.sb.ID) != 1 {
		t.Fatalf("expected one running process, got %d", v.mgr.ProcCount(v.sb.ID))
	}
	resp.Body.Close()
	deadline := time.Now().Add(4 * time.Second)
	for v.mgr.ProcCount(v.sb.ID) != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if v.mgr.ProcCount(v.sb.ID) != 0 {
		t.Fatal("an untagged process must be reaped when its client goes away")
	}
}

// ---- 浏览器终端 ----

func (v *e2eEnv) ticket(t *testing.T) string {
	t.Helper()
	resp := v.post(t, "/e2b/ext/sandboxes/"+v.sb.ID+"/terminal-tickets", "", nil)
	defer resp.Body.Close()
	var out struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expires_in"`
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("ticket: %d %s", resp.StatusCode, b)
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Ticket == "" || out.ExpiresIn != 30 {
		t.Fatalf("ticket = %+v", out)
	}
	return out.Ticket
}

func (v *e2eEnv) dialWS(t *testing.T, id, ticket, origin string) (*websocket.Conn, error) {
	t.Helper()
	cfg, err := websocket.NewConfig("ws"+strings.TrimPrefix(v.srv.URL, "http")+"/e2b/ext/sandboxes/"+id+"/terminal?ticket="+ticket, origin)
	if err != nil {
		t.Fatal(err)
	}
	return websocket.DialConfig(cfg)
}

func wsReadUntil(t *testing.T, ws *websocket.Conn, want string) string {
	t.Helper()
	var sb strings.Builder
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(sb.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %q; got %q", want, sb.String())
		}
		_ = ws.SetReadDeadline(time.Now().Add(time.Second))
		var f wsFrame
		if err := wsFrameCodec.Receive(ws, &f); err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				continue
			}
			t.Fatalf("read: %v (so far %q)", err, sb.String())
		}
		sb.Write(f.Data)
	}
	return sb.String()
}

func TestBrowserTerminalEndToEnd(t *testing.T) {
	v := newE2E(t)
	ws, err := v.dialWS(t, v.sb.ID, v.ticket(t), v.srv.URL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	// 键盘输入（二进制帧）→ shell → 输出（二进制帧）。
	if err := websocket.Message.Send(ws, []byte("echo hello-terminal\n")); err != nil {
		t.Fatal(err)
	}
	wsReadUntil(t, ws, "hello-terminal")

	// 以 "{" 开头的键盘输入不能被当成控制帧：靠帧类型而不是内容判断。
	if err := websocket.Message.Send(ws, []byte(`{"type":"resize","cols":10,"rows":10}`+"\n")); err != nil {
		t.Fatal(err)
	}
	wsReadUntil(t, ws, "not found") // 被 shell 当作命令执行，说明它走了键盘通道

	// 文本帧 resize 被接受（不报错、连接保持）。
	if err := websocket.Message.Send(ws, `{"type":"resize","cols":120,"rows":40}`); err != nil {
		t.Fatal(err)
	}
	// 退出码经控制帧回传。
	websocket.Message.Send(ws, []byte("exit 3\n"))
	var gotExit bool
	deadline := time.Now().Add(5 * time.Second)
	for !gotExit && time.Now().Before(deadline) {
		_ = ws.SetReadDeadline(time.Now().Add(time.Second))
		var f wsFrame
		if err := wsFrameCodec.Receive(ws, &f); err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				continue
			}
			break
		}
		if f.Text {
			var ctl termControl
			if json.Unmarshal(f.Data, &ctl) == nil && ctl.Type == "exit" {
				gotExit = ctl.Code == 3
				break
			}
		}
	}
	if !gotExit {
		t.Fatal("expected an exit control frame with code 3")
	}
	// 会话结束后进程表清空。
	deadline = time.Now().Add(3 * time.Second)
	for v.mgr.ProcCount(v.sb.ID) != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := v.mgr.ProcCount(v.sb.ID); n != 0 {
		t.Fatalf("terminal left %d process(es) behind", n)
	}
}

func TestTerminalTicketIsSingleUse(t *testing.T) {
	v := newE2E(t)
	tk := v.ticket(t)
	ws, err := v.dialWS(t, v.sb.ID, tk, v.srv.URL)
	if err != nil {
		t.Fatalf("first use: %v", err)
	}
	ws.Close()
	if ws2, err := v.dialWS(t, v.sb.ID, tk, v.srv.URL); err == nil {
		ws2.Close()
		t.Fatal("a ticket must work exactly once")
	}
}

func TestTerminalRejectsBadOriginBadTicketAndOtherSandbox(t *testing.T) {
	v := newE2E(t)
	// 跨站来源。
	if ws, err := v.dialWS(t, v.sb.ID, v.ticket(t), "http://evil.example"); err == nil {
		ws.Close()
		t.Fatal("cross-origin WebSocket must be refused")
	}
	// 随机票据。
	if ws, err := v.dialWS(t, v.sb.ID, "not-a-ticket", v.srv.URL); err == nil {
		ws.Close()
		t.Fatal("unknown ticket accepted")
	}
	// 票据绑定沙盒：拿 A 的票据去连 B。
	other, err := v.mgr.Create(context.Background(), "proj-1", sandbox.CreateInput{Name: "other", Start: true}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if ws, err := v.dialWS(t, other.ID, v.ticket(t), v.srv.URL); err == nil {
		ws.Close()
		t.Fatal("a ticket for one sandbox must not open another")
	}
	// 缺 Origin（非浏览器）同样拒绝。
	req, _ := http.NewRequest("GET", v.srv.URL+"/e2b/ext/sandboxes/"+v.sb.ID+"/terminal?ticket="+v.ticket(t), nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("missing Origin: want 403, got %d", resp.StatusCode)
	}
}

func TestTerminalTicketRequiresWriteAndNotAgentSource(t *testing.T) {
	v := newE2E(t)
	// 没有凭据。
	req, _ := http.NewRequest("POST", v.srv.URL+"/e2b/ext/sandboxes/"+v.sb.ID+"/terminal-tickets", nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("no credentials: %d", resp.StatusCode)
	}
	// agent 来源的沙盒是只读的。
	row, _ := v.mgr.GetByID(context.Background(), v.sb.ID)
	_ = row
	ag, _ := v.mgr.Create(context.Background(), "proj-1", sandbox.CreateInput{Name: "agentbox", Start: true, Source: "agent"}, "u")
	r := v.post(t, "/e2b/ext/sandboxes/"+ag.ID+"/terminal-tickets", "", nil)
	r.Body.Close()
	if r.StatusCode != 409 {
		t.Fatalf("agent sandbox ticket: want 409, got %d", r.StatusCode)
	}
	// 不存在的沙盒。
	r = v.post(t, "/e2b/ext/sandboxes/nope/terminal-tickets", "", nil)
	r.Body.Close()
	if r.StatusCode != 404 {
		t.Fatalf("missing sandbox: %d", r.StatusCode)
	}
}

func TestTicketStoreExpiryAndCapacity(t *testing.T) {
	s := newTicketStore()
	now := time.Now()
	tok, ok := s.Issue(terminalTicket{ProjectID: "p", SandboxID: "s"}, now)
	if !ok || tok == "" {
		t.Fatal("issue failed")
	}
	if _, ok := s.RedeemFor(tok, "s", now.Add(ticketTTL+time.Second)); ok {
		t.Fatal("expired ticket redeemed")
	}
	if _, ok := s.RedeemFor(tok, "s", now); ok {
		t.Fatal("a failed redeem must still consume the ticket")
	}
	tok2, _ := s.Issue(terminalTicket{ProjectID: "p", SandboxID: "s"}, now)
	if _, ok := s.RedeemFor(tok2, "other", now); ok {
		t.Fatal("redeemed against another sandbox")
	}
	if _, ok := s.RedeemFor(tok2, "s", now); ok {
		t.Fatal("a ticket presented for the wrong sandbox must be burned")
	}
	// 容量上限：不过期的存量满了就拒绝。
	full := newTicketStore()
	for i := 0; i < ticketMaxLive; i++ {
		if _, ok := full.Issue(terminalTicket{SandboxID: "s"}, now); !ok {
			t.Fatalf("issue %d failed early", i)
		}
	}
	if _, ok := full.Issue(terminalTicket{SandboxID: "s"}, now); ok {
		t.Fatal("store must refuse once full of live tickets")
	}
	// 过期项会被回收腾出空间。
	if _, ok := full.Issue(terminalTicket{SandboxID: "s"}, now.Add(2*ticketTTL)); !ok {
		t.Fatal("expired tickets should be reclaimed")
	}
}

func TestSameOrigin(t *testing.T) {
	mk := func(origin, host string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	for _, tc := range []struct {
		origin, host string
		want         bool
	}{
		{"http://a.example:8080", "a.example:8080", true},
		{"https://A.example", "a.example", true},
		{"http://evil.example", "a.example", false},
		{"http://a.example:9999", "a.example:8080", false},
		{"", "a.example", false},
		{"not a url", "a.example", false},
		{"null", "a.example", false},
	} {
		if got := sameOrigin(mk(tc.origin, tc.host)); got != tc.want {
			t.Errorf("sameOrigin(%q, %q) = %v, want %v", tc.origin, tc.host, got, tc.want)
		}
	}
}
