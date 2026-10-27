// 文件说明：run_events.go — internal/api 的内部实现（runEventCap、runFrame 等）
//
// 用法
//
//	type runFrame（包内使用）
//	type runBuf（包内使用）
//	同包调用：append() → adapter.go, agent_handler.go 等；buf() → api_keys_handler.go, e2b_process_handler.go 等；Wait() → agent_handler.go, e2b_process_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/run_events.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] append() ← adapter.go, agent_handler.go, agent_schedule_handler.go
//
// 主要符号
//
//	const runEventCap
//	type runFrame struct {…}
//	type runBuf struct {…}
//	type runEventHub struct {…}
//	const runEventIdleTTL
//	const runEventDefaultCap
//	... 另有 21 个顶层符号

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent"
)

const runEventCap = 2000

type runFrame struct {
	id int64
	ev gocloudagent.Event
}

type runBuf struct {
	frames []runFrame
	next   int64
	subs   []chan runFrame
	done   bool
	ended  time.Time
	lastAt time.Time // LIFE-05：最近一次 append 时间，用于无 end 时的兜底清理
}

// runEventHub 保存单个 run 的 SSE 环，供断线续传。
type runEventHub struct {
	mu        sync.Mutex
	runs      map[string]*runBuf
	maxRuns   int           // LIFE-05：缓存 run 总量上限，0 用默认值
	idleTTL   time.Duration // LIFE-05：无 end 事件的空闲清理阈值，0 用默认值
	lastSweep time.Time     // LIFE-05：惰性 sweep 节流
}

// LIFE-05 兜底参数：未收到 end 的 run（中途 panic / 提前返回）超过空闲
// TTL 后由 sweep 清理；总量超过 maxRuns 时淘汰最旧条目。
const (
	runEventIdleTTL    = 10 * time.Minute
	runEventDefaultCap = 512
)

func newRunEventHub() *runEventHub {
	return &runEventHub{runs: map[string]*runBuf{}, maxRuns: runEventDefaultCap, idleTTL: runEventIdleTTL}
}

func (h *runEventHub) append(runID string, ev gocloudagent.Event) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.buf(runID)
	b.next++
	frame := runFrame{id: b.next, ev: ev}
	b.frames = append(b.frames, frame)
	b.lastAt = time.Now()
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
	h.sweepLocked() // LIFE-05：惰性兜底清理（带节流）
	h.evictLocked()
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

// evictLocked 需要已持有 h.mu：总量超过上限时淘汰最旧的条目（LIFE-05）。
func (h *runEventHub) evictLocked() {
	cap := h.maxRuns
	if cap <= 0 {
		cap = runEventDefaultCap
	}
	for len(h.runs) > cap {
		oldestID := ""
		var oldest time.Time
		for id, b := range h.runs {
			at := b.lastAt
			if b.done {
				at = b.ended
			}
			if oldestID == "" || at.Before(oldest) {
				oldestID, oldest = id, at
			}
		}
		if oldestID == "" {
			return
		}
		delete(h.runs, oldestID)
	}
}

// sweep 清理超过空闲 TTL 仍未完成的 run（LIFE-05：无 end 事件的兜底）。
// 已完成（done）的条目仍走原有的 2 分钟延迟 drop 语义，sweep 不提前删。
func (h *runEventHub) sweep() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sweepLocked()
}

