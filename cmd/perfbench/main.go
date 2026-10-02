// perfbench 是独立的压测入口二进制（api-db-perf-validation-plan §2.2）。
//
// 不进生产构建产物：装配真实 app（DevMode 本地盘）+ httptest.Server，
// 按 http/db/kv 三组对 DB 的 HTTP 增删改查做闭环压测并输出 p50/p95/p99
// 与九段耗时占比。原始数据落 output/perf/<date>/。
//
// 用法：
//
//	go run ./cmd/perfbench -suite all            # 全量
//	go run ./cmd/perfbench -suite http           # HTTP 链路/管理面
//	go run ./cmd/perfbench -suite db             # SQL + Data 文档 CRUD
//	go run ./cmd/perfbench -suite kv             # 项目 KV CRUD
//	go run ./cmd/perfbench -suite db -c 32 -n 2000 -d 30s
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/linkxzhou/SimpleBase/internal/api/perfbench"
)

func main() {
	var (
		suite   = flag.String("suite", "all", "压测套件: http | db | kv | all")
		conc    = flag.Int("c", 1, "并发 worker 数")
		n       = flag.Int("n", 500, "每用例总请求数")
		dur     = flag.Duration("d", 0, "最短持续时间（如 30s；0=只看请求数）")
		warmup  = flag.Int("warmup", 50, "预热请求数（不计入样本）")
		outRoot = flag.String("out", "output/perf", "原始数据输出根目录")
		cache   = flag.String("cache", "", "覆盖缓存目录（默认临时目录，压后清理）")
	)
	flag.Parse()

	if err := perfbench.Run(context.Background(), perfbench.RunOptions{
		Suite:       *suite,
		Concurrency: *conc,
		Samples:     *n,
		Duration:    *dur,
		Warmup:      *warmup,
		OutputRoot:  *outRoot,
		CacheRoot:   *cache,
		Stdout:      os.Stdout,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "perfbench: %v\n", err)
		os.Exit(1)
	}
}
