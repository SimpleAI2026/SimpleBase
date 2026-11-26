// 文件说明：site_test.go — 站点请求 / 出口配额的计数、跨日重置与刷新测试。
//
// 用法
//
//	go test ./internal/usage -run TestSiteQuota
//
// 调用架构
//
//	[上游] 无（同包测试）
//	[本文件] internal/usage/site_test.go
//	[下游] service.go
//
// 主要符号
//
//	siteTestRepo / TestSiteQuotaRequests / TestSiteQuotaEgressAndFlush / TestSiteQuotaWithoutStore

package usage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

type siteTestRepo struct {
	catalog.Repository
	quota   catalog.ProjectQuota
	traffic map[string]catalog.SiteTraffic
}

func (r *siteTestRepo) GetQuota(context.Context, string) (catalog.ProjectQuota, error) {
	return r.quota, nil
}

func (r *siteTestRepo) IncrementSiteTraffic(_ context.Context, projectID, day string, requests, bytes int64) error {
	if r.traffic == nil {
		r.traffic = map[string]catalog.SiteTraffic{}
	}
	key := projectID + "/" + day
	cur := r.traffic[key]
	cur.Requests += requests
	cur.Bytes += bytes
	r.traffic[key] = cur
	return nil
}

func (r *siteTestRepo) GetSiteTraffic(_ context.Context, projectID, day string) (catalog.SiteTraffic, error) {
	return r.traffic[projectID+"/"+day], nil
}

func TestSiteQuotaRequests(t *testing.T) {
	repo := &siteTestRepo{quota: catalog.ProjectQuota{MaxSiteRequests: 2}}
	svc := NewService(repo, nil)
	svc.now = func() time.Time { return time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC) }
	ctx := context.Background()

	if err := svc.AllowSite(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AllowSite(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AllowSite(ctx, "p1"); !errors.Is(err, ErrSiteQuotaExceeded) {
		t.Fatalf("want ErrSiteQuotaExceeded, got %v", err)
	}
}

func TestSiteQuotaEgressAndFlush(t *testing.T) {
	repo := &siteTestRepo{quota: catalog.ProjectQuota{MaxSiteEgressBytes: 10}}
	svc := NewService(repo, nil)
	svc.now = func() time.Time { return time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC) }
	ctx := context.Background()

	if err := svc.AllowSite(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordSite(ctx, "p1", 20); err != nil {
		t.Fatal(err)
	}
	if err := svc.AllowSite(ctx, "p1"); !errors.Is(err, ErrSiteQuotaExceeded) {
		t.Fatalf("egress quota must reject, got %v", err)
	}
	if err := svc.FlushSite(ctx); err != nil {
		t.Fatal(err)
	}
	got := repo.traffic["p1/2026-11-26"]
	if got.Requests != 1 || got.Bytes != 20 {
		t.Fatalf("flushed traffic=%+v", got)
	}

	// 跨日后计数重置，请求重新允许。
	svc.now = func() time.Time { return time.Date(2026, 11, 27, 0, 5, 0, 0, time.UTC) }
	if err := svc.AllowSite(ctx, "p1"); err != nil {
		t.Fatalf("next day must reset counters: %v", err)
	}
}

func TestSiteQuotaWithoutStore(t *testing.T) {
	repo := &siteTestRepo{quota: catalog.ProjectQuota{MaxSiteRequests: 1}}
	svc := NewService(noSiteStore{repo}, nil)
	svc.now = func() time.Time { return time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC) }
	ctx := context.Background()
	if err := svc.AllowSite(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordSite(ctx, "p1", 10); err != nil {
		t.Fatal(err)
	}
	if err := svc.FlushSite(ctx); err != nil {
		t.Fatal(err)
	}
}

// noSiteStore 只暴露 Repository，不实现 SiteTrafficStore。
type noSiteStore struct {
	catalog.Repository
}

func (s noSiteStore) GetQuota(context.Context, string) (catalog.ProjectQuota, error) {
	return catalog.ProjectQuota{MaxSiteRequests: 1}, nil
}
