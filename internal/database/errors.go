package database

import "errors"

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
