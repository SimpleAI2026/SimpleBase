//go:build !linux

// 文件说明：watch_other.go — 非 Linux 上 agent 不支持目录监听
//
// 用法
//
//	(s *Server) watch(m proto.Msg)    // 回错误
//	(s *Server) unwatch(m proto.Msg)  // 空操作
//	(s *Server) stopWatches()         // 空操作
//
// 调用架构
//
//	[上游] gosandbox/agent（读循环）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/watch_other.go
//	         │
//	         ▼
//	[下游] gosandbox/proto
//	[同包] agent.go, watch_linux.go
//
// 主要符号
//
//	(s *Server) watch(m proto.Msg)
//	(s *Server) unwatch(m proto.Msg)
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
