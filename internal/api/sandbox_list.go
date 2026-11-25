// 文件说明：sandbox_list.go — E2B 兼容 API 的沙盒列表（GET /sandboxes、/v2/sandboxes）：查询参数校验、过滤、排序与分页映射
//
// 用法
//
//	路由：List() ← routes_e2b.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sandbox_list.go
//	         │ 依赖
//	         ▼
//	[下游] internal/sandbox
//	[同包] sandbox_e2b_dto.go（listed / rfc3339）
//	[关键引用] List ← routes_e2b.go
//
// 主要符号
//
//	type sandboxListQuery struct {…}
//	parseSandboxListQuery(...) / parseMetadataFilter(...) / matchMetadata(...) / startOf(...)
//	(*SandboxHandler).List(c echo.Context) error / toE2BSandboxList(...)

package api

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// sandboxListQuery 是 GET /sandboxes 查询参数校验后的结果。
type sandboxListQuery struct {
	limit        int
	wantMeta     map[string]string
	states       map[string]bool
	startedAfter time.Time
	order        string
	nextToken    string
}

// parseSandboxListQuery 解析并校验列表查询参数，失败时返回 400。
func parseSandboxListQuery(q url.Values) (sandboxListQuery, error) {
	out := sandboxListQuery{limit: 100, states: map[string]bool{}, nextToken: q.Get("nextToken")}
	if v := q.Get("limit"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 1 || n > 100 {
			return out, echo.NewHTTPError(http.StatusBadRequest, "limit must be 1-100")
		}
		out.limit = n
	}
	wantMeta, err := parseMetadataFilter(q.Get("metadata"))
	if err != nil {
		return out, echo.NewHTTPError(http.StatusBadRequest, "invalid metadata filter")
	}
	out.wantMeta = wantMeta
	for _, s := range q["state"] {
		for _, part := range strutil.SplitCSV(s) {
			if part != "running" && part != "paused" {
				return out, echo.NewHTTPError(http.StatusBadRequest, "state must be running or paused")
			}
			out.states[part] = true
		}
	}
	if v := q.Get("startedAfter"); v != "" {
		t, perr := time.Parse(time.RFC3339, v)
		if perr != nil {
			return out, echo.NewHTTPError(http.StatusBadRequest, "startedAfter must be RFC3339")
		}
		out.startedAfter = t
	}
	out.order = q.Get("order")
	if out.order != "" && out.order != "asc" && out.order != "desc" {
		return out, echo.NewHTTPError(http.StatusBadRequest, "order must be asc or desc")
	}
	return out, nil
}

// List 对应 GET /v2/sandboxes 与 GET /sandboxes。
func (h *SandboxHandler) List(c echo.Context) error {
	p, err := h.requireRead(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	q, err := parseSandboxListQuery(c.QueryParams())
	if err != nil {
		return writeE2BErr(c, err)
	}

	// 项目内沙盒数量有上限（≤100），翻完整个项目后在内存里过滤 / 排序 / 分页，
	// 这样 metadata、state、startedAfter 的组合都不依赖库内索引。
	var all []sandbox.Sandbox
	cursor := ""
	for {
		page, next, lerr := h.svc.List(c.Request().Context(), p, "", "", cursor, 100)
		if lerr != nil {
			return writeE2BErr(c, lerr)
		}
		all = append(all, page...)
		if next == "" || len(all) > 1000 {
			break
		}
		cursor = next
	}
	kept, running, next, err := h.toE2BSandboxList(all, q)
	if err != nil {
		return writeE2BErr(c, err)
	}
	c.Response().Header().Set("X-Total-Running", strconv.Itoa(running))
	if next != "" {
		c.Response().Header().Set("X-Next-Token", next)
	}
	return c.JSON(http.StatusOK, kept)
}

// toE2BSandboxList 对全量沙盒做 state / startedAfter / metadata 过滤、按启动时间排序，
// 再按 nextToken 与 limit 分页；返回当前页、过滤后处于 running 的总数与下一页令牌。
func (h *SandboxHandler) toE2BSandboxList(all []sandbox.Sandbox, q sandboxListQuery) ([]e2bListed, int, string, error) {
	var kept []sandbox.Sandbox
	running := 0
	for _, r := range all {
		if len(q.states) > 0 && !q.states[stateOf(r.Status)] {
			continue
		}
		if !q.startedAfter.IsZero() && !startOf(r).After(q.startedAfter) {
			continue
		}
		if !matchMetadata(h.metadataOf(r), q.wantMeta) {
			continue
		}
		kept = append(kept, r)
		if stateOf(r.Status) == "running" {
			running++
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := startOf(kept[i]), startOf(kept[j])
		if !a.Equal(b) {
			if q.order == "asc" {
				return a.Before(b)
			}
			return a.After(b)
		}
		if q.order == "asc" {
			return kept[i].ID < kept[j].ID
		}
		return kept[i].ID > kept[j].ID
	})
	// nextToken 是上一页最后一项的 ID：在排好序的结果里定位它的下一项。
	if tok := q.nextToken; tok != "" {
		idx := -1
		for i, r := range kept {
			if r.ID == tok {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, 0, "", echo.NewHTTPError(http.StatusBadRequest, "invalid nextToken")
		}
		kept = kept[idx+1:]
	}
	next := ""
	if len(kept) > q.limit {
		kept = kept[:q.limit]
		next = kept[q.limit-1].ID
	}
	out := make([]e2bListed, 0, len(kept))
	for _, r := range kept {
		out = append(out, h.listed(r))
	}
	return out, running, next, nil
}

func startOf(r sandbox.Sandbox) time.Time {
	if r.StartedAt != nil && !r.StartedAt.IsZero() {
		return *r.StartedAt
	}
	return r.CreatedAt
}

// parseMetadataFilter 解析 "k=v&k2=v2"（键值各自 URL 编码）。
func parseMetadataFilter(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, errors.New("missing =")
		}
		dk, err := url.QueryUnescape(k)
		if err != nil {
			return nil, err
		}
		dv, err := url.QueryUnescape(v)
		if err != nil {
			return nil, err
		}
		out[dk] = dv
	}
	return out, nil
}

func matchMetadata(have, want map[string]string) bool {
	for k, v := range want {
		if got, ok := have[k]; !ok || got != v {
			return false
		}
	}
	return true
}
