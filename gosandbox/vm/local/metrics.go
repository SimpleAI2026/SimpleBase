// 文件说明：metrics.go — gosandbox/vm/local 的内部实现（CanFreeze、cgroupName 等）
//
// 用法
//
//	同包调用：cgroupName() → local.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/local/metrics.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] exec.go, export_test.go, files.go, local.go, local_test.go 等 8 个
//	[关键引用] cgroupName ← local.go
//
// 主要符号
//
//	(d *Driver) CanFreeze() bool
//	cgroupName(name string) string
//	(d *Driver) Metrics(_ context.Context, name string) (vm.Metrics, error)
//	(d *Driver) Pause(_ context.Context, name string) error
//	(d *Driver) Resume(_ context.Context, t vm.Target) error

package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

var (
	_ vm.Driver      = (*Driver)(nil)
	_ vm.Interactive = (*Driver)(nil)
	_ vm.Meter       = (*Driver)(nil)
	_ vm.Pauser      = (*Driver)(nil)
	_ vm.Freezer     = (*Driver)(nil)
)

// CanFreeze 报告本机是否真的能冻结：需要 cgroup v2 已委派（cgroup.freeze 在 v2 才有）。
// 探测结果进程内缓存，失败不影响其它能力。
func (d *Driver) CanFreeze() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	d.freezeOnce.Do(func() {
		policy := d.policy(d.opts.Root, vm.Spec{})
		caps, err := host.Probe(context.Background(), policy)
		d.freezeOK = err == nil && caps.Cgroup == "delegated"
	})
	return d.freezeOK
}

// cgroupName 是本机驱动给某个沙盒用的固定 cgroup 子组名。
// 与 host.CgroupNamePrefix 一致，host.MetricsFor 只接受带该前缀的名字。
func cgroupName(name string) string { return host.CgroupNamePrefix + name }

// Metrics 实现 vm.Meter：读沙盒的 cgroup 用量。
// cgroup 未委派时返回 vm.ErrNotSupported，上层据此标注「无指标」。
func (d *Driver) Metrics(_ context.Context, name string) (vm.Metrics, error) {
	if _, err := d.vmDir(name); err != nil {
		return vm.Metrics{}, d.fail("metrics", name, err)
	}
	m, ok := host.MetricsFor(cgroupName(name))
	if !ok {
		return vm.Metrics{}, fmt.Errorf("%w: metrics need a delegated cgroup v2", vm.ErrNotSupported)
	}
	return vm.Metrics{
		MemoryBytes:     m.MemoryBytes,
		MemoryPeakBytes: m.MemoryPeakBytes,
		CPUUsec:         m.CPUUsec,
		Pids:            m.Pids,
		OOMKills:        m.OOMKills,
		DiskReadBytes:   m.DiskReadBytes,
		DiskWriteBytes:  m.DiskWriteBytes,
		Backend:         m.Backend,
	}, nil
}

// Pause 实现 vm.Pauser：冻结沙盒内的全部进程（cgroup.freeze）。
// 本机驱动没有常驻进程，冻结的是「正在跑的命令」；没有命令在跑时是空操作。
func (d *Driver) Pause(_ context.Context, name string) error {
	if _, err := d.vmDir(name); err != nil {
		return d.fail("pause", name, err)
	}
	if err := host.FreezeCgroup(cgroupName(name), true); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return d.fail("pause", name, err)
	}
	return nil
}

// Resume 实现 vm.Pauser：解冻。沙盒不存在时视为成功（下次 Exec 会懒创建）。
func (d *Driver) Resume(_ context.Context, t vm.Target) error {
	if _, err := d.vmDir(t.Name); err != nil {
		return d.fail("resume", t.Name, err)
	}
	if err := host.FreezeCgroup(cgroupName(t.Name), false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return d.fail("resume", t.Name, err)
	}
	return nil
}
