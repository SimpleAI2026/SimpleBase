// 文件说明：export_test.go — gosandbox/vm/local 的测试辅助代码
//
// 用法
//
//	同包调用：Driver.wsDir → local_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/local/export_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] exec.go, files.go, local.go, local_test.go, metrics.go 等 8 个
//	[关键引用] Driver.wsDir ← local_test.go
//
// 主要符号
//
//	(d *Driver) wsDir(name string) (string, error)

package local

import "path/filepath"

// wsDir 返回 VM 的工作区目录（{root}/{name}/ws）。
// 仅测试可见：测试需要直接触碰工作区目录来构造符号链接逃逸等场景；
// 生产路径统一走 ensure()（含懒创建与符号链接解析）。
func (d *Driver) wsDir(name string) (string, error) {
	dir, err := d.vmDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, wsDirName), nil
}
