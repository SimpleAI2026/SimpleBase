package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/app"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/sbcli"
)

// run 无参数启动服务，reset 重置实例，doctor 自检，其余命令交给资源 CLI。
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "doctor" {
		return runDoctor(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] != "reset" {
		return sbcli.RunIO(context.Background(), args, os.Environ(), stdout, stderr, os.Stdin)
	}

	cfg, err := config.Load()
	if err != nil {
		// 配置阶段失败时结构化 logger 还不存在（级别/出口都来自配置），
		// 这里用纯文本兜底，才能解释「容器里一行日志都没有」。
		logBootEvent(stderr, "config load failed", "config_path", config.ConfigFilePath(), "err", err.Error())
		fmt.Fprintf(stderr, "simplebase: %v\n", err)
		return 1
	}
	if len(args) > 0 {
		return runReset(cfg, args[1:], stdout, stderr)
	}

	logBootEvent(stderr, "bootstrapping",
		"config_path", config.ConfigFilePath(),
		"instance_id", cfg.Instance.ID,
		"profile", cfg.ProfileName(),
		"log_level", cfg.Observability.LogLevel,
		"http_address", cfg.HTTP.Address,
	)

	a, err := app.New(context.Background(), cfg)
	if err != nil {
		logBootEvent(stderr, "app assembly failed", "err", err.Error())
		fmt.Fprintf(stderr, "simplebase: %v\n", err)
		return 1
	}

	timeout := cfg.HTTP.ShutdownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if err := a.RunWithSignal(timeout); err != nil {
		logBootEvent(stderr, "service exited with error", "err", err.Error())
		fmt.Fprintf(stderr, "simplebase: %v\n", err)
		return 1
	}
	return 0
}

// logBootEvent 在 logger 可用之前向 stderr 输出一行 key=value。
// 只输出非密钥字段：这是启动早期唯一的观测手段（配置加载失败时尤其重要）。
func logBootEvent(w io.Writer, msg string, kv ...string) {
	var b strings.Builder
	b.WriteString("simplebase startup: ")
	b.WriteString(msg)
	for i := 0; i+1 < len(kv); i += 2 {
		b.WriteString(" ")
		b.WriteString(kv[i])
		b.WriteString("=")
		b.WriteString(kv[i+1])
	}
	fmt.Fprintln(w, b.String())
}

// runReset 实现 `simplebase reset --confirm=<instance_id> [--local-only] [--dry-run]`。
func runReset(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	fs.SetOutput(stderr)
	confirm := fs.String("confirm", "", "must equal instance.id; wipes ALL databases of this instance (irreversible)")
	localOnly := fs.Bool("local-only", false, "only wipe local cache_dir (remote already wiped by operator)")
	dryRun := fs.Bool("dry-run", false, "list what would be deleted and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	sum, err := app.Reset(context.Background(), cfg, app.ResetOptions{
		Confirm:   *confirm,
		LocalOnly: *localOnly,
		DryRun:    *dryRun,
		Out:       stdout,
	})
	if err != nil {
		fmt.Fprintf(stderr, "simplebase reset: %v\n", err)
		return 1
	}
	if !*dryRun {
		fmt.Fprintf(stdout, "simplebase reset: ok (local=%d remote=%d)\n", len(sum.LocalPaths), sum.RemoteObjects)
	}
	return 0
}

// runDoctor 实现 `simplebase doctor [--json]`（production-readiness-plan §8.3.6）：
// 加载配置 → 列出启动前自检结果（密钥/观测收口/磁盘水位/备份窗口）。
// 不启动 HTTP 服务、不触碰系统库——部署后跑一次验证环境。
func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "output machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "simplebase doctor: config invalid: %v\n", err)
		return 1
	}
	report := app.RunDoctor(cfg)
	if *jsonOut {
		fmt.Fprintf(stdout, "%s\n", report.JSON())
	} else {
		fmt.Fprint(stdout, report.Text())
	}
	if !report.OK() {
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
