// 文件说明：access_db.go — 宿主数据库访问适配（实现 DatabaseAccess 端口）
//
// 用法
//
//	NewCloudAgentDB(cat CatalogReader, opener DatabaseOpener) DatabaseAccess  // 构造；任一为 nil 返回 nil
//	(a *cloudAgentDB) ListDatabases(ctx, principal, projectID)  // 列出主体可见的库
//	(a *cloudAgentDB) ListCollections(ctx, principal, projectID, databaseID)  // 只读列出用户表
//	(a *cloudAgentDB) ReadOnlyQuery(ctx, principal, projectID, databaseID, sqlText, maxRows)  // 只读 SQL
//
// 调用架构
//
//	[上游] internal/app（装配进 RuntimeProvider.DB）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/access_db.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/registry, godatabase/sqlguard, internal/auth, internal/catalog, internal/dbbridge
//	[同包] ports.go
//
// 主要符号
//
//	type CatalogReader interface {…}
//	type DatabaseOpener interface {…}
//	type cloudAgentDB struct {…}
//	NewCloudAgentDB(cat CatalogReader, opener DatabaseOpener) DatabaseAccess
//	... 另有 3 个顶层符号
//
// 归属（internal-ddd-hexagonal-implementation-plan §4）：它实现的就是 agentbridge.DatabaseAccess 端口，
// 属于宿主 ↔ gocloudagent 的防腐层，不是 HTTP 适配（自 internal/api 迁入）。

package agentbridge

import (
	"context"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
)

// CatalogReader 是 Agent 数据库工具需要的目录查询；*catalog.Service 满足。
type CatalogReader interface {
	GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error)
}

// DatabaseOpener 是「按 catalog 行取库句柄」的最小端口；dbbridge.RegistryFrom(...) 满足。
type DatabaseOpener interface {
	Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (*registry.Lease, error)
}

// cloudAgentDB 以只读租约实现 DatabaseAccess：每次调用现取现还，不缓存句柄。
type cloudAgentDB struct {
	catalog CatalogReader
	opener  DatabaseOpener
}

// NewCloudAgentDB 构造适配器；任一依赖为 nil 返回 nil（Agent 侧按「数据库工具不可用」处理）。
func NewCloudAgentDB(cat CatalogReader, opener DatabaseOpener) DatabaseAccess {
	if cat == nil || opener == nil {
		return nil
	}
	return &cloudAgentDB{catalog: cat, opener: opener}
}

func (a *cloudAgentDB) ListDatabases(ctx context.Context, principal auth.Principal, projectID string) ([]DatabaseInfo, error) {
	// 还原成宿主身份：catalog 的项目授权依赖 Role / TenantID / ProjectIDs。
	list, _, err := a.catalog.ListDatabases(ctx, principal, projectID, catalog.Page{Limit: 50})
	if err != nil {
		return nil, err
	}
	out := make([]DatabaseInfo, 0, len(list))
	for _, d := range list {
		out = append(out, DatabaseInfo{ID: d.ID, Name: d.Name, Status: string(d.Status)})
	}
	return out, nil
}

func (a *cloudAgentDB) ListCollections(ctx context.Context, principal auth.Principal, projectID, databaseID string) ([]string, error) {
	db, err := a.catalog.GetDatabase(ctx, principal, projectID, databaseID)
	if err != nil {
		return nil, err
	}
	lease, err := a.opener.Acquire(ctx, db, godatabase.ReadOnly)
	if err != nil {
		return nil, err
	}
	defer lease.Release()
	dialect, err := dbbridge.DialectOf(db)
	if err != nil {
		return nil, err
	}
	result, err := lease.Handle.Query(ctx, godatabase.Statement{SQL: dialect.TableColumnsSQL("")}, 200*8)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(result.Rows))
	seen := make(map[string]struct{}, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) == 0 {
			continue
		}
		name, ok := row[0].(string)
		if !ok || name == "" || dialect.IsInternalTable(name) {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

// ReadOnlyQuery 先过 SQL 只读守卫，再以只读租约执行；行数上限夹在 1–100，缺省 50。
func (a *cloudAgentDB) ReadOnlyQuery(ctx context.Context, principal auth.Principal, projectID, databaseID, sqlText string, maxRows int) (SQLResult, error) {
	if err := sqlguard.Validate(sqlText, sqlguard.ReadOnly); err != nil {
		return SQLResult{}, err
	}
	db, err := a.catalog.GetDatabase(ctx, principal, projectID, databaseID)
	if err != nil {
		return SQLResult{}, err
	}
	lease, err := a.opener.Acquire(ctx, db, godatabase.ReadOnly)
	if err != nil {
		return SQLResult{}, err
	}
	defer lease.Release()
	if maxRows <= 0 || maxRows > 100 {
		maxRows = 50
	}
	result, err := lease.Handle.Query(ctx, godatabase.Statement{SQL: sqlText}, maxRows)
	if err != nil {
		return SQLResult{}, err
	}
	rows, err := godatabase.SerializeRows(result.Rows)
	if err != nil {
		return SQLResult{}, err
	}
	return SQLResult{Columns: result.Columns, Rows: rows, RowCount: len(rows)}, nil
}
