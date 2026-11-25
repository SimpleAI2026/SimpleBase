// 文件说明：godb-cli.go — godatabase/cmd 的内部实现（usageText、main 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] 无（可执行入口）
//	[本文件] godatabase/cmd/godb-cli.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] cli_env.go, cli_output.go, godb-cli_test.go
//
// 主要符号
//
//	const usageText
//	main()
//	sanitize(err error) string
//	cmdOpen(ctx context.Context, args []string) error
//	cmdSQL(ctx context.Context, args []string) error
//	cmdKV(ctx context.Context, args []string) error
//	cmdSnapshot(ctx context.Context, args []string) error
//	cmdInspect(ctx context.Context, args []string) error
//	... 另有 1 个顶层符号

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/kv"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
)

const usageText = `godb — godatabase 本地运维 CLI（本地模式，不连 S3）

用法:
  godb open     --path <dir> --engine duckdb|sqlite --project <id> [--json]
  godb sql      --project <id> [--rw] -- "<sql>"
  godb kv       --project <id> get|set|del|scan [<key>] [--value v] [--ttl d] [--count n]
  godb snapshot --project <id> --out <file>
  godb inspect  --project <id>
  godb bench    --project <id> --ops <n>

项目 ID 规则: 恰好 8 位 [A-Za-z0-9-]（与线上一致）。
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var err error
	switch os.Args[1] {
	case "open":
		err = cmdOpen(ctx, os.Args[2:])
	case "sql":
		err = cmdSQL(ctx, os.Args[2:])
	case "kv":
		err = cmdKV(ctx, os.Args[2:])
	case "snapshot":
		err = cmdSnapshot(ctx, os.Args[2:])
	case "inspect":
		err = cmdInspect(ctx, os.Args[2:])
	case "bench":
		err = cmdBench(ctx, os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	if err != nil {
		// 错误脱敏：不输出任何文件路径。
		fmt.Fprintln(os.Stderr, "godb:", sanitize(err))
		os.Exit(1)
	}
}

// sanitize 去除错误文本中的本地路径（AGENTS.md 第 7 条：路径不进日志/输出）。
func sanitize(err error) string {
	msg := err.Error()
	for _, root := range dataRoots() {
		if root == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, root, "<data-root>")
	}
	if home, e := os.UserHomeDir(); e == nil {
		msg = strings.ReplaceAll(msg, home, "~")
	}
	return msg
}

// —— 子命令 ——

func cmdOpen(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	path := fs.String("path", "", "数据根目录（默认 $GODB_DATA_ROOT）")
	engine := fs.String("engine", godatabase.EngineDuckDB, "duckdb 或 sqlite")
	project := fs.String("project", "", "项目 ID（8 位）")
	jsonOut := fs.Bool("json", false, "JSON 输出")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		*path = os.Getenv("GODB_DATA_ROOT")
		if *path == "" {
			*path = fileDefaultRoot()
		}
	}
	if err := blob.ValidateProjectID(*project); err != nil {
		return errors.New("invalid project id (need 8 chars [A-Za-z0-9-])")
	}
	switch *engine {
	case godatabase.EngineDuckDB, godatabase.EngineSQLite:
	default:
		return errors.New("engine must be duckdb or sqlite")
	}
	os.Setenv("GODB_DATA_ROOT", *path)
	os.Setenv("GODB_ENGINE", *engine)
	e := parseEnv(*jsonOut)
	_, lease, cleanup, err := e.openReg(ctx, *project, true)
	if err != nil {
		return err
	}
	defer cleanup()

	h := lease.Handle
	fi, err := os.Stat(dbFilePath(e, h))
	var size int64
	if err == nil {
		size = fi.Size()
	}
	// 完整性检查（打开时 factory 已跑 quick_check，这里再读一遍表清单）。
	rows, err := h.Query(ctx, godatabase.Statement{SQL: "SELECT count(*) FROM information_schema.tables"}, 1)
	var tables int64
	if err == nil && len(rows.Rows) > 0 {
		tables = asInt(rows.Rows[0][0])
	}
	if *jsonOut {
		return printJSON(map[string]any{
			"project": *project, "engine": *engine,
			"size_bytes": size, "tables": tables,
		})
	}
	fmt.Printf("opened project=%s engine=%s size=%d tables=%d\n", *project, *engine, size, tables)
	return nil
}

func cmdSQL(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sql", flag.ContinueOnError)
	project := fs.String("project", "", "项目 ID")
	rw := fs.Bool("rw", false, "写模式（默认只读）")
	jsonOut := fs.Bool("json", false, "JSON 输出")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if *project == "" || len(rest) == 0 {
		return errors.New("usage: godb sql --project <id> [--rw] -- \"<sql>\"")
	}
	query := strings.TrimSpace(strings.Join(rest, " "))

	e := parseEnv(*jsonOut)
	intent := sqlguard.ReadOnly
	if *rw {
		intent = sqlguard.WriteAllowed
	}
	if err := sqlguard.Validate(query, intent); err != nil {
		return fmt.Errorf("sql rejected: %w", sanitizeGuard(err))
	}
	_, lease, cleanup, err := e.openReg(ctx, *project, *rw)
	if err != nil {
		return err
	}
	defer cleanup()
	h := lease.Handle
	if looksLikeQuery(query) {
		res, err := h.Query(ctx, godatabase.Statement{SQL: query}, 100)
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(map[string]any{
				"columns": res.Columns, "rows": res.Rows, "duration_ms": ms(res.Duration),
			})
		}
		printTable(res.Columns, res.Rows)
		fmt.Printf("(%d rows, %s)\n", len(res.Rows), res.Duration.Round(time.Microsecond))
		return nil
	}
	res, err := h.Execute(ctx, godatabase.Statement{SQL: query})
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]any{"rows_affected": res.RowsAffected, "duration_ms": ms(res.Duration)})
	}
	fmt.Printf("OK rows_affected=%d (%s)\n", res.RowsAffected, res.Duration.Round(time.Microsecond))
	return nil
}

func cmdKV(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("kv", flag.ContinueOnError)
	project := fs.String("project", "", "项目 ID")
	value := fs.String("value", "", "set 的值")
	ttl := fs.Duration("ttl", 0, "过期时长（如 30s / 5m）")
	count := fs.Int("count", 20, "scan 每页数量")
	jsonOut := fs.Bool("json", false, "JSON 输出")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if *project == "" || len(rest) < 1 {
		return errors.New("usage: godb kv --project <id> get|set|del|scan <key> [--value v] [--ttl d]")
	}
	op := rest[0]
	key := ""
	if len(rest) > 1 {
		key = rest[1]
	}

	rw := op == "set" || op == "del"
	e := parseEnv(*jsonOut)
	_, lease, cleanup, err := e.openReg(ctx, *project, rw)
	if err != nil {
		return err
	}
	defer cleanup()
	h := lease.Handle
	store := kv.New(h.Conn())

	switch op {
	case "get":
		if key == "" {
			return errors.New("get needs a key")
		}
		var val []byte
		err := store.View(ctx, func(tx *kv.Tx) error {
			v, err := tx.Str().Get(ctx, key)
			if err != nil && !errors.Is(err, kv.ErrNotFound) {
				return err
			}
			val = v
			return nil
		})
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(map[string]any{"key": key, "value": string(val)})
		}
		if val == nil {
			fmt.Printf("(nil)\n")
		} else {
			fmt.Printf("%s\n", string(val))
		}
	case "set":
		if key == "" {
			return errors.New("set needs a key")
		}
		var ttlms *int64
		if *ttl > 0 {
			ms := ttlMs(*ttl)
			ttlms = &ms
		}
		err := store.Update(ctx, func(tx *kv.Tx) error {
			_, err := tx.Str().SetWith(ctx, key, []byte(*value), kv.SetOptions{TTLms: ttlms})
			return err
		})
		if err != nil {
			return err
		}
		h.NotifyWrite(ctx)
		if *jsonOut {
			return printJSON(map[string]any{"key": key, "ok": true})
		}
		fmt.Println("OK")
	case "del":
		if key == "" {
			return errors.New("del needs a key")
		}
		var n int64
		err := store.Update(ctx, func(tx *kv.Tx) error {
			deleted, err := tx.Key().Delete(ctx, key)
			n = deleted
			return err
		})
		if err != nil {
			return err
		}
		h.NotifyWrite(ctx)
		if *jsonOut {
			return printJSON(map[string]any{"deleted": n})
		}
		fmt.Printf("OK deleted=%d\n", n)
	case "scan":
		var names []string
		var cursor string
		err := store.View(ctx, func(tx *kv.Tx) error {
			metas, next, err := tx.Key().Scan(ctx, "", "*", 0, *count)
			if err != nil {
				return err
			}
			cursor = next
			names = make([]string, 0, len(metas))
			for _, m := range metas {
				names = append(names, m.Key)
			}
			return nil
		})
		if err != nil {
			return err
		}
		sort.Strings(names)
		if *jsonOut {
			return printJSON(map[string]any{"keys": names, "next_cursor": cursor})
		}
		for _, n := range names {
			fmt.Println(n)
		}
		if cursor != "" {
			fmt.Printf("(cursor: %s)\n", cursor)
		}
	default:
		return fmt.Errorf("unknown kv op %q", op)
	}
	return nil
}

func cmdSnapshot(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	project := fs.String("project", "", "项目 ID")
	out := fs.String("out", "", "输出文件路径")
	jsonOut := fs.Bool("json", false, "JSON 输出")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *project == "" || *out == "" {
		return errors.New("usage: godb snapshot --project <id> --out <file>")
	}
	e := parseEnv(*jsonOut)
	spec, err := e.spec(*project)
	if err != nil {
		return err
	}
	_, lease, cleanup, err := e.openReg(ctx, *project, true)
	if err != nil {
		return err
	}
	defer cleanup()
	h := lease.Handle
	engine, _ := godatabase.EngineFromPath(spec.DatabasePath)
	info, err := filedb.SnapshotFile(ctx, h.Conn(), dbFilePath(e, h), engine, *out)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]any{
			"project": *project, "engine": engine,
			"size_bytes": info.SizeBytes, "sha256": info.SHA256,
			"completed_at": info.CompletedAt,
		})
	}
	fmt.Printf("snapshot project=%s engine=%s size=%d\nsha256=%s\n", *project, engine, info.SizeBytes, info.SHA256)
	return nil
}

func cmdInspect(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	project := fs.String("project", "", "项目 ID")
	jsonOut := fs.Bool("json", false, "JSON 输出")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *project == "" {
		return errors.New("usage: godb inspect --project <id>")
	}
	e := parseEnv(*jsonOut)
	_, lease, cleanup, err := e.openReg(ctx, *project, false)
	if err != nil {
		return err
	}
	defer cleanup()
	h := lease.Handle

	tables, _ := h.Query(ctx, godatabase.Statement{SQL: tableListSQL(e)}, 200)
	schemaReady, _ := kv.New(h.Conn()).HasSchema(ctx)
	out := map[string]any{
		"project":      *project,
		"engine":       e.engine(),
		"tables":       tableNames(tables),
		"kv_schema_ok": schemaReady,
	}
	if *jsonOut {
		return printJSON(out)
	}
	fmt.Printf("project=%s engine=%s kv_schema=%v\n", *project, e.engine(), schemaReady)
	fmt.Println("tables:")
	for _, n := range out["tables"].([]string) {
		fmt.Println("  -", n)
	}
	return nil
}

func cmdBench(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	project := fs.String("project", "", "项目 ID")
	ops := fs.Int("ops", 1000, "每项操作次数")
	jsonOut := fs.Bool("json", false, "JSON 输出")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *project == "" {
		return errors.New("usage: godb bench --project <id> --ops <n>")
	}
	if *ops <= 0 {
		return fmt.Errorf("godb: bench ops must be positive, got %d", *ops)
	}
	e := parseEnv(*jsonOut)
	_, lease, cleanup, err := e.openReg(ctx, *project, true)
	if err != nil {
		return err
	}
	defer cleanup()
	h := lease.Handle
	store := kv.New(h.Conn())
	ctx2 := ctx

	// SET（走事务闭包，一次事务写一个 key）
	setDur, err := benchLoop(*ops, func(i int) error {
		return store.Update(ctx2, func(tx *kv.Tx) error {
			_, err := tx.Str().SetWith(ctx2, benchKey(i), []byte(fmt.Sprintf("v%d", i)), kv.SetOptions{})
			return err
		})
	})
	if err != nil {
		return err
	}
	// GET
	getDur, err := benchLoop(*ops, func(i int) error {
		return store.View(ctx2, func(tx *kv.Tx) error {
			_, err := tx.Str().Get(ctx2, benchKey(i))
			if err != nil && !errors.Is(err, kv.ErrNotFound) {
				return err
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	// 批量 EXISTS / DEL：一次调用覆盖全部 ops 个 key（K3：单条 IN 查询）
	keys := make([]string, *ops)
	for i := range keys {
		keys[i] = benchKey(i)
	}
	existsDur, err := benchLoop(1, func(int) error {
		return store.View(ctx2, func(tx *kv.Tx) error {
			_, err := tx.Key().Exists(ctx2, keys...)
			return err
		})
	})
	if err != nil {
		return err
	}
	delDur, err := benchLoop(1, func(int) error {
		return store.Update(ctx2, func(tx *kv.Tx) error {
			_, err := tx.Key().Delete(ctx2, keys...)
			return err
		})
	})
	if err != nil {
		return err
	}
	h.NotifyWrite(ctx)
	setP50, setP99 := percentile(setDur)
	getP50, getP99 := percentile(getDur)
	if *jsonOut {
		return printJSON(map[string]any{
			"ops":     *ops,
			"set_p50": setP50, "set_p99": setP99,
			"get_p50": getP50, "get_p99": getP99,
			"exists_all": existsDur[0], "del_all": delDur[0],
		})
	}
	fmt.Printf("ops=%d\nset: p50=%s p99=%s\nget: p50=%s p99=%s\nexists(%d keys): %s\ndel(%d keys): %s\n",
		*ops, setP50, setP99, getP50, getP99, *ops, existsDur[0], *ops, delDur[0])
	return nil
}
