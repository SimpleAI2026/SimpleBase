//go:build linux

// 文件说明：logdir.go — 沙盒日志目录的路径与列举
//
// 用法
//
//	(d *Driver) logDir(name string) string
//	logNamesIn(dir string) ([]string, error)
//
// 调用架构
//
//	[上游] gosandbox/vm/box（capabilities.go 的 Logs）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/logdir.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] capabilities.go, driver_linux.go
//
// 主要符号
//
//	(d *Driver) logDir(name string) string
//	logNamesIn(dir string) ([]string, error)

package box

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// errNotExist 是 os.ErrNotExist 的别名，便于在本包里统一判断。
var errNotExist = os.ErrNotExist

// logDir 返回一个沙盒的日志目录。
//
// 目录固定在 <root>/<name>/logs：沙盒删掉时随它一起删，
// 不会留下找不到主人的日志。
func (d *Driver) logDir(name string) string {
	if name == "" {
		return ""
	}
	return filepath.Join(d.opts.Root, name, "logs")
}

// logNamesIn 列出日志目录下的进程名（去掉 .log 后缀）。
func logNamesIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errNotExist
		}
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		base := strings.TrimSuffix(name, ".log")
		if seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, base)
	}
	sort.Strings(out)
	return out, nil
}
