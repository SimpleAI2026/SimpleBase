// 文件说明：realtime_handler.go — internal/api 中 RealtimeHandler、NewRealtimeHandler 等符号的实现
//
// 用法
//
//	NewRealtimeHandler(maxConns int) *RealtimeHandler
//	(h *RealtimeHandler) PublishRecord(event collection.RecordEvent)  // PublishRecord 实现 recordEventPublisher：集合变更 → 实时帧
//	(h *RealtimeHandler) Subscribe(c echo.Context) error
//	同包调用：PublishRecord() 实现 data_handler.go 的 recordEventPublisher 端口，由 internal/app 的 fan-out 调用；Subscribe() → e2b_process_handler.go, router.go；RealtimeHandler → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/realtime_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/collection（发布语言）, internal/realtime, internal/utils/strutil
//	[同包] adapter.go, agent_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] PublishRecord() ← internal/app 的 fan-out（经 recordEventPublisher 端口）
//
// 主要符号
//
//	type RealtimeHandler struct {…}
//	NewRealtimeHandler(maxConns int) *RealtimeHandler
//	(h *RealtimeHandler) PublishRecord(event collection.RecordEvent)  // PublishRecord 实现 recordEventPublisher：集合变更 → 实时帧
//	(h *RealtimeHandler) Subscribe(c echo.Context) error
//	(h *RealtimeHandler) UpdateSubscription(c echo.Context) error
//	... 另有 5 个顶层符号

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/collection"
	"github.com/linkxzhou/SimpleBase/internal/realtime"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

type RealtimeHandler struct {
	hub *realtime.Hub
}

func NewRealtimeHandler(maxConns int) *RealtimeHandler {
	return &RealtimeHandler{hub: realtime.NewHub(maxConns)}
}

// PublishRecord 把集合变更事件推给实时订阅者；实现 data_handler 的 recordEventPublisher 端口。
func (h *RealtimeHandler) PublishRecord(event collection.RecordEvent) {
	h.hub.Publish(realtime.Event{ProjectID: event.ProjectID, Collection: event.Collection, RecordID: event.RecordID, Action: event.Action, Changed: event.Changed, Data: event.Data})
}

func (h *RealtimeHandler) Subscribe(c echo.Context) error {
	projectID := c.Param("projectID")
	collections := splitRealtimeNames(c.QueryParam("collections"), true)
	if err := h.checkScope(c, collections); err != nil {
		return WriteError(c, err)
	}
	includeData := c.QueryParam("include_data") == "1" && scopedPolicy(c) == nil
	sub, err := h.hub.Subscribe(projectID, collections, splitRealtimeNames(c.QueryParam("events"), false), includeData)
	if err != nil {
		return realtimeAPIError(c, err)
	}
	defer h.hub.Close(sub.ID)
	beginSSE(c)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	if err := writeRealtimeFrame(c, "", "connect", map[string]any{"client_id": sub.ID}); err != nil {
		return nil
	}
	var after uint64
	if value := c.Request().Header.Get("Last-Event-ID"); value != "" {
		after, _ = strconv.ParseUint(value, 10, 64)
	}
	replay, gap := h.hub.Replay(sub, after)
	if gap {
		if err := writeRealtimeFrame(c, "", "gap", map[string]any{"after": after}); err != nil {
			return nil
		}
	}
	for _, event := range replay {
		if err := writeRealtimeEvent(c, event); err != nil {
			return nil
		}
	}
	c.Response().Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	idle := time.NewTimer(5 * time.Minute)
	maximum := time.NewTimer(30 * time.Minute)
	defer heartbeat.Stop()
	defer idle.Stop()
	defer maximum.Stop()
	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-maximum.C:
			return nil
		case <-idle.C:
			return nil
		case <-heartbeat.C:
			if _, err := c.Response().Write([]byte(": heartbeat\n\n")); err != nil {
				return nil
			}
			c.Response().Flush()
		case event := <-sub.C:
			if err := writeRealtimeEvent(c, event); err != nil {
				return nil
			}
			if !idle.Stop() {
				<-idle.C
			}
			idle.Reset(5 * time.Minute)
		}
	}
}

func (h *RealtimeHandler) UpdateSubscription(c echo.Context) error {
	var body struct {
		ClientID    string   `json:"client_id"`
		Collections []string `json:"collections"`
		Events      []string `json:"events"`
	}
	if err := decodeJSONBody(c, &body, false); err != nil || body.ClientID == "" {
		return WriteError(c, badRequest(c, "client_id and subscription are required"))
	}
	collections := splitRealtimeNames(strings.Join(body.Collections, ","), true)
	if err := h.checkScope(c, collections); err != nil {
		return WriteError(c, err)
	}
	events := splitRealtimeNames(strings.Join(body.Events, ","), false)
	if err := h.hub.Update(body.ClientID, collections, events); err != nil {
		return realtimeAPIError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *RealtimeHandler) checkScope(c echo.Context, collections []string) error {
	for _, name := range collections {
		if name != "*" && !collectionNamePattern.MatchString(name) {
			return badRequest(c, "invalid collection")
		}
		if policy := scopedPolicy(c); policy != nil && !policy.Allows("collection:"+name, "read", time.Now().UTC()) {
			return NewAPIError(http.StatusForbidden, "key_scope_denied", "key scope denied", requestID(c))
		}
	}
	return nil
}

func splitRealtimeNames(raw string, collection bool) []string {
	if strings.TrimSpace(raw) == "" {
		if collection {
			return []string{"*"}
		}
		return []string{"create", "update", "delete"}
	}
	return strutil.SplitCSV(raw)
}

func writeRealtimeEvent(c echo.Context, event realtime.Event) error {
	return writeRealtimeFrame(c, event.ID, event.Event, event)
}

func writeRealtimeFrame(c echo.Context, id, name string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if id != "" {
		if _, err := c.Response().Write([]byte("id: " + id + "\n")); err != nil {
			return err
		}
	}
	if _, err := c.Response().Write([]byte("event: " + name + "\ndata: " + string(body) + "\n\n")); err != nil {
		return err
	}
	c.Response().Flush()
	return nil
}

func realtimeAPIError(c echo.Context, err error) error {
	if errors.Is(err, realtime.ErrConnectionLimit) {
		return WriteError(c, NewAPIError(http.StatusTooManyRequests, "realtime_connection_limit", "realtime connection limit", requestID(c)))
	}
	return WriteError(c, NewAPIError(http.StatusNotFound, "realtime_subscription_not_found", "subscription not found", requestID(c)))
}
