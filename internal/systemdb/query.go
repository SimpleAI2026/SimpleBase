// 文件说明：systemdb 内部的查询 / 写入样板收敛。
//
// 把分散在各实体文件里逐字重复的四段样板集中到这里：
//   - conn：「nil Store 可调用」契约，nil Store 或未打开库时统一返回 ErrUnavailable；
//   - queryOne：单行查询 + scan，行不存在时原样返回 sql.ErrNoRows；
//   - queryList：多行查询 + 逐行 scan，无行时返回非 nil 空切片；
//   - countRows：返回单个整数的聚合查询（COUNT(*) 等）；
//   - execAffectOne：软删除 / 状态更新，0 行受影响返回 sql.ErrNoRows，成功后 notifyWrite；
//   - execNotify：幂等归档，0 行受影响仍成功，执行成功后 notifyWrite；
//   - flushBatch：批量落库失败时把批次退回缓冲头部，等待下一轮重试。
//
// 用法：各实体的 Get* / Archive* / flush*Batch 以一两行调用这些助手，
// 不再各自手写守卫与错误转换。
//
// 调用架构：
//
//	Store.GetXxx / ListXxx / CountXxx / ArchiveXxx / flushXxxBatch
//	      │
//	      ▼
//	conn → queryOne / queryList / countRows / execAffectOne / flushBatch → database/sql
//
// 主要符号：Store.conn、queryOne、queryList、Store.countRows、Store.execAffectOne、Store.execNotify、flushBatch。
package systemdb

import (
	"context"
	"database/sql"
)

// conn 返回已打开的库连接。nil Store 与未打开的库都返回 ErrUnavailable，
// 所以公开方法可以直接对 nil *Store 调用。
func (s *Store) conn() (*sql.DB, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	return s.db, nil
}

// queryOne 执行单行查询并交给 scan 解析。
// 行不存在时 scan 返回的 sql.ErrNoRows 原样透传，调用方用 errors.Is 判断；
// 所有 scan 函数出错时都返回零值，所以这里直接把 (T, error) 交还即可。
func queryOne[T any](ctx context.Context, s *Store, scan func(rowScanner) (T, error), q string, args ...any) (T, error) {
	db, err := s.conn()
	if err != nil {
		var zero T
		return zero, err
	}
	return scan(db.QueryRowContext(ctx, q, args...))
}

// queryList 执行多行查询并逐行交给 scan 解析；无行时返回非 nil 的空切片。
// scan 出错与 rows.Err() 原样返回，调用方只负责 SQL 与参数。
func queryList[T any](ctx context.Context, s *Store, scan func(rowScanner) (T, error), q string, args ...any) ([]T, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]T, 0)
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// countRows 执行返回单个整数的聚合查询（COUNT(*) 等）。
func (s *Store) countRows(ctx context.Context, q string, args ...any) (int, error) {
	db, err := s.conn()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return int(n), nil
}

// execAffectOne 执行一条应当恰好命中目标行的写语句（软删除 / 归档）。
// 0 行受影响返回 sql.ErrNoRows；成功后调用 notifyWrite。
// RowsAffected 的错误被有意忽略：部分驱动不实现它，此时按 0 行处理与历史行为一致。
func (s *Store) execAffectOne(ctx context.Context, q string, args ...any) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return nil
}

// execNotify 执行幂等写并在成功后通知同步；0 行受影响也返回成功。
func (s *Store) execNotify(ctx context.Context, q string, args ...any) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, q, args...); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// flushBatch 把 batch 交给 insert 落库。失败时在 s.mu 保护下调用 requeue
// 把批次退回缓冲头部，避免整批丢失；成功后 notifyWrite。空批与不可用的 Store 是 no-op。
// requeue 在锁内执行，闭包里不得再获取 s.mu。
func flushBatch[T any](ctx context.Context, s *Store, batch []T, insert func(context.Context, execer, []T) error, requeue func([]T)) error {
	if len(batch) == 0 {
		return nil
	}
	db, err := s.conn()
	if err != nil {
		return nil
	}
	if err := insert(ctx, db, batch); err != nil {
		s.mu.Lock()
		requeue(batch)
		s.mu.Unlock()
		return err
	}
	s.notifyWrite(ctx)
	return nil
}
