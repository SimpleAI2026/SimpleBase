// localfilestore.go 提供 FileStore 的本地磁盘实现：文件落在指定根目录下，
// 进程重启后数据保留。仅用于 DevMode，生产环境使用真实 S3 持久层。
package objectstore

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// localFileStore 把对象内容持久化到 root 目录下的相对路径文件。
type localFileStore struct {
	root string
}

// NewLocalFileStore 创建一个以 root 为存储根的 FileStore；root 不存在时自动创建。
func NewLocalFileStore(root string) (FileStore, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("objectstore: resolve root %s: %w", root, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("objectstore: create root %s: %w", abs, err)
	}
	return &localFileStore{root: abs}, nil
}

// physical 把合法 key 映射为 root 内的物理路径，并二次校验防止路径穿越。
func (s *localFileStore) physical(key string) (string, error) {
	if err := ValidateFileKey(key); err != nil {
		return "", err
	}
	p := filepath.Join(s.root, filepath.FromSlash(key))
	if p != s.root && !strings.HasPrefix(p, s.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: key escapes root", ErrInvalidKey)
	}
	return p, nil
}

func (s *localFileStore) List(ctx context.Context, prefix string, maxKeys int) ([]FileObject, error) {
	if maxKeys <= 0 {
		maxKeys = 100
	}
	out := make([]FileObject, 0)
	err := filepath.Walk(s.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		out = append(out, FileObject{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime().UTC(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	if len(out) > maxKeys {
		out = out[:maxKeys]
	}
	return out, nil
}

func (s *localFileStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (FileObject, error) {
	p, err := s.physical(key)
	if err != nil {
		return FileObject{}, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return FileObject{}, err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return FileObject{}, err
	}
	// 先写临时文件再 rename，避免进程中断产生半截文件。
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return FileObject{}, err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return FileObject{}, err
	}
	info, err := os.Stat(p)
	if err != nil {
		return FileObject{}, err
	}
	return FileObject{
		Key:          key,
		Size:         info.Size(),
		LastModified: info.ModTime().UTC(),
	}, nil
}

func (s *localFileStore) Delete(ctx context.Context, key string) error {
	p, err := s.physical(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	// 尝试清理空目录（最多回到 root），忽略非空目录错误。
	dir := filepath.Dir(p)
	for dir != s.root && strings.HasPrefix(dir, s.root) {
		if err := os.Remove(dir); err != nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

func (s *localFileStore) PresignGet(ctx context.Context, key string, expires time.Duration) (string, error) {
	p, err := s.physical(key)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	ct := mime.TypeByExtension(filepath.Ext(p))
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

var _ FileStore = (*localFileStore)(nil)
