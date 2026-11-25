//go:build !linux

// 文件说明：loopback_other.go — gosandbox/agent 的内部实现（bringUpLoopback 等）
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
//	[本文件] gosandbox/agent/loopback_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent.go, agent_test.go, fs.go, loopback_linux.go, netfilter_linux.go 等 16 个
//
// 主要符号
//
//	bringUpLoopback() error

package agent

// bringUpLoopback 在非 Linux 上是空操作：没有 netns，lo 本来就是通的。
func bringUpLoopback() error { return nil }
