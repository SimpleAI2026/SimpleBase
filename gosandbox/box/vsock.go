// 文件说明：vsock.go — 跨平台的 vsock 地址约定（拨号实现按平台分文件）
//
// 状态：为 macOS VM 后端预留，当前没有生产调用方（vm/box 没有 darwin 实现），仅被测试使用。
//
// macOS 的 Virtualization.framework 用 AF_VSOCK 把宿主与 guest 连起来：
// guest 内的 agent 监听一个端口，宿主用同一个端口号拨过去。这与 kiri 的
// kiri-vmm/apple.rs 是同一个结构——**协议与 agent 完全复用**，
// 只有最底层的字节流换了载体。
//
// 这一层刻意做得很薄：只定义「哪个端口、怎么拨、怎么等」，
// 真正的拨号（含 cgo / ioctl）在各平台的 vsock_*.go 里。
// 这样 Linux 上也能用（AF_VSOCK 在 Linux 存在），测试时可以用
// 任意实现了 Transport 的东西替代。
//
// 端口约定：
//
//	VSOCKPortAgent  1024  agent 的控制通道（guest 内监听）
//	VSOCKPortEcho   1025  测试用的回环端点
//
// 为什么从 1024 起：AF_VSOCK 的端口是 u32，但 0-1023 在部分实现里保留给
// 系统服务；从 1024 起避开这类冲突，也不与「任意临时端口」混在一起。
//
// 用法
//
//	tr, err := dialVSock(ctx, cid, VSOCKPortAgent)
//	// tr 可以直接交给 newSessionOn
//
// 调用架构
//
//	[上游] gosandbox/vm/box（darwin 实现）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/vsock.go
//	         │
//	         ▼
//	[下游] gosandbox/box/transport.go
//	[同包] vsock_linux.go、vsock_other.go
//
// 主要符号
//
//	const VSOCKPortAgent / VSOCKPortEcho
//	const HostCID / GuestCID
//	VSockSupported() bool
//	DialVSock(ctx context.Context, cid uint32, port uint32) (Transport, error)

package box

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// vsock 端口约定。
const (
	// VSOCKPortAgent 是 guest 内 agent 的控制通道端口。
	VSOCKPortAgent uint32 = 1024
	// VSOCKPortEcho 是测试端点（回显），只在测试时监听。
	VSOCKPortEcho uint32 = 1025
)

// vsock 的特殊 CID。
const (
	// HostCID 是宿主的固定 cid。
	HostCID uint32 = 2
	// GuestCID 是 guest 侧用来连宿主的保留 cid。
	GuestCID uint32 = 3
	// AnyCID 是监听时表示「任意来源」的 cid。
	AnyCID uint32 = ^uint32(0)
)

// ErrVSockUnsupported 表示当前平台/内核没有 vsock。
var ErrVSockUnsupported = errors.New("box: vsock is not available on this platform")

// dialTimeout 是默认拨号超时。
//
// 比 unix socket 长：VM 启动后 guest 内的 agent 需要一点时间开始监听，
// 太短会让「VM 起来了但连不上」看起来像失败。
const vsockDialTimeout = 30 * time.Second

// DialVSock 连到指定 cid 的端口，带重试。
//
// 重试而不是一次失败就放弃：guest 内的 agent 与宿主拨号是并发的，
// 先连上后监听的情况很常见。ctx 取消时立即返回。
func DialVSock(ctx context.Context, cid, port uint32) (Transport, error) {
	if !VSockSupported() {
		return nil, ErrVSockUnsupported
	}
	deadline := time.Now().Add(vsockDialTimeout)
	var lastErr error
	for {
		tr, err := dialVSockOnce(ctx, cid, port)
		if err == nil {
			return tr, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("box: vsock %d:%d did not come up: %w", cid, port, lastErr)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}
