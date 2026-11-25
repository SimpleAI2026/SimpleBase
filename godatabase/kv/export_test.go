// 文件说明：export_test.go — godatabase/kv 的测试辅助代码
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] godatabase/cmd, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/kv/export_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bugfix_test.go, collections_test.go, conn_schema_test.go, core.go, executor.go 等 20 个
//	[关键引用] WithNowFunc ← kv_test.go
//
// 主要符号
//
//	WithNowFunc(fn func() time.Time) Option
//	(r *KeyRepo) rawCount(ctx context.Context) (int64, error)

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
