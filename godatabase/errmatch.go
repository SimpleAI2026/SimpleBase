// 文件说明：errmatch.go — godatabase 根包中驱动错误文本匹配的唯一收敛处
//
// 用法
//
//	IsMissingRelation(err error) bool      // DuckDB "does not exist" / SQLite "no such table"
//	IsTransactionConflict(err error) bool  // DuckLake 乐观并发 "Transaction conflict"
//	同包调用：无；下游 kv、ducklake 各自调用导出函数
//
// 调用架构
//
//	[上游] godatabase/kv, godatabase/ducklake
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/errmatch.go
//	         │
//	         ▼
//	[下游] 仅标准库
//
// 主要符号
//
//	IsMissingRelation(err error) bool
//	IsTransactionConflict(err error) bool
//
// 说明（C5 收敛）：
// DuckDB / SQLite / DuckLake 的 go 驱动当前都不导出结构化错误类型，
// 只能按错误文本匹配。所有文本匹配集中在此文件并标注驱动版本，
// 驱动升级暴露类型后改为 errors.As / errors.Is。
//
//	duckdb-go v2（github.com/duckdb/duckdb-go/v2）：
//	  - 缺表："... does not exist"（DuckDB 标准报错）
//	  - 乐观并发："Transaction conflict"（DuckLake catalog 提交冲突）
//	mattn/go-sqlite3：
//	  - 缺表："no such table: X"
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
