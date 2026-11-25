// 文件说明：netguard_test.go — SSRF 表驱动测试与 http / web_fetch 工具行为测试（planv6.8 §13.11.1-2）
//
// 覆盖：
//   - IsBlockedIP 全地址段表（回环 / 私网 / 链路本地含 169.254.169.254 / 组播 / 未指定 / 0.0.0.0/8 / IPv6）；
//   - CheckURL 静态规则（scheme 白名单、userinfo、空 host、字面量私网 IP、HTTPSOnly）；
//   - GuardDialContext 在实际连接 IP 上拦截（DNS rebinding 防护：域名解析到回环也被挡）；
//   - NewGuardedClient 禁止跟随重定向（重定向到内网不自动跟随）；
//   - http / web_fetch 工具：响应上限、写请求确认 fail-closed、AllowPrivate、max_chars、redirect / http_error。

package tools

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsBlockedIPTable(t *testing.T) {
	blocked := []string{
		// 回环 127/8
		"127.0.0.1", "127.1.2.3", "127.255.255.255",
		// 私网 10/8、172.16/12、192.168/16
		"10.0.0.1", "10.255.255.255",
		"172.16.0.1", "172.31.255.254",
		"192.168.0.1", "192.168.1.1",
		// 链路本地（含云元数据 169.254.169.254）
		"169.254.0.1", "169.254.169.254",
		// 未指定 / 0.0.0.0/8 / 组播
		"0.0.0.0", "0.1.2.3",
		"224.0.0.1", "239.255.255.255",
		// IPv6：回环、唯一本地 fc00::/7、链路本地 fe80::/10、组播、未指定
		"::1",
		"fc00::1", "fd12:3456::1",
		"fe80::1", "fe80::abcd",
		"ff02::1",
		"::",
		// nil
		"",
	}
	for _, s := range blocked {
		var ip net.IP
		if s != "" {
			ip = net.ParseIP(s)
		}
		if !IsBlockedIP(ip) {
			t.Fatalf("IsBlockedIP(%s) = false, want true", s)
		}
	}
	allowed := []string{
		"8.8.8.8", "1.1.1.1", "172.32.0.1", "172.15.255.254", "192.169.0.1",
		"100.64.0.1", // CGNAT 不在 Go IsPrivate 内；策略一致对待（不过 IsPrivate 只含 RFC1918）
		"2001:db8::1", "2606:4700::1111",
	}
	for _, s := range allowed {
		if IsBlockedIP(net.ParseIP(s)) {
			t.Fatalf("IsBlockedIP(%s) = true, want false", s)
		}
	}
}

func TestCheckURLRules(t *testing.T) {
	p := NetPolicy{}
	// scheme 白名单。
	for _, raw := range []string{"ftp://example.com/", "file:///etc/passwd", "gopher://x", "javascript:alert(1)"} {
		if _, err := p.CheckURL(raw); err == nil {
			t.Fatalf("CheckURL(%q) must be rejected", raw)
		}
	}
	// userinfo 禁止。
	if _, err := p.CheckURL("http://user:pass@example.com/"); err == nil {
		t.Fatal("userinfo must be rejected")
	}
	// 空 host。
	if _, err := p.CheckURL("http:///path"); err == nil {
		t.Fatal("empty host must be rejected")
	}
	// 字面量私网 IP 提前拦截（错误更清楚）。
	_, err := p.CheckURL("http://127.0.0.1:8080/")
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("literal loopback ip: err = %v, want ErrBlockedAddress", err)
	}
	// 合法公网 URL 放行。
	if _, err := p.CheckURL("https://example.com/a?b=c"); err != nil {
		t.Fatalf("public https url must pass: %v", err)
	}
	// AllowPrivate 放行字面量回环。
	if _, err := (NetPolicy{AllowPrivate: true}).CheckURL("http://127.0.0.1:8080/"); err != nil {
		t.Fatalf("AllowPrivate should allow loopback: %v", err)
	}
	// HTTPSOnly 拒绝 http。
	if _, err := (NetPolicy{HTTPSOnly: true}).CheckURL("http://example.com/"); err == nil {
		t.Fatal("HTTPSOnly must reject http")
	}
}

// GuardDialContext 在实际连接 IP 上拦截：域名（如 localhost）解析到回环时，
// Control 钩子看到的仍是被拒 IP——这是 DNS rebinding 的防护点。
func TestGuardDialContextBlocksLoopbackEvenByHostname(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	addr := ln.Addr().String()

	// 字面量 IP。
	dial := GuardDialContext()
	if _, err := dial(context.Background(), "tcp", addr); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("dial %s: err = %v, want ErrBlockedAddress", addr, err)
	}
	// 域名解析到回环（rebinding 机制）。
	if _, err := dial(context.Background(), "tcp", "localhost"+addr[strings.LastIndex(addr, ":"):]); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("dial localhost: err = %v, want ErrBlockedAddress", err)
	}
	// AllowPrivate 放行。
	if _, err := (NetPolicy{AllowPrivate: true}).DialContext()(context.Background(), "tcp", addr); err != nil {
		t.Fatalf("AllowPrivate dial: %v", err)
	}
}

