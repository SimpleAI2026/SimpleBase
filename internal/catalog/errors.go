// 文件说明：errors.go — internal/catalog 中 ErrNotFound、ErrAlreadyExists 等符号的实现
//
// 用法
//
//	IsNotFound(err error) bool  // IsNotFound 判断错误是否为 ErrNotFound（含 wrapped）
//	IsAlreadyExists(err error) bool  // IsAlreadyExists 判断错误是否为 ErrAlreadyExists（含 wrapped）
//	同包调用：Error() → project_storage.go, service.go 等；ErrNotFound → repository.go, service.go 等；ErrInvalidName → project_storage.go, service.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/catalog/errors.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] cache.go, data_model.go, model.go, project_storage.go, repository.go 等 9 个
//	[关键引用] Error() ← project_storage.go, service.go, sql_repository.go
//
// 主要符号
//
//	var ErrNotFound
//	var ErrAlreadyExists
//	var ErrInvalidName
//	var ErrInvalidDatabasePath
//	var ErrInvalidCursor
//	var ErrInvalidState
//	... 另有 10 个顶层符号

package catalog

import (
	"errors"
	"fmt"
)

// Catalog 错误类型。handler 据此映射 HTTP 状态码（见 plan5.md）。
var (
	// ErrNotFound 资源不存在（404）。
	ErrNotFound = errors.New("catalog: not found")
	// ErrAlreadyExists 名称/ID 冲突（409）。
	ErrAlreadyExists = errors.New("catalog: already exists")
	// ErrInvalidName 名称不合法（400）。
	ErrInvalidName = errors.New("catalog: invalid name")
	// ErrInvalidDatabasePath 项目数据库存储元数据不合法（500）。
	ErrInvalidDatabasePath = errors.New("catalog: invalid database path")
	// ErrInvalidCursor 分页游标无效或不属于当前项目（400）。
	ErrInvalidCursor = errors.New("catalog: invalid cursor")
	// ErrInvalidState 非法状态转换（409）。
	ErrInvalidState = errors.New("catalog: invalid state transition")
	// ErrCrossProject 跨 project 访问（403）。
	ErrCrossProject = errors.New("catalog: cross project access denied")
	// ErrDescriptorWrite descriptor 写 S3 失败（503）。
	ErrDescriptorWrite = errors.New("catalog: descriptor write failed")
	// ErrMigrationFailed 迁移失败，server 不 ready（503）。
	ErrMigrationFailed = errors.New("catalog: migration failed")
	// ErrSystemProtected 禁止删除或改写系统库（403）。
	ErrSystemProtected = errors.New("catalog: system database is protected")
	// ErrInvalidDataModel 表示 data_model 不是 collection 或 sql（400）。
	ErrInvalidDataModel = errors.New("catalog: invalid data model")
)

// IsNotFound 判断错误是否为 ErrNotFound（含 wrapped）。
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyExists 判断错误是否为 ErrAlreadyExists（含 wrapped）。
func IsAlreadyExists(err error) bool {
	return errors.Is(err, ErrAlreadyExists)
}

// StateTransitionError 携带 from/to 上下文，便于审计与测试。
type StateTransitionError struct {
	From []DatabaseStatus
	To   DatabaseStatus
}

func (e *StateTransitionError) Error() string {
	froms := make([]string, 0, len(e.From))
	for _, f := range e.From {
		froms = append(froms, string(f))
	}
	return fmt.Sprintf("catalog: invalid state transition from %v to %s", froms, e.To)
}

func (e *StateTransitionError) Unwrap() error { return ErrInvalidState }
