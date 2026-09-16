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