// 加固客户端不跟随重定向：3xx 直接返回，让调用方（CheckURL 之后）自行决定。
func TestGuardedClientDoesNotFollowRedirectToPrivate(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("redirect target must not be reached")
	}))
	defer target.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, (&http.Request{}), target.URL, http.StatusFound)
	}))
	defer srv.Close()

	client := (NetPolicy{AllowPrivate: true}).NewGuardedClient()
	resp, err := client.Get(srv.URL + "/start")
	if err != nil {
		t.Fatalf("guarded get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302 (not followed)", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != target.URL+"/" && loc != target.URL {
		t.Fatalf("location = %q", loc)
	}
}

// ---- http 工具行为 ----

// http / web_fetch 行为测试用真实的加固客户端（AllowPrivate 放行回环拨号，
// 禁止重定向与拨号 IP 校验都是生路径），不注入普通 client。
func TestHTTPToolResponseLimitAndWriteConfirm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", httpBodyLimit+1024)))
	}))
	defer srv.Close()

	h := &HTTPTools{Policy: NetPolicy{AllowPrivate: true}}
	httpTool, err := h.HTTPTool()
	if err != nil {
		t.Fatal(err)
	}

	// GET：正常返回且被截断到上限。
	out := invoke(t, httpTool, `{"url":"`+srv.URL+`"}`)
	if !strings.Contains(out, `"ok":true`) || !strings.Contains(out, `"truncated":true`) {
		t.Fatalf("truncated response: %s", out)
	}

	// 非 GET 且 Approve 为 nil：fail-closed 拒绝。
	h2 := &HTTPTools{Policy: NetPolicy{AllowPrivate: true}}
	post, err := h2.HTTPTool()
	if err != nil {
		t.Fatal(err)
	}
	if out := invoke(t, post, `{"method":"POST","url":"`+srv.URL+`","body":"hi"}`); !strings.Contains(out, "confirmation_denied") {
		t.Fatalf("write without approve must be denied: %s", out)
	}
	// Approve 拒绝同样 fail-closed。
	h3 := &HTTPTools{Policy: NetPolicy{AllowPrivate: true}, Approve: func(string) bool { return false }}
	post2, err := h3.HTTPTool()
	if err != nil {
		t.Fatal(err)
	}
	if out := invoke(t, post2, `{"method":"DELETE","url":"`+srv.URL+`"}`); !strings.Contains(out, "confirmation_denied") {
		t.Fatalf("denied approve: %s", out)
	}

	// 未开启 AllowPrivate 时，字面量回环 URL 在 CheckURL 即被拒。
	h4 := &HTTPTools{Policy: NetPolicy{}}
	guarded, err := h4.HTTPTool()
	if err != nil {
		t.Fatal(err)
	}
	if out := invoke(t, guarded, `{"url":"`+srv.URL+`"}`); !strings.Contains(out, "url_not_allowed") {
		t.Fatalf("private url must be blocked: %s", out)
	}

	// 请求体超上限。
	h5 := &HTTPTools{Policy: NetPolicy{AllowPrivate: true}, Approve: func(string) bool { return true }}
	put, err := h5.HTTPTool()
	if err != nil {
		t.Fatal(err)
	}
	if out := invoke(t, put, `{"method":"PUT","url":"`+srv.URL+`","body":"`+strings.Repeat("a", httpReqLimit+1)+`"}`); !strings.Contains(out, "too_large") {
		t.Fatalf("oversize body: %s", out)
	}
}

func TestWebFetchToolBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html><head><title>Hi</title><style>x{}</style></head><body><p>Hello <b>world</b></p><script>bad()</script></body></html>"))
		case "/big":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(strings.Repeat("a", fetchBodyLimit+1024)))
		case "/redir":
			http.Redirect(w, r, "/page", http.StatusMovedPermanently)
		case "/missing":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	h := &HTTPTools{Policy: NetPolicy{AllowPrivate: true}}
	fetch, err := h.WebFetchTool()
	if err != nil {
		t.Fatal(err)
	}

	// HTML 去标签 + title 提取；script 内容不得泄漏。
	out := invoke(t, fetch, `{"url":"`+srv.URL+`/page"}`)
	if !strings.Contains(out, `"title":"Hi"`) || !strings.Contains(out, "Hello world") || strings.Contains(out, "bad()") {
		t.Fatalf("html extraction: %s", out)
	}

	// max_chars 截断（rune 截断，标记 truncated）。
	out = invoke(t, fetch, `{"url":"`+srv.URL+`/page","max_chars":2}`)
	if !strings.Contains(out, `"truncated":true`) {
		t.Fatalf("max_chars truncation: %s", out)
	}

	// 正文上限 256 KiB。
	out = invoke(t, fetch, `{"url":"`+srv.URL+`/big"}`)
	if !strings.Contains(out, `"truncated":true`) {
		t.Fatalf("body limit truncation: %s", out)
	}

	// 重定向不跟随，fail code = redirect（Location 按服务端原文透传）。
	out = invoke(t, fetch, `{"url":"`+srv.URL+`/redir"}`)
	if !strings.Contains(out, "redirect") || !strings.Contains(out, "/page") {
		t.Fatalf("redirect must be reported explicitly: %s", out)
	}

	// 4xx 报 http_error。
	out = invoke(t, fetch, `{"url":"`+srv.URL+`/missing"}`)
	if !strings.Contains(out, "http_error") {
		t.Fatalf("http error: %s", out)
	}

	// 未开启 AllowPrivate：私网字面量 URL 被拒。
	h2 := &HTTPTools{Policy: NetPolicy{}}
	fetch2, err := h2.WebFetchTool()
	if err != nil {
		t.Fatal(err)
	}
	if out := invoke(t, fetch2, `{"url":"`+srv.URL+`/page"}`); !strings.Contains(out, "url_not_allowed") {
		t.Fatalf("private url must be blocked: %s", out)
	}
}
