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

// Evict 淘汰缓存直到释放出至少 targetBytes，或无可淘汰项。
// 调用方负责确保淘汰期间不会并发打开大量新库。
func (m *Manager) Evict(ctx context.Context, targetBytes int64) (EvictionResult, error) {
	res := EvictionResult{TargetBytes: targetBytes}
	if targetBytes <= 0 {
		return res, nil
	}

	entries, err := os.ReadDir(m.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return res, nil
		}
		return res, fmt.Errorf("cache: read root for eviction: %w", err)
	}

	// 收集可淘汰候选：UUID 命名目录且 registry 非活跃。
	type candidate struct {
		id       string
		path     string
		size     int64
		lastUsed time.Time
	}
	cands := make([]candidate, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if _, perr := m.Path(id); perr != nil {
			// 非 UUID 命名，跳过。
			continue
		}
		if m.registry != nil && m.registry.IsActive(id) {
			res.SkippedActive++
			continue
		}
		p := filepath.Join(m.root, id)
		info, ierr := e.Info()
		if ierr != nil {
			res.SkippedMissing++
			continue
		}
		size, serr := dirSize(p)
		if serr != nil {
			res.SkippedMissing++
			continue
		}
		cands = append(cands, candidate{
			id:       id,
			path:     p,
			size:     size,
			lastUsed: info.ModTime(),
		})
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
		return nil
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
	return nil
}
