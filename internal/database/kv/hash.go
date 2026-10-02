package kv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// HashRepo 是 hash 类型仓库（事务版，由 Tx.Hash() 返回）。
type HashRepo struct{ t *Tx }

// Get 读取单个 field；key/field 不存在返回 ErrNotFound。
func (r *HashRepo) Get(ctx context.Context, key, field string) ([]byte, error) {
	if _, err := r.t.Key().getMetaOf(ctx, key, TypeHash); err != nil {
		return nil, err
	}
	var v []byte
	err := r.t.tx.QueryRowContext(ctx,
		`SELECT value FROM kv.hashes WHERE "key" = ? AND field = ?`, key, field).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kv: hash get: %w", err)
	}
	return v, nil
}

// GetMany 批量读取 fields；values/found 与 fields 按下标一一对应。
// key 不存在时全部 found=false。
func (r *HashRepo) GetMany(ctx context.Context, key string, fields ...string) ([][]byte, []bool, error) {
	values := make([][]byte, len(fields))
	found := make([]bool, len(fields))
	if len(fields) == 0 {
		return values, found, nil
	}
	if _, err := r.t.Key().getMetaOf(ctx, key, TypeHash); errors.Is(err, ErrNotFound) {
		return values, found, nil
	} else if err != nil {
		return nil, nil, err
	}
	ph := make([]string, len(fields))
	args := make([]any, 0, len(fields)+1)
	args = append(args, key)
	for i, f := range fields {
		ph[i] = "?"
		args = append(args, f)
	}
	rows, err := r.t.tx.QueryContext(ctx,
		fmt.Sprintf(`SELECT field, value FROM kv.hashes WHERE "key" = ? AND field IN (%s)`,
			strings.Join(ph, ",")), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("kv: hash get many: %w", err)
	}
	defer rows.Close()
	idx := make(map[string]int, len(fields))
	for i, f := range fields {
		if _, dup := idx[f]; !dup {
			idx[f] = i
		}
	}
	for rows.Next() {
		var f string
		var v []byte
		if err := rows.Scan(&f, &v); err != nil {
			return nil, nil, fmt.Errorf("kv: hash get many row: %w", err)
		}
		if i, ok := idx[f]; ok {
			values[i] = v
			found[i] = true
		}
	}
	return values, found, rows.Err()
}

// Set 写入多个 field（HSET）。返回新增 field 数（更新已有 field 不计）。
func (r *HashRepo) Set(ctx context.Context, key string, fields map[string][]byte) (int, error) {
	if len(fields) == 0 {
		return 0, ErrArgument
	}
	kr := r.t.Key()
	exists, err := kr.guardWrite(ctx, key, TypeHash)
	if err != nil {
		return 0, err
	}
	names := make([]string, 0, len(fields))
	for f := range fields {
		names = append(names, f)
	}
	var added int
	if exists {
		// 查已存在 field 集合，差集为新增。
		_, found, err := r.GetMany(ctx, key, names...)
		if err != nil {
			return 0, err
		}
		for _, ok := range found {
			if !ok {
				added++
			}
		}
	} else {
		added = len(names)
	}
	for _, f := range names {
		v := fields[f]
		if exists {
			res, err := r.t.tx.ExecContext(ctx,
				`UPDATE kv.hashes SET value = ? WHERE "key" = ? AND field = ?`, v, key, f)
			if err != nil {
				return 0, fmt.Errorf("kv: hash set update: %w", err)
			}
			if aff, _ := res.RowsAffected(); aff == 0 {
				if _, err := r.t.tx.ExecContext(ctx,
					`INSERT INTO kv.hashes ("key", field, value) VALUES (?, ?, ?)`, key, f, v); err != nil {
					return 0, fmt.Errorf("kv: hash set insert: %w", err)
				}
			}
		} else {
			if _, err := r.t.tx.ExecContext(ctx,
				`INSERT INTO kv.hashes ("key", field, value) VALUES (?, ?, ?)`, key, f, v); err != nil {
				return 0, fmt.Errorf("kv: hash set insert: %w", err)
			}
		}
	}
	if exists {
		if err := kr.touchAfterWrite(ctx, key, int64(added)); err != nil {
			return 0, err
		}
	} else {
		l := int64(len(names))
		if err := kr.insertKey(ctx, key, TypeHash, nil, &l); err != nil {
			return 0, err
		}
	}
	return added, nil
}

