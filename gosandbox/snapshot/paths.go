package snapshot

import (
	"archive/tar"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrUnsafeArchive 表示归档里有越界条目（路径穿越或绝对路径）。
var ErrUnsafeArchive = errors.New("snapshot: archive contains an unsafe path")

// typeOf 决定 tar 条目类型。
func typeOf(fi os.FileInfo, link string) byte {
	switch {
	case fi.IsDir():
		return tar.TypeDir
	case link != "":
		return tar.TypeSymlink
	}
	return tar.TypeReg
}

// safeRel 规范化归档里的条目名。
//
// 返回空串表示「跳过这个条目」（目录自身、./ 前缀）。绝对路径与
// 含 .. 的路径一律拒绝：这是解包时唯一能用的防线。
func safeRel(name string) (string, error) {
	n := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(n, "/") {
		return "", fmt.Errorf("%w: absolute path %q", ErrUnsafeArchive, name)
	}
	clean := path.Clean(n)
	if clean == "." || clean == "" {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q", ErrUnsafeArchive, name)
	}
	return filepath.FromSlash(clean), nil
}
