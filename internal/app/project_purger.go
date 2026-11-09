// 文件说明：project_purger.go — internal/app 的内部实现（projectPurger、newProjectPurger 等）
//
// 用法
//
//	type projectPurger（包内使用）
//	同包调用：newProjectPurger() → app.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/project_purger.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/filedb, godatabase/registry, internal/catalog, internal/collection
//	[同包] app.go, catalog_engine.go, cron_runner.go, identity.go, reset.go 等 6 个
//	[关键引用] newProjectPurger() ← app.go
//
// 主要符号
//
//	type projectPurger struct {…}
//	newProjectPurger(factory *filedb.Factory, syncer filedb.Syncer, reg *registry.Registry, schemas collectionCleaner) ...
//	(p *projectPurger) ClearProjectData(ctx context.Context, db catalog.Database) error
//	(p *projectPurger) PurgeProject(ctx context.Context, projectID string) error
//	(p *projectPurger) DeletePrefix(context.Context, string) error

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

// projectAcquirer 是删库清空数据所需的最小 registry 能力（*registry.Registry 满足）。
type projectAcquirer interface {
	Acquire(ctx context.Context, db godatabase.Spec, mode godatabase.AccessMode) (*registry.Lease, error)
}

// collectionCleaner 清理项目在系统库里的集合元数据（*collection.Repository 满足）。
type collectionCleaner interface {
	List(ctx context.Context, projectID string) ([]collection.Collection, error)
	Delete(ctx context.Context, projectID, name string) error
}

// projectPurger 是删库的存储平面。
//
// 单文件模型下 SQL 表、集合文档表与 KV（__sb_kv_*）同在 {project}/database.{duckdb|sqlite}。
// 删库只清空「用户数据库」的内容（用户表、视图、序列与集合元数据），保留 KV 等内部表，
// 项目文件本身不删除，所以备份、KV 与同项目的其他能力在删库后照常可用。
// registry 缺失时（测试 / 早期装配）退回旧行为：整体删除项目文件。
type projectPurger struct {
	factory *filedb.Factory
	syncer  filedb.Syncer
	reg     projectAcquirer
	schemas collectionCleaner
}

func newProjectPurger(factory *filedb.Factory, syncer filedb.Syncer, reg *registry.Registry, schemas collectionCleaner) catalog.StoragePurger {
	if factory == nil {
		return nil
	}
	p := &projectPurger{factory: factory, syncer: syncer, schemas: schemas}
	if reg == nil {
		return fileOnlyPurger{p}
	}
	p.reg = reg
	return p
}

// fileOnlyPurger 只暴露整体删文件平面（没有 registry 时无法安全清空数据）。
type fileOnlyPurger struct{ p *projectPurger }

func (f fileOnlyPurger) PurgeProject(ctx context.Context, projectID string) error {
	return f.p.PurgeProject(ctx, projectID)
}
func (fileOnlyPurger) DeletePrefix(context.Context, string) error { return nil }

// ClearProjectData 清空用户数据库：删除所有非内部表、视图与序列，再清理集合元数据。
// 以 ReadWrite 租约执行，写租约门禁与快照同步（AfterWrite → MarkDirty）照常生效。
func (p *projectPurger) ClearProjectData(ctx context.Context, db catalog.Database) error {
	spec := godatabase.Spec{
		ID:           db.ID,
		TenantID:     db.TenantID,
		ProjectID:    db.ProjectID,
		Status:       godatabase.StatusReady,
		DatabasePath: db.DatabasePath,
	}
	lease, err := p.reg.Acquire(ctx, spec, godatabase.ReadWrite)
	if err != nil {
		return err
	}
	defer lease.Release()
	engine := godatabase.EngineDuckDB
	if path := lease.Handle.Database.DatabasePath; path != "" {
		if e, err := godatabase.EngineFromPath(path); err == nil {
			engine = e
		}
	}
	if err := clearUserObjects(ctx, lease.Handle, engine); err != nil {
		return err
	}
	if p.schemas != nil {
		items, err := p.schemas.List(ctx, db.ProjectID)
		if err != nil && !errors.Is(err, collection.ErrUnavailable) {
			return fmt.Errorf("list collection metadata: %w", err)
		}
		for _, item := range items {
			if err := p.schemas.Delete(ctx, db.ProjectID, item.Name); err != nil {
				return fmt.Errorf("delete collection metadata %s: %w", item.Name, err)
			}
		}
	}
	return nil
}