// Delete 删除 fields，返回实际删除数。删空后自动删除 key（Redis 语义）。
func (r *HashRepo) Delete(ctx context.Context, key string, fields ...string) (int, error) {
	if len(fields) == 0 {
		return 0, ErrArgument
	}
	kr := r.t.Key()
	if _, err := kr.getMetaOf(ctx, key, TypeHash); errors.Is(err, ErrNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	ph := make([]string, len(fields))
	args := make([]any, 0, len(fields)+1)
	args = append(args, key)
	for i, f := range fields {
		ph[i] = "?"
		args = append(args, f)
	}
	res, err := r.t.tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM kv.hashes WHERE "key" = ? AND field IN (%s)`, strings.Join(ph, ",")),
		args...)
	if err != nil {
		return 0, fmt.Errorf("kv: hash delete: %w", err)
	}
	aff, _ := res.RowsAffected()
	if aff > 0 {
		if err := kr.touchAfterWrite(ctx, key, -aff); err != nil {
			return 0, err
		}
		if _, err := kr.deleteIfEmpty(ctx, key); err != nil {
			return 0, err
		}
	}
	return int(aff), nil
}

// Items 返回全部 field→value（HGETALL）。key 不存在返回空 map。
func (r *HashRepo) Items(ctx context.Context, key string) (map[string][]byte, error) {
	out := make(map[string][]byte)
	if _, err := r.t.Key().getMetaOf(ctx, key, TypeHash); errors.Is(err, ErrNotFound) {
		return out, nil
	} else if err != nil {
		return nil, err
	}
	rows, err := r.t.tx.QueryContext(ctx,
		`SELECT field, value FROM kv.hashes WHERE "key" = ? ORDER BY field`, key)
	if err != nil {
		return nil, fmt.Errorf("kv: hash items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var f string
		var v []byte
		if err := rows.Scan(&f, &v); err != nil {
			return nil, fmt.Errorf("kv: hash items row: %w", err)
		}
		out[f] = v
	}
	return out, rows.Err()
}

// Incr 对 field 原子自增（HINCRBY）。field 不存在按 0 起始。
// 值非整数或溢出返回 ErrValueType。
func (r *HashRepo) Incr(ctx context.Context, key, field string, delta int64) (int64, error) {
	kr := r.t.Key()
	exists, err := kr.guardWrite(ctx, key, TypeHash)
	if err != nil {
		return 0, err
	}
	var cur int64
	fieldExists := false
	if exists {
		v, err := r.Get(ctx, key, field)
		switch {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return 0, err
		default:
			fieldExists = true
			cur, err = strconv.ParseInt(strings.TrimSpace(string(v)), 10, 64)
			if err != nil {
				return 0, ErrValueType
			}
		}
	}
	if (delta > 0 && cur > math.MaxInt64-delta) || (delta < 0 && cur < math.MinInt64-delta) {
		return 0, ErrValueType
	}
	next := cur + delta
	raw := []byte(strconv.FormatInt(next, 10))
	if fieldExists {
		if _, err := r.t.tx.ExecContext(ctx,
			`UPDATE kv.hashes SET value = ? WHERE "key" = ? AND field = ?`, raw, key, field); err != nil {
			return 0, fmt.Errorf("kv: hash incr update: %w", err)
		}
		if err := kr.touchAfterWrite(ctx, key, 0); err != nil {
			return 0, err
		}
		return next, nil
	}
	if !exists {
		l := int64(1)
		if err := kr.insertKey(ctx, key, TypeHash, nil, &l); err != nil {
			return 0, err
		}
	} else {
		if err := kr.touchAfterWrite(ctx, key, 1); err != nil {
			return 0, err
		}
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`INSERT INTO kv.hashes ("key", field, value) VALUES (?, ?, ?)`, key, field, raw); err != nil {
		return 0, fmt.Errorf("kv: hash incr insert: %w", err)
	}
	return next, nil
}

// Scan 游标式扫描 hash 字段（HSCAN；cursor 为上一页最后 field）。
func (r *HashRepo) Scan(ctx context.Context, key, cursor, pattern string, count int) (map[string][]byte, string, error) {
	if _, err := r.t.Key().getMetaOf(ctx, key, TypeHash); err != nil {
		return nil, "", err
	}
	if pattern == "" {
		pattern = "*"
	}
	if count <= 0 {
		count = 100
	}
	rows, err := r.t.tx.QueryContext(ctx,
		`SELECT field, value FROM kv.hashes
		 WHERE "key" = ? AND field > ? AND field GLOB ?
		 ORDER BY field LIMIT ?`, key, cursor, pattern, count+1)
	if err != nil {
		return nil, "", fmt.Errorf("kv: hash scan: %w", err)
	}
	defer rows.Close()
	type fv struct {
		f string
		v []byte
	}
	page := make([]fv, 0, count)
	for rows.Next() {
		var x fv
		if err := rows.Scan(&x.f, &x.v); err != nil {
			return nil, "", fmt.Errorf("kv: hash scan row: %w", err)
		}
		page = append(page, x)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(page) > count {
		page = page[:count]
		next = page[len(page)-1].f
	}
	out := make(map[string][]byte, len(page))
	for _, x := range page {
		out[x.f] = x.v
	}
	return out, next, nil
}
