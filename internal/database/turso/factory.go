package turso

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// OpenFunc 是 sql.Open 的抽象，便于测试注入 mock 驱动。
// 生产环境使用默认 openFunc（指向 sql.Open）。
type OpenFunc func(driverName, dsn string) (*sql.DB, error)

// defaultOpen 调用标准库 sql.Open。
func defaultOpen(driverName, dsn string) (*sql.DB, error) {
	return sql.Open(driverName, dsn)
}

// PoolOptions 控制 *sql.DB 连接池参数。
type PoolOptions struct {
	MaxOpen     int           // MaxOpenConns；<=0 时默认 8
	MaxIdle     int           // MaxIdleConns；<0 时默认 2
	MaxIdleTime time.Duration // ConnMaxIdleTime；<=0 时默认 5m
}

// defaults 填充零值为合理默认。
func (p PoolOptions) defaults() PoolOptions {
	if p.MaxOpen <= 0 {
		p.MaxOpen = 8
	}
	// MaxIdle 零值与负值都使用默认；显式传 0 无实际意义（池不会保留空闲连接）。
	if p.MaxIdle <= 0 {
		p.MaxIdle = 2
	}
	if p.MaxIdleTime <= 0 {
		p.MaxIdleTime = 5 * time.Minute
	}
	return p
}

// Open 构建 DSN 并打开一个 *sql.DB，设置连接池后做一次 PingContext。
//
// 失败时关闭已创建的 *sql.DB，避免句柄泄漏。
// 错误中绝不包含 DSN、AuthToken 或 S3 凭据（使用 redactDSN）。
//
// 参数：
//   - opts: 数据库打开选项（DatabaseID/CachePath/Storage/Writable）
//   - pool: 连接池参数
//   - logger: 可选，用于记录打开结果（不记录敏感字段）
//   - metrics: 可选，用于记录打开延迟
//   - openFn: 测试可注入 mock；生产传 nil 使用 sql.Open
func Open(ctx context.Context, opts OpenOptions, pool PoolOptions,
	logger observability.Logger, metrics *observability.Metrics,
	openFn OpenFunc) (*sql.DB, error) {

	if openFn == nil {
		openFn = defaultOpen
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	dsn, err := BuildDSN(opts)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	db, err := openFn(DriverName, dsn)
	if err != nil {
		recordOpen(metrics, "error")
		return nil, fmt.Errorf("turso: open driver %s: %w (dsn=%s)",
			DriverName, err, redactDSN(dsn))
	}

	p := pool.defaults()
	db.SetMaxOpenConns(p.MaxOpen)
	db.SetMaxIdleConns(p.MaxIdle)
	db.SetConnMaxIdleTime(p.MaxIdleTime)

	// PingContext 验证连接可用；超时由调用方 ctx 控制。
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		recordOpen(metrics, "error")
		return nil, fmt.Errorf("turso: ping database %s: %w (dsn=%s)",
			opts.DatabaseID, err, redactDSN(dsn))
	}

	recordOpen(metrics, "ok")
	if metrics != nil {
		metrics.DBOpenSeconds.WithLabelValues("ok").Observe(time.Since(start).Seconds())
	}
	if logger != nil {
		logger.Info("turso database opened",
			zap.String("database_id", opts.DatabaseID),
			zap.Bool("writable", opts.Writable),
			zap.String("bucket", opts.Storage.Bucket),
			zap.String("region", opts.Storage.Region),
			// 不记录 prefix 全路径、endpoint、dsn、token
		)
	}
	return db, nil
}

// Close 安全关闭 *sql.DB，忽略已关闭错误。
// sql.DB.Close 对已关闭的池不返回标准可识别错误，这里保守忽略 nil 之外的"已关闭"情形。
func Close(db *sql.DB, logger observability.Logger) error {
	if db == nil {
		return nil
	}
	err := db.Close()
	if err != nil {
		// sql.DB.Close 在已关闭时可能返回 nil 或驱动特定错误；
		// 无法用 errors.Is 精确判别，保守返回非 nil 错误供调用方记录。
		return err
	}
	if logger != nil {
		logger.Info("turso database closed")
	}
	return nil
}

func recordOpen(metrics *observability.Metrics, outcome string) {
	if metrics == nil {
		return
	}
	// DBOpenSeconds 是 HistogramVec，Observe 需要浮点值；
	// 这里在调用点已 Observe 延迟，此函数仅用于未来扩展计数器。
	_ = outcome
}
