//go:build linux

// 文件说明：main_linux.go — gosandbox-inner：沙盒内 helper、agent、supervisor 三个模式
//
// 用法
//
//	gosandbox-inner --policy FILE [--ctty] -- CMD...   一次性命令
//	gosandbox-inner --agent --ctrl N [--uidmap SPEC]   沙盒 PID 1
//	gosandbox-inner --supervisor --socket P --policy F -- CMD...  托管一个沙盒（服务重启不丢）
//
// 三个模式共用一个二进制：supervisor 也需要在宿主侧运行，而它要拉起的
// 正是同一个 helper，多一个产物会让部署多一处可能装错的地方。
//
// 调用架构
//
//	[上游] （无，本包为入口）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/cmd/main_linux.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/host/inner（一次性模式）、gosandbox/agent（agent 模式）、
//	        gosandbox/box（supervisor 模式）
//	[同包] main_other.go
//
// 主要符号
//
//	main()

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
