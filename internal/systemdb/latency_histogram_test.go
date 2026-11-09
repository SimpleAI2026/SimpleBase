package systemdb

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

func TestLatencyHistogramBucketsAndQuantiles(t *testing.T) {
	h := newLatencyHistogram()
	if h.quantile(0.5) != nil {
		t.Fatal("empty histogram must return nil quantile")
	}
	// 左开右闭：1ms 落第 0 桶，1.0001ms 落第 1 桶；<1ms 与负值/NaN 落第 0 桶。
	for _, v := range []float64{0.2, 1, -3, math.NaN()} {
		h.observe(v)
	}
	h.observe(1.0001)
	h.observe(30000)
	h.observe(30001)
	if h[0] != 4 || h[1] != 1 || h[len(latencyBucketsV1)-1] != 1 || h[len(latencyBucketsV1)] != 1 {
		t.Fatalf("bucket placement: %v", h)
	}
	if got := *h.quantile(1); got != LatencyOverflowMS {
		t.Fatalf("overflow quantile = %v", got)
	}
}

// 1000 个已知样本：误差不得超过命中桶宽度。
func TestLatencyHistogramKnownDistribution(t *testing.T) {
	h := newLatencyHistogram()
	samples := make([]float64, 0, 1000)
	for i := 1; i <= 1000; i++ {
		v := float64(i) // 1..1000ms 均匀
		samples = append(samples, v)
		h.observe(v)
	}
	for _, tc := range []struct {
		q     float64
		exact float64
	}{{0.5, 500}, {0.9, 900}, {0.99, 990}} {
		got := *h.quantile(tc.q)
		lower, upper := bucketBounds(tc.exact)
		if got < lower || got > upper {
			t.Fatalf("p%v=%v outside bucket (%v,%v]", tc.q*100, got, lower, upper)
		}
		if math.Abs(got-tc.exact) > upper-lower {
			t.Fatalf("p%v=%v exact=%v error exceeds bucket width", tc.q*100, got, tc.exact)
		}
	}
	_ = samples
}

func bucketBounds(v float64) (float64, float64) {
	lower := 0.0
	for _, b := range latencyBucketsV1 {
		if v <= b {
			return lower, b
		}
		lower = b
	}
	return lower, math.Inf(1)
}

func TestDecodeLatencyHistogramValidation(t *testing.T) {
	h := newLatencyHistogram()
	h.observe(3)
	if _, err := decodeLatencyHistogram(h.encode(), 1); err != nil {
		t.Fatalf("valid row: %v", err)
	}
	bad := []struct {
		labels string
		total  float64
	}{
		{"not json", 1},
		{`{"v":2,"b":[1]}`, 1},
		{`{"v":1,"b":[1,2]}`, 3},
		{h.encode(), 2},
		{h.encode(), math.NaN()},
	}
	for _, b := range bad {
		if _, err := decodeLatencyHistogram(b.labels, b.total); !errors.Is(err, errBadLatencyHistogram) {
			t.Fatalf("labels=%q total=%v: want invalid, got %v", b.labels, b.total, err)
		}
	}
}

func TestMetricsSummaryLatencyPercentiles(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	empty, err := s.MetricsSummary(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if empty.LatencyP50MS != nil || empty.LatencyP99MS != nil || empty.LatencySampleCount != 0 {
		t.Fatalf("empty window must return nil percentiles: %+v", empty)
	}

	// 并发写入 + 跨两次 flush：p1 均匀 1..100ms，p2 全部 5000ms。
	var wg sync.WaitGroup
	for i := 1; i <= 100; i++ {
		wg.Add(1)
		go func(v float64) {
			defer wg.Done()
			s.RecordMetric(MetricSample{ProjectID: "p1", Name: "http_latency_ms", Value: v})
		}(float64(i))
		if i == 50 {
			wg.Wait()
			if err := s.FlushMetrics(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	wg.Wait()
	for i := 0; i < 10; i++ {
		s.RecordMetric(MetricSample{ProjectID: "p2", Name: "http_latency_ms", Value: 5000})
	}
	if err := s.FlushMetrics(ctx); err != nil {
		t.Fatal(err)
	}

	p1, err := s.MetricsSummary(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if p1.LatencySampleCount != 100 || p1.LatencyP50MS == nil {
		t.Fatalf("p1 summary: %+v", p1)
	}
	if *p1.LatencyP50MS <= 20 || *p1.LatencyP50MS > 50 || *p1.LatencyP99MS <= 50 || *p1.LatencyP99MS > 100 {
		t.Fatalf("p1 percentiles p50=%v p99=%v", *p1.LatencyP50MS, *p1.LatencyP99MS)
	}
	if math.Abs(p1.AvgLatencyMS-50.5) > 1e-9 {
		t.Fatalf("weighted avg = %v, want 50.5", p1.AvgLatencyMS)
	}
	admin, err := s.MetricsSummary(ctx, catalog.ReservedSystemProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if admin.LatencySampleCount != 110 || *admin.LatencyP99MS <= 2000 || *admin.LatencyP99MS > 5000 {
		t.Fatalf("admin summary: %+v p99=%v", admin, *admin.LatencyP99MS)
	}
}

func TestMetricsSummarySkipsCorruptHistogramAndLegacyAvg(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Now().UTC()
	// 旧版逐请求样本：无 _count 行，回退 AVG。
	if _, err := s.db.ExecContext(ctx, `INSERT INTO sys_metric_samples(id, project_id, name, value_double, labels_json, occurred_at)
		VALUES (?, 'p1', 'http_latency_ms', 10, '', ?), (?, 'p1', 'http_latency_ms', 30, '', ?),
		       (?, 'p1', 'http_latency_histogram_ms', 1, 'broken', ?)`,
		uuid.NewString(), now, uuid.NewString(), now, uuid.NewString(), now); err != nil {
		t.Fatal(err)
	}
	sum, err := s.MetricsSummary(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if sum.AvgLatencyMS != 20 || sum.LatencySampleCount != 0 || sum.LatencyP50MS != nil {
		t.Fatalf("legacy/corrupt summary: %+v", sum)
	}
}

func TestLatencyHistogramRequeuedOnFlushFailure(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	s.RecordMetric(MetricSample{ProjectID: "p1", Name: "http_latency_ms", Value: 7})
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE sys_metric_samples RENAME TO sys_metric_samples_bak`); err != nil {
		t.Skipf("rename unsupported: %v", err)
	}
	if err := s.FlushMetrics(ctx); err == nil {
		t.Fatal("flush into missing table must fail")
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE sys_metric_samples_bak RENAME TO sys_metric_samples`); err != nil {
		t.Fatal(err)
	}
	if err := s.FlushMetrics(ctx); err != nil {
		t.Fatal(err)
	}
	sum, err := s.MetricsSummary(ctx, "p1")
	if err != nil || sum.LatencySampleCount != 1 {
		t.Fatalf("requeued histogram lost: %+v err=%v", sum, err)
	}
}

func TestMetricsSummaryReturnsQueryError(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.db.Exec(`DROP TABLE sys_metric_samples`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MetricsSummary(context.Background(), "p1"); err == nil {
		t.Fatal("summary must surface query errors instead of returning zeros")
	}
}
