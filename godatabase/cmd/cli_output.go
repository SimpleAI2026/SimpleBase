// 文件说明：cli_output.go — godatabase/cmd 的内部实现（benchKey、benchLoop 等）
//
// 用法
//
//	同包调用：benchKey() → godb-cli.go；benchLoop() → godb-cli.go；percentile() → godb-cli.go
//
// 调用架构
//
//	[上游] 无（可执行入口）
//	[本文件] godatabase/cmd/cli_output.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] cli_env.go, godb-cli.go, godb-cli_test.go
//	[关键引用] benchKey ← godb-cli.go；benchLoop ← godb-cli.go；percentile ← godb-cli.go
//
// 主要符号
//
//	benchKey(i int) string
//	benchLoop(n int, fn func(i int) error) ([]time.Duration, error)
//	percentile(durs []time.Duration) (string, string)
//	ttlMs(d time.Duration) int64
//	ms(d time.Duration) float64
//	asInt(v any) int64
//	looksLikeQuery(q string) bool
//	sanitizeGuard(err error) error
//	... 另有 5 个顶层符号

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
)

// —— 输出与解析辅助（供各子命令共用）——

func benchKey(i int) string { return fmt.Sprintf("godb:bench:%06d", i) }

// benchLoop 逐次执行 fn 并记录耗时；失败返回错误由调用方统一脱敏输出，
// 不在 helper 内直接退出（保证 defer 清理与一致的错误格式）。
func benchLoop(n int, fn func(i int) error) ([]time.Duration, error) {
	durs := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		if err := fn(i); err != nil {
			return nil, err
		}
		durs = append(durs, time.Since(start))
	}
	return durs, nil
}

// percentile 计算一组耗时的 p50 与 p99（字符串形式）。
func percentile(durs []time.Duration) (string, string) {
	if len(durs) == 0 {
		return "n/a", "n/a"
	}
	sorted := append([]time.Duration(nil), durs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	p := func(q float64) time.Duration {
		idx := int(q * float64(len(sorted)-1))
		return sorted[idx]
	}
	return p(0.50).Round(time.Microsecond).String(), p(0.99).Round(time.Microsecond).String()
}

func ttlMs(d time.Duration) int64 { return d.Milliseconds() }

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func asInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case []byte:
		var n int64
		fmt.Sscanf(string(x), "%d", &n)
		return n
	case string:
		var n int64
		fmt.Sscanf(x, "%d", &n)
		return n
	}
	return 0
}

func looksLikeQuery(q string) bool {
	switch sqlguard.FirstKeyword(q) {
	case "SELECT", "WITH", "EXPLAIN", "SHOW", "DESCRIBE", "PRAGMA":
		return true
	}
	return false
}

func sanitizeGuard(err error) error {
	msg := strings.ReplaceAll(err.Error(), "\n", "; ")
	return errors.New(msg)
}

func tableListSQL(e env) string {
	if e.engine() == godatabase.EngineSQLite {
		return "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
	}
	return "SELECT table_name FROM information_schema.tables WHERE table_schema='main' ORDER BY table_name"
}

func tableNames(res godatabase.QueryResult) []string {
	names := make([]string, 0, len(res.Rows))
	for _, row := range res.Rows {
		for _, cell := range row {
			if s, ok := cell.(string); ok {
				names = append(names, s)
			}
		}
	}
	return names
}

func printTable(cols []string, rows [][]any) {
	if len(cols) == 0 {
		return
	}
	fmt.Println(strings.Join(cols, "\t"))
	for _, r := range rows {
		cells := make([]string, 0, len(r))
		for _, c := range r {
			cells = append(cells, fmtCell(c))
		}
		fmt.Println(strings.Join(cells, "\t"))
	}
}

func fmtCell(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
