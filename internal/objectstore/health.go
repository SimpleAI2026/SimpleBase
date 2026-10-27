// 文件说明：health.go — internal/objectstore 中 HealthChecker、AssertHealth 等符号的实现
//
// 用法
//
//	AssertHealth(ctx context.Context, c Client) error  // AssertHealth 对 objectstore 做最小连通与权限验证
//	type HealthChecker（导出类型，同包与上层均可使用）
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/objectstore/health.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] blob.go, client.go, content.go, content_local.go, content_s3.go 等 10 个
//
// 主要符号
//
//	type HealthChecker interface {…}
//	AssertHealth(ctx context.Context, c Client) error

package objectstore

import (
	"context"
	"errors"
)

// HealthChecker 是 objectstore 的健康检查抽象。
// api.HealthChecker.Ready 在 Writable 模式下调用 Check 验证 S3 可达。
type HealthChecker interface {
	Check(ctx context.Context) error
}

// AssertHealth 对 objectstore 做最小连通与权限验证。
// 失败时返回 sanitized 错误；调用方据此将实例标记为 not-ready。
//
// 注意：Check 只做最小 Head/List 验证，不能枚举其他 tenant 的对象。
func AssertHealth(ctx context.Context, c Client) error {
	if c == nil {
		return errors.New("objectstore: client is nil")
	}
	return c.Check(ctx)
}
