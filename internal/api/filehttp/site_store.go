package filehttp

import (
	"context"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// SiteStore 抽象公开路径的读写（handler 不直接依赖 systemdb.Store）。
type SiteStore interface {
	Lookup(ctx context.Context, projectID string) (SiteState, error)
	SetPublicPath(ctx context.Context, projectID, path, kind string, public bool, actor string) error
	DeletePublicFile(ctx context.Context, projectID, key string) error
	Invalidate(projectID string)
}

// SiteState 是单个项目的站点快照。站点没有实例级开关，只有公开路径。
type SiteState struct {
	Paths []systemdb.PublicPath
}

type siteCacheEntry struct {
	state   SiteState
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

func (s *systemSiteStore) Lookup(ctx context.Context, projectID string) (SiteState, error) {
	now := s.now()
	s.mu.Lock()
	if e, ok := s.cache[projectID]; ok && now.Before(e.expires) {
		s.mu.Unlock()
		return e.state, nil
	}
	s.mu.Unlock()
	paths, err := s.store.ListPublicPaths(ctx, projectID)
	if err != nil {
		return SiteState{}, err
	}
	state := SiteState{Paths: paths}
	s.mu.Lock()
	s.cache[projectID] = siteCacheEntry{state: state, expires: now.Add(s.ttl)}
	s.mu.Unlock()
	return state, nil
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
