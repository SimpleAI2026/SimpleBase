//go:build !linux

// 文件说明：netfilter_other.go — 非 Linux 上没有 netns，出网拦截是空操作
//
// 用法
//
//	_ = blockPrivateNetworks()   // 恒返回 nil
//
// 调用架构
//
//	[上游] gosandbox/agent（Main）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/netfilter_other.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] netfilter_linux.go, agent.go
//
// 主要符号
//
//	blockPrivateNetworks() error

package agent

// blockPrivateNetworks 在非 Linux 上是空操作：没有 netns 需要拦截。
func blockPrivateNetworks() error { return nil }

// dropNetAdmin 在非 Linux 上是空操作：没有 CAP_NET_ADMIN 可丢。
func dropNetAdmin() error { return nil }
