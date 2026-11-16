package kv

import (
	"context"
	"time"
)

// WithNowFunc 注入时钟（测试用固定时钟验证 TTL）。默认 time.Now。
// 仅测试可见：生产装配不得替换时钟（TTL 过期判定依赖真实时间）。
func WithNowFunc(fn func() time.Time) Option {
	return func(s *Store) { s.now = fn }
}

// rawCount 返回 __sb_kv_keys 物理行数（含已过期；仅供测试断言惰性删除）。
// 仅测试可见：生产读路径必须走 HasSchema/Get 等语义入口，不得绕过过期过滤。
func (r *KeyRepo) rawCount(ctx context.Context) (int64, error) {
	var n int64
	err := r.t.tx.QueryRowContext(ctx, `SELECT count(*) FROM __sb_kv_keys`).Scan(&n)
	return n, err
}
