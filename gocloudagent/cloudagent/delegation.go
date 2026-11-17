// 文件说明：delegation.go — 委托令牌签发 / 确认等待接口与运行级令牌表

package cloudagent

import (
	"context"
	"time"
)

// DelegationIssuer 签发本轮 CLI 使用的短时令牌。
//
// 实现在宿主侧（internal/agentbridge）：签发会把身份字段写进 claims，
// 因此调用方传入的必须是「已按本轮 writable 降权」的身份副本，
// 不能是原始全权限主体。
type DelegationIssuer interface {
	Issue(p Principal, projectID, runID string, ttl time.Duration) (string, error)
	RevokeRun(runID string)
}

// ConfirmationWaiter 在破坏性命令执行前等待界面确认。
// initiator 是发起人标识（planv6.0 D5），由确认端做比对。
type ConfirmationWaiter interface {
	Wait(ctx context.Context, runID, callID, initiator string, timeout time.Duration) (approved bool, err error)
}

// ConfirmationPreparer 是可选的预登记接口（planv6.0 §3.4）：
// 在 confirmation_required 事件发出前登记等待项，消除早到确认的竞态。
type ConfirmationPreparer interface {
	Prepare(runID, callID, initiator string)
}

func (r *Runtime) confirmTimeout() time.Duration {
	if r.ConfirmTimeout > 0 {
		return r.ConfirmTimeout
	}
	return 2 * time.Minute
}

func (r *Runtime) delegationTTL() time.Duration {
	if r.DelegationTTL > 0 {
		return r.DelegationTTL
	}
	return 15 * time.Minute
}

func (r *Runtime) rememberToken(runID, token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tokens == nil {
		r.tokens = map[string]string{}
	}
	r.tokens[runID] = token
}

func (r *Runtime) forgetToken(runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tokens, runID)
}

func (r *Runtime) tokenFor(runID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokens[runID]
}
