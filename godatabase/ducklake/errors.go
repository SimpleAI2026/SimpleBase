// 文件说明：errors.go — godatabase/ducklake 中 ErrCatalogEngineMismatch、ErrCatalogEngineUnknown 等符号的实现
//
// 用法
//
//	var ErrCatalogEngineMismatch  // ErrCatalogEngineMismatch 表示配置的 catalog 引擎与已有数据（实例标记 / manifest / 本地文件）不一致（duckla...
//	var ErrCatalogEngineUnknown  // ErrCatalogEngineUnknown 表示存在历史数据但没有实例级格式标记， 无法判定其 catalog 引擎（§4.2）
//
// 调用架构
//
//	[上游] internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/errors.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] c4_syncer_lease_test.go, c4_syncer_ordering_test.go, catalog_syncer.go, catalog_syncer_test.go, coverage_test.go 等 35 个
//
// 主要符号
//
//	var ErrCatalogEngineMismatch
//	var ErrCatalogEngineUnknown

package ducklake

import "errors"

// ErrCatalogEngineMismatch 表示配置的 catalog 引擎与已有数据（实例标记 /
// manifest / 本地文件）不一致（ducklake-duckdb-catalog-plan §4.5）。
// 引擎切换不受支持；唯一途径是 reset 清空全部数据。
var ErrCatalogEngineMismatch = errors.New("ducklake: catalog engine mismatch")

// ErrCatalogEngineUnknown 表示存在历史数据但没有实例级格式标记，
// 无法判定其 catalog 引擎（§4.2）。同样要求先 reset。
var ErrCatalogEngineUnknown = errors.New("ducklake: catalog engine of existing data is unknown (missing instance-format marker)")
