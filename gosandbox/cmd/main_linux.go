//go:build linux

package main

import (
	"fmt"
	"os"

	"github.com/linkxzhou/SimpleBase/gosandbox/agent"
	"github.com/linkxzhou/SimpleBase/gosandbox/box"
	"github.com/linkxzhou/SimpleBase/gosandbox/host/inner"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--agent":
			// 沙盒常驻 PID 1：替宿主跑命令、读写文件、开 PTY。
			os.Exit(agent.Main(os.Args[2:], os.Environ()))
		case "--supervisor":
			// 托管一个沙盒：把它的 socketpair 接到一个 unix socket 上，
			// 服务进程重启后能重新接管。
			socket, policy, argv, _ := box.SuperviseArgs(os.Args[1:])
			if len(argv) == 0 {
				fmt.Fprintln(os.Stderr, "gosandbox-inner: --supervisor needs a command after --")
				os.Exit(2)
			}
			os.Exit(box.RunSupervisor(socket, policy, argv))
		}
	}
	os.Exit(inner.Main(os.Args[1:], os.Environ()))
}
