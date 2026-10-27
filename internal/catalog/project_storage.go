// 文件说明：project_storage.go — internal/catalog 中 EngineDuckDB、EngineSQLite 等符号的实现
//
// 用法
//
//	NormalizeEngine(engine string) (string, error)
//	DatabasePathForProject(projectID, engine string) (string, error)
//	同包调用：DatabasePathForProject() → service.go, sql_repository.go；NormalizeEngine() → service.go；ValidateProjectDatabasePath() → service.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/catalog/project_storage.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase
//	[同包] cache.go, data_model.go, errors.go, model.go, repository.go 等 9 个
//	[关键引用] DatabasePathForProject() ← service.go, sql_repository.go
//
// 主要符号
//
//	const EngineDuckDB
//	const EngineSQLite
//	NormalizeEngine(engine string) (string, error)
//	DatabasePathForProject(projectID, engine string) (string, error)
//	DatabaseEngine(databasePath string) (string, error)
//	ValidateDatabasePath(databasePath string) error
//	... 另有 3 个顶层符号

package catalog

import (
	"errors"
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase"
)

const (
	EngineDuckDB = godatabase.EngineDuckDB
	EngineSQLite = godatabase.EngineSQLite
)

// 引擎与路径规则的唯一实现在 godatabase（数据库运行时库）。
// 这里保留 catalog 的错误语义：库返回 ErrInvalidPath，api 层靠
// errors.Is(err, ErrInvalidName) 映射 400，所以必须在此改包为 ErrInvalidName，
// 且错误文本保持 "catalog: invalid name: ..." 不变（project_storage.golden 逐条锁定）。

// wrapInvalid 把库的 ErrInvalidPath 转成 catalog 的 ErrInvalidName，文本逐字保留。
func wrapInvalid(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, godatabase.ErrInvalidPath) {
		return fmt.Errorf("%w%s", ErrInvalidName, strings.TrimPrefix(err.Error(), godatabase.ErrInvalidPath.Error()))
	}
	return err
}

func NormalizeEngine(engine string) (string, error) {
	e, err := godatabase.NormalizeEngine(engine)
	return e, wrapInvalid(err)
}

func DatabasePathForProject(projectID, engine string) (string, error) {
	p, err := godatabase.PathForProject(projectID, engine)
	return p, wrapInvalid(err)
}

func DatabaseEngine(databasePath string) (string, error) {
	e, err := godatabase.EngineFromPath(databasePath)
	return e, wrapInvalid(err)
}

func ValidateDatabasePath(databasePath string) error {
	return wrapInvalid(godatabase.ValidatePath(databasePath))
}

func ValidateProjectDatabasePath(projectID, databasePath string) error {
	return wrapInvalid(godatabase.ValidateProjectPath(ReservedSystemProjectID, projectID, databasePath))
}

func (p Project) Engine() string {
	engine, _ := DatabaseEngine(p.DatabasePath)
	return engine
}
