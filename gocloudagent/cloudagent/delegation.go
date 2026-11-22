package cloudagent

import "time"

func (r *Runtime) confirmTimeout() time.Duration {
	if r.ConfirmTimeout > 0 {
		return r.ConfirmTimeout
	}
	return 2 * time.Minute
}

func boolPtr(v bool) *bool { return &v }
