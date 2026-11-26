package datahttp

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

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
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
	collections := SplitRealtimeNames(c.QueryParam("collections"), true)
	if err := h.CheckScope(c, collections); err != nil {
		return httpkit.WriteError(c, err)
	}
	includeData := c.QueryParam("include_data") == "1" && httpkit.PrincipalOf(c).Scope == nil
	sub, err := h.hub.Subscribe(projectID, collections, SplitRealtimeNames(c.QueryParam("events"), false), includeData)
	if err != nil {
		return realtimeAPIError(c, err)
	}
	defer h.hub.Close(sub.ID)
	httpkit.BeginSSE(c)
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
	if err := httpkit.DecodeJSONBody(c, &body, false); err != nil || body.ClientID == "" {
		return httpkit.WriteError(c, httpkit.BadRequest(c, "client_id and subscription are required"))
	}
	collections := SplitRealtimeNames(strings.Join(body.Collections, ","), true)
	if err := h.CheckScope(c, collections); err != nil {
		return httpkit.WriteError(c, err)
	}
	events := SplitRealtimeNames(strings.Join(body.Events, ","), false)
	if err := h.hub.Update(body.ClientID, collections, events); err != nil {
		return realtimeAPIError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *RealtimeHandler) CheckScope(c echo.Context, collections []string) error {
	for _, name := range collections {
		if name != "*" && !collection.NamePattern.MatchString(name) {
			return httpkit.BadRequest(c, "invalid collection")
		}
		if policy := httpkit.PrincipalOf(c).Scope; policy != nil && !policy.Allows("collection:"+name, "read", time.Now().UTC()) {
			return httpkit.NewAPIError(http.StatusForbidden, "key_scope_denied", "key scope denied", httpkit.RequestID(c))
		}
	}
	return nil
}

func SplitRealtimeNames(raw string, collection bool) []string {
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
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusTooManyRequests, "realtime_connection_limit", "realtime connection limit", httpkit.RequestID(c)))
	}
	return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusNotFound, "realtime_subscription_not_found", "subscription not found", httpkit.RequestID(c)))
}
