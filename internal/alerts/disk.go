// 文件说明：disk.go — 磁盘可用空间水位检测（production-readiness-plan §8.3.4）
//
// 用法
//
//	DiskFreeRatio(path string) (float64, error)  // 0.0-1.0；syscall.Statfs 标准库实现
package alerts

import (
	"syscall"
	"unsafe"
)

// DiskFreeRatio 返回 path 所在卷的可用空间比例（0.0-1.0）。
// 用 syscall.Statfs（标准库，macOS/Linux 均可；无 x/sys 依赖，§8.3.4 已查证）。
func DiskFreeRatio(path string) (float64, error) {
	var st syscall.Statfs_t
	if err := statfs(path, &st); err != nil {
		return 0, err
	}
	total := uint64(st.Bsize) * st.Blocks
	free := uint64(st.Bsize) * st.Bavail
	if total == 0 {
		return 0, nil
	}
	ratio := float64(free) / float64(total)
	if ratio < 0 {
		return 0, nil
	}
	if ratio > 1 {
		return 1, nil
	}
	return ratio, nil
}

// statfs 隔离平台差异（测试可替换）。
var statfs = func(path string, st *syscall.Statfs_t) error {
	// #nosec G104 — 错误显式返回
	return syscall.Statfs(path, st)
}

// ensure unsafe 引用不被裁剪（部分平台 Statfs_t 含 pad 需要 unsafe 断言尺寸）。
var _ = unsafe.Sizeof(syscall.Statfs_t{})
