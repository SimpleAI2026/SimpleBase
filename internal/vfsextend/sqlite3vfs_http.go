package vfsextend

import (
	"github.com/linkxzhou/LessDB/internal/s3"
	"github.com/linkxzhou/LessDB/internal/sqlite3vfs"

	"net/http"
	"strings"
	"sync"
)

var vfscache sync.Map

type HttpVFS struct {
	CacheHandler CacheHandler
	URIHandler   s3.URIHandler
	RoundTripper http.RoundTripper
}

func (vfs *HttpVFS) Open(name string, flags sqlite3vfs.OpenFlag) (sqlite3vfs.File, sqlite3vfs.OpenFlag, error) {
	var opts []Option
	if vfs.CacheHandler != nil {
		opts = append(opts, WithCacheHandler(vfs.CacheHandler))
	}

	if vfs.RoundTripper != nil {
		opts = append(opts, WithRoundTripper(vfs.RoundTripper))
	}

	if vfs.URIHandler != nil {
		opts = append(opts, WithURIHandler(vfs.URIHandler))
	}

	if v, ok := vfscache.Load(name); ok {
		return v.(*httpFile), flags, nil
	}

	hf := &httpFile{rr: New(name, opts...)}
	vfscache.Store(name, hf)

	return hf, flags, nil
}

func (vfs *HttpVFS) Delete(name string, dirSync bool) error {
	return sqlite3vfs.PermissionsErrorDelete
}

func (vfs *HttpVFS) Access(name string, flag sqlite3vfs.AccessFlag) (bool, error) {
	// TODO: access has error
	if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-journal") {
		return false, nil
	}

	return true, nil
}

func (vfs *HttpVFS) FullPathname(name string) string {
	return name
}

type httpFile struct {
	rr *RangeReader
}

func (hf *httpFile) Close() error {
	return nil // TODO: close file
}

func (hf *httpFile) ReadAt(p []byte, off int64) (int, error) {
	return hf.rr.ReadAt(p, off)
}

func (hf *httpFile) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, sqlite3vfs.PermissionsErrorWrite
}

func (hf *httpFile) Truncate(size int64) error {
	return sqlite3vfs.PermissionsErrorTruncate
}

func (hf *httpFile) Sync(flag sqlite3vfs.SyncType) error {
	return nil // TODO: add sync
}

func (hf *httpFile) FileSize() (int64, error) {
	return hf.rr.Size()
}

func (hf *httpFile) Lock(elock sqlite3vfs.LockType) error {
	return nil
}

func (hf *httpFile) Unlock(elock sqlite3vfs.LockType) error {
	return nil
}

func (hf *httpFile) CheckReservedLock() (bool, error) {
	return false, nil
}

func (hf *httpFile) SectorSize() int64 {
	return 0
}

func (hf *httpFile) DeviceCharacteristics() sqlite3vfs.DeviceCharacteristic {
	return sqlite3vfs.IocapImmutable
}
