// 文件说明：confirmation.go — 破坏性工具调用的人工确认编排
//
// 云端 MCP 工具与宿主 CLI 工具共用同一套确认流程：
//
//	headless → 直接拒绝（只发 confirmation_resolved(false)，不发 required）
//	否则：Prepare（先登记，消除 required 先于 Wait 的竞态）→ confirmation_required →
//	      Wait → confirmation_resolved。
//
// 本文件只编排事件与等待，不做任何授权判断：命令分类、只读/阶段限权、失败信封与
// 业务 trace 均由调用方负责，经 OnRequested / OnResolved 观察即可保持各自时点。

package cloudagent

import (
	"context"
	"errors"
	"time"
)

// DefaultConfirmTimeout 是确认窗口的默认时长。
const DefaultConfirmTimeout = 2 * time.Minute

// ConfirmationOutcome 是一次确认的结论。
type ConfirmationOutcome int

const (
	// ConfirmationDenied 表示用户拒绝、确认超时，或没有可用的确认通道。
	ConfirmationDenied ConfirmationOutcome = iota
	// ConfirmationApproved 表示用户批准。
	ConfirmationApproved
	// ConfirmationUnavailable 表示无人值守（headless），不允许确认破坏性操作。
	ConfirmationUnavailable
)

// ConfirmationRequest 描述一次待确认的调用。
type ConfirmationRequest struct {
	RunID  string
	CallID string
	// Subject 是确认归属主体；Prepare 与 Wait 必须使用同一值。
	Subject string
	// Name 是事件里展示的工具名。
	Name string
	// Argv 与 Message 是展示给确认方的内容，必须已经由调用方脱敏。
	Argv    []string
	Message string
	// Headless 为真时不发起确认。
	Headless bool
	// Timeout 为零时用 DefaultConfirmTimeout。
	Timeout time.Duration

	Waiter ConfirmationWaiter
	Emit   func(Event)

	// OnRequested 在 confirmation_required 事件之后、Wait 之前调用；可为 nil。
	OnRequested func()
	// OnResolved 在 confirmation_resolved 事件之后调用；可为 nil。
	OnResolved func(approved bool)
}

// AwaitConfirmation 执行确认流程。
//
// 仅当等待因取消而中断时返回错误（总是 context.Canceled 或 ctx 的取消错误）；
// 拒绝、超时、无确认通道都只是 ConfirmationDenied，由调用方转成各自的失败信封。
// CallID 为空时无法登记确认项，按拒绝处理。
func AwaitConfirmation(ctx context.Context, req ConfirmationRequest) (ConfirmationOutcome, error) {
	if req.Headless {
		if req.Emit != nil {
			req.Emit(Event{Type: "confirmation_resolved", CallID: req.CallID, Name: req.Name, Approve: boolPtr(false), Message: "定时任务不能确认破坏性操作"})
		}
		return ConfirmationUnavailable, nil
	}
	if req.Emit != nil {
		if p, ok := req.Waiter.(ConfirmationPreparer); ok && req.CallID != "" {
			p.Prepare(req.RunID, req.CallID, req.Subject)
		}
		req.Emit(Event{Type: "confirmation_required", CallID: req.CallID, Name: req.Name, Argv: req.Argv, Message: req.Message})
	}
	if req.OnRequested != nil {
		req.OnRequested()
	}
	approved := false
	if req.Waiter != nil && req.CallID != "" {
		timeout := req.Timeout
		if timeout <= 0 {
			timeout = DefaultConfirmTimeout
		}
		ok, err := req.Waiter.Wait(ctx, req.RunID, req.CallID, req.Subject, timeout)
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return ConfirmationDenied, canceledError(ctx)
		}
		approved = err == nil && ok
	}
	if req.Emit != nil {
		req.Emit(Event{Type: "confirmation_resolved", CallID: req.CallID, Name: req.Name, Approve: boolPtr(approved)})
	}
	if req.OnResolved != nil {
		req.OnResolved(approved)
	}
	if approved {
		return ConfirmationApproved, nil
	}
	return ConfirmationDenied, nil
}

func boolPtr(v bool) *bool { return &v }

// canceledError 返回取消错误：优先用 ctx 自身的错误，否则（Wait 报取消而 ctx 仍有效）
// 回落为 context.Canceled，避免把取消当成「无错误」。
func canceledError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return context.Canceled
}
