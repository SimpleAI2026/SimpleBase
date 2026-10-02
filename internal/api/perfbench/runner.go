// runner.go 串起环境装配 → 用例执行 → 报告落盘（cmd/perfbench 的内核）。
package perfbench

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"
)

// RunOptions 是一次 perfbench 运行的全部参数（对应 build.sh perf 参数）。
type RunOptions struct {
	Suite       string        // http | db | kv | all
	Concurrency int           // 并发 worker 数
	Samples     int           // 每用例总请求数
	Duration    time.Duration // 最短持续时间（0 = 只看样本数）
	Warmup      int           // 预热请求数
	OutputRoot  string        // 原始数据目录（默认 output/perf）
	Stdout      io.Writer
	// CacheRoot 覆盖缓存目录（默认临时目录；传入时压测后不清理，便于排查）。
	CacheRoot string
}

// Run 执行选定套件并输出报告。返回非零 exit code 语义的错误。
func Run(ctx context.Context, opt RunOptions) error {
	if opt.Stdout == nil {
		opt.Stdout = os.Stdout
	}
	suite := opt.Suite
	if suite == "" {
		suite = "all"
	}

	cacheRoot := opt.CacheRoot
	cleanup := true
	if cacheRoot == "" {
		dir, err := os.MkdirTemp("", "simplebase-perfbench-")
		if err != nil {
			return fmt.Errorf("perfbench: temp dir: %w", err)
		}
		cacheRoot = dir
	} else {
		cleanup = false
	}
	if cleanup {
		defer os.RemoveAll(cacheRoot)
	}

	env, err := newEnv(ctx, cacheRoot)
	if err != nil {
		return err
	}
	defer env.Close()

	outDir, err := ensureOutputDir(opt.OutputRoot)
	if err != nil {
		return fmt.Errorf("perfbench: output dir: %w", err)
	}
	rc := &reportCtx{OutDir: outDir, Stdout: opt.Stdout}

	// 报告头部：环境信息（plan §2.4.3）。
	fmt.Fprintf(opt.Stdout, "perfbench suite=%s concurrency=%d samples=%d duration=%s warmup=%d\n",
		suite, opt.Concurrency, opt.Samples, opt.Duration, opt.Warmup)
	fmt.Fprintf(opt.Stdout, "env: go=%s os=%s arch=%s cpu=%d cache=%s\n",
		runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), cacheRoot)
	fmt.Fprintf(opt.Stdout, "output: %s\n", outDir)
	fmt.Fprintln(opt.Stdout, "")

	cases, err := selectCases(suite)
	if err != nil {
		return err
	}
	label := fmt.Sprintf("c%d_n%d", opt.Concurrency, opt.Samples)

	var failed int
	for _, bc := range cases {
		// 每条用例独立计时；失败不中断整体（错误率即结果的一部分）。
		res, err := runCase(env, bc, benchOptions{
			Concurrency: opt.Concurrency,
			Samples:     opt.Samples,
			Warmup:      opt.Warmup,
			MinDur:      opt.Duration,
			Label:       label,
		})
		if err != nil {
			fmt.Fprintf(opt.Stdout, "[%-14s] SETUP-FAIL: %v\n", bc.ID, err)
			failed++
			continue
		}
		res.RunEnv = map[string]string{
			"go":   runtime.Version(),
			"os":   runtime.GOOS + "/" + runtime.GOARCH,
			"cpus": fmt.Sprintf("%d", runtime.NumCPU()),
		}
		if err := rc.writeResult(res); err != nil {
			return fmt.Errorf("perfbench: write result %s: %w", bc.ID, err)
		}
	}

	fmt.Fprintln(opt.Stdout, "")
	fmt.Fprintf(opt.Stdout, "done: %d case(s) run, %d setup-failed; raw data: %s\n",
		len(cases)-failed, failed, outDir)
	return nil
}

// selectCases 按套件名聚合用例。
func selectCases(suite string) ([]benchCase, error) {
	var cases []benchCase
	add := func(cs []benchCase) { cases = append(cases, cs...) }
	switch suite {
	case "http":
		add(httpCases())
	case "db":
		add(dbCases())
	case "kv":
		add(kvCases())
	case "all":
		add(httpCases())
		add(dbCases())
		add(kvCases())
	default:
		return nil, fmt.Errorf("perfbench: unknown suite %q (want http|db|kv|all)", suite)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("perfbench: suite %q has no cases", suite)
	}
	return cases, nil
}
