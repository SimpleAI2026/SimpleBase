// principal_singleflight_test.go 验证 planv5.0 §4 P1.1：
// I/O 移出全局锁后的行为契约——同用户单飞、不同用户并行、
// 失效代数阻止旧结果写回、错误不缓存且正确传播。
package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingBlockRepo 统计 GetByID 调用次数，首次调用阻塞直到放行。
// 用 CAS 而非 sync.Once：Once 的内部互斥会把后续调用串行化，
// 干扰「不同用户互不阻塞」的断言。
type countingBlockRepo struct {
	memUserRepo
	mu       sync.Mutex
	calls    int
	firstHit chan struct{}
	release  chan struct{}
	fired    atomic.Bool
}

func (r *countingBlockRepo) GetByID(ctx context.Context, id string) (User, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if r.firstHit != nil && r.fired.CompareAndSwap(false, true) {
		close(r.firstHit)
		<-r.release
	}
	return r.memUserRepo.GetByID(ctx, id)
}

func (r *countingBlockRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// errorUserRepo 的 GetByID 在 err 非 nil 时失败；恢复（err=nil）后透传嵌入仓储。
type errorUserRepo struct {
	memUserRepo
	err error
}

func (r *errorUserRepo) GetByID(ctx context.Context, id string) (User, error) {
	if r.err != nil {
		return User{}, r.err
	}
	return r.memUserRepo.GetByID(ctx, id)
}

func newSingleflightSessions(t *testing.T, repo UserRepository) *SessionService {
	t.Helper()
	users := NewUserService(repo, "tenant-1")
	return NewSessionService(users, newMemSessionRepo(), "secret", SessionConfig{})
}

// TestSameUserSingleFlight：同用户并发未命中只触发一次 GetByID，
// 所有请求共享同一结果。
func TestSameUserSingleFlight(t *testing.T) {
	ctx := context.Background()
	repo := &countingBlockRepo{
		memUserRepo: *newMemUserRepo(),
		firstHit:    make(chan struct{}),
		release:     make(chan struct{}),
	}
	sessions := newSingleflightSessions(t, repo)
	users := sessions.users

	u, err := users.Create(ctx, CreateUserInput{Username: "sf-user", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"})
		}(i)
	}
	<-repo.firstHit
	// 等待 follower 全部就位再放行，确保单飞窗口被覆盖。
	time.Sleep(50 * time.Millisecond)
	close(repo.release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}
	if got := repo.callCount(); got != 1 {
		t.Fatalf("same-user concurrent miss should trigger exactly 1 GetByID, got %d", got)
	}
}

// TestDifferentUsersParallel：用户 A 的慢加载不阻塞用户 B（锁外 I/O）。
func TestDifferentUsersParallel(t *testing.T) {
	ctx := context.Background()
	repo := &countingBlockRepo{
		memUserRepo: *newMemUserRepo(),
		firstHit:    make(chan struct{}),
		release:     make(chan struct{}),
	}
	sessions := newSingleflightSessions(t, repo)
	users := sessions.users

	a, err := users.Create(ctx, CreateUserInput{Username: "user-a", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	b, err := users.Create(ctx, CreateUserInput{Username: "user-b", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		_, _ = sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: a.ID, SessionID: "sa", JWTID: "ja"})
	}()
	<-repo.firstHit // A 的加载阻塞中

	bDone := make(chan error, 1)
	go func() {
		_, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: b.ID, SessionID: "sb", JWTID: "jb"})
		bDone <- err
	}()
	select {
	case err := <-bDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("user B blocked behind user A's load (lock convoy regression)")
	}
	close(repo.release)
}

