// 文件说明：delegation.go — internal/agentbridge 中 DelegationIssuer、NewDelegationIssuer 等符号的实现
//
// 用法
//
//	NewDelegationIssuer(secret string, reg *auth.DelegationRegistry) DelegationIssuer
//	(d DelegationIssuer) Issue(p gocloudagent.Principal, projectID, runID string, ttl ...  // Issue 签发本轮委托令牌
//	(d DelegationIssuer) RevokeRun(runID string)  // RevokeRun 撤销一轮的所有委托令牌
//	同包调用：DelegationIssuer → principal.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/delegation.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, internal/auth
//	[同包] cli.go, command.go, cron_agent.go, errors.go, principal.go 等 6 个
//	[关键引用] DelegationIssuer ← principal.go
//
// 主要符号
//
//	type DelegationIssuer struct {…}
//	NewDelegationIssuer(secret string, reg *auth.DelegationRegistry) DelegationIssuer
//	(d DelegationIssuer) Issue(p gocloudagent.Principal, projectID, runID string, ttl ...
//	(d DelegationIssuer) RevokeRun(runID string)

package agentbridge

import (
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// DelegationIssuer 把模块的委托签发请求转成宿主的 HMAC 签发。
//
// 安全前提：auth.IssueDelegation 只是把传入的权限集合写进 claims，**不会**
// 自行与原主体求交集。因此模块侧必须先按本轮 writable 降权（见
// gocloudagent.stripWritePermissions），本适配器不接受、也不修补权限。
type DelegationIssuer struct {
	inner auth.HMACDelegationIssuer
}

// NewDelegationIssuer 用宿主密钥与注册表构造签发器。
func NewDelegationIssuer(secret string, reg *auth.DelegationRegistry) DelegationIssuer {
	return DelegationIssuer{inner: auth.HMACDelegationIssuer{Secret: secret, Reg: reg}}
}

// Issue 签发本轮委托令牌。
func (d DelegationIssuer) Issue(p gocloudagent.Principal, projectID, runID string, ttl time.Duration) (string, error) {
	return d.inner.Issue(ToHostPrincipal(p), projectID, runID, ttl)
}

// RevokeRun 撤销一轮的所有委托令牌。
func (d DelegationIssuer) RevokeRun(runID string) { d.inner.RevokeRun(runID) }
