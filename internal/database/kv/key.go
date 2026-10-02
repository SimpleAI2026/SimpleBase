package kv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// KeyRepo 是 key 元数据仓库（事务版，由 Tx.Key() 返回）。
type KeyRepo struct{ t *Tx }

// —— 内部辅助（供各类型仓库复用，DuckLake 无触发器/外键的应用层补偿）——

// getMeta 读取 key 元数据；过期或不存在返回 ErrNotFound。
func (r *KeyRepo) getMeta(ctx context.Context, key string) (KeyMeta, error) {
	var m KeyMeta
	var typ int16
	err := r.t.tx.QueryRowContext(ctx,
		`SELECT "key", "type", version, etime, mtime, len FROM kv.keys
		 WHERE "key" = ? AND (etime IS NULL OR etime > ?)`,
		key, r.t.nowMs(),
	).Scan(&m.Key, &typ, &m.Version, &m.Etime, &m.Mtime, &m.Len)
	if errors.Is(err, sql.ErrNoRows) {
		return KeyMeta{}, ErrNotFound
	}
	if err != nil {
		return KeyMeta{}, fmt.Errorf("kv: get key meta: %w", err)
	}
	m.Type = TypeID(typ)
	return m, nil
}

// getMetaAny 读取 key 元数据（含已过期的）；不存在返回 ErrNotFound。
func (r *KeyRepo) getMetaAny(ctx context.Context, key string) (KeyMeta, error) {
	var m KeyMeta
	var typ int16
	err := r.t.tx.QueryRowContext(ctx,
		`SELECT "key", "type", version, etime, mtime, len FROM kv.keys WHERE "key" = ?`,
		key,
	).Scan(&m.Key, &typ, &m.Version, &m.Etime, &m.Mtime, &m.Len)
	if errors.Is(err, sql.ErrNoRows) {
		return KeyMeta{}, ErrNotFound
	}
	if err != nil {
		return KeyMeta{}, fmt.Errorf("kv: get key meta: %w", err)
	}
	m.Type = TypeID(typ)
	return m, nil
}

