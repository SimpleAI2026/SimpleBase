// 文件说明：evict.go — godatabase/cache 中 EvictionResult 等符号的实现
//
// 用法
//
//	type EvictionResult struct{…}  // EvictionResult 描述一次淘汰操作的结果
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/cache/evict.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] coverage_test.go, evict_b13_test.go, manager.go, manager_test.go, more_test.go
//
// 主要符号
//
//	type EvictionResult struct{…}
//	const layoutLegacyUUID / layoutProjectDir
//	type dbDir struct{…}
//	listDBDirs(root string) ([]dbDir, error)
//	dirStats(dirPath string) (int64, time.Time, error)
//	(m *Manager) Evict(ctx context.Context, targetBytes int64) (EvictionResult, error)
//	(m *Manager) evict(ctx context.Context, targetBytes int64, maxCount int) (EvictionResult, error)
//	(m *Manager) EnsureCapacity(ctx context.Context, needBytes int64) error
//	... 另有 1 个顶层符号

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
	layoutLegacyUUID = 1 // {uuid}/
	layoutProjectDir = 2 // projects/{projectID}/（内部子目录为各库）
)

// dbDir 是一个待淘汰候选目录。
type dbDir struct {
	id     string // registry key（UUID 或 projectID）；无法识别时空串
	path   string
	layout int
}

// listDBDirs 枚举 root 下的一级库目录，识别 UUID 与 projects/ 布局。
// projects/ 本身作为 layoutProjectDir 返回（调用方负责枚举项目子目录）。
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

// dirStats 一次遍历同时返回目录总字节数与最新文件 mtime（R2/O2：消除
// 同一目录的二次扫描）。空目录的 mtime 退回目录自身。walk 错误中止并返回
// （与 dirSize 的失败语义一致，由调用方记 SkippedMissing）；文件级错误
// 不中断 mtime 统计（与 dirLatestMtime 的容错一致）。
func dirStats(dirPath string) (int64, time.Time, error) {
	var total int64
	var latest time.Time
	err := filepath.Walk(dirPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
		}
		return nil
	})
	if err != nil {
		return 0, latest, err
	}
	if latest.IsZero() {
		if st, serr := os.Stat(dirPath); serr == nil {
				latest = st.ModTime()
		}
	}
	return total, latest, nil
}

// Evict 淘汰缓存直到释放出至少 targetBytes，或无可淘汰项。
// 调用方负责确保淘汰期间不会并发打开大量新库。
func (m *Manager) Evict(ctx context.Context, targetBytes int64) (EvictionResult, error) {
	if targetBytes <= 0 {
		return EvictionResult{TargetBytes: targetBytes}, nil
	}
	return m.evict(ctx, targetBytes, 0)
}

// evict 是淘汰的统一实现：字节目标与数量上限共同驱动（任一满足即停）。
// targetBytes <= 0 表示不按字节目标（仅数量驱动）；maxCount <= 0 表示不限数量。
func (m *Manager) evict(ctx context.Context, targetBytes int64, maxCount int) (EvictionResult, error) {
	res := EvictionResult{TargetBytes: targetBytes}

	// P5：候选按「库目录」枚举，同时识别两种布局：
	//   - {uuid}/            （database 维度缓存）
	//   - projects/{projectID}/（项目维度缓存；目录名是 8 位短项目 ID）
	type candidate struct {
		id       string // registry key（UUID 或 projectID）；未知时为空
		path     string
		size     int64
		lastUsed time.Time
	}
	var cands []candidate
	// 单次 walk 同时收集 size 与最新 mtime（R2：消除同目录二次扫描）。
	collect := func(dirPath, id string) {
		size, mtime, serr := dirStats(dirPath)
		if serr != nil {
			res.SkippedMissing++
			return
		}
		// registry 的真实 LastUsed 优先（P5：目录 mtime 不反映读活跃度）。
		lastUsed := mtime
		if id != "" && m.registry != nil {
			if lc, ok := m.registry.(lastUsedChecker); ok {
				if t, ok := lc.LastUsedOf(id); ok {
					lastUsed = t
				}
			}
		}
		cands = append(cands, candidate{id: id, path: dirPath, size: size, lastUsed: lastUsed})
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
			collect(b.path, b.id)
		case layoutProjectDir:
			// 项目布局（B13 修复）：真实布局是 projects/{projectID}/database.ext，
			// 目录名是 8 位短项目 ID 而非 UUID；registry 的 findEntryLocked
			// 支持按 ProjectID 匹配，IsActive/CloseDatabase 直接用目录名即可。
			// 每个项目目录独立参与淘汰；projects/ 根目录绝不作为候选，
			// 仅当其下无任何项目目录时才回收空壳。
			subs, serr := os.ReadDir(b.path)
			if serr != nil {
				res.SkippedMissing++
				continue
			}
			projectDirs := 0
			for _, sub := range subs {
				if !sub.IsDir() {
					continue
				}
				projectDirs++
				id := sub.Name()
				if m.registry != nil && m.registry.IsActive(id) {
					res.SkippedActive++
					continue
				}
				collect(filepath.Join(b.path, id), id)
			}
			if projectDirs == 0 {
				collect(b.path, "")
			}
		}
	}

	// LRU：按 lastUsed 升序，先淘汰最久未使用。
	sort.Slice(cands, func(i, j int) bool {
		return cands[i].lastUsed.Before(cands[j].lastUsed)
	})

	for _, c := range cands {
		if targetBytes > 0 && res.BytesFreed >= targetBytes {
			break
		}
		if maxCount > 0 && res.EvictedCount >= maxCount {
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

	res.Achieved = targetBytes <= 0 || res.BytesFreed >= targetBytes
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
	// B4：字节淘汰后重新统计再做数量限制；旧 usage 可能已删除足够目录，
	// 继续用会触发多余淘汰。
	usage, err = m.Usage(ctx)
	if err != nil {
		return err
	}
	return m.evictOverCount(ctx, usage)
}

// evictOverCount 按 LRU 淘汰超出 maxDatabases 数量上限的库（尽力而为）。
// B4：直接按「超额目录数」驱动淘汰数量，不再用字节目标近似（旧算法
// 超额 3 个目录 × 1024 字节，删一个大目录就达字节目标而数量仍超额）。
// maxDatabases<=0 表示不限制。
func (m *Manager) evictOverCount(ctx context.Context, usage Usage) error {
	if m.maxDatabases <= 0 || usage.DatabaseDirs <= m.maxDatabases {
		return nil
	}
	maxEvict := usage.DatabaseDirs - m.maxDatabases
	_, err := m.evict(ctx, 0, maxEvict)
	return err
}
