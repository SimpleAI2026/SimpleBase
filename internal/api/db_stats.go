// 文件说明：db_stats.go — 统计单库用户表总行数（数据库列表「数据量」列）
//
// 用法
//
//	CountDatabaseRows(ctx context.Context, svc SQLService, db catalog.Database) (int64 ...  // CountDatabaseRows 统计库内全部 BASE TABLE 的行数之和
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/db_stats.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, internal/catalog
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//
// 主要符号
//
//	CountDatabaseRows(ctx context.Context, svc SQLService, db catalog.Database) (int64 ...
//	... 另有 5 个顶层符号
//	db_stats.go 统计单库用户表总行数（数据库列表「数据量」列）。

package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// CountDatabaseRows 统计库内全部 BASE TABLE 的行数之和。
// 只读租约；单表 COUNT(*) 逐个累加，表数上限 maxStatTables 防失控。
// 系统库走 systemLeaseAdapter 只读桥接，同样适用。
func CountDatabaseRows(ctx context.Context, svc SQLService, db catalog.Database) (int64, error) {
	lease, err := svc.Acquire(ctx, db, godatabase.ReadOnly)
	if err != nil {
		return 0, err
	}
	defer lease.Release()

	dialect, err := dialectForDatabase(db)
	if err != nil {
		return 0, err
	}
	tables, err := listTableNames(ctx, lease, dialect)
	if err != nil {
		return 0, err
	}
	if len(tables) == 0 {
		return 0, nil
	}
	if len(tables) > maxStatTables {
		tables = tables[:maxStatTables]
	}

	var total int64
	for _, name := range tables {
		n, err := countTableRowsWithDialect(ctx, lease, dialect, name)
		if err != nil {
			// 单表失败不拖垮整库统计（表可能刚删）。
			continue
		}
		total += n
	}
	return total, nil
}

const maxStatTables = 200

func listTableNames(ctx context.Context, lease SQLLease, dialect godatabase.Dialect) ([]string, error) {
	result, err := lease.Query(ctx, godatabase.Statement{
		SQL: dialect.TableColumnsSQL(""),
	}, maxStatTables*8)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(result.Rows))
	out := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) == 0 {
			continue
		}
		name, ok := row[0].(string)
		if !ok || name == "" || dialect.IsInternalTable(name) {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out, nil
}

func countTableRowsWithDialect(ctx context.Context, lease SQLLease, dialect godatabase.Dialect, table string) (int64, error) {
	result, err := lease.Query(ctx, godatabase.Statement{SQL: dialect.CountRowsSQL(table)}, 1)
	if err != nil {
		return 0, err
	}
	if len(result.Rows) == 0 || len(result.Rows[0]) == 0 {
		return 0, nil
	}
	return toInt64(result.Rows[0][0]), nil
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case string:
		s := strings.TrimSpace(n)
		s = strings.TrimSuffix(s, " rows") // DuckDB 有时回显 "N rows"
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

var _ = fmt.Sprintf
