// 文件说明：evict.go — 缓存淘汰策略（plan7.md）
//
// 用法
//
//	(m *Manager) Evict(ctx context.Context, targetBytes int64) (EvictionResult, error)  // Evict 淘汰缓存直到释放出至少 targetBytes，或无可淘汰项
//	(m *Manager) EnsureCapacity(ctx context.Context, needBytes int64) error  // EnsureCapacity 确保缓存中有至少 needBytes 可用空间
//	同包调用：Evict() → manager.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/cache/evict.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] manager.go
//	[关键引用] Evict() ← manager.go
//
// 主要符号
//
//	type EvictionResult struct {…}
//	(m *Manager) Evict(ctx context.Context, targetBytes int64) (EvictionResult, error)
//	(m *Manager) EnsureCapacity(ctx context.Context, needBytes int64) error

package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// EvictionResult 描述一次淘汰操作的结果。
type EvictionResult struct {
	EvictedCount   int
	BytesFreed     int64
	TargetBytes    int64
	Achieved       bool
	SkippedActive  int
	SkippedMissing int
}

// 缓存目录布局（P5）：识别 database 维度与项目维度两种目录形态。
const (
	layoutLegacyUUID  = 1 // {uuid}/
	layoutProjectDir  = 2 // projects/{projectID}/（内部子目录为各库）
)

// dbDir 是一个待淘汰候选目录。
type dbDir struct {
	id     string // UUID；无法识别时空串
	path   string
	layout int
}

// listDBDirs 枚举 root 下的一级库目录，识别 UUID 与 projects/ 布局。
// projects/ 本身作为 layoutProjectDir 返回（调用方负责深入一层）。
func listDBDirs(root string) ([]dbDir, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []dbDir
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		p := filepath.Join(root, name)
		if name == "projects" {
			out = append(out, dbDir{path: p, layout: layoutProjectDir})
			continue
		}
		if _, perr := uuid.Parse(name); perr == nil {
			out = append(out, dbDir{id: name, path: p, layout: layoutLegacyUUID})
		}
	}
	return out, nil
}

// dirLatestMtime 返回目录内（递归）最新文件 mtime；空目录返回目录自身 mtime。
// P5：作为无 registry 记录时的 LRU 兜底依据。
func dirLatestMtime(dirPath string) time.Time {
	var latest time.Time
	_ = filepath.Walk(dirPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // 单文件失败跳过，取其余最大值
		}
		if !info.IsDir() && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	if latest.IsZero() {
		if st, err := os.Stat(dirPath); err == nil {
			latest = st.ModTime()
		}
	}
	return latest
}

