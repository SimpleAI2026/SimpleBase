package kv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ListRepo 是 list 类型仓库（事务版，由 Tx.List() 返回）。
// 元素顺序由 pos（DOUBLE 中点）维护：push back = max(pos)+1，
// push front = min(pos)-1；支持 Redis 负下标语义（-1 = 末尾）。
type ListRepo struct{ t *Tx }

// normalizeRange 将 Redis 风格 start/stop（可负）归一化为闭区间下标。
// 返回 ok=false 表示空区间。
func normalizeRange(start, stop, length int64) (int64, int64, bool) {
	if length <= 0 {
		return 0, 0, false
	}
	if start < 0 {
		start = length + start
		if start < 0 {
			start = 0
		}
	}
	if stop < 0 {
		stop = length + stop
	}
	if stop >= length {
		stop = length - 1
	}
	if start > stop || start >= length {
		return 0, 0, false
	}
	return start, stop, true
}

// PushBack 尾部插入（RPUSH），返回新长度。
func (r *ListRepo) PushBack(ctx context.Context, key string, elems ...[]byte) (int64, error) {
	return r.push(ctx, key, false, elems...)
}

// PushFront 头部插入（LPUSH；多个元素按给定顺序依次入头，最终结果与 Redis 一致：
// LPUSH k a b → 列表为 b,a,...）。
func (r *ListRepo) PushFront(ctx context.Context, key string, elems ...[]byte) (int64, error) {
	return r.push(ctx, key, true, elems...)
}

func (r *ListRepo) push(ctx context.Context, key string, front bool, elems ...[]byte) (int64, error) {
	if len(elems) == 0 {
		return 0, ErrArgument
	}
	kr := r.t.Key()
	exists, err := kr.guardWrite(ctx, key, TypeList)
	if err != nil {
		return 0, err
	}
	var base float64
	if exists {
		dir := "MAX"
		if front {
			dir = "MIN"
		}
		row := r.t.tx.QueryRowContext(ctx,
			fmt.Sprintf(`SELECT COALESCE(%s(pos), 0) FROM kv.lists WHERE "key" = ?`, dir), key)
		if err := row.Scan(&base); err != nil {
			return 0, fmt.Errorf("kv: list push boundary: %w", err)
		}
	}
	for i, e := range elems {
		var pos float64
		if !exists && i == 0 {
			pos = 0
		} else if front {
			base--
			pos = base
		} else {
			base++
			pos = base
		}
		if _, err := r.t.tx.ExecContext(ctx,
			`INSERT INTO kv.lists ("key", pos, elem) VALUES (?, ?, ?)`, key, pos, e); err != nil {
			return 0, fmt.Errorf("kv: list push: %w", err)
		}
	}
	var newLen int64
	if exists {
		m, err := kr.getMetaOf(ctx, key, TypeList)
		if err != nil {
			return 0, err
		}
		newLen = *m.Len + int64(len(elems))
		if err := kr.touchAfterWrite(ctx, key, int64(len(elems))); err != nil {
			return 0, err
		}
	} else {
		newLen = int64(len(elems))
		if err := kr.insertKey(ctx, key, TypeList, nil, &newLen); err != nil {
			return 0, err
		}
	}
	return newLen, nil
}

// PopBack 尾部弹出（RPOP）；空/不存在返回 ErrNotFound。
func (r *ListRepo) PopBack(ctx context.Context, key string) ([]byte, error) {
	return r.pop(ctx, key, false)
}

// PopFront 头部弹出（LPOP）。
func (r *ListRepo) PopFront(ctx context.Context, key string) ([]byte, error) {
	return r.pop(ctx, key, true)
}

