package kv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// SetRepo 是 set 类型仓库（事务版，由 Tx.Set() 返回）。
type SetRepo struct{ t *Tx }

// Add 添加成员（SADD），返回实际新增数（去重）。
func (r *SetRepo) Add(ctx context.Context, key string, elems ...[]byte) (int, error) {
	if len(elems) == 0 {
		return 0, ErrArgument
	}
	kr := r.t.Key()
	exists, err := kr.guardWrite(ctx, key, TypeSet)
	if err != nil {
		return 0, err
	}
	// 入参去重
	uniq := make(map[string][]byte, len(elems))
	for _, e := range elems {
		uniq[string(e)] = e
	}
	added := 0
	if exists {
		for _, e := range uniq {
			var n int64
			if err := r.t.tx.QueryRowContext(ctx,
				`SELECT count(*) FROM kv.sets WHERE "key" = ? AND elem = ?`, key, e).Scan(&n); err != nil {
				return 0, fmt.Errorf("kv: set add check: %w", err)
			}
			if n > 0 {
				continue
			}
			if _, err := r.t.tx.ExecContext(ctx,
				`INSERT INTO kv.sets ("key", elem) VALUES (?, ?)`, key, e); err != nil {
				return 0, fmt.Errorf("kv: set add insert: %w", err)
			}
			added++
		}
		if added > 0 {
			if err := kr.touchAfterWrite(ctx, key, int64(added)); err != nil {
				return 0, err
			}
		}
		return added, nil
	}
	for _, e := range uniq {
		if _, err := r.t.tx.ExecContext(ctx,
			`INSERT INTO kv.sets ("key", elem) VALUES (?, ?)`, key, e); err != nil {
			return 0, fmt.Errorf("kv: set add insert: %w", err)
		}
		added++
	}
	l := int64(added)
	if err := kr.insertKey(ctx, key, TypeSet, nil, &l); err != nil {
		return 0, err
	}
	return added, nil
}

