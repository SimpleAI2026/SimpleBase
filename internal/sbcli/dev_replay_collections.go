// 文件说明：dev_replay_collections.go — 重放器的集合与种子文档步骤
//
// 用法
//
//	同包内由 dev.go 的 runDev 调用；与 skills/simplebase/scripts/sbdev.py 的对应步骤语义一致。
//
// 调用架构
//
//	[上游] dev.go（devCommand）
//	         ▼
//	[本文件] internal/sbcli/dev_replay_collections.go
//	         ▼
//	[同包] dev.go（devError / DevAction / devTarget）、dev_workspace.go（devWorkspace）
//
// 主要符号
//
//	(*devReplayer).collections / devSeedDocMarker

package sbcli

import (
	"context"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func (r *devReplayer) collections(ctx context.Context) error {
	for _, c := range r.w.colls {
		dbID := r.dbIDs[c.database]
		label := c.database + "/" + c.name
		r.step = "collection " + label
		existing := map[string]bool{}
		if dbID != "" {
			list, err := r.c.Database(dbID).ListCollections(ctx)
			if err != nil {
				return err
			}
			for _, n := range list.Collections {
				existing[n] = true
			}
			for _, it := range list.Items {
				existing[it.Name] = true
			}
		}
		present := existing[c.name]
		if present {
			r.act("skip", "collection", label, "")
		} else {
			r.act("create", "collection", label, "")
			if r.apply {
				if err := r.c.Database(dbID).CreateCollection(ctx, c.name); err != nil {
					return err
				}
			}
		}
		if !r.seed || len(c.seed) == 0 {
			continue
		}
		// 服务端建文档时总是生成 UUID、忽略传入的 id，按 id 查重永远落空。
		// 改为与迁移同一张记账表：每篇种子文档写一行 seed-doc:<集合>/<id>，已记账的跳过。
		applied := map[string]bool{}
		if dbID != "" {
			var err error
			if applied, err = r.appliedMigrations(ctx, dbID); err != nil {
				return err
			}
		}
		for _, doc := range c.seed {
			id, _ := doc["id"].(string)
			name := label + "/" + id
			marker := devSeedDocMarker(c.name, id)
			if applied[marker] {
				r.act("skip", "document", name, "")
				continue
			}
			r.act("create", "document", name, "")
			if !r.apply {
				continue
			}
			r.step = "document " + name
			if _, err := r.c.Database(dbID).InsertDocument(ctx, c.name, gosdk.Document(doc)); err != nil {
				return err
			}
			if _, err := r.c.Database(dbID).Execute(ctx, "INSERT INTO "+devMigrationsTable+" (name, applied_at) VALUES (?, ?)",
				[]any{marker, r.now().UTC().Format("2006-01-02T15:04:05Z")}); err != nil {
				return err
			}
		}
	}
	return nil
}

// devSeedDocMarker 是集合种子文档在 _migrations 里的记账名（与 sbdev.py 一致）。
func devSeedDocMarker(collection, id string) string { return "seed-doc:" + collection + "/" + id }
