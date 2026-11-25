// 文件说明：runtime.go — godatabase 中 AccessMode、Factory 等符号的实现
//
// 用法
//
//	type AccessMode uint8  // AccessMode 表示对某个 logical database 的访问意图
//	type Factory interface{…}  // Factory 从 Spec 打开一个可用的 *sql.DB
//	type WriteHookFactory interface{…}  // WriteHookFactory 可选扩展：Factory 若实现此接口，Registry 会在写成功 / 关闭前回调
//
// 调用架构
//
//	[上游] godatabase/cache, godatabase/cmd, godatabase/ducklake, godatabase/filedb 等 16 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/runtime.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] coverage_test.go, dialect.go, errmatch.go, query.go, query_test.go 等 10 个
//
// 主要符号
//
//	type AccessMode uint8
//	const ReadOnly / ReadWrite
//	(m AccessMode) String() string
//	type Factory interface{…}
//	type WriteHookFactory interface{…}
//	var ErrDatabaseDeleting / ErrDatabaseNotReady / ErrWriterUnavailable / ErrRowLimitExceeded / ErrRegistryClosed / ErrUnsupportedValue / ErrQueryConcurrency

package godatabase

import (
	"context"
	"database/sql"
	"errors"
)

// AccessMode 表示对某个 logical database 的访问意图。
// 首期单实例架构下 ReadWrite/ReadOnly 都由同一个 writer 实例提供服务；
// 保留该参数是为未来引入只读副本预留边界，不代表当前具备只读扩展能力。
type AccessMode uint8

const (
	ReadOnly AccessMode = iota
	ReadWrite
)

// String 便于日志与指标输出。
func (m AccessMode) String() string {
	if m == ReadWrite {
		return "read_write"
	}
	return "read_only"
}

// Factory 从 Spec 打开一个可用的 *sql.DB。
// 实现者负责引擎引导、连接池配置和 Ping 验证；失败必须关闭已创建的资源。
type Factory interface {
	Open(ctx context.Context, db Spec, mode AccessMode) (*sql.DB, error)
}

// WriteHookFactory 可选扩展：Factory 若实现此接口，Registry 会在写成功 / 关闭前回调。
type WriteHookFactory interface {
	AfterWrite(ctx context.Context, db Spec, sqlDB *sql.DB) error
	BeforeClose(ctx context.Context, dbID string, sqlDB *sql.DB) error
}

// 运行时错误。handler 层负责映射为稳定的 HTTP 错误码（见 plan5/plan6）。
var (
	// ErrDatabaseDeleting 表示目标数据库处于 deleting/deleted，禁止新的 Acquire。
	ErrDatabaseDeleting = errors.New("database: target database is deleting or deleted")
	// ErrDatabaseNotReady 表示数据库当前状态不允许被访问（如 degraded 且非恢复路径）。
	ErrDatabaseNotReady = errors.New("database: target database is not ready")
	// ErrWriterUnavailable 表示本实例不是可写实例，禁止 ReadWrite 访问。
	ErrWriterUnavailable = errors.New("database: this instance is not writable")
	// ErrRowLimitExceeded 表示查询结果超过 maxRows，Query 必须停止扫描而非静默截断。
	ErrRowLimitExceeded = errors.New("database: row limit exceeded")
	// ErrRegistryClosed 表示 Registry 已开始关闭，不再接受新的 Acquire。
	ErrRegistryClosed = errors.New("database: registry is closed")
	// ErrUnsupportedValue 表示驱动返回的列值类型无法安全序列化。
	ErrUnsupportedValue = errors.New("database: unsupported column value type")
	// ErrQueryConcurrency 表示并发查询数超过上限。
	ErrQueryConcurrency = errors.New("database: query concurrency limit exceeded")
)
