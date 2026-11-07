// 文件说明：ttl.go — godatabase/kv 中 DeleteExpired 等符号的实现
//
// 用法
//
//	(t *Tx) DeleteExpired(ctx context.Context) (int64, error)  // DeleteExpired 批量删除所有已过期 key（后台 TTL 清扫用）
//	同包调用：DeleteExpired() → sweeper.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/kv/ttl.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] core.go, executor.go, hash.go, key.go, list.go 等 10 个
//	[关键引用] DeleteExpired() ← sweeper.go
//
// 主要符号
//
//	(t *Tx) DeleteExpired(ctx context.Context) (int64, error)

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
	// 先锁定本轮 victim 上限（K2：单轮 ≤1000，防止过期 key 积压时拖出长事务）。
	rows, err := t.tx.QueryContext(ctx,
		`SELECT "key", "type" FROM __sb_kv_keys WHERE etime IS NOT NULL AND etime <= ? LIMIT ?`,
		t.nowMs(), maxExpiredPerSweep)
	if err != nil {
		return 0, fmt.Errorf("kv: delete expired scan: %w", err)
	}
	var count int64
	var dataTables []string
	seen := map[string]bool{}
	for rows.Next() {
		var key string
		var typ int16
		if err := rows.Scan(&key, &typ); err != nil {
			rows.Close()
			return 0, fmt.Errorf("kv: delete expired row: %w", err)
		}
		count++
		table, err := dataTableFor(TypeID(typ))
		if err != nil {
			rows.Close()
			return 0, err
		}
		if !seen[table] {
			seen[table] = true
			dataTables = append(dataTables, table)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if count == 0 {
		return 0, nil
	}
	// 各类型数据表各一条批量删除：数据行以过期元数据子查询为准（带单轮
	// 上限，与扫描集合一致；同事务内无并发写入）。
	for _, table := range dataTables {
		if _, err := t.tx.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE "key" IN (
				SELECT "key" FROM __sb_kv_keys WHERE etime IS NOT NULL AND etime <= ? LIMIT ?)`, table),
			t.nowMs(), maxExpiredPerSweep); err != nil {
			return 0, fmt.Errorf("kv: delete expired data rows: %w", err)
		}
	}
	res, err := t.tx.ExecContext(ctx,
		`DELETE FROM __sb_kv_keys WHERE "key" IN (
			SELECT "key" FROM __sb_kv_keys WHERE etime IS NOT NULL AND etime <= ? LIMIT ?)`,
		t.nowMs(), maxExpiredPerSweep)
	if err != nil {
		return 0, fmt.Errorf("kv: delete expired meta: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		count = n
	}
	return count, nil
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
