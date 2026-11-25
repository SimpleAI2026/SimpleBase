//go:build linux

// 文件说明：metrics_linux.go — gosandbox/host 中 MetricsFor、FreezeCgroup 等符号的实现
//
// 用法
//
//	MetricsFor(name string) (Metrics, bool)  // MetricsFor 读某个固定 cgroup 子组的用量
//	FreezeCgroup(name string, frozen bool) error  // FreezeCgroup 冻结或解冻子组内全部进程
//	RemoveCgroup(name string) error  // RemoveCgroup 删除固定子组；组内仍有进程时返回错误（调用方应先杀进程）
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/metrics_linux.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//
// 主要符号
//
//	MetricsFor(name string) (Metrics, bool)
//	FreezeCgroup(name string, frozen bool) error
//	RemoveCgroup(name string) error

package host

import (
	"errors"
	"os"
)

// MetricsFor 读某个固定 cgroup 子组的用量。子组不存在或 cgroup 未委派时返回 false。
func MetricsFor(name string) (Metrics, bool) {
	dir := cgroupPathFor(name)
	if dir == "" {
		return Metrics{}, false
	}
	g := &cgroupGroup{dir: dir}
	u := g.usage()
	return Metrics{
		MemoryBytes:     u.MemoryBytes,
		MemoryPeakBytes: u.MemoryPeak,
		CPUUsec:         u.CPUUsec,
		Pids:            u.Pids,
		OOMKills:        u.OOMKills,
		DiskReadBytes:   u.DiskReadByte,
		DiskWriteBytes:  u.DiskWriteByt,
		Backend:         "cgroup",
	}, true
}

// FreezeCgroup 冻结或解冻子组内全部进程。子组不存在时返回 os.ErrNotExist。
func FreezeCgroup(name string, frozen bool) error {
	dir := cgroupPathFor(name)
	if dir == "" {
		return os.ErrNotExist
	}
	return (&cgroupGroup{dir: dir}).freeze(frozen)
}

// RemoveCgroup 删除固定子组；组内仍有进程时返回错误（调用方应先杀进程）。
func RemoveCgroup(name string) error {
	dir := cgroupPathFor(name)
	if dir == "" {
		return nil
	}
	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