// guardWrite 写前类型守卫（所有写路径第一步）：
//   - 不存在 → 返回 (false, nil)，调用方可按新 key 插入
//   - 存在且类型匹配 → (true, nil)
//   - 存在但类型不符 → ErrKeyType
//   - 已过期 → 级联删除后视为不存在 (false, nil)（Redis 惰性过期语义）
func (r *KeyRepo) guardWrite(ctx context.Context, key string, want TypeID) (bool, error) {
	m, err := r.getMetaAny(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if m.Etime != nil && *m.Etime <= r.t.nowMs() {
		if err := r.deleteCascade(ctx, key, m.Type); err != nil {
			return false, err
		}
		return false, nil
	}
	if m.Type != want {
		return false, ErrKeyType
	}
	return true, nil
}

// getMetaOf 读取 key 元数据并校验类型；类型不符返回 ErrKeyType（Redis WRONGTYPE），
// 不存在/已过期返回 ErrNotFound。供各类型仓库的读路径使用。
func (r *KeyRepo) getMetaOf(ctx context.Context, key string, want TypeID) (KeyMeta, error) {
	m, err := r.getMeta(ctx, key)
	if err != nil {
		return KeyMeta{}, err
	}
	if m.Type != want {
		return KeyMeta{}, ErrKeyType
	}
	return m, nil
}

// insertKey 插入新 key 元数据行（version=1）。initialLen 仅集合类型有意义。
func (r *KeyRepo) insertKey(ctx context.Context, key string, typ TypeID, etime *int64, initialLen *int64) error {
	_, err := r.t.tx.ExecContext(ctx,
		`INSERT INTO kv.keys ("key", "type", version, etime, mtime, len) VALUES (?, ?, 1, ?, ?, ?)`,
		key, int16(typ), etime, r.t.nowMs(), initialLen)
	if err != nil {
		return fmt.Errorf("kv: insert key: %w", err)
	}
	return nil
}

// touchAfterWrite 元素写后维护元数据（触发器的应用层替代）：
// version+1、mtime 更新、len += lenDelta（lenDelta 可为 0/负）。
func (r *KeyRepo) touchAfterWrite(ctx context.Context, key string, lenDelta int64) error {
	_, err := r.t.tx.ExecContext(ctx,
		`UPDATE kv.keys SET version = version + 1, mtime = ?, len = COALESCE(len, 0) + ?
		 WHERE "key" = ?`,
		r.t.nowMs(), lenDelta, key)
	if err != nil {
		return fmt.Errorf("kv: touch key meta: %w", err)
	}
	return nil
}

// deleteCascade 删除 key 及其数据表所有行（外键 ON DELETE CASCADE 的应用层替代）。
func (r *KeyRepo) deleteCascade(ctx context.Context, key string, typ TypeID) error {
	table, err := dataTableFor(typ)
	if err != nil {
		return err
	}
	if _, err := r.t.tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM %s WHERE "key" = ?`, table), key); err != nil {
		return fmt.Errorf("kv: delete key data: %w", err)
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`DELETE FROM kv.keys WHERE "key" = ?`, key); err != nil {
		return fmt.Errorf("kv: delete key meta: %w", err)
	}
	return nil
}

// deleteIfEmpty 集合类型元素删空后删除 key 行（Redis 语义：空结构不存在）。
func (r *KeyRepo) deleteIfEmpty(ctx context.Context, key string) (bool, error) {
	m, err := r.getMeta(ctx, key)
	if err != nil {
		return false, err
	}
	if m.Len == nil || *m.Len > 0 {
		return false, nil
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`DELETE FROM kv.keys WHERE "key" = ?`, key); err != nil {
		return false, fmt.Errorf("kv: delete empty key: %w", err)
	}
	return true, nil
}

// —— 对外方法 ——

// Get 返回 key 元数据；不存在或已过期返回 ErrNotFound。
func (r *KeyRepo) Get(ctx context.Context, key string) (KeyMeta, error) {
	return r.getMeta(ctx, key)
}

// Count 返回未过期 key 总数（可选 glob pattern 过滤）。
func (r *KeyRepo) Count(ctx context.Context, pattern string) (int64, error) {
	if pattern == "" {
		pattern = "*"
	}
	var n int64
	err := r.t.tx.QueryRowContext(ctx,
		`SELECT count(*) FROM kv.keys
		 WHERE "key" GLOB ? AND (etime IS NULL OR etime > ?)`,
		pattern, r.t.nowMs()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("kv: count keys: %w", err)
	}
	return n, nil
}

// Delete 删除多个 key，返回实际删除数（不存在/已过期的 key 不计）。
func (r *KeyRepo) Delete(ctx context.Context, keys ...string) (int64, error) {
	var n int64
	for _, key := range keys {
		m, err := r.getMetaAny(ctx, key)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return n, err
		}
		if m.Etime != nil && *m.Etime <= r.t.nowMs() {
			// 已过期：物理清除但按 Redis 语义不计入删除数。
			if err := r.deleteCascade(ctx, key, m.Type); err != nil {
				return n, err
			}
			continue
		}
		if err := r.deleteCascade(ctx, key, m.Type); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Expire 设置相对过期时间（毫秒）。key 不存在/已过期返回 false。
// ttlMs <= 0 立即过期（Redis EXPIRE 非正数语义：等价删除）。
func (r *KeyRepo) Expire(ctx context.Context, key string, ttlMs int64) (bool, error) {
	return r.ExpireAt(ctx, key, r.t.nowMs()+ttlMs)
}

// ExpireAt 设置绝对过期时间（unix 毫秒）。
func (r *KeyRepo) ExpireAt(ctx context.Context, key string, atMs int64) (bool, error) {
	if _, err := r.getMeta(ctx, key); err != nil {
		return false, err
	}
	if atMs <= r.t.nowMs() {
		m, err := r.getMetaAny(ctx, key)
		if err != nil {
			return false, err
		}
		if err := r.deleteCascade(ctx, key, m.Type); err != nil {
			return false, err
		}
		return true, nil
	}
	res, err := r.t.tx.ExecContext(ctx,
		`UPDATE kv.keys SET etime = ?, version = version + 1, mtime = ? WHERE "key" = ?`,
		atMs, r.t.nowMs(), key)
	if err != nil {
		return false, fmt.Errorf("kv: expire key: %w", err)
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// Persist 移除过期时间。key 不存在或无 TTL 返回 false。
func (r *KeyRepo) Persist(ctx context.Context, key string) (bool, error) {
	if _, err := r.getMeta(ctx, key); err != nil {
		return false, err
	}
	res, err := r.t.tx.ExecContext(ctx,
		`UPDATE kv.keys SET etime = NULL, version = version + 1, mtime = ?
		 WHERE "key" = ? AND etime IS NOT NULL`,
		r.t.nowMs(), key)
	if err != nil {
		return false, fmt.Errorf("kv: persist key: %w", err)
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// Rename 重命名 key（目标已存在则覆盖）。源不存在返回 ErrNotFound。
func (r *KeyRepo) Rename(ctx context.Context, oldKey, newKey string) error {
	return r.rename(ctx, oldKey, newKey, false)
}

// RenameNX 仅当目标不存在时重命名；目标已存在返回 ErrKeyExists。
func (r *KeyRepo) RenameNX(ctx context.Context, oldKey, newKey string) error {
	return r.rename(ctx, oldKey, newKey, true)
}

func (r *KeyRepo) rename(ctx context.Context, oldKey, newKey string, notExists bool) error {
	if oldKey == newKey {
		if _, err := r.getMeta(ctx, oldKey); err != nil {
			return err
		}
		if notExists {
			return ErrKeyExists
		}
		return nil
	}
	src, err := r.getMeta(ctx, oldKey)
	if err != nil {
		return err
	}
	dst, err := r.getMetaAny(ctx, newKey)
	if err == nil {
		dstExpired := dst.Etime != nil && *dst.Etime <= r.t.nowMs()
		if notExists && !dstExpired {
			return ErrKeyExists
		}
		if err := r.deleteCascade(ctx, newKey, dst.Type); err != nil {
			return err
		}
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	table, err := dataTableFor(src.Type)
	if err != nil {
		return err
	}
	if _, err := r.t.tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE %s SET "key" = ? WHERE "key" = ?`, table), newKey, oldKey); err != nil {
		return fmt.Errorf("kv: rename key data: %w", err)
	}
	if _, err := r.t.tx.ExecContext(ctx,
		`UPDATE kv.keys SET "key" = ?, version = version + 1, mtime = ? WHERE "key" = ?`,
		newKey, r.t.nowMs(), oldKey); err != nil {
		return fmt.Errorf("kv: rename key meta: %w", err)
	}
	return nil
}

