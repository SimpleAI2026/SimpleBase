package kv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ZSetItem 是 zset 的成员+分值。
type ZSetItem struct {
	Elem  []byte
	Score float64
}

// ZSetRepo 是 zset（有序集合）仓库（事务版，由 Tx.ZSet() 返回）。
// 排序键为 (score ASC, elem ASC)，与 Redis 一致。
type ZSetRepo struct{ t *Tx }

// Add 添加/更新成员（ZADD），返回新增成员数（分数变更不计）。
func (r *ZSetRepo) Add(ctx context.Context, key string, items ...ZSetItem) (int, error) {
	if len(items) == 0 {
		return 0, ErrArgument
	}
	kr := r.t.Key()
	exists, err := kr.guardWrite(ctx, key, TypeZSet)
	if err != nil {
		return 0, err
	}
	// 入参去重（同 elem 后者覆盖前者）
	uniq := make(map[string]ZSetItem, len(items))
	for _, it := range items {
		uniq[string(it.Elem)] = it
	}
	added := 0
	for _, it := range uniq {
		var cur float64
		err := r.t.tx.QueryRowContext(ctx,
			`SELECT score FROM kv.zsets WHERE "key" = ? AND elem = ?`, key, it.Elem).Scan(&cur)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := r.t.tx.ExecContext(ctx,
				`INSERT INTO kv.zsets ("key", elem, score) VALUES (?, ?, ?)`, key, it.Elem, it.Score); err != nil {
				return 0, fmt.Errorf("kv: zset add insert: %w", err)
			}
			added++
		case err != nil:
			return 0, fmt.Errorf("kv: zset add check: %w", err)
		default:
			if cur != it.Score {
				if _, err := r.t.tx.ExecContext(ctx,
					`UPDATE kv.zsets SET score = ? WHERE "key" = ? AND elem = ?`, it.Score, key, it.Elem); err != nil {
					return 0, fmt.Errorf("kv: zset add update: %w", err)
				}
			}
		}
	}
	if exists {
		if err := kr.touchAfterWrite(ctx, key, int64(added)); err != nil {
			return 0, err
		}
	} else {
		l := int64(added)
		if err := kr.insertKey(ctx, key, TypeZSet, nil, &l); err != nil {
			return 0, err
		}
	}
	return added, nil
}