// Evict 淘汰缓存直到释放出至少 targetBytes，或无可淘汰项。
// 调用方负责确保淘汰期间不会并发打开大量新库。
func (m *Manager) Evict(ctx context.Context, targetBytes int64) (EvictionResult, error) {
	res := EvictionResult{TargetBytes: targetBytes}
	if targetBytes <= 0 {
		return res, nil
	}

	// P5：候选按「库目录」枚举，同时识别两种布局：
	//   - {uuid}/            （database 维度缓存）
	//   - projects/{id}/     （项目维度缓存）
	type candidate struct {
		id       string // UUID（closer/registry 识别用）；非 UUID 目录为空
		path     string
		size     int64
		lastUsed time.Time
	}
	var cands []candidate
	collect := func(dirPath, id string, lastUsed time.Time) {
		size, serr := dirSize(dirPath)
		if serr != nil {
			res.SkippedMissing++
			return
		}
		cands = append(cands, candidate{id: id, path: dirPath, size: size, lastUsed: lastUsed})
	}
	// registry 的真实 LastUsed 优先（P5：目录 mtime 不反映读活跃度）。
	lastUsedOf := func(id string, dirPath string) time.Time {
		if id != "" && m.registry != nil {
			if lc, ok := m.registry.(lastUsedChecker); ok {
				if t, ok := lc.LastUsedOf(id); ok {
					return t
				}
			}
		}
		return dirLatestMtime(dirPath)
	}

	buckets, err := listDBDirs(m.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return res, nil
		}
		return res, fmt.Errorf("cache: read root for eviction: %w", err)
	}
	for _, b := range buckets {
		switch b.layout {
		case layoutLegacyUUID:
			// UUID 布局：registry 活跃的跳过。
			if m.registry != nil && m.registry.IsActive(b.id) {
				res.SkippedActive++
				continue
			}
			collect(b.path, b.id, lastUsedOf(b.id, b.path))
		case layoutProjectDir:
			// 项目布局：目录内每个库子目录独立参与淘汰。
			subDirs, serr := listDBDirs(b.path)
			if serr != nil {
				res.SkippedMissing++
				continue
			}
			for _, sub := range subDirs {
				id := sub.id
				if id == "" {
					id = b.id // 目录名即 projectID 时退化为库 ID 不明；仍允许淘汰
				}
				if id != "" && m.registry != nil && m.registry.IsActive(id) {
					res.SkippedActive++
					continue
				}
				collect(sub.path, id, lastUsedOf(id, sub.path))
			}
			// 项目目录本身的空壳也回收（无活跃子库时）。
			if len(subDirs) == 0 {
				collect(b.path, "", dirLatestMtime(b.path))
			}
		}
	}

	// LRU：按 lastUsed 升序，先淘汰最久未使用。
	sort.Slice(cands, func(i, j int) bool {
		return cands[i].lastUsed.Before(cands[j].lastUsed)
	})

	for _, c := range cands {
		if res.BytesFreed >= targetBytes {
			break
		}
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		// 先关闭 registry 句柄（若提供 closer），再删目录。
		// §3.6：CloseDatabase 失败（含关闭前 Flush catalog 失败）必须跳过
		// 淘汰并保留目录——否则可能删除唯一未同步的 catalog 副本。
		if m.closer != nil {
			if cerr := m.closer.CloseDatabase(ctx, c.id); cerr != nil {
				res.SkippedActive++
				if m.logger != nil {
					m.logger.Warn("cache: close registry before evict failed; keep local dir",
						zap.String("database_id", c.id),
						zap.String("err", cerr.Error()))
				}
				continue
			}
		}
		if rerr := removeAll(c.path); rerr != nil {
			if m.logger != nil {
				m.logger.Warn("cache: remove dir failed",
					zap.String("database_id", c.id),
					zap.String("err", rerr.Error()))
			}
			continue
		}
		res.EvictedCount++
		res.BytesFreed += c.size
		if m.observer != nil {
			m.observer.CacheEvicted()
		}
		if m.logger != nil {
			m.logger.Info("cache: evicted",
				zap.String("database_id", c.id),
				zap.Int64("bytes", c.size))
		}
	}

	res.Achieved = res.BytesFreed >= targetBytes
	if !res.Achieved {
		return res, ErrCacheCapacityExceeded
	}
	return res, nil
}

// EnsureCapacity 确保缓存中有至少 needBytes 可用空间。
// 先统计当前用量，若 (maxBytes - usage) >= needBytes 直接返回；
// 否则尝试淘汰 (needBytes - free) 字节。
// C3：库目录数超过 maxDatabases 时按 LRU 多余淘汰（此前只赋值不生效）。
func (m *Manager) EnsureCapacity(ctx context.Context, needBytes int64) error {
	if needBytes <= 0 {
		return nil
	}
	usage, err := m.Usage(ctx)
	if err != nil {
		return err
	}
	free := m.maxBytes - usage.TotalBytes
	if free >= needBytes {
		return m.evictOverCount(ctx, usage)
	}
	deficit := needBytes - free
	res, err := m.Evict(ctx, deficit)
	if err != nil {
		if errors.Is(err, ErrCacheCapacityExceeded) {
			return fmt.Errorf("cache: cannot free %d bytes (evicted %d bytes from %d dirs): %w",
				deficit, res.BytesFreed, res.EvictedCount, err)
		}
		return err
	}
	return m.evictOverCount(ctx, usage)
}

// evictOverCount 按 LRU 淘汰超出 maxDatabases 数量上限的库（尽力而为）。
// usage 传最近一次统计避免重复 walk；maxDatabases<=0 表示不限制。
func (m *Manager) evictOverCount(ctx context.Context, usage Usage) error {
	if m.maxDatabases <= 0 || usage.DatabaseDirs <= m.maxDatabases {
		return nil
	}
	target := int64((usage.DatabaseDirs - m.maxDatabases) * 1024) // 目标字节仅驱动淘汰数量
	_, err := m.Evict(ctx, target)
	if err != nil && !errors.Is(err, ErrCacheCapacityExceeded) {
		return err
	}
	return nil
}
