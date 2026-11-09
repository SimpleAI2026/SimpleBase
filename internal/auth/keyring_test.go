// 文件说明：keyring_test.go — 版本化密钥环与轮换语义测试（§8.2.4）
package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestParseKeyring(t *testing.T) {
	k, err := ParseKeyring("v2:new-secret,v1:old-secret")
	if err != nil {
		t.Fatal(err)
	}
	if k.Len() != 2 {
		t.Fatalf("len=%d want 2", k.Len())
	}
	v, secret := k.Primary()
	if v != "v2" || secret != "new-secret" {
		t.Fatalf("primary=%s,%s want v2,new-secret", v, secret)
	}

	if _, err := ParseKeyring(""); err != nil || k.Len() == 0 {
		t.Fatal("empty spec must yield zero keyring without error")
	}
	for spec, wantErr := range map[string]bool{
		"v1:only":         false,
		"v10:a,v9:b,v2:c": false, // 乱序输入按数字降排
		"v1:":             true,
		":secret":         true,
		"secret":          true,
		"x1:secret":       true,
		"v1:a,v1:b":       true,  // 重复版本
		"v1:a,,v2:b":      false, // 空段跳过
	} {
		_, err := ParseKeyring(spec)
		if (err != nil) != wantErr {
			t.Errorf("ParseKeyring(%q) err=%v wantErr=%v", spec, err, wantErr)
		}
	}
	k3, _ := ParseKeyring("v10:a,v9:b,v2:c")
	if v, _ := k3.Primary(); v != "v10" {
		t.Fatalf("primary=%s want v10", v)
	}
}

func TestKeyringRotationAuthenticate(t *testing.T) {
	// 轮换前：单盐 v1 签发的 Key。
	oldSvc := NewService(newFakeRepo(), "old-secret")
	rawKey := "sb_live_rotated"
	oldHash := oldSvc.HashKey(rawKey)

	repo := newFakeRepo()
	repo.records[oldHash] = APIKeyRecord{ID: "k1", TenantID: "t1", KeyHash: oldHash}

	// 轮换后：keyring v2（新）+ v1（旧）。
	ring, err := ParseKeyring("v2:new-secret,v1:old-secret")
	if err != nil {
		t.Fatal(err)
	}
	newSvc := NewService(repo, "ignored").WithKeyring(ring)

	// 旧 Key（v1 摘要存储）仍能认证。
	if _, err := newSvc.Authenticate(context.Background(), rawKey); err != nil {
		t.Fatalf("old key after rotation: %v", err)
	}

	// 新签发的 Key 用 v2 摘要；v1 摘要不存在 → 也应能通过主版本命中。
	newHash := newSvc.HashKey(rawKey)
	if newHash == oldHash {
		t.Fatal("primary version must differ from old salt")
	}
	repo.records[newHash] = APIKeyRecord{ID: "k2", TenantID: "t1", KeyHash: newHash}
	if _, err := newSvc.Authenticate(context.Background(), rawKey); err != nil {
		t.Fatalf("new key: %v", err)
	}

	// 下个窗口去掉 v1：只剩 v2 的记录可认证。
	ring2, _ := ParseKeyring("v2:new-secret")
	finalSvc := NewService(newFakeRepo(), "ignored").WithKeyring(ring2)
	repo2 := newFakeRepo()
	repo2.records[newHash] = APIKeyRecord{ID: "k2", TenantID: "t1", KeyHash: newHash}
	finalSvc.repo = repo2
	if _, err := finalSvc.Authenticate(context.Background(), rawKey); err != nil {
		t.Fatalf("new-only keyring: %v", err)
	}
	delete(repo2.records, newHash)
	// 新实例绕过进程内缓存：记录删除后必须 invalid。
	finalSvc2 := NewService(newFakeRepo(), "ignored").WithKeyring(ring2)
	finalSvc2.repo = repo2
	if _, err := finalSvc2.Authenticate(context.Background(), rawKey); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("removed record must be invalid, got %v", err)
	}
}

func TestKeyringRevokedStillRevoked(t *testing.T) {
	// 吊销的 Key 在任何版本摘要下都必须是 ErrKeyRevoked（不被「试旧版本」掩盖）。
	ring, _ := ParseKeyring("v2:new,v1:old")
	svc := NewService(newFakeRepo(), "ignored").WithKeyring(ring)
	rawKey := "sb_live_rev"
	oldHash := hmacHex("old", rawKey)
	now := time.Now()
	repo := newFakeRepo()
	repo.records[oldHash] = APIKeyRecord{ID: "k", TenantID: "t", KeyHash: oldHash, RevokedAt: &now}
	svc.repo = repo
	if _, err := svc.Authenticate(context.Background(), rawKey); !errors.Is(err, ErrKeyRevoked) {
		t.Fatalf("revoked must surface ErrKeyRevoked, got %v", err)
	}
}
