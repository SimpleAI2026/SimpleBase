package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Statement 是一条参数化 SQL 语句。调用方必须使用占位符参数，禁止拼接用户输入。
type Statement struct {
	SQL  string
	Args []any
}

// QueryResult 是 Query/Execute/Batch 的统一返回结构。
// Rows 中的每个元素是驱动返回的原生 Go 值（nil/int64/float64/bool/string/[]byte/time.Time）；
// 面向 HTTP 的 JSON 安全转换由上层 serialize（plan6）负责，本包不做协议假设。
type QueryResult struct {
	Columns      []string
	Rows         [][]any
	RowsAffected int64
	// LastInsertID 在 DuckDB/DuckLake 下恒为 0（无 last_insert_rowid，且不支持 sequences）。
	// API 层已废弃该字段；请使用 RETURNING 或应用侧 UUID。
	LastInsertID int64
	Duration     time.Duration
}

// Query 在给定 *sql.DB 上执行只读查询，最多扫描 maxRows 行。
// 超过 maxRows 时停止扫描并返回 ErrRowLimitExceeded（不静默截断）。
// maxRows <= 0 表示不限制（调用方应在更上层强制配置的默认上限）。
func Query(ctx context.Context, db *sql.DB, stmt Statement, maxRows int) (QueryResult, error) {
	start := time.Now()
	rows, err := db.QueryContext(ctx, stmt.SQL, stmt.Args...)
	if err != nil {
		return QueryResult{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return QueryResult{}, err
	}

	out := make([][]any, 0, 16)
	for rows.Next() {
		if maxRows > 0 && len(out) >= maxRows {
			return QueryResult{}, ErrRowLimitExceeded
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return QueryResult{}, err
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return QueryResult{}, err
	}

	return QueryResult{
		Columns:  cols,
		Rows:     out,
		Duration: time.Since(start),
	}, nil
}

// Execute 在给定 *sql.DB 上执行单条写语句。不接受多语句字符串；
// 调用方需要执行多条语句时必须使用 Batch。
func Execute(ctx context.Context, db *sql.DB, stmt Statement) (QueryResult, error) {
	start := time.Now()
	res, err := db.ExecContext(ctx, stmt.SQL, stmt.Args...)
	if err != nil {
		return QueryResult{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		// 部分驱动/语句不支持 RowsAffected；不视为致命错误。
		affected = 0
	}
	return QueryResult{
		RowsAffected: affected,
		Duration:     time.Since(start),
	}, nil
}

// execer 抽象 *sql.DB 和 *sql.Tx 共有的 ExecContext 方法，便于 Batch 复用 Execute 逻辑。
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// executeOn 与 Execute 相同，但作用于任意 execer（*sql.DB 或 *sql.Tx）。
func executeOn(ctx context.Context, e execer, stmt Statement) (QueryResult, error) {
	start := time.Now()
	res, err := e.ExecContext(ctx, stmt.SQL, stmt.Args...)
	if err != nil {
		return QueryResult{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		affected = 0
	}
	return QueryResult{
		RowsAffected: affected,
		Duration:     time.Since(start),
	}, nil
}

// —— 批量执行（原 transaction.go）——

// Batch 依次执行多条语句。transactional=true 时使用单个事务，
// 任一失败立即 Rollback；transactional=false 时逐条独立执行，
// 某条失败不影响后续语句执行，但会在返回的 error 中标明失败位置。
//
// 事务不能跨 HTTP 请求悬挂：Batch 内部完成 BeginTx/Commit，调用方不得
// 持有跨请求的 *sql.Tx。
func Batch(ctx context.Context, db *sql.DB, stmts []Statement, transactional bool) ([]QueryResult, error) {
	if len(stmts) == 0 {
		return nil, nil
	}
	if transactional {
		return batchTx(ctx, db, stmts)
	}
	return batchIndependent(ctx, db, stmts)
}

func batchTx(ctx context.Context, db *sql.DB, stmts []Statement) ([]QueryResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	results := make([]QueryResult, 0, len(stmts))
	for i, stmt := range stmts {
		res, err := executeOn(ctx, tx, stmt)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("database: batch statement %d failed, transaction rolled back: %w", i, err)
		}
		results = append(results, res)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("database: batch commit failed: %w", err)
	}
	return results, nil
}

// batchIndependent 逐条独立执行；第 N 条失败时停止并返回已成功的结果与错误，
// 不回滚之前已提交的语句（它们各自已经是独立的自动提交语句）。
func batchIndependent(ctx context.Context, db *sql.DB, stmts []Statement) ([]QueryResult, error) {
	results := make([]QueryResult, 0, len(stmts))
	for i, stmt := range stmts {
		res, err := executeOn(ctx, db, stmt)
		if err != nil {
			return results, fmt.Errorf("database: batch statement %d failed (non-transactional, %d succeeded): %w", i, len(results), err)
		}
		results = append(results, res)
	}
	return results, nil
}
