// 文件说明：spec.go — godatabase 中 Status、StatusCreating 等符号的实现
//
// 用法
//
//	(s Spec) IsSystem() bool// IsSystem 判断是否为系统库
//	NormalizeEngine(engine string) (string, error)  // NormalizeEngine 把引擎名规范化为 duckdb / sqlite
//	同包调用：EngineDuckDB → dialect.go；EngineSQLite → dialect.go；Spec → runtime.go
//
// 调用架构
//
//	[上游] godatabase/cache, godatabase/ducklake, godatabase/filedb 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/spec.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/blob
//	[同包] dialect.go, query.go, runtime.go, serialize.go, snapshot.go 等 6 个
//	[关键引用] EngineDuckDB ← dialect.go
//
// 主要符号
//
//	type Status string
//	const StatusCreating
//	const StatusReady
//	const StatusDegraded
//	const StatusDeleting
//	const StatusDeleted
//	... 另有 17 个顶层符号

package godatabase

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"go.uber.org/zap"
)

// ---------- 数据库规格 ----------

// Status 是数据库的可访问状态。取值与宿主 catalog 的状态字符串一致，
// 库只关心 deleting / deleted / degraded 三种需要拒绝访问的状态。
type Status string

const (
	StatusCreating Status = "creating"
	StatusReady    Status = "ready"
	StatusDegraded Status = "degraded"
	StatusDeleting Status = "deleting"
	StatusDeleted  Status = "deleted"
)

// Kind 取值。空串视为 user。
const (
	KindUser   = "user"
	KindSystem = "system"
)

// Spec 描述要打开的一个数据库。只包含运行时真正读取的字段，
// 业务元数据（DataModel / StoragePrefix / 时间戳等）留在宿主 catalog。
type Spec struct {
	ID        string
	TenantID  string
	ProjectID string
	// Kind 为 "user" 或 "system"；空视为 user。
	Kind   string
	Status Status
	// DatabasePath 是项目文件相对路径，仅服务端使用，不得进入日志与 API 响应。
	DatabasePath string
}

// IsSystem 判断是否为系统库。
func (s Spec) IsSystem() bool { return s.Kind == KindSystem }

// PathResolver 为缺失 DatabasePath 的用户库补全路径。
// 宿主用 catalog 实现它（含租户归属校验）；库不接触租户逻辑。
type PathResolver interface {
	ResolveDatabasePath(ctx context.Context, tenantID, projectID string) (string, error)
}

// ---------- 引擎与路径 ----------

const (
	EngineDuckDB = "duckdb"
	EngineSQLite = "sqlite"
)

// ErrInvalidPath 表示引擎名或数据库路径不合法。宿主应把它映射为自己的"名称不合法"错误。
var ErrInvalidPath = errors.New("godatabase: invalid database path or engine")

func invalidPath(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidPath}, args...)...)
}

// NormalizeEngine 把引擎名规范化为 duckdb / sqlite；空串视为 duckdb。
func NormalizeEngine(engine string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "", EngineDuckDB:
		return EngineDuckDB, nil
	case EngineSQLite:
		return EngineSQLite, nil
	default:
		return "", invalidPath("unsupported database engine %q", engine)
	}
}

// PathForProject 返回项目文件的规范相对路径：{projectID}/database.{ext}。
func PathForProject(projectID, engine string) (string, error) {
	if err := blob.ValidateProjectID(projectID); err != nil {
		return "", invalidPath("invalid project id")
	}
	engine, err := NormalizeEngine(engine)
	if err != nil {
		return "", err
	}
	ext := ".duckdb"
	if engine == EngineSQLite {
		ext = ".sqlite"
	}
	return projectID + "/database" + ext, nil
}

