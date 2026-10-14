package app

// b4_auth_seed_test.go：planv6.0 B4「auth/user/session、Seed、retention/flush」
// 与「maintenance、cache manager、usage/audit」块的表征测试。
// 在提取私有装配方法前锁定旧实现行为：
//   1. writable 本地装配后 auth/users/sessions 均构造完成；
//   2. OnWrite 通知已接入（users/sessions 各持有 store.NotifyWrite）——
//      通过公共行为表征：无 panic 且服务可用；
//   3. Seed 幂等：同一 data_dir 重复装配不报错（seed 已有的 bootstrap user）；
//   4. 维护任务与缓存管理器装配成功（cacheMgr 非 nil）；
//   5. usage/audit 服务装配成功（usageSvc/auditSvc 非 nil）；
//   6. closer 注册轨迹：Shutdown 后再次 Shutdown 不 panic（幂等收尾）。
// 全部走 NewWithRegistry 真实入口，不直接调用将提取的 helper。

import (
	"context"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

func authSeedTestConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := testConfig(true)
	cfg.Storage = config.StorageConfig{
		Database: config.StorageModeLocal,
		Files:    config.StorageModeLocal,
		DataDir:  t.TempDir(),
	}
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	return cfg
}

func TestB4_AuthServicesAndSeedAssembled(t *testing.T) {
	cfg := authSeedTestConfig(t)
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
		// 幂等收尾：重复 Shutdown 不 panic。
		_ = a.Shutdown(ctx)
	})
	if a.auth == nil || a.users == nil || a.sessions == nil {
		t.Fatalf("auth services must be assembled: auth=%v users=%v sessions=%v",
			a.auth == nil, a.users == nil, a.sessions == nil)
	}
	if a.cacheMgr == nil {
		t.Fatal("cache manager must be assembled")
	}
	if a.usageSvc == nil || a.auditSvc == nil {
		t.Fatal("usage/audit services must be assembled")
	}
	if a.systemStore == nil || a.catalog == nil || a.registry == nil {
		t.Fatal("system store / catalog / registry must be assembled")
	}
}

func TestB4_SeedIdempotentOnSameDataDir(t *testing.T) {
	cfg := authSeedTestConfig(t)
	a1, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	_ = a1.Shutdown(ctx1)

	// 同一 data_dir/cache_dir 再次装配：Seed 对既有行幂等，不报错。
	a2, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("re-assemble on same data dir must succeed: %v", err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_ = a2.Shutdown(ctx2)
}
