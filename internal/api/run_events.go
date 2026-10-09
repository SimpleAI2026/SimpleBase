package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
)

const runEventCap = 2000

type runFrame struct {
	id int64
	ev cloudagent.Event
}

type runBuf struct {
	frames []runFrame
	next   int64
	subs   []chan runFrame
	done   bool
	ended  time.Time
}

// runEventHub 保存单个 run 的 SSE 环，供断线续传。
type runEventHub struct {
	mu   sync.Mutex
	runs map[string]*runBuf
}

func newRunEventHub() *runEventHub {
	return &runEventHub{runs: map[string]*runBuf{}}
}

func (h *runEventHub) append(runID string, ev cloudagent.Event) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.buf(runID)
	b.next++
	frame := runFrame{id: b.next, ev: ev}
	b.frames = append(b.frames, frame)
	if len(b.frames) > runEventCap {
		b.frames = b.frames[len(b.frames)-runEventCap:]
	}
	for _, sub := range b.subs {
		select {
		case sub <- frame:
		default:
		}
	}
	if ev.Type == "end" {
		b.done = true
		b.ended = time.Now()
		time.AfterFunc(2*time.Minute, func() { h.drop(runID) })
	}
	return frame.id
}

func (h *runEventHub) buf(runID string) *runBuf {
	b := h.runs[runID]
	if b == nil {
		b = &runBuf{}
		h.runs[runID] = b
	}
	return b
}

func (h *runEventHub) drop(runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.runs[runID]
	if b != nil && b.done && time.Since(b.ended) >= 2*time.Minute {
		delete(h.runs, runID)
	}
}

// replay 返回 id>after 的帧。gap 表示 after 已经早于环的起点。
func (h *runEventHub) replay(runID string, after int64) (frames []runFrame, gap, live bool, sub chan runFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.runs[runID]
	if b == nil {
		return nil, false, false, nil
	}
	if len(b.frames) > 0 && after > 0 && after < b.frames[0].id-1 {
		return nil, true, false, nil
	}
	for _, f := range b.frames {
		if f.id > after {
			frames = append(frames, f)
		}
	}
	if b.done {
		return frames, false, false, nil
	}
	sub = make(chan runFrame, 16)
	b.subs = append(b.subs, sub)
	return frames, false, true, sub
}

func (h *runEventHub) unsubscribe(runID string, sub chan runFrame) {
	if sub == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.runs[runID]
	if b == nil {
		return
	}
	next := b.subs[:0]
	for _, s := range b.subs {
		if s != sub {
			next = append(next, s)
		}
	}
	b.subs = next
}

type confirmHub struct {
	mu      sync.Mutex
	pending map[string]chan bool
}

func newConfirmHub() *confirmHub {
	return &confirmHub{pending: map[string]chan bool{}}
}

func confirmKey(runID, callID string) string { return runID + "\n" + callID }

func (h *confirmHub) Wait(ctx context.Context, runID, callID string, timeout time.Duration) (bool, error) {
	if h == nil {
		return false, nil
	}
	ch := make(chan bool, 1)
	key := confirmKey(runID, callID)
	h.mu.Lock()
	h.pending[key] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, key)
		h.mu.Unlock()
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case v := <-ch:
		return v, nil
	case <-timer.C:
		return false, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (h *confirmHub) Resolve(runID, callID string, approve bool) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	ch := h.pending[confirmKey(runID, callID)]
	h.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- approve:
		return true
	default:
		return false
	}
}

func (h *cloudAgentHandler) ConfirmRun(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	runID := c.Param("runID")
	callID := c.Param("callID")
	if _, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, runID); err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	var body struct {
		Approve bool `json:"approve"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if h.confirms == nil || !h.confirms.Resolve(runID, callID, body.Approve) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "confirmation not found"))
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "approve": body.Approve})
}

func (h *cloudAgentHandler) ReplayEvents(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	runID := c.Param("runID")
	if _, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, runID); err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	after, _ := strconv.ParseInt(c.QueryParam("after"), 10, 64)
	if h.events == nil {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	frames, gap, live, sub := h.events.replay(runID, after)
	defer h.events.unsubscribe(runID, sub)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	write := func(id int64, ev cloudagent.Event) {
		data, _ := json.Marshal(ev)
		_, _ = c.Response().Write([]byte(fmt.Sprintf("id: %d\ndata: %s\n\n", id, data)))
		if flusher != nil {
			flusher.Flush()
		}
	}
	if gap {
		write(0, cloudagent.Event{Type: "error", Code: "stream_gap", Message: "事件已过期，请刷新消息", RunID: runID})
		return nil
	}
	for _, f := range frames {
		write(f.id, f.ev)
	}
	if !live || sub == nil {
		return nil
	}
	ctx := c.Request().Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case f := <-sub:
			write(f.id, f.ev)
			if f.ev.Type == "end" {
				return nil
			}
		}
	}
}
