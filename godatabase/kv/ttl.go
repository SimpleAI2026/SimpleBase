// 文件说明：ttl.go — godatabase/kv 中 Tx.DeleteExpired、Store.HasExpired 等符号的实现
//
// 用法
//
//	(t *Tx) DeleteExpired(ctx context.Context) (int64, error)  // DeleteExpired 批量删除所有已过期 key（后台 TTL 清扫用）
//	(s *Store) HasExpired(ctx context.Context) (bool, error)  // HasExpired 报告是否存在已过期 key（只读探测，K2：Sweeper 据此决定是否取写租约）
//
// 调用架构
//
//	[上游] godatabase/cmd, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/kv/ttl.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bugfix_test.go, collections_test.go, conn_schema_test.go, core.go, executor.go 等 20 个
//
// 主要符号
//
//	(t *Tx) DeleteExpired(ctx context.Context) (int64, error)
//	keysToArgs(keys []string) []any
//	const maxExpiredPerSweep
//	(s *Store) HasExpired(ctx context.Context) (bool, error)

package kv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DeleteExpired 批量删除所有已过期 key（后台 TTL 清扫用）。
// 同事务内删除各类型数据表行 + __sb_kv_keys 元数据行（外键级联的应用层替代）。
// 每张数据表一条 DELETE（按过期元数据子查询），单轮上限 maxExpiredPerSweep，
// 避免长事务；返回实际删除的 key 数，无过期 key 时不产生任何写。
func (t *Tx) DeleteExpired(ctx context.Context) (int64, error) {
	// B17：先用同一时间快照扫描并固定本轮 victim 集合，再按显式 key 集合
	// 删除——旧实现每条 DELETE 重取时间并重算无 ORDER BY 的 LIMIT 子查询，
	// victim 会随时间/引擎计划漂移，导致元数据与数据行删除集合不一致。
	now := t.nowMs()
	rows, err := t.tx.QueryContext(ctx,
		`SELECT "key", "type" FROM __sb_kv_keys WHERE etime IS NOT NULL AND etime <= ? LIMIT ?`,
		now, maxExpiredPerSweep)
	if err != nil {
		return 0, fmt.Errorf("kv: delete expired scan: %w", err)
	}
	type victim struct {
		key   string
		table string
	}
	var victims []victim
	tableKeys := map[string][]string{}
	for rows.Next() {
		var key string
		var typ int16
		if err := rows.Scan(&key, &typ); err != nil {
			rows.Close()
			return 0, fmt.Errorf("kv: delete expired row: %w", err)
		}
		table, err := dataTableFor(TypeID(typ))
		if err != nil {
			rows.Close()
			return 0, err
		}
		victims = append(victims, victim{key: key, table: table})
		tableKeys[table] = append(tableKeys[table], key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if len(victims) == 0 {
		return 0, nil
	}
	// 各类型数据表按显式 victim key 集合删除，与元数据删除严格同一集合。
	for table, keys := range tableKeys {
		if _, err := t.tx.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE "key" IN (%s)`, table, placeholders(len(keys))),
			keysToArgs(keys)...); err != nil {
			return 0, fmt.Errorf("kv: delete expired data rows: %w", err)
		}
	}
	allKeys := make([]any, len(victims))
	for i, v := range victims {
		allKeys[i] = v.key
	}
	res, err := t.tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM __sb_kv_keys WHERE "key" IN (%s)`, placeholders(len(victims))),
		allKeys...)
	if err != nil {
		return 0, fmt.Errorf("kv: delete expired meta: %w", err)
	}
	count := int64(len(victims))
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		count = n
	}
	return count, nil
}

func keysToArgs(keys []string) []any {
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	return args
}

// maxExpiredPerSweep 单轮 TTL 清扫删除的 key 上限。
const maxExpiredPerSweep = 1000

// HasExpired 报告是否存在已过期 key（只读探测，K2：Sweeper 据此决定是否取写租约）。
// 调用方应先 HasSchema 短路空库。
func (s *Store) HasExpired(ctx context.Context) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM __sb_kv_keys WHERE etime IS NOT NULL AND etime <= ? LIMIT 1`,
		s.now().UnixMilli()).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("kv: probe expired: %w", err)
	}
	return true, nil
}
