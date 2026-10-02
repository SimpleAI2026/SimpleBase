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

// SetOptions 是 StringRepo.SetWith 的选项（对齐 Redis SET 命令选项）。
type SetOptions struct {
	// NX 仅当 key 不存在时写入；XX 仅当 key 存在时写入。互斥。
	NX, XX bool
	// TTLms 设置过期时间（毫秒）；nil 表示不指定。
	TTLms *int64
	// KeepTTL 保留原有 TTL（与 TTLms 互斥）。
	KeepTTL bool
	// Get 返回写入前的旧值。
	Get bool
}

// SetResult 是 SetWith 的返回。
type SetResult struct {
	// Set 表示写入是否生效（NX/XX 条件不满足时为 false）。
	Set bool
	// Old 为写入前的旧值（Get=true 时；不存在为 nil）。
	Old []byte
}

// StringRepo 是 string 类型仓库（事务版，由 Tx.Str() 返回）。
type StringRepo struct{ t *Tx }

// Get 读取 string 值；不存在或已过期返回 ErrNotFound。
func (r *StringRepo) Get(ctx context.Context, key string) ([]byte, error) {
	var v []byte
	err := r.t.tx.QueryRowContext(ctx,
		`SELECT s.value FROM kv.strings s
		 JOIN kv.keys k ON k."key" = s."key"
		 WHERE s."key" = ? AND (k.etime IS NULL OR k.etime > ?)`,
		key, r.t.nowMs()).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kv: string get: %w", err)
	}
	return v, nil
}

// Set 写入 string（覆盖写，清除原 TTL，对齐 Redis SET 无选项语义）。
func (r *StringRepo) Set(ctx context.Context, key string, value []byte) error {
	_, err := r.SetWith(ctx, key, value, SetOptions{})
	return err
}

// SetWith 带选项写入。NX/XX 条件不满足时返回 Set=false、err=nil。
func (r *StringRepo) SetWith(ctx context.Context, key string, value []byte, opts SetOptions) (SetResult, error) {
	kr := r.t.Key()
	exists, err := kr.guardWrite(ctx, key, TypeString)
	if err != nil {
		return SetResult{}, err
	}
	res := SetResult{Set: true}
	if opts.NX && exists {
		res.Set = false
	}
	if opts.XX && !exists {
		res.Set = false
	}
	if opts.Get && exists {
		old, err := r.Get(ctx, key)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return SetResult{}, err
		}
		res.Old = old
	}
	if !res.Set {
		return res, nil
	}

	var etime *int64
	switch {
	case opts.KeepTTL && exists:
		m, err := kr.getMeta(ctx, key)
		if err != nil {
			return SetResult{}, err
		}
		etime = m.Etime
	case opts.TTLms != nil:
		if *opts.TTLms <= 0 {
			return SetResult{}, ErrArgument
		}
		v := r.t.nowMs() + *opts.TTLms
		etime = &v
	}

	if exists {
		if _, err := r.t.tx.ExecContext(ctx,
			`UPDATE kv.keys SET version = version + 1, etime = ?, mtime = ? WHERE "key" = ?`,
			etime, r.t.nowMs(), key); err != nil {
			return SetResult{}, fmt.Errorf("kv: string set meta: %w", err)
		}
		if _, err := r.t.tx.ExecContext(ctx,
			`UPDATE kv.strings SET value = ? WHERE "key" = ?`, value, key); err != nil {
			return SetResult{}, fmt.Errorf("kv: string set value: %w", err)
		}
		return res, nil
	}
	if err := kr.insertKey(ctx, key, TypeString, etime, nil); err != nil {
		return SetResult{}, err
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`INSERT INTO kv.strings ("key", value) VALUES (?, ?)`, key, value); err != nil {
		return SetResult{}, fmt.Errorf("kv: string insert value: %w", err)
	}
	return res, nil
}

