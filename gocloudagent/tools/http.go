// 文件说明：http.go — http 与 web_fetch 工具（planv6.8 §13.6）
//
// 两者都走 NetPolicy：请求前 CheckURL，连接时校验实际 IP，禁止重定向。
//   - http：通用请求（method/url/headers/body），响应体上限 1 MiB，非 GET/HEAD 视为写操作需确认；
//   - web_fetch：只读 GET，HTML 去标签转文本，正文上限 256 KiB。

package tools

import (
	"context"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/tool"
)

const (
	httpBodyLimit  = 1 << 20
	fetchBodyLimit = 256 << 10
	httpReqLimit   = 256 << 10
)

// HTTPTools 提供 http / web_fetch 两个工具。
type HTTPTools struct {
	Policy NetPolicy
	// Approve 在写请求（非 GET/HEAD）前确认；nil 表示拒绝写请求（fail-closed）。
	Approve func(question string) bool
	// Client 测试注入；nil 时按 Policy 构造加固客户端。
	Client *http.Client
}

func (h *HTTPTools) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return h.Policy.NewGuardedClient()
}

type httpArgs struct {
	Method  string            `json:"method,omitempty" jsonschema:"description=HTTP method, default GET"`
	URL     string            `json:"url" jsonschema:"description=absolute http(s) URL"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"description=request headers"`
	Body    string            `json:"body,omitempty" jsonschema:"description=request body (text)"`
}

// HTTPTool 返回 http 工具。
func (h *HTTPTools) HTTPTool() (tool.BaseTool, error) {
	return Exec("http",
		"Send an HTTP request to a public URL and return status, headers and body text (max 1 MiB). Private/loopback addresses are blocked unless explicitly allowed. Requests other than GET/HEAD need user confirmation. Redirects are not followed.",
		func(ctx context.Context, in httpArgs) (any, error) {
			method := strings.ToUpper(strings.TrimSpace(in.Method))
			if method == "" {
				method = http.MethodGet
			}
			u, err := h.Policy.CheckURL(in.URL)
			if err != nil {
				return nil, Failf("url_not_allowed", "%v", err)
			}
			if len(in.Body) > httpReqLimit {
				return nil, Failf("too_large", "request body exceeds %d bytes", httpReqLimit)
			}
			if method != http.MethodGet && method != http.MethodHead {
				if h.Approve == nil || !h.Approve("将发送 "+method+" 请求："+u.Redacted()) {
					return nil, Failf("confirmation_denied", "用户未批准该请求")
				}
			}
			var body io.Reader
			if in.Body != "" {
				body = strings.NewReader(in.Body)
			}
			req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
			if err != nil {
				return nil, Failf("bad_request", "%v", err)
			}
			for k, v := range in.Headers {
				req.Header.Set(k, v)
			}
			resp, err := h.client().Do(req)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, Failf("request_failed", "%v", err)
			}
			defer resp.Body.Close()
			raw, truncated := readLimited(resp.Body, httpBodyLimit)
			// 正文在信封内先行截断：Exec 的 64 KiB 输出上限只会切到 body 尾部，
			// status/headers/truncated 字段始终存活（否则模型看不到状态与截断标记）。
			textOut := string(raw)
			if bodyLimit := OutputLimit - 2048; len(textOut) > bodyLimit {
				textOut = truncateText(textOut, bodyLimit)
				truncated = true
			}
			headers := map[string]string{}
			for _, k := range []string{"Content-Type", "Content-Length", "Location", "Last-Modified", "ETag"} {
				if v := resp.Header.Get(k); v != "" {
					headers[k] = v
				}
			}
			return map[string]any{
				"ok": resp.StatusCode < 400, "status": resp.StatusCode, "headers": headers,
				"body": textOut, "truncated": truncated,
			}, nil
		})
}

type webFetchArgs struct {
	URL      string `json:"url" jsonschema:"description=absolute http(s) URL of the page to read"`
	MaxChars int    `json:"max_chars,omitempty" jsonschema:"description=max characters of text to return, default 20000"`
}

// WebFetchTool 返回 web_fetch 工具。
func (h *HTTPTools) WebFetchTool() (tool.BaseTool, error) {
	return Exec("web_fetch",
		"Fetch a public web page with GET and return its readable text (HTML tags stripped). Use after web_search to read a result. Private/loopback addresses are blocked.",
		func(ctx context.Context, in webFetchArgs) (any, error) {
			u, err := h.Policy.CheckURL(in.URL)
			if err != nil {
				return nil, Failf("url_not_allowed", "%v", err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return nil, Failf("bad_request", "%v", err)
			}
			req.Header.Set("User-Agent", "gocloudagent/web_fetch")
			req.Header.Set("Accept", "text/html,text/plain,application/json;q=0.9,*/*;q=0.5")
			resp, err := h.client().Do(req)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, Failf("fetch_failed", "%v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 && resp.StatusCode < 400 {
				return nil, Failf("redirect", "page redirects to %s; fetch that URL explicitly", resp.Header.Get("Location"))
			}
			if resp.StatusCode >= 400 {
				return nil, Failf("http_error", "http %d", resp.StatusCode)
			}
			raw, truncated := readLimited(resp.Body, fetchBodyLimit)
			text := string(raw)
			title := ""
			if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "html") || looksHTML(text) {
				title, text = htmlToText(text)
			}
			limit := in.MaxChars
			if limit <= 0 {
				limit = 20000
			}
			if r := []rune(text); len(r) > limit {
				text = string(r[:limit])
				truncated = true
			}
			// 同 http：字节维度再压一次，保证信封字段不被 Exec 的输出上限切掉。
			if textLimit := OutputLimit - 512; len(text) > textLimit {
				text = truncateText(text, textLimit)
				truncated = true
			}
			return map[string]any{"ok": true, "url": u.String(), "title": title, "text": text, "truncated": truncated}, nil
		})
}

// readLimited 最多读 limit 字节，并报告是否还有剩余。
func readLimited(r io.Reader, limit int64) ([]byte, bool) {
	raw, _ := io.ReadAll(io.LimitReader(r, limit+1))
	if int64(len(raw)) > limit {
		return raw[:limit], true
	}
	return raw, false
}

var (
	htmlDropRe  = regexp.MustCompile(`(?is)<(script|style|noscript|svg|head)[^>]*>.*?</(script|style|noscript|svg|head)>`)
	htmlTitleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlBlockRe = regexp.MustCompile(`(?i)</?(p|div|br|li|h[1-6]|tr|section|article|pre|blockquote)[^>]*>`)
	blankRe     = regexp.MustCompile(`\n[ \t]*\n(\s*\n)+`)
	inlineWSRe  = regexp.MustCompile(`[ \t]+`)
)

func looksHTML(s string) bool {
	head := strings.ToLower(s[:min(len(s), 512)])
	return strings.Contains(head, "<html") || strings.Contains(head, "<!doctype html")
}

// htmlToText 是不依赖第三方库的简化版正文提取：去脚本/样式、块级标签换行、去标签、反转义。
func htmlToText(doc string) (title, text string) {
	if m := htmlTitleRe.FindStringSubmatch(doc); len(m) == 2 {
		title = strings.TrimSpace(html.UnescapeString(stripTags(m[1])))
	}
	doc = htmlDropRe.ReplaceAllString(doc, "")
	doc = htmlBlockRe.ReplaceAllString(doc, "\n")
	doc = html.UnescapeString(stripTags(doc))
	lines := strings.Split(doc, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(inlineWSRe.ReplaceAllString(l, " "))
	}
	text = blankRe.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return title, strings.TrimSpace(text)
}
