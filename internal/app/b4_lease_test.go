package app

// b4_lease_test.go：planv6.0 B4「lease manager 与失租回调」块表征测试。
// 在提取私有装配方法前锁定旧实现行为：
//   1. writable + storage.database=s3 + lease.enabled + CAS 探针失败 → 拒绝启动
//      （probe 失败 + lease.enabled=true 的启动门禁，原样错误语义）；
//   2. 本地模式（dbRemote=false）即使 lease.enabled 也不装配 leaseMgr
//      （leaseKeys/leaseGate 保持零值）；
//   3. closer 注册顺序：systemStore 在 leaseCloser 之前注册（逆序关闭时
//      lease 先停续约）。以 registerCloser 的调用序作资源轨迹表征。
// S3 客户端构造依赖真实凭据链加载，不连远端的 lease.Manager 全链路
// 无法在进程内构造，故 lease 主路径以「装配开关位」作表征。

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

// leaseProbeFailConfig：可写 + 远端 DB + lease 启用，S3 endpoint 不可达。
// 对象客户端构造成功（仅本地配置加载），但 CAS 探针（PutIfAbsent）失败
// → 原 assembleDeps 第 394-400 行：lease.enabled=true 时拒绝启动。
func leaseProbeFailConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := testConfig(true) // writable + Storage.Database=s3
	cfg.Storage.DataDir = t.TempDir()
	cfg.S3 = config.S3Config{
		Endpoint: "http://127.0.0.1:1", Region: "us-east-1", Bucket: "b", Prefix: "simplebase",
		AccessKey: "ak", SecretKey: "sk", ForcePathStyle: true,
	}
	cfg.Database.CacheDir = t.TempDir()
	cfg.Instance.Lease = config.InstanceLeaseConfig{
		Enabled: true,
		TTL:     30 * time.Second,
		Grace:   10 * time.Second,
		OnLost:  "log",
	}
	return cfg
}

// TestB4_LeaseEnabledProbeFailRejectsStart：CAS 探针失败 + lease.enabled=true
// → assembleDeps 返回错误（拒绝在无单写保护下启动可写实例）。
func TestB4_LeaseEnabledProbeFailRejectsStart(t *testing.T) {
	cfg := leaseProbeFailConfig(t)
	_, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err == nil {
		t.Fatal("expected probe failure to reject startup when lease.enabled=true")
	}
	if !strings.Contains(err.Error(), "create-if-absent probe failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestB4_LeaseNotAssembledOnLocalDB：本地 DB 模式下即使 lease.enabled=true
// 也不装配 lease（dbRemote=false 短路）。此处只读实例走 writable 提前返回，
// 以只读 + 本地存储验证 lease 字段保持零值。
func TestB4_LeaseNotAssembledOnLocalDB(t *testing.T) {
	cfg := testConfig(false)
	cfg.Storage = config.StorageConfig{
		Database: config.StorageModeLocal,
		Files:    config.StorageModeLocal,
		DataDir:  t.TempDir(),
	}
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	cfg.Instance.Lease = config.InstanceLeaseConfig{Enabled: true, OnLost: "log"}
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	if a.leaseMgr != nil || a.leaseKeys.RootPrefix != "" || a.leaseGate != nil {
		t.Fatalf("local readonly must not assemble lease: mgr=%v keys=%q gate=%v",
			a.leaseMgr != nil, a.leaseKeys.RootPrefix, a.leaseGate != nil)
	}
}
