package objectstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// BlobStore 读写二进制对象（DuckLake catalog.sqlite 等）。
// 与 Client（JSON 平台对象）分离，避免把大文件塞进 PutJSON。
type BlobStore interface {
	Head(ctx context.Context, key string) (ObjectInfo, error)
	PutBytes(ctx context.Context, key string, data []byte, contentType string) error
	GetBytes(ctx context.Context, key string) ([]byte, ObjectInfo, error)
	DownloadFile(ctx context.Context, key, destPath string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}

// PutBytes 上传二进制对象。
func (c *s3Client) PutBytes(ctx context.Context, key string, data []byte, contentType string) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	}
	if len(data) >= 0 {
		in.ContentLength = aws.Int64(int64(len(data)))
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if _, err := c.api.PutObject(ctx, in); err != nil {
		c.recordOp("put_bytes", "error")
		return c.sanitizeErr(err)
	}
	c.recordOp("put_bytes", "ok")
	return nil
}

// GetBytes 下载整个对象到内存。
func (c *s3Client) GetBytes(ctx context.Context, key string) ([]byte, ObjectInfo, error) {
	out, err := c.api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		c.recordOp("get_bytes", "error")
		return nil, ObjectInfo{}, mapNotFoundErr(c.sanitizeErr(err))
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		c.recordOp("get_bytes", "error")
		return nil, ObjectInfo{}, c.sanitizeErr(err)
	}
	info := ObjectInfo{
		Key:          key,
		Size:         int64(len(data)),
		LastModified: derefTime(out.LastModified),
		ETag:         derefStr(out.ETag),
	}
	c.recordOp("get_bytes", "ok")
	return data, info, nil
}

// DownloadFile 下载对象到本地文件（先写 .tmp 再 rename）。
func (c *s3Client) DownloadFile(ctx context.Context, key, destPath string) (ObjectInfo, error) {
	data, info, err := c.GetBytes(ctx, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return ObjectInfo{}, fmt.Errorf("objectstore: mkdir for download: %w", err)
	}
	tmp := destPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return ObjectInfo{}, fmt.Errorf("objectstore: write download tmp: %w", err)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		_ = os.Remove(tmp)
		return ObjectInfo{}, fmt.Errorf("objectstore: rename download: %w", err)
	}
	return info, nil
}

// Delete 删除单个对象。
func (c *s3Client) Delete(ctx context.Context, key string) error {
	_, err := c.api.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(c.bucket),
		Delete: &types.Delete{
			Objects: []types.ObjectIdentifier{{Key: aws.String(key)}},
			Quiet:   aws.Bool(true),
		},
	})
	if err != nil {
		c.recordOp("delete", "error")
		return c.sanitizeErr(err)
	}
	c.recordOp("delete", "ok")
	return nil
}

/* ---------- 内存 BlobStore（单测 / DevMode 可选） ---------- */

type memoryBlobStore struct {
	mu    sync.RWMutex
	store map[string][]byte
	meta  map[string]time.Time
}

// NewMemoryBlobStore 构造进程内 BlobStore。
func NewMemoryBlobStore() BlobStore {
	return &memoryBlobStore{
		store: map[string][]byte{},
		meta:  map[string]time.Time{},
	}
}

func (m *memoryBlobStore) Head(_ context.Context, key string) (ObjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.store[key]
	if !ok {
		return ObjectInfo{}, ErrNotFound
	}
	return ObjectInfo{Key: key, Size: int64(len(b)), LastModified: m.meta[key]}, nil
}

func (m *memoryBlobStore) PutBytes(_ context.Context, key string, data []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]byte(nil), data...)
	m.store[key] = cp
	m.meta[key] = time.Now().UTC()
	return nil
}

func (m *memoryBlobStore) GetBytes(_ context.Context, key string) ([]byte, ObjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.store[key]
	if !ok {
		return nil, ObjectInfo{}, ErrNotFound
	}
	cp := append([]byte(nil), b...)
	return cp, ObjectInfo{Key: key, Size: int64(len(cp)), LastModified: m.meta[key]}, nil
}

func (m *memoryBlobStore) DownloadFile(ctx context.Context, key, destPath string) (ObjectInfo, error) {
	data, info, err := m.GetBytes(ctx, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return ObjectInfo{}, err
	}
	tmp := destPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return ObjectInfo{}, err
	}
	if err := os.Rename(tmp, destPath); err != nil {
		_ = os.Remove(tmp)
		return ObjectInfo{}, err
	}
	return info, nil
}

func (m *memoryBlobStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	delete(m.meta, key)
	return nil
}
