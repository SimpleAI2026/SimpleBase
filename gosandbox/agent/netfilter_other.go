//go:build !linux

// 文件说明：netfilter_other.go — gosandbox/agent 的内部实现（blockPrivateNetworks、dropNetAdmin 等）
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
//	[本文件] gosandbox/agent/netfilter_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent.go, agent_test.go, fs.go, loopback_linux.go, loopback_other.go 等 16 个
//
// 主要符号
//
//	blockPrivateNetworks() error
//	dropNetAdmin() error

package agent

// blockPrivateNetworks 在非 Linux 上是空操作：没有 netns 需要拦截。
func blockPrivateNetworks() error { return nil }

// dropNetAdmin 在非 Linux 上是空操作：没有 CAP_NET_ADMIN 可丢。
func dropNetAdmin() error { return nil }
