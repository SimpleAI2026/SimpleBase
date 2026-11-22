//go:build !linux

package agent

// bringUpLoopback 在非 Linux 上是空操作：没有 netns，lo 本来就是通的。
func bringUpLoopback() error { return nil }
