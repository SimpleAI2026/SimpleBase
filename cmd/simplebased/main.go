package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/app"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run 分派子命令：无参数启动服务；`reset` 清空实例数据（不启动 HTTP、不打开数据库）。
func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "simplebased: %v\n", err)
		return 1
	}

	if len(args) > 0 {
		switch args[0] {
		case "reset":
			return runReset(cfg, args[1:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "simplebased: unknown command %q (supported: reset)\n", args[0])
			return 2
		}
	}

	a, err := app.New(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(stderr, "simplebased: %v\n", err)
		return 1
	}

	timeout := cfg.HTTP.ShutdownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if err := a.RunWithSignal(timeout); err != nil {
		fmt.Fprintf(stderr, "simplebased: %v\n", err)
		return 1
	}
	return 0
}

// runReset 实现 `simplebased reset --confirm=<instance_id> [--local-only] [--dry-run]`
// （ducklake-duckdb-catalog-plan §5）。
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
		fmt.Fprintf(stderr, "simplebased reset: %v\n", err)
		return 1
	}
	if !*dryRun {
		fmt.Fprintf(stdout, "simplebased reset: ok (local=%d remote=%d)\n", len(sum.LocalPaths), sum.RemoteObjects)
	}
	return 0
}
