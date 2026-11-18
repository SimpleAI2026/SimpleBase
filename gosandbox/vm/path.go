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
// os.Root 保证。workdir 必须是已 Clean 的绝对路径。
func CleanPath(workdir, p string, allowRoot bool) (string, error) {
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
	if !strings.HasPrefix(clean, workdir+"/") {
		return "", fmt.Errorf("%w: path must be under %s", ErrInvalidPath, workdir)
	}
	return clean, nil
}
