// 文件说明：errmatch.go — godatabase 中 IsMissingRelation、IsTransactionConflict 等符号的实现
//
// 用法
//
//	IsMissingRelation(err error) bool  // IsMissingRelation 判断错误是否为「表不存在」（DuckDB / SQLite）
//	IsTransactionConflict(err error) bool  // IsTransactionConflict 判断错误是否为 DuckLake 乐观并发冲突（提交失败，语句未生效）
//
// 调用架构
//
//	[上游] godatabase/cache, godatabase/cmd, godatabase/ducklake, godatabase/filedb 等 16 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/errmatch.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] coverage_test.go, dialect.go, query.go, query_test.go, runtime.go 等 10 个
//
// 主要符号
//
//	IsMissingRelation(err error) bool
//	IsTransactionConflict(err error) bool

package godatabase

import "strings"

// IsMissingRelation 判断错误是否为「表不存在」（DuckDB / SQLite）。
func IsMissingRelation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such table")
}

// IsTransactionConflict 判断错误是否为 DuckLake 乐观并发冲突（提交失败，语句未生效）。
func IsTransactionConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Transaction conflict")
}