// TestInvalidateDuringLoadDropsStaleBackfill：加载期间调用 InvalidateUser
// （代数前进）后，旧结果不得写回缓存；下一次请求重新加载。
func TestInvalidateDuringLoadDropsStaleBackfill(t *testing.T) {
	ctx := context.Background()
	repo := &countingBlockRepo{
		memUserRepo: *newMemUserRepo(),
		firstHit:    make(chan struct{}),
		release:     make(chan struct{}),
	}
	sessions := newSingleflightSessions(t, repo)
	users := sessions.users

	u, err := users.Create(ctx, CreateUserInput{Username: "stale-user", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"})
		done <- err
	}()
	<-repo.firstHit
	// 加载进行中：权限变更推进代数。
	sessions.InvalidateUser(u.ID)
	close(repo.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// 旧结果已被丢弃：再次请求必须重新查库（第二次 GetByID）。
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j2"}); err != nil {
		t.Fatal(err)
	}
	if got := repo.callCount(); got != 2 {
		t.Fatalf("stale backfill must be dropped; want 2 loads, got %d", got)
	}

	// 回填后的缓存有效：第三次请求不再查库。
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j3"}); err != nil {
		t.Fatal(err)
	}
	if got := repo.callCount(); got != 2 {
		t.Fatalf("valid cache should suppress reload; want 2 loads, got %d", got)
	}
}

// TestLoadErrorNotCached：查库失败不写缓存，错误按原语义传播；
// follower 不会永久阻塞，且自行重查。
func TestLoadErrorNotCached(t *testing.T) {
	ctx := context.Background()
	repo := &errorUserRepo{memUserRepo: *newMemUserRepo(), err: errors.New("db down")}
	sessions := newSingleflightSessions(t, repo)
	users := sessions.users

	a, err := users.Create(ctx, CreateUserInput{Username: "err-user", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		_, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: a.ID, SessionID: "s1", JWTID: "j1"})
		if !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("want ErrInvalidToken, got %v", err)
		}
	}

	// 错误未缓存：恢复后下一次请求成功。
	repo.err = nil
	if _, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: a.ID, SessionID: "s1", JWTID: "j1"}); err != nil {
		t.Fatalf("should recover after repo heals: %v", err)
	}
}

// TestDisabledUserRejectedEveryRequest：禁用用户不被缓存绕过。
func TestDisabledUserRejectedEveryRequest(t *testing.T) {
	ctx := context.Background()
	repo := newMemUserRepo()
	sessions := newSingleflightSessions(t, repo)
	users := sessions.users

	u, err := users.Create(ctx, CreateUserInput{Username: "dis-user", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	claims := JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"}

	if _, err := sessions.PrincipalFromClaims(ctx, claims); err != nil {
		t.Fatal(err)
	}

	// 禁用并失效（OnChange 回调真实链路）。
	if _, err := users.Disable(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := sessions.PrincipalFromClaims(ctx, claims); !errors.Is(err, ErrUserDisabled) {
			t.Fatalf("disabled user must be rejected, got %v", err)
		}
	}
}

// TestFollowerUnblocksAfterOwnerError：owner 失败后 follower 自行重查，
// 不会卡死在 flight.done 上。
func TestFollowerUnblocksAfterOwnerError(t *testing.T) {
	ctx := context.Background()
	repo := &countingBlockRepo{
		memUserRepo: *newMemUserRepo(),
		firstHit:    make(chan struct{}),
		release:     make(chan struct{}),
	}
	sessions := newSingleflightSessions(t, repo)
	users := sessions.users

	u, err := users.Create(ctx, CreateUserInput{Username: "fol-user", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}

	ownerErr := make(chan error, 1)
	go func() {
		_, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j1"})
		ownerErr <- err
	}()
	<-repo.firstHit
	close(repo.release)

	// owner 完成后 follower 到来：缓存已回填，直接命中。
	done := make(chan error, 1)
	go func() {
		_, err := sessions.PrincipalFromClaims(ctx, JWTClaims{Subject: u.ID, SessionID: "s1", JWTID: "j2"})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follower stuck after owner completed")
	}
	if err := <-ownerErr; err != nil {
		t.Fatalf("owner should succeed, got %v", err)
	}
}
