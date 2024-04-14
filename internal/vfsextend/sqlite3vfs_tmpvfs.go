package vfsextend

import (
	"github.com/linkxzhou/LessDB/internal/sqlite3vfs"

	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

type TmpVFS struct {
	tmpdir string
}

func newTmpVFS(dir string) *TmpVFS {
	dir, err := os.MkdirTemp(dir, "sqlite3vfs_tmpdir_")
	if err != nil {
		panic(err)
	}

	return &TmpVFS{
		tmpdir: dir,
	}
}

func (vfs *TmpVFS) Open(name string, flags sqlite3vfs.OpenFlag) (sqlite3vfs.File, sqlite3vfs.OpenFlag, error) {
	var (
		f   *os.File
		err error
	)

	if name == "" {
		f, err = os.CreateTemp(vfs.tmpdir, "")
		if err != nil {
			return nil, 0, sqlite3vfs.CantOpenError
		}
	} else {
		fname := filepath.Join(vfs.tmpdir, name)
		if !strings.HasPrefix(fname, vfs.tmpdir) {
			return nil, 0, sqlite3vfs.PermError
		}
		var fileFlags int
		if flags&sqlite3vfs.OpenExclusive != 0 {
			fileFlags |= os.O_EXCL
		}
		if flags&sqlite3vfs.OpenCreate != 0 {
			fileFlags |= os.O_CREATE
		}
		if flags&sqlite3vfs.OpenReadOnly != 0 {
			fileFlags |= os.O_RDONLY
		}
		if flags&sqlite3vfs.OpenReadWrite != 0 {
			fileFlags |= os.O_RDWR
		}
		f, err = os.OpenFile(fname, fileFlags, 0600)
		if err != nil {
			return nil, 0, sqlite3vfs.CantOpenError
		}
	}

	tf := &TmpFile{f: f}
	return tf, flags, nil
}

func (vfs *TmpVFS) Delete(name string, dirSync bool) error {
	fname := filepath.Join(vfs.tmpdir, name)
	if !strings.HasPrefix(fname, vfs.tmpdir) {
		return errors.New("illegal path")
	}
	return os.Remove(fname)
}

func (vfs *TmpVFS) Access(name string, flag sqlite3vfs.AccessFlag) (bool, error) {
	fname := filepath.Join(vfs.tmpdir, name)
	if !strings.HasPrefix(fname, vfs.tmpdir) {
		return false, errors.New("illegal path")
	}

	exists := true
	_, err := os.Stat(fname)
	if err != nil && os.IsNotExist(err) {
		exists = false
	} else if err != nil {
		return false, err
	}

	if flag == sqlite3vfs.AccessExists {
		return exists, nil
	}

	return true, nil
}

func (vfs *TmpVFS) FullPathname(name string) string {
	fname := filepath.Join(vfs.tmpdir, name)
	if !strings.HasPrefix(fname, vfs.tmpdir) {
		return ""
	}

	return strings.TrimPrefix(fname, vfs.tmpdir)
}

type TmpFile struct {
	lockCount int64
	f         *os.File
}

func (tf *TmpFile) Close() error {
	return tf.f.Close()
}

func (tf *TmpFile) ReadAt(p []byte, off int64) (n int, err error) {
	return tf.f.ReadAt(p, off)
}

func (tf *TmpFile) WriteAt(b []byte, off int64) (n int, err error) {
	return tf.f.WriteAt(b, off)
}

func (tf *TmpFile) Truncate(size int64) error {
	return tf.f.Truncate(size)
}

func (tf *TmpFile) Sync(flag sqlite3vfs.SyncType) error {
	return tf.f.Sync()
}

func (tf *TmpFile) FileSize() (int64, error) {
	cur, _ := tf.f.Seek(0, io.SeekCurrent)
	end, err := tf.f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}

	tf.f.Seek(cur, io.SeekStart)
	return end, nil
}

func (tf *TmpFile) Lock(elock sqlite3vfs.LockType) error {
	if elock == sqlite3vfs.LockNone {
		return nil
	}
	atomic.AddInt64(&tf.lockCount, 1)
	return nil
}

func (tf *TmpFile) Unlock(elock sqlite3vfs.LockType) error {
	if elock == sqlite3vfs.LockNone {
		return nil
	}
	atomic.AddInt64(&tf.lockCount, -1)
	return nil
}

func (tf *TmpFile) CheckReservedLock() (bool, error) {
	count := atomic.LoadInt64(&tf.lockCount)
	return count > 0, nil
}

func (tf *TmpFile) SectorSize() int64 {
	return 0
}

func (tf *TmpFile) DeviceCharacteristics() sqlite3vfs.DeviceCharacteristic {
	return 0
}
