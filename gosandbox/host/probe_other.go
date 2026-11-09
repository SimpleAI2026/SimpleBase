//go:build !linux

// 文件说明：probe_other.go — gosandbox/host 的内部实现（landlockABI、seccompSupported 等）
//
// 用法
//
//	同包调用：landlockABI() → probe.go, probe_linux.go；seccompSupported() → probe.go, probe_linux.go；detectCgroup() → probe.go, probe_linux.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/probe_other.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 13 个
//	[关键引用] landlockABI() ← probe.go, probe_linux.go
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
