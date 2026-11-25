// 文件说明：dev_replay.go — 重放器核心（状态记账、错误映射、运行主流程）与库 / 迁移步骤
//
// 用法
//
//	同包内由 dev.go 的 runDev 调用；与 skills/simplebase/scripts/sbdev.py 的对应步骤语义一致。
//
// 调用架构
//
//	[上游] dev.go（devCommand）
//	         ▼
//	[本文件] internal/sbcli/dev_replay.go
//	         ▼
//	[同包] dev.go（devError / DevAction / devTarget）、dev_workspace.go（devWorkspace）
//
// 主要符号
//
//	type devReplayer / run / databases / migrations

package sbcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

type devReplayer struct {
	w       *devWorkspace
	c       *gosdk.Client
	target  devTarget
	entry   *devLockEntry
	apply   bool
	seed    bool
	now     func() time.Time
	step    string
	dbOrder []string
	dbIDs   map[string]string
	actions []DevAction
	changes int
	skips   int
	warns   []string
}

func newDevReplayer(w *devWorkspace, c *gosdk.Client, t devTarget, lock *devLock, apply, seed bool) *devReplayer {
	entry := lock.Targets[t.key()]
	if entry == nil {
		entry = &devLockEntry{}
		if apply {
			lock.Targets[t.key()] = entry
		}
	}
	return &devReplayer{w: w, c: c, target: t, entry: entry, apply: apply, seed: seed, now: time.Now, dbIDs: map[string]string{}}
}

func (r *devReplayer) act(verb, kind, name, detail string) {
	if verb == "skip" {
		r.skips++
	} else {
		r.changes++
	}
	r.actions = append(r.actions, DevAction{Verb: verb, Kind: kind, Name: name, Detail: detail})
}

func (r *devReplayer) warn(format string, args ...any) {
	r.warns = append(r.warns, fmt.Sprintf(format, args...))
}

func (r *devReplayer) fail(err error) error {
	var de *devError
	if errors.As(err, &de) {
		return err
	}
	var api *gosdk.APIError
	if errors.As(err, &api) {
		return &devError{code: api.Code, msg: fmt.Sprintf("%s 失败：%s", r.step, api.Message), exit: exitAPI}
	}
	return &devError{code: "dev_failed", msg: fmt.Sprintf("%s 失败：%v", r.step, err), exit: exitAPI}
}

func (r *devReplayer) run(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = r.fail(err)
		}
	}()
	if err = r.databases(ctx); err != nil {
		return err
	}
	for _, d := range r.w.databases {
		if err = r.migrations(ctx, d); err != nil {
			return err
		}
	}
	if err = r.collections(ctx); err != nil {
		return err
	}
	if err = r.functions(ctx); err != nil {
		return err
	}
	if err = r.cron(ctx); err != nil {
		return err
	}
	if r.apply {
		r.entry.AppliedAt = r.now().UTC().Format("2006-01-02T15:04:05Z")
		r.entry.Sbdev = devToolVersion
	}
	return nil
}

// checkEngine 记录目标项目的数据库引擎，并与其他环境的记录比对。
// 迁移只按一种方言写，本地与云端引擎不一致时应当提醒（服务端没给库级引擎，只能读项目级）。
func (r *devReplayer) checkEngine(ctx context.Context, lock *devLock) {
	r.step = "project list"
	raw, err := r.c.ListProjects(ctx)
	if err != nil {
		// 不阻断重放，但要说清楚「没校验成」，别让人以为已经校验过。
		var api *gosdk.APIError
		if errors.As(err, &api) && (api.Status == 401 || api.Status == 403) {
			r.warn("无法读取项目引擎（%s）：跳过本地与云端的引擎一致性检查，请自行确认两端引擎相同", api.Code)
			return
		}
		if errors.As(err, &api) && api.Status == 404 {
			r.warn("服务端没有 /v1/projects 接口（%s）：跳过引擎一致性检查", api.Code)
			return
		}
		r.warn("读取项目引擎失败（%v）：跳过引擎一致性检查", err)
		return
	}
	var listed struct {
		Projects []struct {
			ID     string `json:"id"`
			Engine string `json:"engine"`
		} `json:"projects"`
	}
	if json.Unmarshal(raw, &listed) != nil {
		r.warn("项目列表格式无法解析：跳过引擎一致性检查")
		return
	}
	engine := ""
	for _, p := range listed.Projects {
		if p.ID == r.target.Project {
			engine = p.Engine
			break
		}
	}
	if engine == "" {
		r.warn("项目 %s 不在 project list 返回里：跳过引擎一致性检查", r.target.Project)
		return
	}
	if r.apply {
		r.entry.Engine = engine
	}
	for key, e := range lock.Targets {
		if key == r.target.key() || e.Engine == "" || e.Engine == engine {
			continue
		}
		r.warn("%s 的数据库引擎是 %s，%s 是 %s：迁移只按一种方言写，两端要保持一致",
			key, e.Engine, r.target.key(), engine)
	}
}

