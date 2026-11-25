//go:build linux

// 文件说明：metrics_linux.go — gosandbox/vm/box 中 Driver.Metrics、Driver.Pause 等符号的实现
//
// 用法
//
//	(d *Driver) Metrics(ctx context.Context, name string) (vm.Metrics, error)  // Metrics 读沙盒用量
//	(d *Driver) Pause(_ context.Context, name string) error  // Pause 冻结沙盒内全部进程（cgroup.freeze）
//	(d *Driver) Resume(_ context.Context, t vm.Target) error  // Resume 解冻
//	(d *Driver) CanFreeze() bool  // CanFreeze 报告 cgroup 是否已委派（冻结需要 cgroup v2）
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/metrics_linux.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] capabilities.go, contracts_linux_test.go, diskuse_linux.go, driver_linux.go, driver_other.go 等 15 个
//
// 主要符号
//
//	(d *Driver) Metrics(ctx context.Context, name string) (vm.Metrics, error)
//	(d *Driver) Pause(_ context.Context, name string) error
//	(d *Driver) Resume(_ context.Context, t vm.Target) error
//	(d *Driver) CanFreeze() bool

package box

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Metrics 读沙盒用量。
func (d *Driver) Metrics(ctx context.Context, name string) (vm.Metrics, error) {
	s := d.session(name)
	if s == nil {
		return vm.Metrics{}, fmt.Errorf("%w: sandbox is not running", vm.ErrNotSupported)
	}
	st, err := s.Stats()
	if err != nil {
		return vm.Metrics{}, fmt.Errorf("%w: metrics", vm.ErrBackend)
	}
	// 磁盘用量在宿主侧算（沙盒里看到的是 overlay 合并视图，分不出用户写入）。
	used := diskUsed(filepath.Join(d.opts.Root, name))
	// 超限就地冻结：等到 reaper 下一轮再处理，用户可能已经把盘写满。
	d.checkDiskLimit(name, used)
	return vm.Metrics{
		MemoryBytes:     st.MemoryBytes,
		MemoryPeakBytes: st.MemoryPeak,
		CPUUsec:         st.CPUUsec,
		Pids:            st.Pids,
		OOMKills:        st.OOMKills,
		DiskReadBytes:   st.DiskReadBytes,
		DiskWriteBytes:  st.DiskWriteBytes,
		DiskUsedBytes:   used,
		Backend:         "cgroup",
	}, nil
}

// Pause 冻结沙盒内全部进程（cgroup.freeze）。
func (d *Driver) Pause(_ context.Context, name string) error {
	if err := host.FreezeCgroup(host.CgroupNamePrefix+name, true); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%w: pause", vm.ErrBackend)
	}
	return nil
}

// Resume 解冻。
func (d *Driver) Resume(_ context.Context, t vm.Target) error {
	if err := host.FreezeCgroup(host.CgroupNamePrefix+t.Name, false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%w: resume", vm.ErrBackend)
	}
	return nil
}

// CanFreeze 报告 cgroup 是否已委派（冻结需要 cgroup v2）。
func (d *Driver) CanFreeze() bool {
	d.freezeOnce.Do(func() {
		caps, err := host.Probe(context.Background(), host.Policy{
			Enabled: true, Mode: host.ModeWorkspaceWrite, Workspace: d.opts.Root,
		})
		d.freezeOK = err == nil && caps.Cgroup == "delegated"
	})
	return d.freezeOK
}
