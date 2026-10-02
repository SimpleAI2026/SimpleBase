// cache.go 实现 §7.2 P1.3/P1.4：project→tenant 与 database 行的进程内缓存。
//
// 语义与失效（plan §7.2 P1）：
//   - project→tenant：项目创建后 tenant 不可变，仅删除项目时失效；
//   - database 行：catalog Service 是写入唯一入口，状态迁移成功后同步失效，
//     本实例强一致；只读实例依赖短 TTL（2s）兜底。
package catalog

import (
	"context"
	"sync"
	"time"
)

const (
	projectTenantTTL    = 5 * time.Minute // project→tenant：近乎不可变，长 TTL
	databaseRowTTL      = 2 * time.Second // database 行：短 TTL（只读实例一致性兜底）
	databaseCacheMax    = 5000
	projectCacheMax     = 10000
	kvRowTTL            = 2 * time.Second
)

type cacheEntry[T any] struct {
	val     T
	expires time.Time
}

// ttlCache 是带 TTL 与容量上限的通用缓存（读多写少，mutex 足够）。
type ttlCache[T any] struct {
	mu      sync.Mutex
	entries map[string]cacheEntry[T]
	max     int
	now     func() time.Time
}

func newTTLCache[T any](max int) *ttlCache[T] {
	return &ttlCache[T]{entries: map[string]cacheEntry[T]{}, max: max, now: time.Now}
}

func (c *ttlCache[T]) get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || c.now().After(e.expires) {
		if ok {
			delete(c.entries, key)
		}
		var zero T
		return zero, false
	}
	return e.val, true
}

func (c *ttlCache[T]) put(key string, val T, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		now := c.now()
		for k, e := range c.entries {
			if now.After(e.expires) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= c.max {
			c.entries = map[string]cacheEntry[T]{}
		}
	}
	c.entries[key] = cacheEntry[T]{val: val, expires: c.now().Add(ttl)}
}

func (c *ttlCache[T]) delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func (c *ttlCache[T]) purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]cacheEntry[T]{}
}

// —— Service 集成 ——

// enableCache 在 Service 上懒初始化缓存（NewService 即启用）。
func (s *Service) enableCache() {
	if s.tenantCache == nil {
		s.tenantCache = newTTLCache[string](projectCacheMax)
	}
	if s.dbCache == nil {
		s.dbCache = newTTLCache[Database](databaseCacheMax)
	}
	if s.kvCache == nil {
		s.kvCache = newTTLCache[Database](projectCacheMax)
	}
}

// cachedProjectTenant 读 project→tenant 缓存；未命中执行 fn 并回填。
func (s *Service) cachedProjectTenant(ctx context.Context, projectID string, fn func(context.Context, string) (string, error)) (string, error) {
	if s.tenantCache != nil {
		if v, ok := s.tenantCache.get(projectID); ok {
			return v, nil
		}
	}
	v, err := fn(ctx, projectID)
	if err != nil {
		return "", err
	}
	if s.tenantCache != nil {
		s.tenantCache.put(projectID, v, projectTenantTTL)
	}
	return v, nil
}

// cachedDatabase 读 (projectID,databaseID) 行缓存；未命中执行 fn 并回填。
func (s *Service) cachedDatabase(ctx context.Context, projectID, databaseID string, fn func(context.Context, string, string) (Database, error)) (Database, error) {
	key := projectID + "/" + databaseID
	if s.dbCache != nil {
		if v, ok := s.dbCache.get(key); ok {
			return v, nil
		}
	}
	v, err := fn(ctx, projectID, databaseID)
	if err != nil {
		return Database{}, err
	}
	if s.dbCache != nil {
		s.dbCache.put(key, v, databaseRowTTL)
	}
	return v, nil
}

// cachedKVRow 读项目 KV 行缓存；未命中执行 fn 并回填。
func (s *Service) cachedKVRow(ctx context.Context, projectID string, fn func(context.Context, string) (Database, error)) (Database, error) {
	if s.kvCache != nil {
		if v, ok := s.kvCache.get(projectID); ok {
			return v, nil
		}
	}
	v, err := fn(ctx, projectID)
	if err != nil {
		return Database{}, err
	}
	if s.kvCache != nil {
		s.kvCache.put(projectID, v, kvRowTTL)
	}
	return v, nil
}

// InvalidateDatabase 写路径（状态迁移/删除/建库）后同步失效对应行。
func (s *Service) InvalidateDatabase(projectID, databaseID string) {
	if s == nil {
		return
	}
	key := projectID + "/" + databaseID
	s.dbCache.delete(key)
	s.kvCache.delete(projectID)
}

// InvalidateProject 删除项目时失效 tenant 与该项目全部行缓存。
func (s *Service) InvalidateProject(projectID string) {
	if s == nil {
		return
	}
	s.tenantCache.delete(projectID)
	s.kvCache.delete(projectID)
	// dbCache 按 key 前缀清理。
	s.dbCache.mu.Lock()
	prefix := projectID + "/"
	for k := range s.dbCache.entries {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			delete(s.dbCache.entries, k)
		}
	}
	s.dbCache.mu.Unlock()
}

// InvalidateAllCatalogCache 清空全部 catalog 缓存（运维/测试入口）。
func (s *Service) InvalidateAllCatalogCache() {
	if s == nil {
		return
	}
	s.tenantCache.purge()
	s.dbCache.purge()
	s.kvCache.purge()
}

var _ = context.Background
