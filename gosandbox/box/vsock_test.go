//go:build linux

package box

// vsock_test.go：vsock 拨号的重试与失败语义。
//
// 真正的 vsock 需要内核模块与另一个 VM，CI 里通常没有。
// 所以这里测的是「拨号层的行为」：不可用时明确报错、超时可取消，
// 而不是 hang。真正的端到端留给 macOS VM 后端（M5-21 的剩余部分）。

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 不支持的平台/环境：明确报错，不是 hang。
func TestDialVSockUnsupported(t *testing.T) {
	if VSockSupported() {
		t.Skip("this host has vsock; the unsupported path cannot be tested here")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := DialVSock(ctx, HostCID, VSOCKPortAgent)
	if !errors.Is(err, ErrVSockUnsupported) {
		t.Fatalf("err = %v, want ErrVSockUnsupported", err)
	}
}

// ctx 取消后立即返回，不等满 30 秒的拨号窗口。
func TestDialVSockHonorsContext(t *testing.T) {
	if !VSockSupported() {
		t.Skip("no vsock on this host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	// 一个几乎不可能有服务的 cid:port。
	_, err := DialVSock(ctx, 42, 65000)
	if err == nil {
		t.Skip("unexpectedly connected; nothing to assert")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("DialVSock ignored the context (%v)", elapsed)
	}
}

// 端口常量不与系统保留段冲突。
func TestVSockPortConventions(t *testing.T) {
	if VSOCKPortAgent < 1024 || VSOCKPortEcho < 1024 {
		t.Fatal("avoid the reserved 0-1023 range")
	}
	if VSOCKPortAgent == VSOCKPortEcho {
		t.Fatal("the two ports must differ")
	}
	// HostCID / GuestCID 是 virtio 规范里固定的值。
	if HostCID != 2 || GuestCID != 3 {
		t.Fatalf("cid constants drifted: host=%d guest=%d", HostCID, GuestCID)
	}
}
