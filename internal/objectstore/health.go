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
