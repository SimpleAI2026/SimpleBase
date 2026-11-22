//go:build linux

package box

import (
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
)

// diskScanInterval 是 du 的最小间隔。
const diskScanInterval = 10 * time.Second

// diskCache 缓存每个沙盒上一次的用量。
type diskCacheEntry struct {
	at   time.Time
	used int64
}

var (
	diskMu    sync.Mutex
	diskCache = map[string]diskCacheEntry{}
	// diskFrozen 记录已经因为超限冻结过的沙盒，避免重复冻结刷日志。
	diskFrozen = map[string]bool{}
)

// diskUsed 统计一个沙盒可写部分的占用（带缓存）。
//
// 只算 upper 与 ws：rootfs/ 是镜像的拷贝（copy 模式），算进去会让
// 刚启动就「超限」；logs/ 是我们自己的日志，也不该算用户的额度。
func diskUsed(boxDir string) int64 {
	diskMu.Lock()
	if e, ok := diskCache[boxDir]; ok && time.Since(e.at) < diskScanInterval {
		diskMu.Unlock()
		return e.used
	}
	diskMu.Unlock()

	var total int64
	for _, d := range []string{"upper", "ws"} {
		total += dirSize(filepath.Join(boxDir, d))
	}
	diskMu.Lock()
	diskCache[boxDir] = diskCacheEntry{at: time.Now(), used: total}
	diskMu.Unlock()
	return total
}

// dirSize 递归累加一个目录里常规文件的大小。
//
// 不跟随符号链接：链接指向宿主时会把宿主的文件算进沙盒用量。
func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// checkDiskLimit 检查是否超限；超限则冻结沙盒并返回 true。
//
// 冻结而不是杀：用户还能 pause / 看日志 / 导出文件。
// 只冻一次——重复写 cgroup.freeze 没有意义，只会刷日志。
func (d *Driver) checkDiskLimit(name string, used int64) bool {
	limit := d.opts.DiskLimitBytes
	if limit <= 0 || used <= limit {
		return false
	}
	diskMu.Lock()
	already := diskFrozen[name]
	if !already {
		diskFrozen[name] = true
	}
	diskMu.Unlock()
	if already {
		return true
	}
	if err := host.FreezeCgroup(host.CgroupNamePrefix+name, true); err != nil {
		d.logf("warn", "sandbox is over its disk limit but could not be frozen",
			"name", name, "used", used, "limit", limit, "error", err.Error())
		return false
	}
	d.logf("warn", "sandbox frozen: disk limit exceeded",
		"name", name, "used", used, "limit", limit)
	return true
}

// forgetDiskFrozen 清理一个沙盒的冻结标记（沙盒删除时）。
func forgetDiskFrozen(name string) {
	diskMu.Lock()
	delete(diskFrozen, name)
	diskMu.Unlock()
}