// Scan 游标式扫描 key（字典序；cursor 为上一页最后一个 key，空串从头开始）。
// pattern 为 glob 模式（空串 = "*"）；typeFilter 传 TypeAll 表示不过滤。
// 返回一页元数据与下一游标（空串表示扫完）。count<=0 时取默认 100。
func (r *KeyRepo) Scan(ctx context.Context, cursor, pattern string, typeFilter TypeID, count int) ([]KeyMeta, string, error) {
	if pattern == "" {
		pattern = "*"
	}
	if count <= 0 {
		count = 100
	}
	rows, err := r.t.tx.QueryContext(ctx,
		`SELECT "key", "type", version, etime, mtime, len FROM kv.keys
		 WHERE "key" > ? AND "key" GLOB ?
		   AND (? <= 0 OR "type" = ?)
		   AND (etime IS NULL OR etime > ?)
		 ORDER BY "key" LIMIT ?`,
		cursor, pattern, int16(typeFilter), int16(typeFilter), r.t.nowMs(), count+1)
	if err != nil {
		return nil, "", fmt.Errorf("kv: scan keys: %w", err)
	}
	defer rows.Close()

	out := make([]KeyMeta, 0, count)
	for rows.Next() {
		var m KeyMeta
		var typ int16
		if err := rows.Scan(&m.Key, &typ, &m.Version, &m.Etime, &m.Mtime, &m.Len); err != nil {
			return nil, "", fmt.Errorf("kv: scan keys row: %w", err)
		}
		m.Type = TypeID(typ)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > count {
		out = out[:count]
		next = out[len(out)-1].Key
	}
	return out, next, nil
}

// —— 测试/调试辅助 ——

// rawCount 返回 kv.keys 物理行数（含已过期；仅供测试断言惰性删除）。
func (r *KeyRepo) rawCount(ctx context.Context) (int64, error) {
	var n int64
	err := r.t.tx.QueryRowContext(ctx, `SELECT count(*) FROM kv.keys`).Scan(&n)
	return n, err
}
