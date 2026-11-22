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