// userObject 是待删除的用户对象；kind 取 TABLE / VIEW / SEQUENCE。
type userObject struct{ kind, name string }

func listUserObjects(ctx context.Context, h *registry.Handle, engine string) ([]userObject, error) {
	var stmt string
	if engine == godatabase.EngineSQLite {
		stmt = `SELECT CASE type WHEN 'view' THEN 'VIEW' ELSE 'TABLE' END, name FROM sqlite_master
			WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite\_%' ESCAPE '\'`
	} else {
		stmt = `SELECT CASE table_type WHEN 'VIEW' THEN 'VIEW' ELSE 'TABLE' END, table_name FROM information_schema.tables
			WHERE table_catalog = current_database() AND table_schema = current_schema()
			UNION ALL
			SELECT 'SEQUENCE', sequence_name FROM duckdb_sequences()
			WHERE database_name = current_database() AND schema_name = current_schema()`
	}
	res, err := h.Query(ctx, godatabase.Statement{SQL: stmt}, 0)
	if err != nil {
		return nil, fmt.Errorf("list user objects: %w", err)
	}
	out := make([]userObject, 0, len(res.Rows))
	for _, row := range res.Rows {
		if len(row) < 2 {
			continue
		}
		kind, _ := row[0].(string)
		name, _ := row[1].(string)
		if name == "" || godatabase.IsInternalTableName(name) {
			continue
		}
		out = append(out, userObject{kind: kind, name: name})
	}
	return out, nil
}

// clearUserObjects 先删视图、再删表、最后删序列；外键等依赖导致的失败在下一轮重试，
// 直到一轮没有进展为止（最多对象个数 + 1 轮）。
func clearUserObjects(ctx context.Context, h *registry.Handle, engine string) error {
	objects, err := listUserObjects(ctx, h, engine)
	if err != nil {
		return err
	}
	order := map[string]int{"VIEW": 0, "TABLE": 1, "SEQUENCE": 2}
	pending := make([]userObject, 0, len(objects))
	for rank := 0; rank <= 2; rank++ {
		for _, o := range objects {
			if order[o.kind] == rank {
				pending = append(pending, o)
			}
		}
	}
	var lastErr error
	for round := 0; len(pending) > 0 && round <= len(objects); round++ {
		next := pending[:0:0]
		for _, o := range pending {
			if _, err := h.Execute(ctx, godatabase.Statement{SQL: "DROP " + o.kind + " IF EXISTS " + sqlx.Ident(o.name)}); err != nil {
				lastErr = err
				next = append(next, o)
			}
		}
		if len(next) == len(pending) {
			break
		}
		pending = next
	}
	if len(pending) > 0 {
		names := make([]string, 0, len(pending))
		for _, o := range pending {
			names = append(names, o.name)
		}
		return fmt.Errorf("drop user objects %s: %w", strings.Join(names, ", "), lastErr)
	}
	return nil
}

// PurgeProject 整体删除项目数据库文件（本地主文件 + 伴生文件 + 远端快照），不删除 {project}/files/。
// 删库默认走 ClearProjectData；这里只在没有 registry 时作为退路。
func (p *projectPurger) PurgeProject(ctx context.Context, projectID string) error {
	if p == nil || p.factory == nil {
		return nil
	}
	if p.syncer != nil {
		if fs, ok := p.syncer.(interface {
			PurgeProject(ctx context.Context, projectID string) error
		}); ok {
			if err := fs.PurgeProject(ctx, projectID); err != nil {
				return err
			}
		}
	}
	return p.factory.PurgeProjectFiles(projectID)
}

// DeletePrefix 让 projectPurger 同时满足旧 StoragePurger 接口（不会走该分支）。
func (p *projectPurger) DeletePrefix(context.Context, string) error { return nil }
