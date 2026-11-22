//go:build !linux

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
