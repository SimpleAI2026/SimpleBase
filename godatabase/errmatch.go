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
