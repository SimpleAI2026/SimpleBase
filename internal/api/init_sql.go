// 文件说明：init_sql.go — internal/api 的内部实现（maxInitSQLBytes、maxInitSQLStatements 等）
//
// 用法
//
//	同包调用：prepareInitSQL() → database_handler.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/init_sql.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/sqlguard
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] prepareInitSQL() ← database_handler.go
//
// 主要符号
//
//	const maxInitSQLBytes
//	const maxInitSQLStatements
//	prepareInitSQL(raw string) ([]string, error)

package api

import (
	"errors"
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
)

const (
	maxInitSQLBytes      = 64 * 1024
	maxInitSQLStatements = 100
)

// prepareInitSQL 拆开并校验创建 SQL 库时的初始化脚本。
// 空脚本返回 nil。注释-only 片段被跳过。失败时还没有建库。
func prepareInitSQL(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if len(raw) > maxInitSQLBytes {
		return nil, fmt.Errorf("%w: init sql exceeds %d bytes", sqlguard.ErrSQLNotAllowed, maxInitSQLBytes)
	}
	parts, err := sqlguard.SplitStatements(raw)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if err := sqlguard.Validate(part, sqlguard.WriteAllowed); err != nil {
			if errors.Is(err, sqlguard.ErrEmptySQL) {
				continue
			}
			return nil, err
		}
		switch sqlguard.FirstKeyword(part) {
		case "CREATE", "ALTER", "INSERT":
			out = append(out, part)
		default:
			return nil, fmt.Errorf("%w: init sql allows CREATE, ALTER, INSERT", sqlguard.ErrSQLNotAllowed)
		}
	}
	if len(out) > maxInitSQLStatements {
		return nil, fmt.Errorf("%w: too many init statements", sqlguard.ErrSQLNotAllowed)
	}
	return out, nil
}
