package opshttp

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

// QuotaHandler 依赖 httpkit.UsageService。
// perf §1 P1-D：配额状态按项目缓存 10s + 单飞，仪表盘刷新不再逐次打系统库。
type QuotaHandler struct {
	svc httpkit.UsageService

	mu       sync.Mutex
	cache    map[string]quotaCacheEntry
	inflight map[string]*quotaCall
}

type quotaCacheEntry struct {
	llmOK     bool
	dbOK      bool
	expiresAt time.Time
}

type quotaCall struct {
	done  chan struct{}
	llmOK bool
	dbOK  bool
}

// GetQuota 返回当前 project 的配额可用状态。
func (h *QuotaHandler) GetQuota(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	llmOK, dbOK := h.quotaFor(c.Request().Context(), pc.ID)
	return c.JSON(http.StatusOK, map[string]bool{
		"llm_allowed":      llmOK,
		"database_allowed": dbOK,
	})
}

// quotaFor 带 10s TTL 与 per-project 单飞的配额查询。
func (h *QuotaHandler) quotaFor(ctx context.Context, projectID string) (llmOK, dbOK bool) {
	now := time.Now()
	h.mu.Lock()
	if h.cache == nil {
		h.cache = map[string]quotaCacheEntry{}
		h.inflight = map[string]*quotaCall{}
	}
	if e, hit := h.cache[projectID]; hit && now.Before(e.expiresAt) {
		h.mu.Unlock()
		return e.llmOK, e.dbOK
	}
	if call, ok := h.inflight[projectID]; ok {
		h.mu.Unlock()
		<-call.done
		return call.llmOK, call.dbOK
	}
	call := &quotaCall{done: make(chan struct{})}
	h.inflight[projectID] = call
	h.mu.Unlock()

	call.llmOK = h.svc.CheckQuota(ctx, projectID, "llm") == nil
	call.dbOK = h.svc.CheckQuota(ctx, projectID, "database") == nil
	close(call.done)

	h.mu.Lock()
	delete(h.inflight, projectID)
	h.cache[projectID] = quotaCacheEntry{llmOK: call.llmOK, dbOK: call.dbOK, expiresAt: time.Now().Add(10 * time.Second)}
	h.mu.Unlock()
	return call.llmOK, call.dbOK
}

// AuditHandler 依赖 httpkit.AuditService。
type AuditHandler struct {
	svc httpkit.AuditService
}

// ListOperations 返回当前 project 的审计事件列表（读 sys_operations）。
func (h *AuditHandler) ListOperations(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	limit := httpkit.QueryLimit(c, 50, 0)
	ops, err := h.svc.ListOperations(c.Request().Context(), pc.ID, c.QueryParam("database_id"), limit)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"operations": ops,
		"limit":      limit,
	})
}
