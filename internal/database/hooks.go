package database

import (
	"context"
	"database/sql"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// WriteHookFactory 可选扩展：Factory 若实现此接口，Registry 会在写成功 / 关闭前回调。
type WriteHookFactory interface {
	AfterWrite(ctx context.Context, db catalog.Database, sqlDB *sql.DB) error
	BeforeClose(ctx context.Context, dbID string, sqlDB *sql.DB) error
}
