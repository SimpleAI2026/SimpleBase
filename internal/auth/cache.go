// cache.go 实现 §7.2 P1.2：API Key 认证缓存。
//
// 每请求一次 FindByHash 系统库点查（约 4ms）是固定开销的主体。
// 缓存 keyHash → Principal，TTL 30s；本进程吊销/删除时主动失效，
// 多实例靠 TTL 兜底（最坏 30s 吊销延迟，安全语义见 plan §7.2 P1）。
// 无效 Key 负缓存 5s，防随机 Key 打穿缓存；同时保留逐 Key 限流由
// 上层（未来）补充。
package auth

import (
	"context"
	"sync"
	"time"
)

// apiKeyCacheTTL 是正向缓存有效期。
const apiKeyCacheTTL = 30 * time.Second

// apiKeyNegTTL 是「无效 key」负缓存有效期（防打穿）。
const apiKeyNegTTL = 5 * time.Second

// apiKeyCacheMax 是正向条目上限（LRU 近似：超限全量清空，key 数有限且
// 由管理员创建，实际不会触顶；保留上限防御异常流量）。
const apiKeyCacheMax = 10000

type apiKeyCacheEntry struct {
	principal Principal
	expires   time.Time
}

type apiKeyNegEntry struct {
	expires time.Time
}

// apiKeyCache 是 keyHash → Principal 的进程内缓存。
type apiKeyCache struct {
	mu      sync.Mutex
	entries map[string]apiKeyCacheEntry
	neg     map[string]apiKeyNegEntry
	now     func() time.Time
}

func newAPIKeyCache() *apiKeyCache {
	return &apiKeyCache{
		entries: map[string]apiKeyCacheEntry{},
		neg:     map[string]apiKeyNegEntry{},
		now:     time.Now,
	}
}

// get 命中且未过期时返回 Principal。
func (c *apiKeyCache) get(keyHash string) (Principal, bool) {
	if c == nil {
		return Principal{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[keyHash]; ok {
		if c.now().Before(e.expires) {
			return e.principal, true
		}
		delete(c.entries, keyHash)
	}
	return Principal{}, false
}

// isNegative 报告该 hash 是否在负缓存期内（近期确认无效）。
func (c *apiKeyCache) isNegative(keyHash string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.neg[keyHash]; ok {
		if c.now().Before(e.expires) {
			return true
		}
		delete(c.neg, keyHash)
	}
	return false
}

// put 写入正向条目；超限时先清理过期项，仍超限则全量清空。
func (c *apiKeyCache) put(keyHash string, p Principal) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= apiKeyCacheMax {
		now := c.now()
		for k, e := range c.entries {
			if now.After(e.expires) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= apiKeyCacheMax {
			c.entries = map[string]apiKeyCacheEntry{}
		}
	}
	c.entries[keyHash] = apiKeyCacheEntry{principal: p, expires: c.now().Add(apiKeyCacheTTL)}
	delete(c.neg, keyHash)
}

// putNegative 写入负缓存（无效/吊销 key 短期拒绝，减少系统库查询）。
func (c *apiKeyCache) putNegative(keyHash string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.neg) >= apiKeyCacheMax {
		c.neg = map[string]apiKeyNegEntry{}
	}
	c.neg[keyHash] = apiKeyNegEntry{expires: c.now().Add(apiKeyNegTTL)}
	delete(c.entries, keyHash)
}

// InvalidateKey 按 hash 失效单条（吊销/删除路径调用）。
func (c *apiKeyCache) InvalidateKey(keyHash string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, keyHash)
	delete(c.neg, keyHash)
}

// InvalidateAll 清空全部缓存（Key 批量管理/测试用）。
func (c *apiKeyCache) InvalidateAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]apiKeyCacheEntry{}
	c.neg = map[string]apiKeyNegEntry{}
}

// invalidateUser 禁用用户时按 TenantID 粗粒度失效（Principal 不含用户维度，
// JWT 通道的用户禁用依赖 SessionService 校验；此处兜底 API Key 侧影响面）。
func (c *apiKeyCache) invalidateTenant(tenantID string) {
	if c == nil || tenantID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if e.principal.TenantID == tenantID {
			delete(c.entries, k)
		}
	}
}

// CachedAuthenticate 是带缓存的认证入口（Service 内部使用）。
// 返回值与 Authenticate 完全一致；缓存只影响查询次数，不改判定逻辑。
func (s *Service) CachedAuthenticate(ctx context.Context, rawKey string) (Principal, error) {
	if rawKey == "" {
		return Principal{}, ErrMissingCredentials
	}
	if s.repo == nil {
		return Principal{}, &notConfiguredError{}
	}
	hash := s.HashKey(rawKey)
	if p, ok := s.cache.get(hash); ok {
		return p, nil
	}
	if s.cache.isNegative(hash) {
		return Principal{}, ErrInvalidCredentials
	}
	p, err := s.authenticateUncached(ctx, hash)
	if err != nil {
		if isInvalidCredentialsErr(err) {
			s.cache.putNegative(hash)
		}
		return Principal{}, err
	}
	s.cache.put(hash, p)
	return p, nil
}

type notConfiguredError struct{}

func (e *notConfiguredError) Error() string { return "auth: repository not configured" }

func isInvalidCredentialsErr(err error) bool {
	return err == ErrInvalidCredentials || err == ErrKeyRevoked
}
