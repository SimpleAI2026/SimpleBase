package kv

import (
	"context"
	"fmt"
)

// DeleteExpired 批量删除所有已过期 key（后台 TTL 清扫用）。
// 同事务内删除各类型数据表行 + kv.keys 元数据行（外键级联的应用层替代）。
// 返回实际删除的 key 数；无过期 key 时不产生任何写。
func (t *Tx) DeleteExpired(ctx context.Context) (int64, error) {
	rows, err := t.tx.QueryContext(ctx,
		`SELECT "key", "type" FROM kv.keys WHERE etime IS NOT NULL AND etime <= ?`, t.nowMs())
	if err != nil {
		return 0, fmt.Errorf("kv: delete expired scan: %w", err)
	}
	type victim struct {
		key string
		typ TypeID
	}
	var victims []victim
	for rows.Next() {
		var v victim
		var typ int16
		if err := rows.Scan(&v.key, &typ); err != nil {
			rows.Close()
			return 0, fmt.Errorf("kv: delete expired row: %w", err)
		}
		v.typ = TypeID(typ)
		victims = append(victims, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if len(victims) == 0 {
		return 0, nil
	}
	kr := t.Key()
	for _, v := range victims {
		if err := kr.deleteCascade(ctx, v.key, v.typ); err != nil {
			return 0, err
		}
	}
	return int64(len(victims)), nil
}
