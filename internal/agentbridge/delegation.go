package agentbridge

import (
	"time"

	"github.com/linkxzhou/SimpleBase/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// DelegationIssuer 把模块的委托签发请求转成宿主的 HMAC 签发。
//
// 安全前提：auth.IssueDelegation 只是把传入的权限集合写进 claims，**不会**
// 自行与原主体求交集。因此模块侧必须先按本轮 writable 降权（见
// cloudagent.stripWritePermissions），本适配器不接受、也不修补权限。
type DelegationIssuer struct {
	inner auth.HMACDelegationIssuer
}

// NewDelegationIssuer 用宿主密钥与注册表构造签发器。
func NewDelegationIssuer(secret string, reg *auth.DelegationRegistry) DelegationIssuer {
	return DelegationIssuer{inner: auth.HMACDelegationIssuer{Secret: secret, Reg: reg}}
}

// Issue 签发本轮委托令牌。
func (d DelegationIssuer) Issue(p cloudagent.Principal, projectID, runID string, ttl time.Duration) (string, error) {
	return d.inner.Issue(ToHostPrincipal(p), projectID, runID, ttl)
}

// RevokeRun 撤销一轮的所有委托令牌。
func (d DelegationIssuer) RevokeRun(runID string) { d.inner.RevokeRun(runID) }
