// Package database 定义 SimpleBase 数据库运行时的核心类型：访问模式、
// 数据库工厂（打开 *sql.DB）、SQL 执行原语（Query/Execute/Batch）和错误。
//
// 本包不做进程内单写编排——那是 internal/database/registry 的职责。
// 本包只提供在已获得的 *sql.DB 上安全执行 SQL 的可测试函数。
package database

import (
	"context"
	"database/sql"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// AccessMode 表示对某个 logical database 的访问意图。
// 首期单实例架构下 ReadWrite/ReadOnly 都由同一个 writer 实例提供服务；
// 保留该参数是为未来引入只读副本预留边界，不代表当前具备只读扩展能力。
type AccessMode uint8

const (
	ReadOnly AccessMode = iota
	ReadWrite
)

// String 便于日志与指标输出。
func (m AccessMode) String() string {
	if m == ReadWrite {
		return "read_write"
	}
	return "read_only"
}

// Factory 从 catalog.Database 打开一个可用的 *sql.DB。
// 实现者负责构建 Turso DSN、连接池配置和 Ping 验证；失败必须关闭已创建的资源。
type Factory interface {
	Open(ctx context.Context, db catalog.Database, mode AccessMode) (*sql.DB, error)
}