// Delete 移除成员（ZREM），返回实际删除数。删空后自动删除 key。
func (r *ZSetRepo) Delete(ctx context.Context, key string, elems ...[]byte) (int, error) {
	if len(elems) == 0 {
		return 0, ErrArgument
	}
	kr := r.t.Key()
	if _, err := kr.getMetaOf(ctx, key, TypeZSet); errors.Is(err, ErrNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	ph := make([]string, len(elems))
	args := make([]any, 0, len(elems)+1)
	args = append(args, key)
	for i, e := range elems {
		ph[i] = "?"
		args = append(args, e)
	}
	res, err := r.t.tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM kv.zsets WHERE "key" = ? AND elem IN (%s)`, strings.Join(ph, ",")),
		args...)
	if err != nil {
		return 0, fmt.Errorf("kv: zset delete: %w", err)
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

// Score 返回成员分数（ZSCORE）；不存在返回 ErrNotFound。
func (r *ZSetRepo) Score(ctx context.Context, key string, elem []byte) (float64, error) {
	if _, err := r.t.Key().getMetaOf(ctx, key, TypeZSet); err != nil {
		return 0, err
	}
	var score float64
	err := r.t.tx.QueryRowContext(ctx,
		`SELECT score FROM kv.zsets WHERE "key" = ? AND elem = ?`, key, elem).Scan(&score)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("kv: zset score: %w", err)
	}
	return score, nil
}

// Rank 返回成员排名（0 起，升序）与分数；不存在返回 ErrNotFound。
func (r *ZSetRepo) Rank(ctx context.Context, key string, elem []byte) (int64, float64, error) {
	if _, err := r.t.Key().getMetaOf(ctx, key, TypeZSet); err != nil {
		return 0, 0, err
	}
	var score float64
	err := r.t.tx.QueryRowContext(ctx,
		`SELECT score FROM kv.zsets WHERE "key" = ? AND elem = ?`, key, elem).Scan(&score)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, 0, fmt.Errorf("kv: zset rank score: %w", err)
	}
	var rank int64
	err = r.t.tx.QueryRowContext(ctx,
		`SELECT count(*) FROM kv.zsets WHERE "key" = ?
		 AND (score < ? OR (score = ? AND elem < ?))`, key, score, score, elem).Scan(&rank)
	if err != nil {
		return 0, 0, fmt.Errorf("kv: zset rank count: %w", err)
	}
	return rank, score, nil
}

// RangeByRank 按排名区间返回（ZRANGE start stop；支持负下标）。
// desc=true 时按 (score DESC, elem DESC) 排列（ZREVRANGE 语义，排名同样倒序）。
func (r *ZSetRepo) RangeByRank(ctx context.Context, key string, start, stop int64, desc bool) ([]ZSetItem, error) {
	m, err := r.t.Key().getMetaOf(ctx, key, TypeZSet)
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
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	rows, err := r.t.tx.QueryContext(ctx,
		fmt.Sprintf(`SELECT elem, score FROM kv.zsets WHERE "key" = ?
		 ORDER BY score %s, elem %s LIMIT ? OFFSET ?`, dir, dir),
		key, hi-lo+1, lo)
	if err != nil {
		return nil, fmt.Errorf("kv: zset range rank: %w", err)
	}
	defer rows.Close()
	return scanZItems(rows)
}

// RangeByScore 按分数闭区间返回（ZRANGEBYSCORE min max LIMIT offset count）。
// count<=0 表示不限。
func (r *ZSetRepo) RangeByScore(ctx context.Context, key string, min, max float64, offset, count int64) ([]ZSetItem, error) {
	if _, err := r.t.Key().getMetaOf(ctx, key, TypeZSet); errors.Is(err, ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	query := `SELECT elem, score FROM kv.zsets WHERE "key" = ? AND score >= ? AND score <= ?
	 ORDER BY score ASC, elem ASC`
	args := []any{key, min, max}
	if count > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, count, offset)
	} else if offset > 0 {
		query += ` OFFSET ?`
		args = append(args, offset)
	}
	rows, err := r.t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("kv: zset range score: %w", err)
	}
	defer rows.Close()
	return scanZItems(rows)
}

// Incr 成员分数自增（ZINCRBY），返回新分数。成员不存在按 0 起始。
func (r *ZSetRepo) Incr(ctx context.Context, key string, elem []byte, delta float64) (float64, error) {
	kr := r.t.Key()
	exists, err := kr.guardWrite(ctx, key, TypeZSet)
	if err != nil {
		return 0, err
	}
	var next float64
	memberExists := false
	if exists {
		var cur float64
		err := r.t.tx.QueryRowContext(ctx,
			`SELECT score FROM kv.zsets WHERE "key" = ? AND elem = ?`, key, elem).Scan(&cur)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			next = delta
		case err != nil:
			return 0, fmt.Errorf("kv: zset incr check: %w", err)
		default:
			memberExists = true
			next = cur + delta
		}
	} else {
		next = delta
	}
	if memberExists {
		if _, err := r.t.tx.ExecContext(ctx,
			`UPDATE kv.zsets SET score = ? WHERE "key" = ? AND elem = ?`, next, key, elem); err != nil {
			return 0, fmt.Errorf("kv: zset incr update: %w", err)
		}
		if err := kr.touchAfterWrite(ctx, key, 0); err != nil {
			return 0, err
		}
		return next, nil
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`INSERT INTO kv.zsets ("key", elem, score) VALUES (?, ?, ?)`, key, elem, next); err != nil {
		return 0, fmt.Errorf("kv: zset incr insert: %w", err)
	}
	if exists {
		if err := kr.touchAfterWrite(ctx, key, 1); err != nil {
			return 0, err
		}
	} else {
		l := int64(1)
		if err := kr.insertKey(ctx, key, TypeZSet, nil, &l); err != nil {
			return 0, err
		}
	}
	return next, nil
}

// CountByScore 统计分数在 [min,max] 内的成员数（ZCOUNT）。
func (r *ZSetRepo) CountByScore(ctx context.Context, key string, min, max float64) (int64, error) {
	if _, err := r.t.Key().getMetaOf(ctx, key, TypeZSet); errors.Is(err, ErrNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	var n int64
	err := r.t.tx.QueryRowContext(ctx,
		`SELECT count(*) FROM kv.zsets WHERE "key" = ? AND score >= ? AND score <= ?`,
		key, min, max).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("kv: zset count: %w", err)
	}
	return n, nil
}

func scanZItems(rows *sql.Rows) ([]ZSetItem, error) {
	out := make([]ZSetItem, 0, 16)
	for rows.Next() {
		var it ZSetItem
		if err := rows.Scan(&it.Elem, &it.Score); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
