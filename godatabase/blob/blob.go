// 文件说明：blob.go — 定义数据库运行时依赖的对象存储抽象：Store 接口、对象元数据、
//
// 用法
//
//	NewMemory() *Memory
//	(m *Memory) PutBytes(_ context.Context, key string, data []byte, _ string) error  // PutBytes 无条件写入
//	(m *Memory) Head(_ context.Context, key string) (ObjectInfo, error)  // Head 返回对象元数据，不存在返回 ErrNotFound
//	同包调用：PutIfAbsent() → keys.go
//
// 调用架构
//
//	[上游] godatabase, godatabase/ducklake, godatabase/filedb 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/blob/blob.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/internal/atomicfile, godatabase/internal/hashx, godatabase/internal/limit
//	[同包] keys.go
//	[关键引用] PutIfAbsent() ← keys.go
//
// 主要符号
//
//	var ErrNotFound
//	var ErrPreconditionFailed
//	type ObjectInfo struct {…}
//	type Store interface {…}
//	type ListResult struct {…}
//	type Memory struct {…}
//	... 另有 10 个顶层符号

package blob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase/internal/atomicfile"
	"github.com/linkxzhou/SimpleBase/godatabase/internal/hashx"
	"github.com/linkxzhou/SimpleBase/godatabase/internal/limit"
)

// ErrNotFound 表示请求的对象不存在。调用方用 errors.Is 判别。
var ErrNotFound = errors.New("objectstore: object not found")

// ErrPreconditionFailed 表示 create-if-absent 写入冲突（对象已存在）。
// COS 返回 409 ObjectAlreadyExists、S3/R2 返回 412，统一映射到本错误。
var ErrPreconditionFailed = errors.New("objectstore: object already exists")

// ObjectInfo 描述对象元数据。禁止包含 body 或预签名 URL。
type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	LastModified time.Time
}

// Store 读写二进制对象（catalog 快照、manifest、租约等）。
//
// PutIfAbsent 是唯一支持条件写的原语（multi-instance-consistency-plan §4.5）：
// 仅当 key 不存在时写入，已存在返回 ErrPreconditionFailed。
// 故意不提供 IfMatch 能力——目标端点 COS 会静默忽略它，制造假保护。
type Store interface {
	Head(ctx context.Context, key string) (ObjectInfo, error)
	PutBytes(ctx context.Context, key string, data []byte, contentType string) error
	PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (ObjectInfo, error)
	GetBytes(ctx context.Context, key string) ([]byte, ObjectInfo, error)
	DownloadFile(ctx context.Context, key, destPath string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	// List 分页列举前缀下对象（ducklake-duckdb-catalog-plan §6）。
	// cursor 为空从头开始；返回本页 keys、下一页 cursor（空表示结束）。
	List(ctx context.Context, prefix, cursor string, limit int) (keys []string, next string, err error)
	// DeleteMany 批量删除（S3 上限 1000/批）；容忍 key 不存在。
	DeleteMany(ctx context.Context, keys []string) error
}

// ListResult 是 List 的单页结果。
type ListResult struct {
	Keys []string
	Next string
}

/* ---------- 内存实现（单测与单机使用） ---------- */

// Memory 是进程内 Store。零值不可用，请用 NewMemory。
type Memory struct {
	mu    sync.RWMutex
	store map[string][]byte
	meta  map[string]time.Time
}

// NewMemory 构造进程内 Store。
func NewMemory() *Memory {
	return &Memory{
		store: map[string][]byte{},
		meta:  map[string]time.Time{},
	}
}

// PutBytes 无条件写入。
func (m *Memory) PutBytes(_ context.Context, key string, data []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = append([]byte(nil), data...)
	m.meta[key] = time.Now().UTC()
	return nil
}

// Head 返回对象元数据，不存在返回 ErrNotFound。
func (m *Memory) Head(_ context.Context, key string) (ObjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.store[key]
	if !ok {
		return ObjectInfo{}, ErrNotFound
	}
	return ObjectInfo{Key: key, Size: int64(len(b)), LastModified: m.meta[key]}, nil
}

// PutIfAbsent 仅当 key 不存在时写入；ETag 用内容 SHA256 前 16 hex（便于测试断言）。
func (m *Memory) PutIfAbsent(_ context.Context, key string, data []byte, _ string) (ObjectInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.store[key]; ok {
		return ObjectInfo{}, ErrPreconditionFailed
	}
	cp := append([]byte(nil), data...)
	m.store[key] = cp
	now := time.Now().UTC()
	m.meta[key] = now
	return ObjectInfo{Key: key, Size: int64(len(cp)), ETag: hashx.Short(cp, 8), LastModified: now}, nil
}

// GetBytes 读取对象内容，不存在返回 ErrNotFound。
func (m *Memory) GetBytes(_ context.Context, key string) ([]byte, ObjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.store[key]
	if !ok {
		return nil, ObjectInfo{}, ErrNotFound
	}
	cp := append([]byte(nil), b...)
	return cp, ObjectInfo{Key: key, Size: int64(len(cp)), LastModified: m.meta[key]}, nil
}

// DownloadFile 把对象原子写入 destPath。
func (m *Memory) DownloadFile(ctx context.Context, key, destPath string) (ObjectInfo, error) {
	data, info, err := m.GetBytes(ctx, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return ObjectInfo{}, err
	}
	if err := atomicfile.Write(destPath, data, 0o644); err != nil {
		return ObjectInfo{}, err
	}
	return info, nil
}

// Delete 删除对象，不存在不报错。
func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	delete(m.meta, key)
	return nil
}

// List 分页列举前缀下对象。
func (m *Memory) List(_ context.Context, prefix, cursor string, n int) ([]string, string, error) {
	n = limit.Clamp(n, 1000, 0)
	m.mu.RLock()
	keys := make([]string, 0, len(m.store))
	for k := range m.store {
		if strings.HasPrefix(k, prefix) && k > cursor {
			keys = append(keys, k)
		}
	}
	m.mu.RUnlock()
	sort.Strings(keys)
	if len(keys) > n {
		return keys[:n], keys[n-1], nil
	}
	return keys, "", nil
}

// DeleteMany 批量删除，容忍 key 不存在。
func (m *Memory) DeleteMany(_ context.Context, keys []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.store, k)
		delete(m.meta, k)
	}
	return nil
}

var _ Store = (*Memory)(nil)
