//go:build !linux

// 文件说明：rlimit_other.go — 非 Linux 上没有 rlimit 可设
//
// 用法
//
//	applyRlimits(rs []proto.Rlimit) (func(), error)  // 恒返回空恢复函数
//
// 调用架构
//
//	[上游] gosandbox/agent/proc.go
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/rlimit_other.go
//	         │
//	         ▼
//	[下游] gosandbox/proto
//	[同包] rlimit_linux.go
//
// 主要符号
//
//	applyRlimits(rs []proto.Rlimit) (func(), error)

package agent

import "github.com/linkxzhou/SimpleBase/gosandbox/proto"

// applyRlimits 在非 Linux 上什么也不做：agent 只在 Linux 沙盒里跑。
func applyRlimits([]proto.Rlimit) (func(), error) { return func() {}, nil }
