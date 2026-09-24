// db_stats.go 统计单库用户表总行数（数据库列表「数据量」列）。
package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
)

// CountDatabaseRows 统计库内全部 BASE TABLE 的行数之和。
// 只读租约；单表 COUNT(*) 逐个累加，表数上限 maxStatTables 防失控。
// 系统库走 systemLeaseAdapter 只读桥接，同样适用。
func CountDatabaseRows(ctx context.Context, svc SQLService, db catalog.Database) (int64, error) {
	lease, err := svc.Acquire(ctx, db, database.ReadOnly)
	if err != nil {
		return 0, err
	}
	defer lease.Release()

	tables, err := listTableNames(ctx, lease)
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
		n, err := countTableRows(ctx, lease, name)
		if err != nil {
			// 单表失败不拖垮整库统计（表可能刚删）。
			continue
		}
		total += n
	}
	return total, nil
}

const maxStatTables = 200

func listTableNames(ctx context.Context, lease SQLLease) ([]string, error) {
	result, err := lease.Query(ctx, database.Statement{
		SQL: `SELECT table_name FROM information_schema.tables
			WHERE table_catalog = current_database()
			  AND table_schema = current_schema()
			  AND table_type = 'BASE TABLE'
			ORDER BY table_name`,
	}, maxStatTables)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) > 0 {
			if name, ok := row[0].(string); ok && name != "" {
				out = append(out, name)
			}
		}
	}
	return out, nil
}

func countTableRows(ctx context.Context, lease SQLLease, table string) (int64, error) {
	// quoteIdentifier 来自 data_handler.go，防标识符注入。
	result, err := lease.Query(ctx, database.Statement{
		SQL: "SELECT COUNT(*) FROM " + quoteIdentifier(table),
	}, 1)
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
