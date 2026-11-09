package app

// r2_write_handlers_test.go：planv6.0 R2-B2 表征测试（提取前先行）。
// 锁定 NewWithRegistry 中「可写实例 SQL/数据/Schema/KV handler 装配块」：
//   1. 可写实例：execute/batch/schema/data/kv/collection meta 路由均挂载，
//      且 dbHandler.InitSQL 被注入（经 DatabaseHandler 可观察的路由层面不变）；
//   2. 只读实例：上述写 handler 全部为 nil → 对应路由不挂载。

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

func writeHandlerRoutes(t *testing.T, writable bool) map[string]bool {
	t.Helper()
	cfg := testConfig(writable)
	cfg.Storage = config.StorageConfig{Database: config.StorageModeLocal, Files: config.StorageModeLocal, DataDir: t.TempDir()}
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	got := map[string]bool{}
	for _, r := range a.echo.Routes() {
		got[r.Method+" "+r.Path] = true
	}
	return got
}

var r2WriteRoutes = []string{
	http.MethodPost + " /v1/projects/:projectID/databases/:databaseID/query",
	http.MethodPost + " /v1/projects/:projectID/databases/:databaseID/execute",
	http.MethodPost + " /v1/projects/:projectID/databases/:databaseID/batch",
	http.MethodGet + " /v1/projects/:projectID/databases/:databaseID/schema",
	http.MethodPost + " /v1/projects/:projectID/databases/:databaseID/data/collections",
	http.MethodPut + " /v1/projects/:projectID/databases/:databaseID/data/collections/:collection/meta",
	http.MethodPost + " /v1/projects/:projectID/kv",
}

func TestR2_WritableAssemblesWriteHandlers(t *testing.T) {
	got := writeHandlerRoutes(t, true)
	for _, r := range r2WriteRoutes {
		if !got[r] {
			t.Errorf("writable instance missing route %s", r)
		}
	}
}

func TestR2_ReadOnlySkipsWriteHandlers(t *testing.T) {
	got := writeHandlerRoutes(t, false)
	for _, r := range r2WriteRoutes {
		if got[r] {
			t.Errorf("readonly instance must not mount %s", r)
		}
	}
}
