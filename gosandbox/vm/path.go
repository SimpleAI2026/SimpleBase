// 文件说明：path.go — gosandbox/vm 中 CleanPath 等符号的实现
//
// 用法
//
//	CleanPath(workdir, p string, allowRoot bool) (string, error)  // CleanPath 校验 p 落在 workdir 之下，返回清理后的逻辑绝对路径； allowRoot=false 时拒绝 workdir 本身
//
// 调用架构
//
//	[上游] gosandbox/vm/box, gosandbox/vm/fake, gosandbox/vm/local, internal/api 等 7 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/path.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] driver.go, env.go, env_test.go, errors.go, output.go 等 9 个
//	[关键引用] CleanPath ← path_test.go
//
// 主要符号
//
//	var ErrInvalidPath
//	CleanPath(workdir, p string, allowRoot bool) (string, error)

package vm

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// ErrInvalidPath 表示逻辑路径不合法：空、含 NUL、非绝对，或越出工作目录。
var ErrInvalidPath = errors.New("sandbox: invalid path")

// CleanPath 校验 p 落在 workdir 之下，返回清理后的逻辑绝对路径；
// allowRoot=false 时拒绝 workdir 本身。
//
// 逻辑路径是 API 层看到的路径（Spec.Workdir 之下），真实目录由驱动映射。
// 这里只做字符串层面的收敛，不碰文件系统：符号链接与实际可写边界由驱动用
// os.Root 保证。
func CleanPath(workdir, p string, allowRoot bool) (string, error) {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" || !strings.HasPrefix(workdir, "/") || strings.ContainsRune(workdir, 0) {
		return "", fmt.Errorf("%w: workdir must be an absolute path", ErrInvalidPath)
	}
	workdir = path.Clean(workdir)
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("%w: path is required", ErrInvalidPath)
	}
	if strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("%w: path contains NUL", ErrInvalidPath)
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%w: path must be absolute", ErrInvalidPath)
	}
	clean := path.Clean(p)
	if clean == workdir {
		if allowRoot {
			return clean, nil
		}
		return "", fmt.Errorf("%w: path must be a file under %s", ErrInvalidPath, workdir)
	}
	prefix := workdir
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if !strings.HasPrefix(clean, prefix) {
		return "", fmt.Errorf("%w: path must be under %s", ErrInvalidPath, workdir)
	}
	return clean, nil
}
