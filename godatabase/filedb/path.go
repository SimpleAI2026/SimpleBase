package filedb

import (
	"fmt"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"os"
	"path/filepath"
	"strings"
)

func localDatabaseFile(dataRoot, cacheDir, projectID, databasePath string, remote bool) (string, error) {
	if err := godatabase.ValidateProjectPath("", projectID, databasePath); err != nil {
		return "", err
	}
	root := dataRoot
	if remote || root == "" {
		root = filepath.Join(cacheDir, "projects")
	}
	if root == "" {
		return "", fmt.Errorf("filedb: data root or cache dir is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("filedb: resolve storage root: %w", err)
	}
	filePath := filepath.Join(absRoot, filepath.FromSlash(databasePath))
	rel, err := filepath.Rel(absRoot, filePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("filedb: database path escapes storage root")
	}
	parent := filepath.Dir(filePath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("filedb: create project directory: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("filedb: resolve storage root: %w", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("filedb: resolve project directory: %w", err)
	}
	rel, err = filepath.Rel(resolvedRoot, resolvedParent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("filedb: project directory escapes storage root")
	}
	if info, err := os.Lstat(filePath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("filedb: database file must not be a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return filePath, nil
}

func sidecarFiles(filePath, engine string) []string {
	if engine == godatabase.EngineSQLite {
		return []string{filePath, filePath + "-wal", filePath + "-shm", filePath + "-journal"}
	}
	return []string{filePath, filePath + ".wal"}
}
