//go:build linux

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
