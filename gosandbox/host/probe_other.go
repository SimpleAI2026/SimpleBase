//go:build !linux

// 文件说明：probe_other.go — gosandbox/host 的内部实现（landlockABI、seccompSupported 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/probe_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//
// 主要符号
//
//	landlockABI() int
//	seccompSupported() bool
//	detectCgroup() string

package host

func landlockABI() int       { return 0 }
func seccompSupported() bool { return false }
func detectCgroup() string   { return cgroupNone }
