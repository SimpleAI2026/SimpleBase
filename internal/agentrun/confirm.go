// 文件说明：confirm.go — 工具审批的进程内等待表（ConfirmHub）
//
// 用法
//
//	confirms := NewConfirmHub()                       // app 先构造，注入 Runtime.Confirm / RuntimeProvider.Confirm
//	(h *ConfirmHub) Wait(ctx, runID, callID, initiator, timeout)  // run 侧等待放行（实现 cloudagent.ConfirmationWaiter）
//	(h *ConfirmHub) Prepare / Resolve / DropRun        // 预登记、放行（带发起人二次校验）、清理
//
// 调用架构
//
//	[上游] internal/app（装配）、internal/agentrun（ResolveConfirm）、gocloudagent/cloudagent（Wait / Prepare）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/confirm.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/cloudagent（接口契约）
//	[同包] service.go, run.go
//
// 主要符号
//
//	type ConfirmHub struct {…}
//	NewConfirmHub() *ConfirmHub
//	(h *ConfirmHub) Wait(ctx context.Context, runID, callID, initiator string, timeout time.Duration) (bool, error)
//	(h *ConfirmHub) Prepare(runID, callID, initiator string)
//	... 另有 4 个顶层符号
//
// 归属（internal-ddd-hexagonal-implementation-plan §3.4）：等待表是 Agent 协作的长期状态，
// 必须先于路由树构造（router 不再回写 Runtime.Confirm）；回收与二次校验语义原样保留。

package agentrun

import (
	"context"
	"strings"
	"sync"
	"time"
)

// ConfirmHub 是工具审批的进程内等待表：run 侧 Wait 登记等待，确认接口 Resolve 放行。
// 实现 cloudagent.ConfirmationWaiter / ConfirmationPreparer，由 app 先构造并注入 Runtime 与 RuntimeProvider。
type ConfirmHub struct {
	mu      sync.Mutex
	pending map[string]chan bool
	// initiators 登记 Wait 发起人（planv6.0 D5）：Resolve 时与确认人比对，
	// handler 层的 run.InitiatorID 校验之外的二次防线。
	initiators map[string]string
}

func NewConfirmHub() *ConfirmHub {
	return &ConfirmHub{pending: map[string]chan bool{}, initiators: map[string]string{}}
}

func confirmKey(runID, callID string) string { return runID + "\n" + callID }

func (h *ConfirmHub) Wait(ctx context.Context, runID, callID, initiator string, timeout time.Duration) (bool, error) {
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
func (h *ConfirmHub) release(runID, callID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	delete(h.pending, confirmKey(runID, callID))
	h.mu.Unlock()
}

// prepare 登记确认等待项并返回信号 channel。
// 发出 confirmation_required 事件之前调用可消除「确认先到、登记后到」竞态。
func (h *ConfirmHub) prepare(runID, callID, initiator string) chan bool {
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

// Prepare 预登记确认等待项（实现 cloudagent.ConfirmationPreparer）。
func (h *ConfirmHub) Prepare(runID, callID, initiator string) {
	if h == nil {
		return
	}
	h.prepare(runID, callID, initiator)
}

func (h *ConfirmHub) DropRun(runID string) {
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
// caller 为确认人 Principal.Actor()；与 Wait 登记的发起人不一致且非空时拒绝。
func (h *ConfirmHub) Resolve(runID, callID string, approve bool, caller string) bool {
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
