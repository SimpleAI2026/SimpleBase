//go:build linux || darwin

// 文件说明：ptyx_unix_test.go — gosandbox/internal/ptyx 的单元测试（TestOpenPair、TestSetWinsize）
//
// 用法
//
//	go test ./internal/ptyx -run 'TestOpenPair|TestSetWinsize'
//
// 调用架构
//
//	[上游] 测试
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/internal/ptyx/ptyx_unix_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 + golang.org/x/sys/unix
//	[同包] ptyx.go, ptyx_darwin.go, ptyx_linux.go, ptyx_unix.go
//
// 主要符号
//
//	readFull(t *testing.T, f *os.File, n int) string
//	TestOpenPair(t *testing.T)
//	TestSetWinsize(t *testing.T)

package ptyx

import (
	"io"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// readFull 恰好读 n 字节；5 秒读不到就失败，不让测试挂死。
func readFull(t *testing.T, f *os.File, n int) string {
	t.Helper()
	type result struct {
		s   string
		err error
	}
	done := make(chan result, 1)
	go func() {
		buf := make([]byte, n)
		_, err := io.ReadFull(f, buf)
		done <- result{string(buf), err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("read %d bytes: %v", n, r.err)
		}
		return r.s
	case <-time.After(5 * time.Second):
		t.Fatalf("read %d bytes: timed out", n)
		return ""
	}
}

// 打开的一对伪终端可双向读写：slave 的输出能从 master 读到，master 的输入能从 slave 读到。
func TestOpenPair(t *testing.T) {
	master, slave, err := Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	if _, err := slave.Write([]byte("pong")); err != nil {
		t.Fatalf("slave write: %v", err)
	}
	// 不带换行：输出方向的 ONLCR 会把 \n 改写成 \r\n。
	if got := readFull(t, master, 4); got != "pong" {
		t.Fatalf("master read %q, want %q", got, "pong")
	}

	// 输入方向：从端默认是规范模式（按行交付），所以要带换行写。
	if _, err := master.Write([]byte("ping\n")); err != nil {
		t.Fatalf("master write: %v", err)
	}
	if got := readFull(t, slave, 5); got != "ping\n" {
		t.Fatalf("slave read %q, want %q", got, "ping\n")
	}
}

// 对一端设置窗口大小后，另一端读到的也是新值：两端指向同一个 tty，调用方不必区分。
func TestSetWinsize(t *testing.T) {
	master, slave, err := Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer master.Close()
	defer slave.Close()
	cases := []struct {
		name       string
		set, get   *os.File
		rows, cols uint16
	}{
		{"master to slave", master, slave, 40, 120},
		{"slave to master", slave, master, 24, 80},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := SetWinsize(tc.set, tc.rows, tc.cols); err != nil {
				t.Fatalf("set winsize: %v", err)
			}
			ws, err := unix.IoctlGetWinsize(int(tc.get.Fd()), unix.TIOCGWINSZ)
			if err != nil {
				t.Fatalf("get winsize: %v", err)
			}
			if ws.Row != tc.rows || ws.Col != tc.cols {
				t.Fatalf("winsize = %dx%d, want %dx%d", ws.Row, ws.Col, tc.rows, tc.cols)
			}
		})
	}
}
