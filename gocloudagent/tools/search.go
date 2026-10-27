// 文件说明：search.go — gocloudagent/tools 中 SearchProvider、SearchDuckDuckGo 等符号的实现
//
// 用法
//
//	(c *SearchClient) Search(ctx context.Context, query string, limit int) ...  // Search 按配置的后端执行搜索
//	(c *SearchClient) SearchTool() (tool.BaseTool, error)  // SearchTool 返回 web 搜索工具
//	同包调用：Search() → tools.go
//
// 调用架构
//
//	[上游] gocloudagent/cli
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/tools/search.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] fsutil.go, tools.go
//	[关键引用] Search() ← tools.go
//
// 主要符号
//
//	type SearchProvider string
//	const SearchDuckDuckGo
//	const SearchBrave
//	const SearchNone
//	type SearchClient struct {…}
//	type SearchResult struct {…}
//	... 另有 13 个顶层符号

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// SearchProvider 是 web 搜索后端（planv6.0 §12 Q1 的双通道）。
type SearchProvider string

const (
	// SearchDuckDuckGo 免 key（HTML 端点，可能因反爬失效）。
	SearchDuckDuckGo SearchProvider = "duckduckgo"
	// SearchBrave 需 key（JSON API，稳定）。
	SearchBrave SearchProvider = "brave"
	// SearchNone 表示未配置搜索后端。
	SearchNone SearchProvider = ""
)

// SearchClient 执行 web 搜索。
type SearchClient struct {
	Provider SearchProvider
	APIKey   string
	HTTP     *http.Client
	// MaxResults 上限；0 用默认值。
	MaxResults int
}

func (c *SearchClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// SearchResult 是一条搜索结果。
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// Search 按配置的后端执行搜索。未配置时返回 ErrSearchNotConfigured。
func (c *SearchClient) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if c == nil || c.Provider == SearchNone {
		return nil, ErrSearchNotConfigured
	}
	if limit <= 0 {
		limit = c.MaxResults
	}
	if limit <= 0 {
		limit = 5
	}
	switch c.Provider {
	case SearchBrave:
		return c.brave(ctx, query, limit)
	case SearchDuckDuckGo:
		return c.duckduckgo(ctx, query, limit)
	default:
		return nil, fmt.Errorf("unknown search provider %q", c.Provider)
	}
}

// ErrSearchNotConfigured 表示未配置搜索后端。
var ErrSearchNotConfigured = errors.New("web search is not configured: set search_provider in ~/.gocloudagent/config.json (duckduckgo is free, brave needs an api key)")

type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func (c *SearchClient) brave(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, errors.New("brave search requires search_api_key")
	}
	endpoint := "https://api.search.brave.com/res/v1/web/search?q=" + url.QueryEscape(query) +
		"&count=" + strconv.Itoa(limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", c.APIKey)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("brave search failed (http %d)", resp.StatusCode)
	}
	var parsed braveResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode brave response: %w", err)
	}
	out := make([]SearchResult, 0, len(parsed.Web.Results))
	for _, r := range parsed.Web.Results {
		if len(out) >= limit {
			break
		}
		out = append(out, SearchResult{Title: html.UnescapeString(r.Title), URL: r.URL, Snippet: html.UnescapeString(stripTags(r.Description))})
	}
	return out, nil
}

// duckResultRe 提取结果块的标题与链接。
var (
	duckLinkRe    = regexp.MustCompile(`<a[^>]+class="[^"]*result__a[^"]*"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	duckSnippetRe = regexp.MustCompile(`<a[^>]+class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
	tagRe         = regexp.MustCompile(`<[^>]*>`)
	spaceRe       = regexp.MustCompile(`\s+`)
)

func (c *SearchClient) duckduckgo(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	endpoint := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// 不带浏览器 UA 时该端点常返回 403/202 挑战页。
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept", "text/html")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("duckduckgo search failed (http %d); configure a brave api key for reliable search", resp.StatusCode)
	}
	body := string(raw)
	links := duckLinkRe.FindAllStringSubmatch(body, -1)
	snippets := duckSnippetRe.FindAllStringSubmatch(body, -1)
	if len(links) == 0 {
		return nil, errors.New("duckduckgo returned no parsable results (endpoint may be rate limiting); configure a brave api key for reliable search")
	}
	out := make([]SearchResult, 0, limit)
	for i, m := range links {
		if len(out) >= limit {
			break
		}
		href := html.UnescapeString(m[1])
		if strings.HasPrefix(href, "//duckduckgo.com/l/?uddg=") {
			if u, err := url.Parse("https:" + href); err == nil {
				if target := u.Query().Get("uddg"); target != "" {
					href = target
				}
			}
		}
		snippet := ""
		if i < len(snippets) {
			snippet = html.UnescapeString(spaceRe.ReplaceAllString(stripTags(snippets[i][1]), " "))
		}
		out = append(out, SearchResult{
			Title:   html.UnescapeString(spaceRe.ReplaceAllString(stripTags(m[2]), " ")),
			URL:     href,
			Snippet: strings.TrimSpace(snippet),
		})
	}
	return out, nil
}

func stripTags(s string) string { return tagRe.ReplaceAllString(s, "") }

// webSearchArgs 是 web_search 的参数。
type webSearchArgs struct {
	Query string `json:"query" jsonschema:"description=search query"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=max results to return, default 5"`
}

// SearchTool 返回 web 搜索工具；未配置后端时返回 nil（调用方不注册该工具）。
func (c *SearchClient) SearchTool() (tool.BaseTool, error) {
	if c == nil || c.Provider == SearchNone {
		return nil, nil
	}
	return utils.InferTool("web_search",
		"Search the public web and return titles, URLs and snippets. Use it for up-to-date information the local workspace does not contain.",
		func(ctx context.Context, in webSearchArgs) (string, error) {
			if strings.TrimSpace(in.Query) == "" {
				return ErrEnvelope("usage", "query is required"), nil
			}
			results, err := c.Search(ctx, in.Query, in.Limit)
			if err != nil {
				if errors.Is(err, ErrSearchNotConfigured) {
					return ErrEnvelope("search_not_configured", err.Error()), nil
				}
				return ErrEnvelope("search_failed", err.Error()), nil
			}
			if len(results) == 0 {
				return ErrEnvelope("no_results", "no results for: "+in.Query), nil
			}
			b, _ := json.Marshal(map[string]any{"ok": true, "query": in.Query, "results": results})
			return string(b), nil
		})
}
