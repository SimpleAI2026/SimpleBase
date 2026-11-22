package local

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

func (d *Driver) openRoot(t vm.Target) (*os.Root, string, error) {
	ws, err := d.ensure(t)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(ws)
	if err != nil {
		return nil, "", err
	}
	return root, d.workdir(t), nil
}

// ReadFile 读取至多 MaxFileBytes+1 字节（上层据此判断截断）。
func (d *Driver) ReadFile(_ context.Context, t vm.Target, p string) ([]byte, error) {
	root, workdir, err := d.openRoot(t)
	if err != nil {
		return nil, d.fail("read", t.Name, err, "path", p)
	}
	defer root.Close()
	r, err := rel(workdir, p)
	if err != nil {
		return nil, d.fail("read", t.Name, err, "path", p)
	}
	f, err := root.OpenFile(r, os.O_RDONLY|nonBlock, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, vm.ErrFileNotFound
	}
	if err != nil {
		return nil, d.fail("read", t.Name, err, "path", p)
	}
	defer f.Close()
	// 沙盒内的命令可能留下 FIFO / 设备文件；只读普通文件。
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		if err == nil {
			err = errors.New("not a regular file")
		}
		return nil, d.fail("read", t.Name, err, "path", p)
	}
	limit := d.opts.MaxFileBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, d.fail("read", t.Name, err, "path", p)
	}
	return b, nil
}

// WriteFile 写入文件并按需创建父目录。
func (d *Driver) WriteFile(_ context.Context, t vm.Target, p string, data []byte) error {
	root, workdir, err := d.openRoot(t)
	if err != nil {
		return d.fail("write", t.Name, err, "path", p)
	}
	defer root.Close()
	r, err := rel(workdir, p)
	if err != nil || r == "." {
		if err == nil {
			err = errors.New("path is the workspace itself")
		}
		return d.fail("write", t.Name, err, "path", p)
	}
	// 目标若已被沙盒内命令替换成 FIFO / 设备，O_WRONLY 打开会阻塞，所以先确认它是普通文件。
	if st, lerr := root.Lstat(r); lerr == nil && !st.Mode().IsRegular() && st.Mode()&fs.ModeSymlink == 0 {
		return d.fail("write", t.Name, errors.New("target is not a regular file"), "path", p)
	}
	if dir := path.Dir(r); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return d.fail("write", t.Name, err, "path", p)
		}
	}
	if err := root.WriteFile(r, data, 0o644); err != nil {
		return d.fail("write", t.Name, err, "path", p, "bytes", len(data))
	}
	return nil
}

// RemoveFile 删除文件；不存在视为成功。
func (d *Driver) RemoveFile(_ context.Context, t vm.Target, p string) error {
	root, workdir, err := d.openRoot(t)
	if err != nil {
		return d.fail("remove file", t.Name, err, "path", p)
	}
	defer root.Close()
	r, err := rel(workdir, p)
	if err != nil || r == "." {
		if err == nil {
			err = errors.New("path is the workspace itself")
		}
		return d.fail("remove file", t.Name, err, "path", p)
	}
	if err := root.Remove(r); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return d.fail("remove file", t.Name, err, "path", p)
	}
	return nil
}

// ListDir 单层列目录，至多 501 项（上层截到 500）。
func (d *Driver) ListDir(_ context.Context, t vm.Target, p string) ([]vm.FileEntry, error) {
	root, workdir, err := d.openRoot(t)
	if err != nil {
		return nil, d.fail("list", t.Name, err, "path", p)
	}
	defer root.Close()
	r, err := rel(workdir, p)
	if err != nil {
		return nil, d.fail("list", t.Name, err, "path", p)
	}
	f, err := root.OpenFile(r, os.O_RDONLY|nonBlock, 0)
	if err != nil {
		return nil, d.fail("list", t.Name, err, "path", p)
	}
	defer f.Close()
	entries, err := f.ReadDir(501)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, d.fail("list", t.Name, err, "path", p)
	}
	base := path.Clean(p)
	out := make([]vm.FileEntry, 0, len(entries))
	for _, e := range entries {
		entry := vm.FileEntry{Name: e.Name(), Path: path.Join(base, e.Name()), Kind: kindOf(e.Type())}
		if info, ierr := e.Info(); ierr == nil {
			if info.Mode().IsRegular() {
				entry.Size = info.Size()
			}
			entry.Mode = uint32(info.Mode().Perm())
			entry.ModTime = info.ModTime()
		}
		out = append(out, entry)
	}
	return out, nil
}

func kindOf(m fs.FileMode) string {
	switch {
	case m.IsRegular():
		return "file"
	case m.IsDir():
		return "directory"
	case m&fs.ModeSymlink != 0:
		return "symlink"
	default:
		return "other"
	}
}
