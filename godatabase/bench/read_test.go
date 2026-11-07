// read_test.go 复现并量化「系统库读路径慢」的核心机制：读放大。
//
// 每个 Benchmark 都成对出现：Dense（inlining=0，每次写入一个小文件，最坏情况）
// vs Inlined（inlining=1000，当前配置的内联阈值）。两者比较可观察本地盘上
// 「小文件堆积」对读延迟的影响；不能外推远端网络耗时。
//
// 运行（在 godatabase 模块根执行）：
//
//	GOWORK=off go test -run='^$' -bench=. -benchtime=20x -timeout 20m ./bench
//	GOWORK=off go test -run='TestReadAmplificationReport' -v ./bench   # 打印文件数与耗时对照
package bench

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

// BenchmarkLogsQueryDense 复现 /v1/projects/:id/logs 的读放大。
// 每轮先按 flushes 参数堆积小文件，再测一次日志查询。
func BenchmarkLogsQueryDense(b *testing.B) {
	for _, flushes := range []int{10, 50, 150} {
		b.Run(name("dense", flushes), func(b *testing.B) {
			db := newBenchDB(b, storeOpts{Inlining: 0})
			seedLogFlushes(b, db, benchProject, flushes)
			files, bytes := liveDataFiles(b, db)
			b.ReportMetric(float64(files), "files")
			b.ReportMetric(float64(bytes)/1024, "KB")
			ctx := context.Background()
			b.ResetTimer()
			for b.Loop() {
				if _, err := queryLogs(ctx, db, benchProject, 200); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLogsQueryInlined 是同一查询在内联形态（本地开发）下的基线。
func BenchmarkLogsQueryInlined(b *testing.B) {
	db := newBenchDB(b, storeOpts{Inlining: 1000})
	seedLogFlushes(b, db, benchProject, 150)
	files, bytes := liveDataFiles(b, db)
	b.ReportMetric(float64(files), "files")
	b.ReportMetric(float64(bytes)/1024, "KB")
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		if _, err := queryLogs(ctx, db, benchProject, 200); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMetricsSummaryDense 复现 /metrics/summary 的条件聚合读放大。
func BenchmarkMetricsSummaryDense(b *testing.B) {
	for _, flushes := range []int{10, 50, 120} {
		b.Run(name("dense", flushes), func(b *testing.B) {
			db := newBenchDB(b, storeOpts{Inlining: 0})
			seedMetricFlushes(b, db, benchProject, flushes)
			files, _ := liveDataFiles(b, db)
			b.ReportMetric(float64(files), "files")
			ctx := context.Background()
			b.ResetTimer()
			for b.Loop() {
				if err := metricsSummary(ctx, db, benchProject); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMetricsSummaryInlined 内联形态基线。
func BenchmarkMetricsSummaryInlined(b *testing.B) {
	db := newBenchDB(b, storeOpts{Inlining: 1000})
	seedMetricFlushes(b, db, benchProject, 120)
	files, _ := liveDataFiles(b, db)
	b.ReportMetric(float64(files), "files")
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		if err := metricsSummary(ctx, db, benchProject); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUserPointLookupDense 复现 auth 每请求的 sys_users 点查读放大。
func BenchmarkUserPointLookupDense(b *testing.B) {
	for _, users := range []int{1, 20, 60} {
		b.Run(name("dense", users), func(b *testing.B) {
			db := newBenchDB(b, storeOpts{Inlining: 0})
			seedUsers(b, db, users)
			files, _ := liveDataFiles(b, db)
			b.ReportMetric(float64(files), "files")
			ctx := context.Background()
			b.ResetTimer()
			for b.Loop() {
				if err := userByID(ctx, db, "user-0000"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSingleWriteCost 量化「一次写入 = 一个新文件 + 一个新快照」的写放大。
// 指标按建表后的基线做增量，避免把初始化开销算进每次写入。
func BenchmarkSingleWriteCost(b *testing.B) {
	for _, inlining := range []int{0, 1000} {
		b.Run(name("inlining", inlining), func(b *testing.B) {
			db := newBenchDB(b, storeOpts{Inlining: inlining})
			baseFiles, baseBytes := liveDataFiles(b, db)
			baseSnaps := snapshotCount(b, db)
			b.ResetTimer()
			for b.Loop() {
				insertLog(b, db, benchProject, "x", time.Now())
			}
			b.StopTimer()
			files, bytes := liveDataFiles(b, db)
			n := float64(b.N)
			b.ReportMetric(float64(files-baseFiles)/n, "newfiles/write")
			b.ReportMetric(float64(bytes-baseBytes)/n, "newbytes/write")
			b.ReportMetric(float64(snapshotCount(b, db)-baseSnaps)/n, "newsnaps/write")
		})
	}
}

// TestReadAmplificationReport 打印「存活文件数 → 查询耗时」对照表，
// 作为结论的可复现证据（不参与 -bench 运行）。
func TestReadAmplificationReport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping read amplification report in -short mode")
	}
	ctx := context.Background()
	for _, flushes := range []int{1, 10, 50, 150} {
		db := newBenchDB(t, storeOpts{Inlining: 0})
		seedLogFlushes(t, db, benchProject, flushes)
		files, bytes := liveDataFiles(t, db)
		q := timeIt(t, 20, func() {
			if _, err := queryLogs(ctx, db, benchProject, 200); err != nil {
				t.Fatal(err)
			}
		})
		m := timeIt(t, 20, func() {
			if err := metricsSummary(ctx, db, benchProject); err != nil {
				t.Fatal(err)
			}
		})
		t.Logf("logs=%4d live_files=%4d catalog=%5.1fKB  QueryLogs=%7.3fms  MetricsSummary=%7.3fms",
			flushes, files, float64(bytes)/1024, q, m)
	}
}

// TestReadLatencyPhases 对同一临时库分别报告首次读取与热读分位数。
// 首读仅指造数完成后首次调用，不代表远端 S3 冷缓存。
func TestReadLatencyPhases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping local read latency report in -short mode")
	}
	for _, inlining := range []int{0, 1000} {
		db := newBenchDB(t, storeOpts{Inlining: inlining})
		seedLogFlushes(t, db, benchProject, 20)
		files, _ := liveDataFiles(t, db)
		targetFiles := liveTableFiles(t, db, "sys_log_events")
		ctx := context.Background()
		query := func() error {
			_, err := queryLogs(ctx, db, benchProject, 200)
			return err
		}
		start := time.Now()
		if err := query(); err != nil {
			t.Fatal(err)
		}
		first := time.Since(start)
		const samples = 100
		durations := make([]time.Duration, 0, samples)
		for i := 0; i < samples; i++ {
			start = time.Now()
			if err := query(); err != nil {
				t.Fatal(err)
			}
			durations = append(durations, time.Since(start))
		}
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		t.Logf("inlining=%d live_files_all_tables=%d live_files_sys_log_events=%d local_first=%s local_hot_p50=%s local_hot_p95=%s local_hot_p99=%s successful_samples=%d",
			inlining, files, targetFiles, first, durations[49], durations[94], durations[98], samples)
		for _, concurrency := range []int{1, 8, 32} {
			const total = 128
			var wg sync.WaitGroup
			var mu sync.Mutex
			parallelDurations := make([]time.Duration, 0, total)
			failures := 0
			jobs := make(chan struct{}, total)
			for i := 0; i < total; i++ {
				jobs <- struct{}{}
			}
			close(jobs)
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range jobs {
						began := time.Now()
						err := query()
						d := time.Since(began)
						mu.Lock()
						if err != nil {
							failures++
						} else {
							parallelDurations = append(parallelDurations, d)
						}
						mu.Unlock()
					}
				}()
			}
			wg.Wait()
			sort.Slice(parallelDurations, func(i, j int) bool { return parallelDurations[i] < parallelDurations[j] })
			if failures > 0 || len(parallelDurations) != total {
				t.Fatalf("concurrency=%d successes=%d failures=%d", concurrency, len(parallelDurations), failures)
			}
			t.Logf("inlining=%d concurrency=%d table_files=%d success=%d failures=%d local_p50=%s local_p95=%s local_p99=%s",
				inlining, concurrency, targetFiles, len(parallelDurations), failures,
				parallelDurations[total/2-1], parallelDurations[total*95/100-1], parallelDurations[total*99/100-1])
		}
	}
}

func timeIt(tb testing.TB, n int, fn func()) float64 {
	tb.Helper()
	// 预热，避免首轮元数据加载计入。
	fn()
	start := time.Now()
	for i := 0; i < n; i++ {
		fn()
	}
	return float64(time.Since(start).Microseconds()) / 1000 / float64(n)
}

func name(prefix string, v int) string {
	return prefix + "-" + strconv.Itoa(v)
}
