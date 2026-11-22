// Package atomicfile writes files atomically within their existing directory.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write writes path+".tmp" and renames it over path.
// Errors are returned unchanged and temporary files are removed on failure.
func Write(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// WriteEnsureDir creates the parent directory before calling Write.
func WriteEnsureDir(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return Write(path, data, perm)
}
