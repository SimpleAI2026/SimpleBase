// 文件说明：snapshot.go — godatabase 中 SnapshotInfo 等符号的实现
//
// 用法
//
//	type SnapshotInfo（导出类型，同包与上层均可使用）
//
// 调用架构
//
//	[上游] godatabase/cache, godatabase/ducklake, godatabase/filedb 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/snapshot.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] dialect.go, query.go, runtime.go, serialize.go, spec.go 等 6 个
//
// 主要符号
//
//	type SnapshotInfo struct {…}

package godatabase

import "time"

type SnapshotInfo struct {
	Engine      string    `json:"engine"`
	FormatVer   int       `json:"format_version"`
	SizeBytes   int64     `json:"size_bytes"`
	SHA256      string    `json:"sha256"`
	CompletedAt time.Time `json:"completed_at"`
}
