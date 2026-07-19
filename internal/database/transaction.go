package database

import (
	"context"
	"database/sql"
	"fmt"
)

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
