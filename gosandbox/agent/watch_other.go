//go:build !linux

// 文件说明：watch_other.go — gosandbox/agent 的内部实现（watcher、Server.watch 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] gosandbox/cmd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/watch_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent.go, agent_test.go, fs.go, loopback_linux.go, loopback_other.go 等 16 个
//
// 主要符号
//
//	type watcher struct{…}
//	(s *Server) watch(proto.Msg)
//	(s *Server) unwatch(proto.Msg)
//	(s *Server) stopWatches()

package agent

import "github.com/linkxzhou/SimpleBase/gosandbox/proto"

// watcher 在非 Linux 上是空类型：没有 inotify，也没有监听。
type watcher struct{}

// watch 在非 Linux 上不可用：inotify 是 Linux 专有的。
func (s *Server) watch(proto.Msg) {
	_ = s.send(&proto.Msg{Kind: proto.KindError, Err: "directory watching is only available on linux"})
}

// unwatch 无事可做。
func (s *Server) unwatch(proto.Msg) {}

// stopWatches 无事可做。
func (s *Server) stopWatches() {}