func (r *ListRepo) pop(ctx context.Context, key string, front bool) ([]byte, error) {
	kr := r.t.Key()
	if _, err := kr.getMetaOf(ctx, key, TypeList); err != nil {
		return nil, err
	}
	dir := "ASC"
	if !front {
		dir = "DESC"
	}
	var pos float64
	var elem []byte
	err := r.t.tx.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT pos, elem FROM kv.lists WHERE "key" = ? ORDER BY pos %s LIMIT 1`, dir),
		key).Scan(&pos, &elem)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kv: list pop scan: %w", err)
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`DELETE FROM kv.lists WHERE "key" = ? AND pos = ?`, key, pos); err != nil {
		return nil, fmt.Errorf("kv: list pop delete: %w", err)
	}
	if err := kr.touchAfterWrite(ctx, key, -1); err != nil {
		return nil, err
	}
	if _, err := kr.deleteIfEmpty(ctx, key); err != nil {
		return nil, err
	}
	return elem, nil
}

// Range 返回闭区间元素（LRANGE；支持负下标）。key 不存在返回空。
func (r *ListRepo) Range(ctx context.Context, key string, start, stop int64) ([][]byte, error) {
	m, err := r.t.Key().getMetaOf(ctx, key, TypeList)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lo, hi, ok := normalizeRange(start, stop, *m.Len)
	if !ok {
		return nil, nil
	}
	rows, err := r.t.tx.QueryContext(ctx,
		`SELECT elem FROM kv.lists WHERE "key" = ? ORDER BY pos ASC LIMIT ? OFFSET ?`,
		key, hi-lo+1, lo)
	if err != nil {
		return nil, fmt.Errorf("kv: list range: %w", err)
	}
	defer rows.Close()
	out := make([][]byte, 0, hi-lo+1)
	for rows.Next() {
		var e []byte
		if err := rows.Scan(&e); err != nil {
			return nil, fmt.Errorf("kv: list range row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Set 按下标改写元素（LSET；支持负下标）。越界返回 ErrNotFound。
func (r *ListRepo) Set(ctx context.Context, key string, index int64, elem []byte) error {
	m, err := r.t.Key().getMetaOf(ctx, key, TypeList)
	if err != nil {
		return err
	}
	idx := index
	if idx < 0 {
		idx = *m.Len + idx
	}
	if idx < 0 || idx >= *m.Len {
		return ErrNotFound
	}
	var pos float64
	err = r.t.tx.QueryRowContext(ctx,
		`SELECT pos FROM kv.lists WHERE "key" = ? ORDER BY pos ASC LIMIT 1 OFFSET ?`,
		key, idx).Scan(&pos)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("kv: list set locate: %w", err)
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`UPDATE kv.lists SET elem = ? WHERE "key" = ? AND pos = ?`, elem, key, pos); err != nil {
		return fmt.Errorf("kv: list set: %w", err)
	}
	return r.t.Key().touchAfterWrite(ctx, key, 0)
}

// Trim 保留闭区间元素，删除其余（LTRIM；支持负下标）。空区间 = 清空并删 key。
func (r *ListRepo) Trim(ctx context.Context, key string, start, stop int64) error {
	kr := r.t.Key()
	m, err := kr.getMetaOf(ctx, key, TypeList)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil // LTRIM 不存在的 key 是 no-op
		}
		return err
	}
	lo, hi, ok := normalizeRange(start, stop, *m.Len)
	if !ok {
		// 清空
		if _, err := r.t.tx.ExecContext(ctx,
			`DELETE FROM kv.lists WHERE "key" = ?`, key); err != nil {
			return fmt.Errorf("kv: list trim clear: %w", err)
		}
		if err := kr.touchAfterWrite(ctx, key, -*m.Len); err != nil {
			return err
		}
		_, err := kr.deleteIfEmpty(ctx, key)
		return err
	}
	// 边界 pos：保留区间内最小/最大 pos
	var loPos, hiPos float64
	if err := r.t.tx.QueryRowContext(ctx,
		`SELECT pos FROM kv.lists WHERE "key" = ? ORDER BY pos ASC LIMIT 1 OFFSET ?`,
		key, lo).Scan(&loPos); err != nil {
		return fmt.Errorf("kv: list trim lo: %w", err)
	}
	if err := r.t.tx.QueryRowContext(ctx,
		`SELECT pos FROM kv.lists WHERE "key" = ? ORDER BY pos ASC LIMIT 1 OFFSET ?`,
		key, hi).Scan(&hiPos); err != nil {
		return fmt.Errorf("kv: list trim hi: %w", err)
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`DELETE FROM kv.lists WHERE "key" = ? AND (pos < ? OR pos > ?)`,
		key, loPos, hiPos); err != nil {
		return fmt.Errorf("kv: list trim delete: %w", err)
	}
	keep := hi - lo + 1
	return kr.touchAfterWrite(ctx, key, keep-*m.Len)
}