// sweepLocked 需要已持有 h.mu：惰性清理入口，由 append 周期性触发，
// 不引入独立后台 goroutine（router 可能被测试反复创建，避免泄漏）。
func (h *runEventHub) sweepLocked() {
	ttl := h.idleTTL
	if ttl <= 0 {
		ttl = runEventIdleTTL
	}
	now := time.Now()
	if now.Sub(h.lastSweep) < ttl/2 {
		return
	}
	h.lastSweep = now
	for id, b := range h.runs {
		if !b.done && now.Sub(b.lastAt) > ttl {
			delete(h.runs, id)
		}
	}
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
//
// BUG-B1（planv6.0 v2 §2.2）：条目因 2 分钟 drop / 总量淘汰 / 空闲 TTL 被
// 抹掉时，缓存起点已不可达 —— 与 after 早于环起点等价，必须返回 gap=true，
// 否则客户端拿到 200 空流，只会看到「流断了」而非「请刷新消息」。
func (h *runEventHub) replay(runID string, after int64) (frames []runFrame, gap, live bool, sub chan runFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.runs[runID]
	if b == nil {
		return nil, true, false, nil
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
	// initiators 登记 Wait 发起人（planv6.0 D5）：Resolve 时与确认人比对，
	// handler 层的 run.InitiatorID 校验之外的二次防线。
	initiators map[string]string
}

func newConfirmHub() *confirmHub {
	return &confirmHub{pending: map[string]chan bool{}, initiators: map[string]string{}}
}

func confirmKey(runID, callID string) string { return runID + "\n" + callID }

func (h *confirmHub) Wait(ctx context.Context, runID, callID, initiator string, timeout time.Duration) (bool, error) {
	if h == nil {
		return false, nil
	}
	ch := h.prepare(runID, callID, initiator)
	// BUG-B4（planv6.0 v2 §2.2）：确认项必须回收 —— 此前每次破坏性确认都留下
	// 一条永久 map 条目（Resolve 不删、DropRun 只删 initiators）。
	defer h.release(runID, callID)
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

// release 回收一个确认等待项。channel 保留（可能有并发 Resolve 已取到），
// 但删除 map 条目即可让后续 prepare 重新建 channel，内存不再增长。
func (h *confirmHub) release(runID, callID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	delete(h.pending, confirmKey(runID, callID))
	h.mu.Unlock()
}

// pendingCount 返回当前登记数（测试与诊断用）。
func (h *confirmHub) pendingCount() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.pending)
}

// prepare 登记确认等待项并返回信号 channel。
// 发出 confirmation_required 事件之前调用可消除「确认先到、登记后到」竞态。
func (h *confirmHub) prepare(runID, callID, initiator string) chan bool {
	key := confirmKey(runID, callID)
	h.mu.Lock()
	if h.pending == nil {
		h.pending = map[string]chan bool{}
	}
	if h.initiators == nil {
		h.initiators = map[string]string{}
	}
	ch, ok := h.pending[key]
	if !ok {
		ch = make(chan bool, 1)
		h.pending[key] = ch
	}
	if initiator != "" {
		h.initiators[runID] = initiator
	}
	h.mu.Unlock()
	return ch
}

// Prepare 预登记确认等待项（实现 gocloudagent.ConfirmationPreparer）。
func (h *confirmHub) Prepare(runID, callID, initiator string) {
	if h == nil {
		return
	}
	h.prepare(runID, callID, initiator)
}

func (h *confirmHub) DropRun(runID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	delete(h.initiators, runID)
	// BUG-B4：key 形如 runID + "\n" + callID，需要遍历清理该 run 的残留项。
	prefix := runID + "\n"
	for key := range h.pending {
		if strings.HasPrefix(key, prefix) {
			delete(h.pending, key)
		}
	}
	h.mu.Unlock()
}

// Resolve 放行确认并做发起人二次校验（planv6.0 D5）。
// caller 为确认人 principalKey；与 Wait 登记的发起人不一致且非空时拒绝。
func (h *confirmHub) Resolve(runID, callID string, approve bool, caller string) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	ch := h.pending[confirmKey(runID, callID)]
	initiator := h.initiators[runID]
	h.mu.Unlock()
	if ch == nil {
		return false
	}
	// 二次校验：登记过发起人时，确认人必须与发起人一致。
	if initiator != "" && caller != "" && initiator != caller {
		return false
	}
	select {
	case ch <- approve:
		// BUG-B4：只在真正放行时回收条目。校验失败或放不进去时保留，
		// 由 Wait 的 defer 在超时/取消路径统一回收，避免「抢删」导致
		// 后续合法确认拿到 404。
		h.release(runID, callID)
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
	// SEC-04：确认放行的是工具执行（可能写库），属于破坏性操作；
	// 只读主体拒绝，与 CancelRun 同一判定。
	p, hasPrincipal := PrincipalFromContext(c.Request().Context())
	if hasPrincipal && !h.canWritePrincipal(p) {
		return WriteError(c, echo.NewHTTPError(http.StatusForbidden, "write permission required"))
	}
	runID := c.Param("runID")
	callID := c.Param("callID")
	run, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, runID)
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	// planv6.0 D5：确认与取消同一发起人判定；非发起人的同项目写主体拒绝，
	// 防止「项目内互相放行破坏性工具」。
	if hasPrincipal && !canCancelConfirm(p, run.InitiatorID) {
		return WriteError(c, echo.NewHTTPError(http.StatusForbidden, "only the initiator or a project admin can confirm this run"))
	}
	var body struct {
		Approve bool `json:"approve"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if h.confirms == nil || !h.confirms.Resolve(runID, callID, body.Approve, principalIdentity(p)) {
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
	// planv6.0 v2 §11：续传同样是长连接，必须解除 WriteTimeout。
	beginSSE(c)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	write := func(id int64, ev gocloudagent.Event) {
		data, _ := json.Marshal(ev)
		_, _ = c.Response().Write([]byte(fmt.Sprintf("id: %d\ndata: %s\n\n", id, data)))
		if flusher != nil {
			flusher.Flush()
		}
	}
	if gap {
		write(0, gocloudagent.Event{Type: "error", Code: "stream_gap", Message: "事件已过期，请刷新消息", RunID: runID})
		return nil
	}
	for _, f := range frames {
		write(f.id, f.ev)
	}
	if !live || sub == nil {
		return nil
	}
	// BUG-B2（planv6.0 v2 §2.2）：sub 容量 16，append 满时静默丢帧。
	// 帧 id 由 hub 单调分配，跳号即代表丢帧，必须让客户端知道要去刷新消息，
	// 否则前端会拿一段有缺口的事件序列当完整流。
	// 期望值：有补发帧时是末帧 id，否则是客户端声明的 after。
	last := after
	if len(frames) > 0 {
		last = frames[len(frames)-1].id
	}
	ctx := c.Request().Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case f := <-sub:
			if f.id != last+1 {
				write(f.id, gocloudagent.Event{Type: "error", Code: "stream_gap", Message: "事件已过期，请刷新消息", RunID: runID})
				return nil
			}
			last = f.id
			write(f.id, f.ev)
			if f.ev.Type == "end" {
				return nil
			}
		}
	}
}