// Delete 移除成员（SREM），返回实际删除数。删空后自动删除 key。
func (r *SetRepo) Delete(ctx context.Context, key string, elems ...[]byte) (int, error) {
	if len(elems) == 0 {
		return 0, ErrArgument
	}
	kr := r.t.Key()
	if _, err := kr.getMetaOf(ctx, key, TypeSet); errors.Is(err, ErrNotFound) {
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
		fmt.Sprintf(`DELETE FROM kv.sets WHERE "key" = ? AND elem IN (%s)`, strings.Join(ph, ",")),
		args...)
	if err != nil {
		return 0, fmt.Errorf("kv: set delete: %w", err)
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

// IsMember 判断成员存在性（SISMEMBER）。key 不存在返回 false。
func (r *SetRepo) IsMember(ctx context.Context, key string, elem []byte) (bool, error) {
	if _, err := r.t.Key().getMetaOf(ctx, key, TypeSet); errors.Is(err, ErrNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var n int64
	err := r.t.tx.QueryRowContext(ctx,
		`SELECT count(*) FROM kv.sets WHERE "key" = ? AND elem = ?`, key, elem).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("kv: set ismember: %w", err)
	}
	return n > 0, nil
}

// Members 返回全部成员（SMEMBERS；按 elem 字典序）。key 不存在返回空。
func (r *SetRepo) Members(ctx context.Context, key string) ([][]byte, error) {
	if _, err := r.t.Key().getMetaOf(ctx, key, TypeSet); errors.Is(err, ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return r.membersOf(ctx, key)
}

func (r *SetRepo) membersOf(ctx context.Context, key string) ([][]byte, error) {
	rows, err := r.t.tx.QueryContext(ctx,
		`SELECT elem FROM kv.sets WHERE "key" = ? ORDER BY elem`, key)
	if err != nil {
		return nil, fmt.Errorf("kv: set members: %w", err)
	}
	defer rows.Close()
	out := make([][]byte, 0, 16)
	for rows.Next() {
		var e []byte
		if err := rows.Scan(&e); err != nil {
			return nil, fmt.Errorf("kv: set members row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Pop 随机弹出一个成员（SPOP）；空/不存在返回 ErrNotFound。
func (r *SetRepo) Pop(ctx context.Context, key string) ([]byte, error) {
	kr := r.t.Key()
	if _, err := kr.getMetaOf(ctx, key, TypeSet); err != nil {
		return nil, err
	}
	var elem []byte
	err := r.t.tx.QueryRowContext(ctx,
		`SELECT elem FROM kv.sets WHERE "key" = ? ORDER BY random() LIMIT 1`, key).Scan(&elem)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kv: set pop pick: %w", err)
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`DELETE FROM kv.sets WHERE "key" = ? AND elem = ?`, key, elem); err != nil {
		return nil, fmt.Errorf("kv: set pop delete: %w", err)
	}
	if err := kr.touchAfterWrite(ctx, key, -1); err != nil {
		return nil, err
	}
	if _, err := kr.deleteIfEmpty(ctx, key); err != nil {
		return nil, err
	}
	return elem, nil
}

// validSetKeys 校验集合运算的输入 keys：存在且未过期的 key 必须是 set 类型；
// 返回参与运算的有效 key 列表（去重）。不存在/已过期的 key 按空集处理。
func (r *SetRepo) validSetKeys(ctx context.Context, keys []string) ([]string, error) {
	kr := r.t.Key()
	seen := make(map[string]bool, len(keys))
	valid := make([]string, 0, len(keys))
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		m, err := kr.getMeta(ctx, k)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if m.Type != TypeSet {
			return nil, ErrKeyType
		}
		valid = append(valid, k)
	}
	return valid, nil
}

func placeholders(n int) string {
	ph := make([]string, n)
	for i := range ph {
		ph[i] = "?"
	}
	return strings.Join(ph, ",")
}

// Union 并集（SUNION）。
func (r *SetRepo) Union(ctx context.Context, keys ...string) ([][]byte, error) {
	valid, err := r.validSetKeys(ctx, keys)
	if err != nil || len(valid) == 0 {
		return nil, err
	}
	args := make([]any, len(valid))
	for i, k := range valid {
		args[i] = k
	}
	rows, err := r.t.tx.QueryContext(ctx,
		fmt.Sprintf(`SELECT DISTINCT elem FROM kv.sets WHERE "key" IN (%s) ORDER BY elem`,
			placeholders(len(valid))), args...)
	if err != nil {
		return nil, fmt.Errorf("kv: set union: %w", err)
	}
	defer rows.Close()
	return scanElems(rows)
}

// Inter 交集（SINTER）。
func (r *SetRepo) Inter(ctx context.Context, keys ...string) ([][]byte, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	uniq := dedupeStrings(keys)
	valid, err := r.validSetKeys(ctx, uniq)
	if err != nil {
		return nil, err
	}
	// 任一输入 key 不存在/已过期（空集）→ 交集为空
	if len(valid) < len(uniq) {
		return nil, nil
	}
	args := make([]any, 0, len(valid)+1)
	for _, k := range valid {
		args = append(args, k)
	}
	args = append(args, len(valid))
	rows, err := r.t.tx.QueryContext(ctx,
		fmt.Sprintf(`SELECT elem FROM kv.sets WHERE "key" IN (%s)
		 GROUP BY elem HAVING count(DISTINCT "key") = ? ORDER BY elem`,
			placeholders(len(valid))), args...)
	if err != nil {
		return nil, fmt.Errorf("kv: set inter: %w", err)
	}
	defer rows.Close()
	return scanElems(rows)
}

// Diff 差集（SDIFF：第一个 key 减去其余并集）。
func (r *SetRepo) Diff(ctx context.Context, keys ...string) ([][]byte, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	valid, err := r.validSetKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	// 第一个 key 必须在有效集合里（否则结果为空）
	first := keys[0]
	inValid := false
	for _, k := range valid {
		if k == first {
			inValid = true
			break
		}
	}
	if !inValid {
		return nil, nil
	}
	if len(valid) == 1 {
		return r.membersOf(ctx, first)
	}
	rest := make([]string, 0, len(valid)-1)
	for _, k := range valid {
		if k != first {
			rest = append(rest, k)
		}
	}
	args := make([]any, 0, len(rest)+1)
	args = append(args, first)
	for _, k := range rest {
		args = append(args, k)
	}
	rows, err := r.t.tx.QueryContext(ctx,
		fmt.Sprintf(`SELECT elem FROM kv.sets WHERE "key" = ?
		 AND elem NOT IN (SELECT elem FROM kv.sets WHERE "key" IN (%s))
		 ORDER BY elem`, placeholders(len(rest))), args...)
	if err != nil {
		return nil, fmt.Errorf("kv: set diff: %w", err)
	}
	defer rows.Close()
	return scanElems(rows)
}

// UnionStore / InterStore / DiffStore：运算结果写入 dest（覆盖写），返回元素数。
func (r *SetRepo) UnionStore(ctx context.Context, dest string, keys ...string) (int, error) {
	return r.storeResult(ctx, dest, func() ([][]byte, error) { return r.Union(ctx, keys...) })
}

func (r *SetRepo) InterStore(ctx context.Context, dest string, keys ...string) (int, error) {
	return r.storeResult(ctx, dest, func() ([][]byte, error) { return r.Inter(ctx, keys...) })
}

func (r *SetRepo) DiffStore(ctx context.Context, dest string, keys ...string) (int, error) {
	return r.storeResult(ctx, dest, func() ([][]byte, error) { return r.Diff(ctx, keys...) })
}

func (r *SetRepo) storeResult(ctx context.Context, dest string, compute func() ([][]byte, error)) (int, error) {
	elems, err := compute()
	if err != nil {
		return 0, err
	}
	kr := r.t.Key()
	// 覆盖写：dest 已存在先级联删除（无论类型）
	if m, err := kr.getMetaAny(ctx, dest); err == nil {
		if err := kr.deleteCascade(ctx, dest, m.Type); err != nil {
			return 0, err
		}
	} else if !errors.Is(err, ErrNotFound) {
		return 0, err
	}
	if len(elems) == 0 {
		return 0, nil // 空结果不落 key（Redis 语义：STORE 空结果删除 dest）
	}
	for _, e := range elems {
		if _, err := r.t.tx.ExecContext(ctx,
			`INSERT INTO kv.sets ("key", elem) VALUES (?, ?)`, dest, e); err != nil {
			return 0, fmt.Errorf("kv: set store insert: %w", err)
		}
	}
	l := int64(len(elems))
	if err := kr.insertKey(ctx, dest, TypeSet, nil, &l); err != nil {
		return 0, err
	}
	return len(elems), nil
}

func scanElems(rows *sql.Rows) ([][]byte, error) {
	out := make([][]byte, 0, 16)
	for rows.Next() {
		var e []byte
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
