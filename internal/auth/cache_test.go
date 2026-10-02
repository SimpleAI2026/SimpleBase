package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo 计数查询次数，验证缓存命中与失效契约（§7.2 P1.2）。
type countingRepo struct {
	rec     APIKeyRecord
	queries int
	revoked bool
}

func (r *countingRepo) FindByHash(ctx context.Context, keyHash string) (APIKeyRecord, error) {
	r.queries++
	if r.revoked {
		return APIKeyRecord{}, ErrInvalidCredentials
	}
	if keyHash != r.rec.KeyHash {
		return APIKeyRecord{}, ErrInvalidCredentials
	}
	return r.rec, nil
}

func TestAPIKeyCacheHitsAndInvalidation(t *testing.T) {
	repo := &countingRepo{rec: APIKeyRecord{
		ID:        "k1",
		TenantID:  "t1",
		KeyHash:   "", // 由 Service.HashKey 填充比对
		ProjectIDs: []string{"p1"},
	}}
	svc := NewService(repo, "secret")
	repo.rec.KeyHash = svc.HashKey("raw-key")

	ctx := context.Background()
	// 第一次：查库并缓存。
	if _, err := svc.Authenticate(ctx, "raw-key"); err != nil {
		t.Fatalf("auth: %v", err)
	}
	// 第二、三次：命中缓存，不查库。
	for i := 0; i < 2; i++ {
		if _, err := svc.Authenticate(ctx, "raw-key"); err != nil {
			t.Fatalf("auth cached: %v", err)
		}
	}
	if repo.queries != 1 {
		t.Fatalf("queries=%d, want 1 (cache miss only)", repo.queries)
	}

	// 吊销 → 全量失效 → 下次查询重新查库并拒绝。
	svc.InvalidateKeyHash(svc.HashKey("raw-key"))
	repo.revoked = true
	if _, err := svc.Authenticate(ctx, "raw-key"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("revoked auth: %v", err)
	}
	if repo.queries != 2 {
		t.Fatalf("queries=%d after invalidate, want 2", repo.queries)
	}
	// 负缓存期内不再查库。
	if _, err := svc.Authenticate(ctx, "raw-key"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("revoked auth (neg cached): %v", err)
	}
	if repo.queries != 2 {
		t.Fatalf("queries=%d in negative cache window, want 2", repo.queries)
	}
}

func TestAPIKeyCacheTTExpiry(t *testing.T) {
	repo := &countingRepo{rec: APIKeyRecord{ID: "k1", TenantID: "t1"}}
	svc := NewService(repo, "secret")
	repo.rec.KeyHash = svc.HashKey("raw")

	// 注入可控时钟。
	svc.cache.mu.Lock()
	svc.cache.now = func() time.Time { return fakeNow }
	svc.cache.mu.Unlock()

	fakeNow = time.Now()
	if _, err := svc.Authenticate(context.Background(), "raw"); err != nil {
		t.Fatalf("auth: %v", err)
	}
	fakeNow = fakeNow.Add(apiKeyCacheTTL + time.Second)
	if _, err := svc.Authenticate(context.Background(), "raw"); err != nil {
		t.Fatalf("auth after ttl: %v", err)
	}
	if repo.queries != 2 {
		t.Fatalf("queries=%d, want 2 (TTL expiry forces requery)", repo.queries)
	}
}

var fakeNow time.Time
