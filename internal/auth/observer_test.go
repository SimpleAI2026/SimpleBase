// observer_test.go 验证 planv5.0 §4 P0.1 的认证阶段观测：
// 锁等待/缓存命中/查用户/项目集分段在命中与未命中路径都正确上报。
package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingAuthObserver 收集观察事件（测试断言用）。
type recordingAuthObserver struct {
	mu     sync.Mutex
	stages []recordedStage
	caches []AuthCacheState
}

type recordedStage struct {
	Stage AuthStage
	D     time.Duration
}

func (r *recordingAuthObserver) ObserveAuthStage(_ context.Context, stage AuthStage, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stages = append(r.stages, recordedStage{Stage: stage, D: d})
}

func (r *recordingAuthObserver) ObserveAuthCache(_ context.Context, state AuthCacheState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caches = append(r.caches, state)
}

func (r *recordingAuthObserver) hasStage(s AuthStage) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, st := range r.stages {
		if st.Stage == s {
			return true
		}
	}
	return false
}

func (r *recordingAuthObserver) lastCache() AuthCacheState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.caches) == 0 {
		return AuthCacheNone
	}
	return r.caches[len(r.caches)-1]
}

// blockingUserRepo 在 GetByID 时阻塞一次，用于制造可观测的加载窗口。
// 用 CAS 而非 sync.Once：Once 的内部互斥会把后续调用串行化，
// 干扰「不同用户互不阻塞」的断言。
type blockingUserRepo struct {
	memUserRepo
	blocked atomic.Bool
	block   func()
}

func (b *blockingUserRepo) GetByID(ctx context.Context, id string) (User, error) {
	if b.block != nil && b.blocked.CompareAndSwap(false, true) {
		b.block()
	}
	return b.memUserRepo.GetByID(ctx, id)
}