// EngineFromPath 由文件扩展名推断引擎。路径必须先通过 ValidatePath 的形态校验。
func EngineFromPath(databasePath string) (string, error) {
	if err := ValidatePath(databasePath); err != nil {
		return "", err
	}
	switch path.Ext(databasePath) {
	case ".duckdb":
		return EngineDuckDB, nil
	case ".sqlite":
		return EngineSQLite, nil
	default:
		return "", invalidPath("unsupported database file extension")
	}
}

// ValidatePath 校验项目文件相对路径的形态：{projectID}/database.duckdb|sqlite。
func ValidatePath(databasePath string) error {
	if databasePath == "" || strings.Contains(databasePath, `\`) || strings.ContainsRune(databasePath, '\x00') {
		return invalidPath("invalid database path")
	}
	if strings.HasPrefix(databasePath, "/") || path.Clean(databasePath) != databasePath {
		return invalidPath("database path must be a clean relative path")
	}
	parts := strings.Split(databasePath, "/")
	if len(parts) != 2 || parts[0] == "" || (parts[1] != "database.duckdb" && parts[1] != "database.sqlite") {
		return invalidPath("invalid project database path")
	}
	if err := blob.ValidateProjectID(parts[0]); err != nil {
		return invalidPath("invalid project id in database path")
	}
	return nil
}

// ValidateProjectPath 校验 databasePath 属于 projectID 且为规范路径。
// systemProjectID 非空且等于 projectID 时视为系统项目，其路径必须为空；
// systemProjectID 为空表示调用方没有系统项目概念（filedb 不打开系统库）。
func ValidateProjectPath(systemProjectID, projectID, databasePath string) error {
	if systemProjectID != "" && projectID == systemProjectID {
		if databasePath != "" {
			return invalidPath("system project database path must be empty")
		}
		return nil
	}
	if err := ValidatePath(databasePath); err != nil {
		return err
	}
	expected, err := PathForProject(projectID, strings.TrimPrefix(path.Ext(databasePath), "."))
	if err != nil {
		return err
	}
	if databasePath != expected {
		return invalidPath("database path does not belong to project")
	}
	return nil
}

// ---------- 日志与指标 ----------

// Logger 是库使用的最小日志接口。*zap.Logger 的 Debug/Info/Warn/Error 签名与之匹配。
// 传入 nil 接口表示静默；注意不要把 nil 的具体指针赋给它（会得到非 nil 接口）。
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

// Observer 收集库内的运行指标。所有方法必须并发安全。nil 表示不采集。
type Observer interface {
	// OpenHandleDelta 在句柄打开（+1）/ 关闭（-1）时调用。
	OpenHandleDelta(delta int)
	// CacheBytes 上报当前缓存字节用量。
	CacheBytes(bytes int64)
	// CacheEvicted 在缓存目录被淘汰一次时调用。
	CacheEvicted()
	// CatalogSync 记录一次 catalog 同步的结果。
	CatalogSync(outcome string, d time.Duration, failed bool)
	// CatalogSyncLag 上报某库的同步滞后快照数。
	CatalogSyncLag(dbID string, lag int64)
	// CatalogSyncConflict 在检测到 split-brain（manifest 冲突）时调用。
	CatalogSyncConflict(dbID string)
	// CatalogSize 上报某库 catalog 文件体积。
	CatalogSize(dbID string, bytes int64)
	// CatalogLocalAhead 在本地 catalog 领先远端、被拦截时调用。
	CatalogLocalAhead()
}

// NopObserver 丢弃所有指标。
type NopObserver struct{}

func (NopObserver) OpenHandleDelta(int)                     {}
func (NopObserver) CacheBytes(int64)                        {}
func (NopObserver) CacheEvicted()                           {}
func (NopObserver) CatalogSync(string, time.Duration, bool) {}
func (NopObserver) CatalogSyncLag(string, int64)            {}
func (NopObserver) CatalogSyncConflict(string)              {}
func (NopObserver) CatalogSize(string, int64)               {}
func (NopObserver) CatalogLocalAhead()                      {}
