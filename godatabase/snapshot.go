// 文件说明：snapshot.go — godatabase 中 SnapshotInfo 等符号的实现
//
// 用法
//
//	type SnapshotInfo struct{…}
//
// 调用架构
//
//	[上游] godatabase/cache, godatabase/cmd, godatabase/ducklake, godatabase/filedb 等 16 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/snapshot.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] coverage_test.go, dialect.go, errmatch.go, query.go, query_test.go 等 10 个
//
// 主要符号
//
//	type SnapshotInfo struct{…}

package godatabase

import "time"

type SnapshotInfo struct {
	Engine      string    `json:"engine"`
	FormatVer   int       `json:"format_version"`
	SizeBytes   int64     `json:"size_bytes"`
	SHA256      string    `json:"sha256"`
	CompletedAt time.Time `json:"completed_at"`
}
