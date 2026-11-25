//go:build !linux

// 文件说明：rlimit_other.go — gosandbox/agent 的内部实现（applyRlimits 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] gosandbox/cmd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/rlimit_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent.go, agent_test.go, fs.go, loopback_linux.go, loopback_other.go 等 16 个
//
// 主要符号
//
//	applyRlimits([]proto.Rlimit) (func(), error)

package agent

import "github.com/linkxzhou/SimpleBase/gosandbox/proto"

// applyRlimits 在非 Linux 上什么也不做：agent 只在 Linux 沙盒里跑。
func applyRlimits([]proto.Rlimit) (func(), error) { return func() {}, nil }
