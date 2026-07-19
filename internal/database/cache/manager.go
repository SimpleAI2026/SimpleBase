// Package cache 实现本地数据库缓存目录的管理与淘汰（见 plan7.md）。
//
// 设计原则：
//   - 本地缓存视作可丢失：S3 是持久层，缓存仅为加速访问。删除缓存目录不影响数据安全。
//   - 所有缓存操作仅作用于本地路径，绝不调用 S3 删除。
//   - 超过容量时优先淘汰已关闭、非活跃的库；无法腾出空间则拒绝 open，禁止破坏活跃库。
package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// ActiveChecker 抽象 registry 查询能力：判断某库是否可安全淘汰。
// registry.Registry 自动满足此接口（IsActive 方法）。
type ActiveChecker interface {
	// IsActive 返回 true 表示该库仍有活跃引用，不可淘汰。
	IsActive(databaseID string) bool
}

// Closer 抽象 registry.CloseDatabase：淘汰前先关闭 registry 句柄。
type Closer interface {
	// CloseDatabase 关闭 registry 中的句柄；若库不存在或已关闭返回 nil。
	CloseDatabase(ctx context.Context, databaseID string) error
}

// Manager 管理本地缓存目录。
type Manager struct {
	root         string
	maxBytes     int64
	maxDatabases int
	registry     ActiveChecker
	closer       Closer
	logger       observability.Logger

	// metrics 用于更新 CacheBytes gauge；可为 nil。
	metrics CacheMetrics
}

// CacheMetrics 是 observability.Metrics 的最小子集接口，避免循环依赖。
type CacheMetrics interface {
	ObserveCacheBytes(bytes int64)
	IncCacheEvictions()
}

// Usage 描述当前缓存用量。
type Usage struct {
	TotalBytes   int64
	DatabaseDirs int
}

// Options 配置 Manager。
type Options struct {
	Root         string
	MaxBytes     int64
	MaxDatabases int
	// Registry 用于检查库是否活跃；可为 nil（此时 Evict 视为不可淘汰）。
	Registry ActiveChecker
	// Closer 用于淘汰前关闭句柄；若为 nil 则淘汰前不关闭 registry。
	Closer  Closer
	Logger  observability.Logger
	Metrics CacheMetrics
}

// ErrCacheCapacityExceeded 表示缓存容量不足且无法淘汰。
var ErrCacheCapacityExceeded = errors.New("cache: capacity exceeded")

// NewManager 构造 Manager。root 必须存在且可写（由 Preflight 校验）。
func NewManager(opts Options) (*Manager, error) {
	if opts.Root == "" {
		return nil, errors.New("cache: root is required")
	}
	abs, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, fmt.Errorf("cache: resolve root: %w", err)
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 1 << 30 // 默认 1GiB
	}
	if opts.MaxDatabases <= 0 {
		opts.MaxDatabases = 256
	}
	return &Manager{
		root:         abs,
		maxBytes:     opts.MaxBytes,
		maxDatabases: opts.MaxDatabases,
		registry:     opts.Registry,
		closer:       opts.Closer,
		logger:       opts.Logger,
		metrics:      opts.Metrics,
	}, nil
}

// Root 返回缓存根目录绝对路径。
func (m *Manager) Root() string { return m.root }

// Path 返回指定 databaseID 的缓存目录路径。
// databaseID 必须是合法 UUID；结果必须仍在 root 下（防穿越）。
func (m *Manager) Path(databaseID string) (string, error) {
	if _, err := uuid.Parse(databaseID); err != nil {
		return "", fmt.Errorf("cache: invalid database id: %w", err)
	}
	p := filepath.Join(m.root, databaseID)
	// 防目录穿越：确保解析后的路径仍在 root 下。
	rel, err := filepath.Rel(m.root, p)
	if err != nil || rel == "" || rel == ".." || len(rel) >= 2 && rel[:2] == ".." {
		return "", fmt.Errorf("cache: path escapes root")
	}
	return p, nil
}

// Usage 统计当前缓存用量。遍历 root 下的一级子目录并汇总大小。
func (m *Manager) Usage(ctx context.Context) (Usage, error) {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Usage{}, nil
		}
		return Usage{}, fmt.Errorf("cache: read root: %w", err)
	}
	var u Usage
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		u.DatabaseDirs++
		size, err := dirSize(filepath.Join(m.root, e.Name()))
		if err != nil {
			// 单个目录统计失败不应中断整体；记录后继续。
			if m.logger != nil {
				m.logger.Warn("cache: skip dir size", zap.String("dir", e.Name()), zap.String("err", err.Error()))
			}
			continue
		}
		u.TotalBytes += size
	}
	if m.metrics != nil {
		m.metrics.ObserveCacheBytes(u.TotalBytes)
	}
	return u, nil
}

// dirSize 递归计算目录总字节数。
func dirSize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// Remove 删除指定 databaseID 的缓存目录。仅作用于本地路径。
// 若 registry 仍活跃则返回错误，避免破坏运行中的库。
func (m *Manager) Remove(databaseID string) error {
	p, err := m.Path(databaseID)
	if err != nil {
		return err
	}
	if m.registry != nil && m.registry.IsActive(databaseID) {
		return fmt.Errorf("cache: cannot remove active database %s", databaseID)
	}
	return removeAll(p)
}

// removeAll 删除路径，容忍 ENOENT。
func removeAll(path string) error {
	err := os.RemoveAll(path)
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	// macOS 上某些 tmpfs 可能返回 ENOENT 但不包裹 ErrNotExist。
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == syscall.ENOENT {
		return nil
	}
	return err
}