// GetMany 批量读取；values/found 与 keys 按下标一一对应。
func (r *StringRepo) GetMany(ctx context.Context, keys ...string) ([][]byte, []bool, error) {
	values := make([][]byte, len(keys))
	found := make([]bool, len(keys))
	if len(keys) == 0 {
		return values, found, nil
	}
	ph := make([]string, len(keys))
	args := make([]any, 0, len(keys)+1)
	for i, k := range keys {
		ph[i] = "?"
		args = append(args, k)
	}
	args = append(args, r.t.nowMs())
	rows, err := r.t.tx.QueryContext(ctx,
		fmt.Sprintf(`SELECT s."key", s.value FROM kv.strings s
		 JOIN kv.keys k ON k."key" = s."key"
		 WHERE s."key" IN (%s) AND (k.etime IS NULL OR k.etime > ?)`, strings.Join(ph, ",")),
		args...)
	if err != nil {
		return nil, nil, fmt.Errorf("kv: string get many: %w", err)
	}
	defer rows.Close()
	idx := make(map[string]int, len(keys))
	for i, k := range keys {
		if _, dup := idx[k]; !dup {
			idx[k] = i
		}
	}
	for rows.Next() {
		var k string
		var v []byte
		if err := rows.Scan(&k, &v); err != nil {
			return nil, nil, fmt.Errorf("kv: string get many row: %w", err)
		}
		if i, ok := idx[k]; ok {
			values[i] = v
			found[i] = true
		}
	}
	return values, found, rows.Err()
}

// SetMany 批量写入（同事务；全部清除 TTL，对齐 MSET 语义）。
func (r *StringRepo) SetMany(ctx context.Context, items map[string][]byte) error {
	for k, v := range items {
		if err := r.Set(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}

// Incr 原子自增 delta（可为负）。值必须是十进制整数，否则 ErrValueType；
// 结果溢出 int64 返回 ErrValueType（对齐 Redis）。保留原 TTL。
func (r *StringRepo) Incr(ctx context.Context, key string, delta int64) (int64, error) {
	kr := r.t.Key()
	exists, err := kr.guardWrite(ctx, key, TypeString)
	if err != nil {
		return 0, err
	}
	var cur int64
	if exists {
		v, err := r.Get(ctx, key)
		if err != nil {
			return 0, err
		}
		cur, err = strconv.ParseInt(strings.TrimSpace(string(v)), 10, 64)
		if err != nil {
			return 0, ErrValueType
		}
	}
	if (delta > 0 && cur > math.MaxInt64-delta) || (delta < 0 && cur < math.MinInt64-delta) {
		return 0, ErrValueType
	}
	next := cur + delta
	return next, r.writeNumeric(ctx, key, []byte(strconv.FormatInt(next, 10)), exists)
}

// IncrFloat 原子自增浮点 delta。值必须是浮点数，否则 ErrValueType。保留原 TTL。
func (r *StringRepo) IncrFloat(ctx context.Context, key string, delta float64) (float64, error) {
	kr := r.t.Key()
	exists, err := kr.guardWrite(ctx, key, TypeString)
	if err != nil {
		return 0, err
	}
	var cur float64
	if exists {
		v, err := r.Get(ctx, key)
		if err != nil {
			return 0, err
		}
		cur, err = strconv.ParseFloat(strings.TrimSpace(string(v)), 64)
		if err != nil {
			return 0, ErrValueType
		}
	}
	next := cur + delta
	if math.IsNaN(next) || math.IsInf(next, 0) {
		return 0, ErrValueType
	}
	return next, r.writeNumeric(ctx, key, []byte(strconv.FormatFloat(next, 'f', -1, 64)), exists)
}

// writeNumeric 回写数值内容，保留原 TTL（Incr 系不重置过期）。
func (r *StringRepo) writeNumeric(ctx context.Context, key string, value []byte, exists bool) error {
	kr := r.t.Key()
	if exists {
		if _, err := r.t.tx.ExecContext(ctx,
			`UPDATE kv.keys SET version = version + 1, mtime = ? WHERE "key" = ?`,
			r.t.nowMs(), key); err != nil {
			return fmt.Errorf("kv: string incr meta: %w", err)
		}
		if _, err := r.t.tx.ExecContext(ctx,
			`UPDATE kv.strings SET value = ? WHERE "key" = ?`, value, key); err != nil {
			return fmt.Errorf("kv: string incr value: %w", err)
		}
		return nil
	}
	if err := kr.insertKey(ctx, key, TypeString, nil, nil); err != nil {
		return err
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`INSERT INTO kv.strings ("key", value) VALUES (?, ?)`, key, value); err != nil {
		return fmt.Errorf("kv: string incr insert: %w", err)
	}
	return nil
}
