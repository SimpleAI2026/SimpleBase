//go:build linux

// 文件说明：rlimit_linux.go — agent 在 fork 前后应用资源上限
//
// 用法
//
//	restore, err := applyRlimits(rs []proto.Rlimit)  // 先设上限，返回恢复函数
//	defer restore()  // cmd.Start() 之后调用：子进程已继承，agent 自己不受影响
//
// 调用架构
//
//	[上游] gosandbox/agent/proc.go（start）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/rlimit_linux.go
//	         │
//	         ▼
//	[下游] gosandbox/proto、golang.org/x/sys/unix
//	[同包] rlimit_other.go
//
// 主要符号
//
//	applyRlimits(rs []proto.Rlimit) (func(), error)

package agent

import (
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// rlimitNames 把协议里的名字映射到内核常量。
// 只暴露这几个：沙盒里放大别的限制只会让宿主更难控。
var rlimitNames = map[string]int{
	"nofile": unix.RLIMIT_NOFILE,
	"nproc":  unix.RLIMIT_NPROC,
	"core":   unix.RLIMIT_CORE,
	"fsize":  unix.RLIMIT_FSIZE,
	"as":     unix.RLIMIT_AS,
	"stack":  unix.RLIMIT_STACK,
}

// applyRlimits 在当前进程上设上限，返回恢复函数。
//
// Go 的 exec.Cmd 没有「只给子进程设 rlimit」的钩子，而 fork 会继承父进程的
// 上限，所以顺序必须是：设上限 → cmd.Start() → 恢复。调用方负责在 Start
// 之后（含失败路径）调用返回的恢复函数，否则 agent 自己也会被限制住。
func applyRlimits(rs []proto.Rlimit) (func(), error) {
	if len(rs) == 0 {
		return func() {}, nil
	}
	restore := make([]func(), 0, len(rs))
	undo := func() {
		for i := len(restore) - 1; i >= 0; i-- {
			restore[i]()
		}
	}
	for _, r := range rs {
		id, ok := rlimitNames[r.Name]
		if !ok {
			// 不认识的名字不当作错误：宿主可能是新版本。
			continue
		}
		var old unix.Rlimit
		if err := unix.Getrlimit(id, &old); err != nil {
			continue
		}
		cur, max := r.Cur, r.Max
		if cur > max {
			cur = max
		}
		next := unix.Rlimit{Cur: cur, Max: max}
		if err := unix.Setrlimit(id, &next); err != nil {
			undo()
			return func() {}, fmt.Errorf("setrlimit %s: %w", r.Name, err)
		}
		prev, prevID := old, id
		restore = append(restore, func() { _ = unix.Setrlimit(prevID, &prev) })
	}
	return undo, nil
}
