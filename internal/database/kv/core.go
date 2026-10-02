// Package kv 在 DuckLake 用户库上提供 Redis 语义的 Key-Value 数据服务。
// 数据模型与命令语义参考 thirdparty/redka，但所有 SQL 针对 DuckDB/DuckLake
// 重写（无 ON CONFLICT / 触发器 / 外键 / 自增 rowid，全部由应用层在事务内补偿）。
// 详见 plan/planv3.0/key-value-ducklake-plan.md。
package kv

import (
	"errors"
	"fmt"
)

// TypeID 是 key 的类型标识（取值与 redka internal/core 对齐）。
type TypeID int16

const (
	TypeString TypeID = 1
	TypeList   TypeID = 2
	TypeSet    TypeID = 3
	TypeHash   TypeID = 4
	TypeZSet   TypeID = 5
)

// TypeAll 作为 Scan 的类型筛选零值：不匹配任何存储类型，表示「全部」。
const TypeAll TypeID = 0

func (t TypeID) String() string {
	switch t {
	case TypeString:
		return "string"
	case TypeList:
		return "list"
	case TypeSet:
		return "set"
	case TypeHash:
		return "hash"
	case TypeZSet:
		return "zset"
	default:
		return "unknown"
	}
}

// ParseType 解析 API/UI 传入的类型名；ok=false 表示非法类型名。
func ParseType(s string) (TypeID, bool) {
	switch s {
	case "string":
		return TypeString, true
	case "list":
		return TypeList, true
	case "set":
		return TypeSet, true
	case "hash":
		return TypeHash, true
	case "zset":
		return TypeZSet, true
	default:
		return TypeAll, false
	}
}

// 通用错误。HTTP 映射见 internal/api/error.go（404/409/400）。
var (
	// ErrNotFound：key 不存在或已过期。
	ErrNotFound = errors.New("kv: key not found")
	// ErrKeyType：对已有 key 执行了类型不符的写操作（Redis WRONGTYPE）。
	ErrKeyType = errors.New("kv: key type mismatch")
	// ErrValueType：INCR 等数值操作遇到非数值内容。
	ErrValueType = errors.New("kv: value is not a valid number")
	// ErrKeyExists：RenameNX 等场景目标 key 已存在。
	ErrKeyExists = errors.New("kv: key already exists")
	// ErrArgument：非法参数（负 TTL、越界下标等）。
	ErrArgument = errors.New("kv: invalid argument")
)

// KeyMeta 描述 key 的元数据（对应 kv.keys 行）。
type KeyMeta struct {
	Key     string
	Type    TypeID
	Version int64
	// Etime 为过期时间（unix 毫秒）；nil 表示永久。
	Etime *int64
	// Mtime 为最后修改时间（unix 毫秒）。
	Mtime int64
	// Len 为元素计数（list/set/hash/zset）；string 类型为 nil。
	Len *int64
}

// TTLms 返回剩余 TTL（毫秒）；永久返回 nil，已过期返回 0 指针。
func (m KeyMeta) TTLms(nowMs int64) *int64 {
	if m.Etime == nil {
		return nil
	}
	rem := *m.Etime - nowMs
	if rem < 0 {
		rem = 0
	}
	return &rem
}

// dataTableFor 返回类型对应的数据表名（用于级联删除）。
func dataTableFor(t TypeID) (string, error) {
	switch t {
	case TypeString:
		return "kv.strings", nil
	case TypeList:
		return "kv.lists", nil
	case TypeSet:
		return "kv.sets", nil
	case TypeHash:
		return "kv.hashes", nil
	case TypeZSet:
		return "kv.zsets", nil
	default:
		return "", fmt.Errorf("kv: unknown type id %d", int16(t))
	}
}
