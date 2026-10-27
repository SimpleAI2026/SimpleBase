// 文件说明：hub.go — internal/realtime 中 ErrConnectionLimit、ErrSubscription 等符号的实现
//
// 用法
//
//	NewHub(maxConns int) *Hub
//	(h *Hub) Subscribe(projectID string, collections, events []string, includeData ...
//	(h *Hub) Close(id string)
//
// 调用架构
//
//	[上游] internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/realtime/hub.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	var ErrConnectionLimit
//	var ErrSubscription
//	type Event struct {…}
//	type Subscription struct {…}
//	type Hub struct {…}
//	NewHub(maxConns int) *Hub
//	... 另有 11 个顶层符号

package realtime

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrConnectionLimit = errors.New("realtime_connection_limit")
	ErrSubscription    = errors.New("realtime_subscription_not_found")
)

type Event struct {
	ProjectID  string         `json:"-"`
	ID         string         `json:"id"`
	Event      string         `json:"event"`
	Collection string         `json:"collection"`
	RecordID   string         `json:"record_id"`
	Action     string         `json:"action"`
	Changed    []string       `json:"changed,omitempty"`
	At         time.Time      `json:"at"`
	Data       map[string]any `json:"data,omitempty"`
}

type Subscription struct {
	ID          string
	ProjectID   string
	Collections map[string]bool
	Events      map[string]bool
	IncludeData bool
	C           chan Event
}

type Hub struct {
	mu       sync.Mutex
	nextID   uint64
	events   []Event
	subs     map[string]*Subscription
	maxConns int
	dropped  uint64
}

func NewHub(maxConns int) *Hub {
	if maxConns <= 0 {
		maxConns = 200
	}
	return &Hub{subs: map[string]*Subscription{}, maxConns: maxConns}
}

func (h *Hub) Subscribe(projectID string, collections, events []string, includeData bool) (*Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	connections := 0
	for _, sub := range h.subs {
		if sub.ProjectID == projectID {
			connections++
		}
	}
	if connections >= h.maxConns {
		return nil, ErrConnectionLimit
	}
	sub := &Subscription{ID: uuid.NewString(), ProjectID: projectID, Collections: scopeSet(collections), Events: scopeSet(events), IncludeData: includeData, C: make(chan Event, 16)}
	h.subs[sub.ID] = sub
	return sub, nil
}

func scopeSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func (h *Hub) Close(id string) {
	h.mu.Lock()
	delete(h.subs, id)
	h.mu.Unlock()
}

func (h *Hub) Update(id string, collections, events []string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	sub, ok := h.subs[id]
	if !ok {
		return ErrSubscription
	}
	sub.Collections, sub.Events = scopeSet(collections), scopeSet(events)
	return nil
}

func (h *Hub) Publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	event.ID = formatID(h.nextID)
	event.At = time.Now().UTC()
	event.Event = "record." + event.Action
	h.events = append(h.events, event)
	if len(h.events) > 2000 {
		copy(h.events, h.events[len(h.events)-2000:])
		h.events = h.events[:2000]
	}
	for _, sub := range h.subs {
		if !matches(sub, event) {
			continue
		}
		select {
		case sub.C <- eventFor(sub, event):
		default:
			h.dropped++
		}
	}
}

func (h *Hub) Replay(sub *Subscription, after uint64) ([]Event, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []Event{}
	gap := after > 0 && len(h.events) > 0 && after < parseID(h.events[0].ID)-1
	for _, event := range h.events {
		if parseID(event.ID) > after && matches(sub, event) {
			out = append(out, eventFor(sub, event))
		}
	}
	return out, gap
}

func (h *Hub) Dropped() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dropped
}

func matches(sub *Subscription, event Event) bool {
	if sub.ProjectID == "" || sub.ProjectID != event.ProjectID || len(sub.Collections) > 0 && !sub.Collections["*"] && !sub.Collections[event.Collection] {
		return false
	}
	return len(sub.Events) == 0 || sub.Events["*"] || sub.Events[event.Action]
}

func eventFor(sub *Subscription, event Event) Event {
	if !sub.IncludeData {
		event.Data = nil
	}
	return event
}

func formatID(id uint64) string {
	return strconv.FormatUint(id, 10)
}
func parseID(id string) uint64 {
	value, _ := strconv.ParseUint(id, 10, 64)
	return value
}
