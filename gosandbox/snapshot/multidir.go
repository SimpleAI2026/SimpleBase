// 文件说明：multidir.go — gosandbox/snapshot 的内部实现（packEntry、packAll 等）
//
// 用法
//
//	同包调用：packEntry → snapshot.go；packAll() → snapshot.go；unpackAll() → snapshot.go
//
// 调用架构
//
//	[上游] gosandbox/vm/box, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/snapshot/multidir.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] paths.go, snapshot.go, snapshot_test.go
//	[关键引用] packEntry ← snapshot.go；packAll ← snapshot.go；unpackAll ← snapshot.go
//
// 主要符号
//
//	type packEntry struct{…}
//	packAll(ctx context.Context, entries []packEntry, w io.Writer) error
//	walkPrefixed(src, prefix string, tw *tar.Writer) error
//	unpackAll(ctx context.Context, archivePath, boxDir string) error
//	writeEntry(tr *tar.Reader, hdr *tar.Header, target string) error

package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// packEntry 是一个要打包的目录及其在归档里的前缀。
type packEntry struct {
	dir    string
	prefix string
}

// packAll 把多个目录打进一个 tar.gz。
//
// 打包顺序固定（调用方给的顺序），这样同一状态多次快照得到相同的
// 未压缩字节流，便于内容寻址去重。
func packAll(ctx context.Context, entries []packEntry, w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	var err error
	for _, e := range entries {
		if cerr := ctx.Err(); cerr != nil {
			err = cerr
			break
		}
		if werr := walkPrefixed(e.dir, e.prefix, tw); werr != nil {
			err = werr
			break
		}
	}
	if cerr := tw.Close(); err == nil {
		err = cerr
	}
	if cerr := gz.Close(); err == nil {
		err = cerr
	}
	return err
}

// walkPrefixed 递归写一个目录，条目名带上前缀。
func walkPrefixed(src, prefix string, tw *tar.Writer) error {
	info, err := os.Lstat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("snapshot: %s is not a directory", src)
	}
	return filepath.WalkDir(src, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			// 打不开的条目跳过：快照宁可不完整，也不该整块失败。
			if errors.Is(werr, os.ErrNotExist) || errors.Is(werr, os.ErrPermission) {
				return nil
			}
			return werr
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		fi, ierr := d.Info()
		if ierr != nil {
			if errors.Is(ierr, os.ErrNotExist) {
				return nil
			}
			return ierr
		}
		link := ""
		switch {
		case fi.IsDir():
		case fi.Mode()&os.ModeSymlink != 0:
			if link, err = os.Readlink(p); err != nil {
				return nil
			}
		case fi.Mode().IsRegular():
		default:
			return nil // 设备 / FIFO / socket：跳过
		}
		hdr := &tar.Header{
			Name:     prefix + "/" + filepath.ToSlash(rel),
			Mode:     int64(fi.Mode().Perm()),
			ModTime:  fi.ModTime(),
			Typeflag: typeOf(fi, link),
			Linkname: link,
		}
		if fi.Mode().IsRegular() {
			hdr.Size = fi.Size()
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			f, oerr := os.Open(p)
			if oerr != nil {
				return nil
			}
			n, cerr := io.Copy(tw, io.LimitReader(f, fi.Size()))
			f.Close()
			if cerr != nil {
				return cerr
			}
			// 文件在遍历期间被截断：补齐声明长度，让归档仍然可读。
			for n < fi.Size() {
				if _, werr := tw.Write([]byte{0}); werr != nil {
					return werr
				}
				n++
			}
		}
		return nil
	})
}

// unpackAll 把归档解到 boxDir。
//
// 只替换归档里出现的前缀（upper / ws）；归档里没有 upper 时，
// boxDir 下残留的 upper 会被删掉——否则恢复出来的沙盒会带上
// 不属于这个快照的改动。
func unpackAll(ctx context.Context, archivePath, boxDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	// 清空已知前缀，避免与上次恢复的残留混在一起。
	known := []string{"upper", "ws"}
	for _, k := range known {
		if err := os.RemoveAll(filepath.Join(boxDir, k)); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(boxDir, "ws"), 0o700); err != nil {
		return err
	}
	root := filepath.Clean(boxDir)
	for {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		hdr, rerr := tr.Next()
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
		rel, nerr := safeRel(hdr.Name)
		if nerr != nil {
			return nerr
		}
		if rel == "" {
			continue
		}
		// 只接受已知前缀，防止归档往沙盒目录外写。
		slash := filepath.ToSlash(rel)
		prefix, rest, ok := strings.Cut(slash, "/")
		if !ok || (prefix != "upper" && prefix != "ws") {
			continue
		}
		target := filepath.Join(root, filepath.FromSlash(prefix), filepath.FromSlash(rest))
		if !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("%w: %s", ErrUnsafeArchive, hdr.Name)
		}
		if err := writeEntry(tr, hdr, target); err != nil {
			return err
		}
	}
}

// writeEntry 落一个归档条目。
func writeEntry(tr *tar.Reader, hdr *tar.Header, target string) error {
	mode := os.FileMode(hdr.Mode).Perm()
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, mode)
	case tar.TypeSymlink:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		_ = os.Remove(target)
		return os.Symlink(hdr.Linkname, target)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return err
		}
		_, cerr := io.Copy(f, io.LimitReader(tr, hdr.Size))
		f.Close()
		return cerr
	}
	return nil
}
