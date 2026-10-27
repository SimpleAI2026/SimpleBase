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
//	[下游] godatabase/filedb, internal/catalog
//	[同包] app.go, catalog_engine.go, cron_runner.go, identity.go, reset.go 等 6 个
//	[关键引用] newProjectPurger() ← app.go
//
// 主要符号
//
//	type projectPurger struct {…}
//	newProjectPurger(factory *filedb.Factory, syncer filedb.Syncer) ...
//	(p *projectPurger) PurgeProject(ctx context.Context, projectID string) error
//	(p *projectPurger) DeletePrefix(context.Context, string) error

package app

import (
	"context"

	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// projectPurger 清理项目数据库文件（本地主文件 + 伴生文件 + 远端快照）。
// 只作用于 {project}/database.{duckdb|sqlite}，不删除 {project}/files/。
type projectPurger struct {
	factory *filedb.Factory
	syncer  filedb.Syncer
}

func newProjectPurger(factory *filedb.Factory, syncer filedb.Syncer) catalog.StoragePurger {
	if factory == nil {
		return nil
	}
	return &projectPurger{factory: factory, syncer: syncer}
}

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
