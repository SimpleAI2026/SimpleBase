package ducklake

import (
	"os"
	"path/filepath"
)

// Layout 描述单个 logical database 在本地缓存中的目录布局。
// 与计划中的 S3 前缀镜像：catalog/catalog.sqlite + data/。
type Layout struct {
	Root        string
	CatalogFile string
	DataDir     string
	TempDir     string
}

func layoutFor(cacheDir, databaseID string) Layout {
	root := filepath.Join(cacheDir, databaseID)
	return Layout{
		Root:        root,
		CatalogFile: filepath.Join(root, "catalog", "catalog.sqlite"),
		DataDir:     filepath.Join(root, "data"),
		TempDir:     filepath.Join(root, "tmp"),
	}
}

func (l Layout) ensure() error {
	for _, dir := range []string{
		filepath.Dir(l.CatalogFile),
		l.DataDir,
		l.TempDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (l Layout) dataPathArg() string {
	p := filepath.ToSlash(l.DataDir)
	if p != "" && p[len(p)-1] != '/' {
		p += "/"
	}
	return p
}
