// 文件说明：delegation.go — internal/agentbridge 中 DelegationIssuer、NewDelegationIssuer 等符号的实现
//
// 用法
//
//	type DelegationIssuer struct{…}  // DelegationIssuer issues SimpleBase HMAC delegation tokens.
//	NewDelegationIssuer(secret string, reg *auth.DelegationRegistry) DelegationIssuer
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/delegation.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//
// 主要符号
//
//	type DelegationIssuer struct{…}
//	NewDelegationIssuer(secret string, reg *auth.DelegationRegistry) DelegationIssuer
//	(d DelegationIssuer) Issue(p auth.Principal, projectID, runID string, ttl time.Duration) (string, error)
//	(d DelegationIssuer) RevokeRun(runID string)

package agentbridge

import (
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// DelegationIssuer issues SimpleBase HMAC delegation tokens.
type DelegationIssuer struct {
	inner auth.HMACDelegationIssuer
}

func NewDelegationIssuer(secret string, reg *auth.DelegationRegistry) DelegationIssuer {
	return DelegationIssuer{inner: auth.HMACDelegationIssuer{Secret: secret, Reg: reg}}
}

func (d DelegationIssuer) Issue(p auth.Principal, projectID, runID string, ttl time.Duration) (string, error) {
	return d.inner.Issue(p, projectID, runID, ttl)
}

func (d DelegationIssuer) RevokeRun(runID string) { d.inner.RevokeRun(runID) }
