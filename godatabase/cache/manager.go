// 文件说明：manager.go — godatabase/cache 中 ActiveChecker、Closer 等符号的实现
//
// 用法
//
//	type ActiveChecker interface{…}  // ActiveChecker 抽象 registry 查询能力：判断某库是否可安全淘汰
//	type Closer interface{…}  // Closer 抽象 registry.CloseDatabase：淘汰前先关闭 registry 句柄
//	type Manager struct{…}  // Manager 管理本地缓存目录
//	type Usage struct{…}  // Usage 描述当前缓存用量
//	type Options struct{…}  // Options 配置 Manager
//	NewManager(opts Options) (*Manager, error)  // NewManager 构造 Manager
//	同包调用：lastUsedChecker → evict.go；removeAll() → coverage_test.go, evict.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/cache/manager.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] coverage_test.go, evict.go, evict_b13_test.go, manager_test.go, more_test.go
//	[关键引用] lastUsedChecker ← evict.go；Manager ← evict.go；Options ← coverage_test.go, evict_b13_test.go
//
// 主要符号
//
//	type ActiveChecker interface{…}
//	type lastUsedChecker interface{…}
//	type Closer interface{…}
//	type Manager struct{…}
//	type Usage struct{…}
//	type Options struct{…}
//	var ErrCacheCapacityExceeded
//	NewManager(opts Options) (*Manager, error)
//	... 另有 6 个顶层符号

package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ActiveChecker 抽象 registry 查询能力：判断某库是否可安全淘汰。
// registry.Registry 自动满足此接口（IsActive 方法）。
// P5：实现方可选提供 LastUsedOf（LRU 依据）；未提供时退回目录 mtime。
type ActiveChecker interface {
	// IsActive 返回 true 表示该库仍有活跃引用，不可淘汰。
	IsActive(databaseID string) bool
}

// lastUsedChecker 是 ActiveChecker 的可选扩展（P5）。
type lastUsedChecker interface {
	LastUsedOf(databaseID string) (time.Time, bool)
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
	logger       godatabase.Logger

	// observer 用于上报 CacheBytes；可为 nil。
	observer godatabase.Observer
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
	// DataRoot 是持久数据目录（storage.data_dir）。设置时校验缓存根
	// 不得等于或落在数据目录内：缓存淘汰随时删除文件，绝不能波及数据。
	DataRoot string
	// Registry 用于检查库是否活跃；可为 nil（此时 Evict 视为不可淘汰）。
	Registry ActiveChecker
	// Closer 用于淘汰前关闭句柄；若为 nil 则淘汰前不关闭 registry。
	Closer Closer
	Logger godatabase.Logger
	// Observer 接收缓存用量与淘汰指标；可为 nil。
	Observer godatabase.Observer
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
	if opts.DataRoot != "" {
		dataAbs, err := filepath.Abs(opts.DataRoot)
		if err != nil {
			return nil, fmt.Errorf("cache: resolve data root: %w", err)
		}
		if abs == dataAbs {
			return nil, fmt.Errorf("cache: root %s must not equal data dir %s", abs, dataAbs)
		}
		if rel, err := filepath.Rel(dataAbs, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("cache: root %s must not live inside data dir %s", abs, dataAbs)
		}
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
		observer:     opts.Observer,
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
// projects/ 布局按内部项目子目录计数（B13：目录数口径与淘汰单位一致，
// 一级目录计数会把整个 projects/ 算 1 个，虚低）。
func (m *Manager) Usage(ctx context.Context) (Usage, error) {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Usage{}, nil
		}
		return Usage{}, fmt.Errorf("cache: read root: %w", err)
	}
	var u Usage
	countDir := func(name string) {
		u.DatabaseDirs++
		size, err := dirSize(filepath.Join(m.root, name))
		if err != nil {
			// 单个目录统计失败不应中断整体；记录后继续。
			if m.logger != nil {
				m.logger.Warn("cache: skip dir size", zap.String("dir", name), zap.String("err", err.Error()))
			}
			return
		}
		u.TotalBytes += size
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == "projects" {
			subs, serr := os.ReadDir(filepath.Join(m.root, e.Name()))
			if serr != nil {
				if m.logger != nil {
					m.logger.Warn("cache: skip dir size", zap.String("dir", e.Name()), zap.String("err", serr.Error()))
				}
				continue
			}
			for _, sub := range subs {
				if sub.IsDir() {
					u.DatabaseDirs++
				}
			}
			size, serr := dirSize(filepath.Join(m.root, e.Name()))
			if serr != nil {
				if m.logger != nil {
					m.logger.Warn("cache: skip dir size", zap.String("dir", e.Name()), zap.String("err", serr.Error()))
				}
				continue
			}
			u.TotalBytes += size
			continue
		}
		countDir(e.Name())
	}
	if m.observer != nil {
		m.observer.CacheBytes(u.TotalBytes)
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
