//go:build !linux

package agent

// blockPrivateNetworks 在非 Linux 上是空操作：没有 netns 需要拦截。
func blockPrivateNetworks() error { return nil }

// dropNetAdmin 在非 Linux 上是空操作：没有 CAP_NET_ADMIN 可丢。
func dropNetAdmin() error { return nil }
