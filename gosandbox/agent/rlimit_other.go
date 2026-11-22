//go:build !linux

package agent

import "github.com/linkxzhou/SimpleBase/gosandbox/proto"

// applyRlimits 在非 Linux 上什么也不做：agent 只在 Linux 沙盒里跑。
func applyRlimits([]proto.Rlimit) (func(), error) { return func() {}, nil }
