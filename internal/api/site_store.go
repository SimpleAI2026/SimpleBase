// 文件说明：site_store.go — 对象存储发布网站（planv6.5 §5.4/§5.7）：站点开关与公开路径的缓存适配
//
// 用法
//
//	store := NewSiteStore(systemStore, ttl)
//	store.Lookup(ctx, pid) (siteState, error)        // 站点面 / 列表接口读取（TTL 缓存）
//	store.SetSite / SetPublicPath / DeletePublicFile  // 管理 API 写入，写后本进程立即失效
//
// 只读副本的缓存靠 TTL 收敛（默认 30s）。

package api

import (
	"context"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// SiteStore 抽象站点开关与公开路径的读写（handler 不直接依赖 systemdb.Store）。
type SiteStore interface {
	Lookup(ctx context.Context, projectID string) (siteState, error)
	SetSite(ctx context.Context, projectID string, enabled bool, actor string) error
	SetPublicPath(ctx context.Context, projectID, path, kind string, public bool, actor string) error
	DeletePublicFile(ctx context.Context, projectID, key string) error
	Invalidate(projectID string)
}

// siteState 是单个项目的站点快照。
type siteState struct {
	Enabled bool
	Paths   []systemdb.PublicPath
}

type siteCacheEntry struct {
	state   siteState
	expires time.Time
}

// systemSiteStore 以 systemdb.Store 为后端、带 TTL 缓存的 SiteStore。
type systemSiteStore struct {
	store *systemdb.Store
	ttl   time.Duration
	now   func() time.Time

	mu    sync.Mutex
	cache map[string]siteCacheEntry
}

// NewSiteStore 构造带缓存的 SiteStore；store 为 nil 时返回 nil（不挂站点功能）。
func NewSiteStore(store *systemdb.Store, ttl time.Duration) SiteStore {
	if store == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &systemSiteStore{store: store, ttl: ttl, now: time.Now, cache: map[string]siteCacheEntry{}}
}

func (s *systemSiteStore) Lookup(ctx context.Context, projectID string) (siteState, error) {
	now := s.now()
	s.mu.Lock()
	if e, ok := s.cache[projectID]; ok && now.Before(e.expires) {
		s.mu.Unlock()
		return e.state, nil
	}
	s.mu.Unlock()
	site, err := s.store.GetProjectSite(ctx, projectID)
	if err != nil {
		return siteState{}, err
	}
	paths, err := s.store.ListPublicPaths(ctx, projectID)
	if err != nil {
		return siteState{}, err
	}
	state := siteState{Enabled: site.Enabled, Paths: paths}
	s.mu.Lock()
	s.cache[projectID] = siteCacheEntry{state: state, expires: now.Add(s.ttl)}
	s.mu.Unlock()
	return state, nil
}

func (s *systemSiteStore) SetSite(ctx context.Context, projectID string, enabled bool, actor string) error {
	defer s.Invalidate(projectID)
	return s.store.SetProjectSite(ctx, projectID, enabled, actor)
}

func (s *systemSiteStore) SetPublicPath(ctx context.Context, projectID, path, kind string, public bool, actor string) error {
	defer s.Invalidate(projectID)
	return s.store.SetPublicPath(ctx, projectID, path, kind, public, actor)
}

func (s *systemSiteStore) DeletePublicFile(ctx context.Context, projectID, key string) error {
	defer s.Invalidate(projectID)
	return s.store.DeletePublicFile(ctx, projectID, key)
}

func (s *systemSiteStore) Invalidate(projectID string) {
	s.mu.Lock()
	delete(s.cache, projectID)
	s.mu.Unlock()
}
