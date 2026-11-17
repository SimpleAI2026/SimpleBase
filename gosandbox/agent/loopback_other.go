//go:build !linux

// 文件说明：loopback_other.go — 非 Linux 上不处理 lo
//
// 用法
//
//	_ = bringUpLoopback()  // 恒返回 nil
//
// 调用架构
//
//	[上游] gosandbox/agent（Main）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/loopback_other.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] agent.go, loopback_linux.go
//
// 主要符号
//
//	bringUpLoopback() error

package agent

// bringUpLoopback 在非 Linux 上是空操作：没有 netns，lo 本来就是通的。
func bringUpLoopback() error { return nil }
