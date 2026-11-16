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