func TestPrincipalFromClaimsObserverStages(t *testing.T) {
	ctx := context.Background()
	repo := &blockingUserRepo{memUserRepo: *newMemUserRepo()}
	users := NewUserService(repo, "tenant-1")
	obs := &recordingAuthObserver{}
	sessions := NewSessionService(users, newMemSessionRepo(), "secret", SessionConfig{}).WithAuthObserver(obs)

	u, err := users.Create(ctx, CreateUserInput{Username: "obs-user", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	claims := JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"}

	// 未命中路径：lock_wait + user_load + project_expand + cache miss。
	if _, err := sessions.PrincipalFromClaims(ctx, claims); err != nil {
		t.Fatal(err)
	}
	if !obs.hasStage(AuthStageLockWait) {
		t.Fatal("lock_wait stage missing on miss path")
	}
	if !obs.hasStage(AuthStageUserLoad) {
		t.Fatal("user_load stage missing on miss path")
	}
	if !obs.hasStage(AuthStageProjectExpand) {
		t.Fatal("project_expand stage missing on miss path")
	}
	if obs.lastCache() != AuthCacheMiss {
		t.Fatalf("want cache miss, got %s", obs.lastCache())
	}

	// 命中路径：lock_wait + cache hit，不再查库。
	obs.mu.Lock()
	obs.stages = nil
	obs.mu.Unlock()
	if _, err := sessions.PrincipalFromClaims(ctx, claims); err != nil {
		t.Fatal(err)
	}
	if !obs.hasStage(AuthStageLockWait) {
		t.Fatal("lock_wait stage missing on hit path")
	}
	if obs.hasStage(AuthStageUserLoad) {
		t.Fatal("user_load must not fire on cache hit")
	}
	if obs.lastCache() != AuthCacheHit {
		t.Fatalf("want cache hit, got %s", obs.lastCache())
	}
}

func TestPrincipalFromClaimsLockWaitObservable(t *testing.T) {
	// P1.1 后：用户 A 未命中且 GetByID 阻塞期间，用户 B 的 PrincipalFromClaims
	// 不再被全局锁阻塞——B 应在 A 释放前完成，lock_wait 保持微秒级。
	ctx := context.Background()
	repo := &blockingUserRepo{memUserRepo: *newMemUserRepo()}
	inflight := make(chan struct{})
	release := make(chan struct{})
	repo.block = func() {
		close(inflight)
		<-release
	}
	users := NewUserService(repo, "tenant-1")
	obs := &recordingAuthObserver{}
	sessions := NewSessionService(users, newMemSessionRepo(), "secret", SessionConfig{}).WithAuthObserver(obs)

	a, err := users.Create(ctx, CreateUserInput{Username: "aaa", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	b, err := users.Create(ctx, CreateUserInput{Username: "bbb", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: a.ID, SessionID: "sa", JWTID: "ja"})
		done <- err
	}()
	<-inflight // A 已进入锁外查库

	// B 必须在 A 释放前完成（不同用户互不阻塞）。
	bDone := make(chan struct{})
	go func() {
		defer close(bDone)
		_, _ = sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: b.ID, SessionID: "sb", JWTID: "jb"})
	}()
	select {
	case <-bDone:
	case <-time.After(2 * time.Second):
		t.Fatal("user B blocked behind user A's in-flight load (lock convoy regression)")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// 两次调用各记录一次 lock_wait，且都应是短暂等待（无 I/O 阻塞）。
	obs.mu.Lock()
	defer obs.mu.Unlock()
	counts := map[AuthStage]int{}
	var maxLockWait time.Duration
	for _, st := range obs.stages {
		counts[st.Stage]++
		if st.Stage == AuthStageLockWait && st.D > maxLockWait {
			maxLockWait = st.D
		}
	}
	if counts[AuthStageLockWait] < 2 {
		t.Fatalf("expected >=2 lock_wait records, got %d", counts[AuthStageLockWait])
	}
	if maxLockWait > 100*time.Millisecond {
		t.Fatalf("lock_wait should stay tiny after P1.1, got %v", maxLockWait)
	}
}

func TestRefreshObserverSessionLoad(t *testing.T) {
	ctx := context.Background()
	users := NewUserService(newMemUserRepo(), "tenant-1")
	srepo := newMemSessionRepo()
	obs := &recordingAuthObserver{}
	sessions := NewSessionService(users, srepo, "secret", SessionConfig{
		AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour,
	}).WithAuthObserver(obs)

	_, _, err := users.EnsureBootstrapUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, pair, err := sessions.Login(ctx, BootstrapUsername, BootstrapPassword, "ua", "ip")
	if err != nil {
		t.Fatal(err)
	}
	obs.mu.Lock()
	obs.stages = nil
	obs.mu.Unlock()
	if _, _, err := sessions.Refresh(ctx, pair.RefreshToken, "ua", "ip"); err != nil {
		t.Fatal(err)
	}
	if !obs.hasStage(AuthStageSessionLoad) {
		t.Fatal("session_load stage missing on refresh")
	}
}

func TestAPIKeyObserverCacheStates(t *testing.T) {
	// API Key 通道：命中/未命中/负缓存状态正确上报，key_load 只在未命中时记录。
	ctx := context.Background()
	repo := &countingRepo{rec: APIKeyRecord{ID: "k1", TenantID: "t1"}}
	svc := NewService(repo, "secret")
	repo.rec.KeyHash = svc.HashKey("raw-key")
	obs := &recordingAuthObserver{}
	svc.WithAuthObserver(obs)

	if _, err := svc.Authenticate(ctx, "raw-key"); err != nil {
		t.Fatal(err)
	}
	if obs.lastCache() != AuthCacheMiss {
		t.Fatalf("first authenticate should be miss, got %s", obs.lastCache())
	}
	if !obs.hasStage(AuthStageKeyLoad) {
		t.Fatal("key_load stage missing on miss")
	}

	obs.mu.Lock()
	obs.stages = nil
	obs.caches = nil
	obs.mu.Unlock()
	if _, err := svc.Authenticate(ctx, "raw-key"); err != nil {
		t.Fatal(err)
	}
	if obs.lastCache() != AuthCacheHit {
		t.Fatalf("second authenticate should hit, got %s", obs.lastCache())
	}
	if obs.hasStage(AuthStageKeyLoad) {
		t.Fatal("key_load must not fire on cache hit")
	}

	// 无效 key：miss + 负缓存写入；再次无效 key → hit（负缓存）。
	obs.mu.Lock()
	obs.caches = nil
	obs.mu.Unlock()
	if _, err := svc.Authenticate(ctx, "sb_live_invalid"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want invalid credentials, got %v", err)
	}
	if obs.lastCache() != AuthCacheMiss {
		t.Fatalf("invalid key should be miss, got %s", obs.lastCache())
	}
	obs.mu.Lock()
	obs.caches = nil
	obs.mu.Unlock()
	if _, err := svc.Authenticate(ctx, "sb_live_invalid"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want invalid credentials, got %v", err)
	}
	if obs.lastCache() != AuthCacheHit {
		t.Fatalf("repeat invalid key should hit negative cache, got %s", obs.lastCache())
	}
}