func (r *devReplayer) databases(ctx context.Context) error {
	if len(r.w.databases) == 0 {
		return nil
	}
	r.step = "database list"
	var remote []gosdk.DatabaseInfo
	cursor := ""
	for i := 0; i < 50; i++ {
		page, err := r.c.ListDatabases(ctx, 200, cursor)
		if err != nil {
			return devHintProjectMissing(err, r.target)
		}
		remote = append(remote, page.Databases...)
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if r.entry.Databases == nil {
		r.entry.Databases = map[string]devLockDB{}
	}
	for _, d := range r.w.databases {
		r.step = "database " + d.name
		r.dbOrder = append(r.dbOrder, d.name)
		var hit *gosdk.DatabaseInfo
		if lid := r.entry.Databases[d.name].ID; lid != "" {
			for i := range remote {
				if remote[i].ID == lid {
					hit = &remote[i]
					break
				}
			}
			if hit == nil {
				r.warn("lock 中 %s 的库 ID %s 在远端不存在，改按名称匹配", d.name, lid)
			}
		}
		if hit == nil {
			var same []*gosdk.DatabaseInfo
			for i := range remote {
				if remote[i].Name == d.name {
					same = append(same, &remote[i])
				}
			}
			if len(same) > 1 {
				return &devError{code: "database_ambiguous", exit: exitAPI,
					msg: fmt.Sprintf("远端有 %d 个名为 %s 的库，请在 lock 的 databases.%s.id 指定一个", len(same), d.name, d.name)}
			}
			if len(same) == 1 {
				hit = same[0]
			}
		}
		switch {
		case hit != nil:
			rmodel := hit.DataModel
			if rmodel == "" {
				rmodel = "collection"
			}
			if rmodel != d.dataModel {
				return &devError{code: "data_model_mismatch", exit: exitAPI,
					msg: fmt.Sprintf("远端库 %s 是 %s，清单要求 %s；换库名或修正 data_model", d.name, rmodel, d.dataModel)}
			}
			r.dbIDs[d.name] = hit.ID
			r.act("skip", "database", d.name, hit.ID)
		case r.apply:
			created, err := r.c.CreateDatabaseWith(ctx, gosdk.CreateDatabaseInput{Name: d.name, DataModel: d.dataModel})
			if err != nil {
				return err
			}
			r.dbIDs[d.name] = created.ID
			r.act("create", "database", d.name, fmt.Sprintf("%s → %s", d.dataModel, created.ID))
		default:
			r.dbIDs[d.name] = ""
			r.act("create", "database", d.name, d.dataModel)
		}
		if id := r.dbIDs[d.name]; id != "" {
			r.entry.Databases[d.name] = devLockDB{ID: id}
		}
	}
	return nil
}

func (r *devReplayer) appliedMigrations(ctx context.Context, dbID string) (map[string]bool, error) {
	db := r.c.Database(dbID)
	if r.apply {
		if _, err := db.Execute(ctx, "CREATE TABLE IF NOT EXISTS "+devMigrationsTable+" (name VARCHAR, applied_at VARCHAR)", nil); err != nil {
			return nil, err
		}
	}
	res, err := db.Query(ctx, "SELECT name FROM "+devMigrationsTable, nil, 0)
	if err != nil {
		// plan 不建表：记账表还不存在时查询会失败，按「一个都没应用」处理。
		// 不用 schema show 判断，因为它只对 sql 库可用，而 collection 库同样可以跑迁移 SQL。
		var api *gosdk.APIError
		if !r.apply && errors.As(err, &api) && api.Status >= 400 && api.Status != 401 && api.Status != 403 {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	out := map[string]bool{}
	for _, row := range res.Rows {
		if len(row) > 0 {
			if s, ok := row[0].(string); ok {
				out[s] = true
			}
		}
	}
	return out, nil
}

func (r *devReplayer) migrations(ctx context.Context, d devDatabase) error {
	items := append([]devSQLFile(nil), d.migrations...)
	if r.seed && d.seed != nil {
		items = append(items, *d.seed)
	}
	if len(items) == 0 {
		return nil
	}
	dbID := r.dbIDs[d.name]
	applied := map[string]bool{}
	if dbID != "" {
		r.step = "migration " + d.name
		var err error
		if applied, err = r.appliedMigrations(ctx, dbID); err != nil {
			return err
		}
	}
	if r.entry.Migrations == nil {
		r.entry.Migrations = map[string]map[string]string{}
	}
	if r.entry.Migrations[d.name] == nil {
		r.entry.Migrations[d.name] = map[string]string{}
	}
	sums := r.entry.Migrations[d.name]
	for _, it := range items {
		label := d.name + "/" + it.marker
		if applied[it.marker] {
			old := sums[it.marker]
			if old != "" && old != it.sha && !strings.HasPrefix(it.marker, "seed:") {
				return &devError{code: "migration_modified", exit: exitUsage,
					msg: fmt.Sprintf("migration_modified: %s 已应用但内容变了；不要改旧迁移，新增一个文件", label)}
			}
			if old == "" {
				sums[it.marker] = it.sha
			}
			r.act("skip", "migration", label, "")
			continue
		}
		r.act("apply", "migration", label, fmt.Sprintf("%d 条", len(it.stmts)))
		if !r.apply {
			continue
		}
		r.step = "migration " + label
		batch := make([]gosdk.SQLStatement, 0, len(it.stmts)+1)
		for _, s := range it.stmts {
			batch = append(batch, gosdk.SQLStatement{SQL: s, Args: []any{}})
		}
		batch = append(batch, gosdk.SQLStatement{
			SQL:  "INSERT INTO " + devMigrationsTable + " (name, applied_at) VALUES (?, ?)",
			Args: []any{it.marker, r.now().UTC().Format("2006-01-02T15:04:05Z")},
		})
		res, err := r.c.Database(dbID).Batch(ctx, batch, true)
		if err != nil {
			return err
		}
		if res.Error != nil {
			return &devError{code: "migration_failed", exit: exitAPI,
				msg: fmt.Sprintf("迁移 %s 失败（已整体回滚）：%s: %s", label, res.Error.Code, res.Error.Message)}
		}
		for _, item := range res.Results {
			if item.ErrorCode != "" {
				return &devError{code: "migration_failed", exit: exitAPI,
					msg: fmt.Sprintf("迁移 %s 失败（已整体回滚）：%s: %s", label, item.ErrorCode, item.ErrorMessage)}
			}
		}
		sums[it.marker] = it.sha
	}
	return nil
}
