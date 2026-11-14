// 文件说明：dev.go — `simplebase dev plan|apply|status`：把本地工作区幂等重放到任意 SimpleBase 实例（planv6.4 P0b）
//
// 用法
//
//	simplebase dev plan   [--dir DIR] [--manifest FILE] [--seed]          只读：列出将要执行的变更
//	simplebase dev apply  [--dir DIR] [--manifest FILE] [--seed] [--yes]  幂等重放；远端目标不带 --yes 只打印计划并以退出码 3 退出
//	simplebase dev status [--dir DIR]                   查看 .simplebase/lock.json 里各环境的记录
//
// 与 skills/simplebase/scripts/sbdev.py 行为一一对应（同一份清单、同一组 fixture）：
// 固定顺序 库 → 迁移 →（种子）→ 集合 → 函数 → 定时任务；每步先查后写；失败即停，
// 已完成的步骤下次运行自然变为 skip。不删除远端资源，不触发 cron。
//
// 连接信息只取自已加载的 cfg（flag / 环境变量 / config.json），不读工作区 .env；
// 凭据不进 lock、env 提示文件和输出。
//
// 主要符号
//
//	cmdDevPlan / cmdDevApply / cmdDevStatus（commandFunc）
//	type devError（RunIO 识别，映射退出码 3 等）
//	type DevAction

package sbcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"

	"github.com/linkxzhou/SimpleBase/internal/utils/atomicfile"
	"github.com/linkxzhou/SimpleBase/internal/utils/hashx"
)

// exitConfirm 表示远端 apply 缺少 --yes：计划已输出，未做任何写入。
const exitConfirm = 3

const devToolVersion = "simplebase-dev/1"

// DevAction 是一步重放动作，plan 与 apply 共用。
type DevAction struct {
	Verb   string `json:"verb"` // create | update | apply | skip
	Kind   string `json:"kind"` // database | migration | collection | document | function | cron
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
}

// devResult 是 plan/apply/status 的 data 载荷。
type devResult struct {
	Mode     string      `json:"mode"` // plan | apply
	Target   devTarget   `json:"target"`
	Seed     bool        `json:"seed"`
	Actions  []DevAction `json:"actions"`
	Changes  int         `json:"changes"`
	Skips    int         `json:"skips"`
	Warnings []string    `json:"warnings,omitempty"`
	EnvHint  string      `json:"env_hint,omitempty"`
}

type devTarget struct {
	Label   string `json:"label"`
	URL     string `json:"url"`
	Project string `json:"project"`
}

// devError 携带稳定错误码与退出码；data 非空时 RunIO 先把它写到 stdout。
type devError struct {
	code string
	msg  string
	exit int
	data any
}

func (e *devError) Error() string { return e.msg }

var devLocalHosts = map[string]bool{"127.0.0.1": true, "localhost": true, "::1": true, "0.0.0.0": true}

var devLabelStrip = regexp.MustCompile(`[^a-z0-9]+`)

func devTargetOf(cfg loadedConfig) devTarget {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	host := ""
	if u, err := url.Parse(endpoint); err == nil {
		host = u.Hostname()
	}
	label := "local"
	if !devLocalHosts[host] {
		label = strings.Trim(devLabelStrip.ReplaceAllString(strings.ToLower(host), "-"), "-")
		if label == "" {
			label = "remote"
		}
	}
	return devTarget{Label: label, URL: endpoint, Project: cfg.ProjectID}
}

func (t devTarget) isLocal() bool { return t.Label == "local" }

// key 是 lock 的分区键。本地目标不写端口：每人本地端口不同，写进 lock 会
// 在提交后散成多份分区；库 ID 命中失败会回退到按名称匹配，共用分区是安全的。
func (t devTarget) key() string {
	if t.isLocal() {
		return "local#" + t.Project
	}
	return t.URL + "#" + t.Project
}

// ── lock ─────────────────────────────────────────────────

type devLockDB struct {
	ID string `json:"id"`
}

type devLockFunc struct {
	SHA     string `json:"sha"`
	Version int64  `json:"version"`
}

type devLockCron struct {
	ID string `json:"id"`
}

// devLockEntry 字段顺序与 sbdev.py 的 sort_keys 输出一致，保证两端写出的 lock 可互相读取。
type devLockEntry struct {
	AppliedAt  string                       `json:"applied_at,omitempty"`
	Engine     string                       `json:"engine,omitempty"`
	Cron       map[string]devLockCron       `json:"cron,omitempty"`
	Databases  map[string]devLockDB         `json:"databases,omitempty"`
	Functions  map[string]devLockFunc       `json:"functions,omitempty"`
	Migrations map[string]map[string]string `json:"migrations,omitempty"`
	Sbdev      string                       `json:"sbdev,omitempty"`
}

type devLock struct {
	Targets map[string]*devLockEntry `json:"targets"`
	Version int                      `json:"version"`
}

func devLockPath(w *devWorkspace) string { return filepath.Join(w.root, devStateDir, devLockName) }

func loadDevLock(w *devWorkspace) (*devLock, error) {
	lock := &devLock{Targets: map[string]*devLockEntry{}, Version: 1}
	b, err := os.ReadFile(devLockPath(w))
	if errors.Is(err, os.ErrNotExist) {
		return lock, nil
	}
	if err != nil {
		return nil, &devError{code: "workspace_invalid", msg: err.Error(), exit: exitUsage}
	}
	if err := json.Unmarshal(b, lock); err != nil || lock.Targets == nil {
		return nil, &devError{code: "workspace_invalid", exit: exitUsage,
			msg: devLockPath(w) + " 损坏：修复或删除后重跑（删除会丢失迁移 checksum）"}
	}
	return lock, nil
}

func saveDevLock(w *devWorkspace, lock *devLock) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(lock); err != nil {
		return err
	}
	return atomicfile.WriteEnsureDir(devLockPath(w), buf.Bytes(), 0o644)
}

// ── 重放器 ───────────────────────────────────────────────

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

func devIsNotFound(err error) bool {
	var api *gosdk.APIError
	return errors.As(err, &api) && (api.Status == 404 || strings.Contains(api.Code, "not_found"))
}

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

type devRemoteFunc struct {
	Description   string `json:"description"`
	ActiveVersion int64  `json:"active_version"`
	LatestVersion int64  `json:"latest_version"`
	Source        string `json:"source"`
}

func (r *devReplayer) testFunction(ctx context.Context, f devFunction, version int64) error {
	if f.test == nil {
		return nil
	}
	raw, err := r.c.TestFunction(ctx, f.name, strconv.FormatInt(version, 10), f.test.export, json.RawMessage(f.test.body))
	if err != nil {
		return err
	}
	var res struct {
		OK    bool            `json:"ok"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || !res.OK {
		detail := strings.Trim(string(res.Error), `"`)
		if detail == "" || detail == "null" {
			detail = string(raw)
		}
		return &devError{code: "function_test_failed", exit: exitAPI,
			msg: fmt.Sprintf("函数 %s v%d 试跑失败：%s", f.name, version, detail)}
	}
	return nil
}

func (r *devReplayer) latestVersion(ctx context.Context, name string, created json.RawMessage) (int64, error) {
	var c struct {
		LatestVersion int64 `json:"latest_version"`
	}
	if json.Unmarshal(created, &c) == nil && c.LatestVersion > 0 {
		return c.LatestVersion, nil
	}
	raw, err := r.c.ListFunctionVersions(ctx, name)
	if err != nil {
		return 0, err
	}
	var listed struct {
		Versions []struct {
			Version int64 `json:"version"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return 0, err
	}
	var max int64
	for _, v := range listed.Versions {
		if v.Version > max {
			max = v.Version
		}
	}
	return max, nil
}

func (r *devReplayer) functions(ctx context.Context) error {
	if len(r.w.functions) > 0 && r.entry.Functions == nil {
		r.entry.Functions = map[string]devLockFunc{}
	}
	for _, f := range r.w.functions {
		r.step = "function " + f.name
		var remote *devRemoteFunc
		raw, err := r.c.GetFunction(ctx, f.name)
		switch {
		case err == nil:
			remote = &devRemoteFunc{}
			if err := json.Unmarshal(raw, remote); err != nil {
				return err
			}
		default:
			var api *gosdk.APIError
			if !errors.As(err, &api) || (api.Code != "not_found" && api.Code != "gofunction_not_found") {
				return err
			}
		}
		if remote == nil {
			r.act("create", "function", f.name, "")
			if !r.apply {
				continue
			}
			desc := ""
			if f.description != nil {
				desc = *f.description
			}
			// 先建未生效版本，试跑通过后再 activate
			created, err := r.c.CreateFunctionWith(ctx, f.name, f.source, desc, false)
			if err != nil {
				return err
			}
			ver, err := r.latestVersion(ctx, f.name, created)
			if err != nil {
				return err
			}
			if err := r.testFunction(ctx, f, ver); err != nil {
				return err
			}
			if _, err := r.c.ActivateFunctionVersion(ctx, f.name, strconv.FormatInt(ver, 10)); err != nil {
				return err
			}
			r.entry.Functions[f.name] = devLockFunc{SHA: f.sha, Version: ver}
			continue
		}
		if remote.ActiveVersion > 0 && hashx.SumString(remote.Source) == f.sha {
			r.act("skip", "function", f.name, fmt.Sprintf("v%d", remote.ActiveVersion))
			if r.apply {
				r.entry.Functions[f.name] = devLockFunc{SHA: f.sha, Version: remote.ActiveVersion}
			}
		} else {
			var reuse int64
			if remote.LatestVersion > 0 && remote.LatestVersion != remote.ActiveVersion {
				vraw, err := r.c.GetFunctionVersion(ctx, f.name, strconv.FormatInt(remote.LatestVersion, 10))
				if err != nil {
					return err
				}
				var v struct {
					Source string `json:"source"`
				}
				if json.Unmarshal(vraw, &v) == nil && hashx.SumString(v.Source) == f.sha {
					reuse = remote.LatestVersion
				}
			}
			detail := fmt.Sprintf("v%d → 新版本", remote.ActiveVersion)
			if reuse > 0 {
				detail = fmt.Sprintf("v%d → 复用 v%d", remote.ActiveVersion, reuse)
			}
			r.act("update", "function", f.name, detail)
			if r.apply {
				ver := reuse
				if ver == 0 {
					created, err := r.c.CreateFunctionVersion(ctx, f.name, f.source, "dev "+f.sha[:12], false)
					if err != nil {
						return err
					}
					if ver, err = r.latestVersion(ctx, f.name, created); err != nil {
						return err
					}
				}
				if err := r.testFunction(ctx, f, ver); err != nil {
					return err
				}
				if _, err := r.c.ActivateFunctionVersion(ctx, f.name, strconv.FormatInt(ver, 10)); err != nil {
					return err
				}
				r.entry.Functions[f.name] = devLockFunc{SHA: f.sha, Version: ver}
			}
		}
		if f.description != nil && remote.Description != *f.description {
			r.act("update", "function", f.name, "description")
			if r.apply {
				if _, err := r.c.UpdateFunction(ctx, f.name, *f.description); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type devRemoteCron struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	ScheduleKind    string `json:"schedule_kind"`
	CronExpr        string `json:"cron_expr"`
	IntervalSeconds int64  `json:"interval_seconds"`
	RunAt           string `json:"run_at"`
	TargetKind      string `json:"target_kind"`
	FuncFile        string `json:"func_file"`
	FuncExport      string `json:"func_export"`
	InputJSON       string `json:"input_json"`
	Enabled         bool   `json:"enabled"`
}

func devCronInput(w devCron) gosdk.CronJobInput {
	enabled := w.enabled
	desc := ""
	if w.description != nil {
		desc = *w.description
	}
	in := gosdk.CronJobInput{
		Name: w.name, Description: desc, ScheduleKind: w.kind,
		FuncFile: w.funcFile, FuncExport: w.funcExport, InputJSON: w.inputJSON, Enabled: &enabled,
	}
	switch w.kind {
	case "cron":
		in.CronExpr = w.cronExpr
	case "interval":
		in.IntervalSeconds = w.interval
	default:
		in.RunAt = w.runAt
	}
	return in
}

func devJSONEqual(a, b string) bool {
	if a == "" {
		a = "{}"
	}
	var va, vb any
	if json.Unmarshal([]byte(a), &va) != nil || json.Unmarshal([]byte(b), &vb) != nil {
		return a == b
	}
	return reflect.DeepEqual(va, vb)
}

func devCronDiff(old devRemoteCron, w devCron) []string {
	var diff []string
	switch {
	case old.ScheduleKind != w.kind:
		diff = append(diff, "schedule_kind")
	case w.kind == "cron" && old.CronExpr != w.cronExpr:
		diff = append(diff, "cron_expr")
	case w.kind == "interval" && old.IntervalSeconds != w.interval:
		diff = append(diff, "interval_seconds")
	case w.kind == "once":
		ot, e1 := time.Parse(time.RFC3339, old.RunAt)
		wt, e2 := time.Parse(time.RFC3339, w.runAt)
		if e1 != nil || e2 != nil || !ot.Equal(wt) {
			diff = append(diff, "run_at")
		}
	}
	if old.FuncFile != w.funcFile {
		diff = append(diff, "func_file")
	}
	if old.FuncExport != w.funcExport {
		diff = append(diff, "func_export")
	}
	if !devJSONEqual(old.InputJSON, w.inputJSON) {
		diff = append(diff, "input_json")
	}
	if old.Enabled != w.enabled {
		diff = append(diff, "enabled")
	}
	// nil 表示清单没写 description：保留远端值。写了（含空串）才比较，这样能把描述清空。
	if w.description != nil && old.Description != *w.description {
		diff = append(diff, "description")
	}
	return diff
}

// devHintProjectMissing 把「目标实例上还没有这个项目」的 404 换成可执行的提示：
// 项目要由用户在控制台用同一 ID 建（dev 不建项目）。
func devHintProjectMissing(err error, t devTarget) error {
	var api *gosdk.APIError
	if !errors.As(err, &api) {
		return err
	}
	if api.Status != 404 || (api.Code != "database_not_found" && api.Code != "not_found") {
		return err
	}
	return &devError{code: "project_not_found", exit: exitAPI,
		msg: fmt.Sprintf("目标 %s 上没有项目 %s：先在控制台「新建项目」里用同一个项目 ID 建项目并签发 Key", t.Label, t.Project)}
}

func (r *devReplayer) cron(ctx context.Context) error {
	if len(r.w.cronJobs) == 0 {
		return nil
	}
	r.step = "cron list"
	raw, err := r.c.ListCronJobs(ctx)
	if err != nil {
		return err
	}
	var listed struct {
		Jobs []devRemoteCron `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return err
	}
	if r.entry.Cron == nil {
		r.entry.Cron = map[string]devLockCron{}
	}
	for _, w := range r.w.cronJobs {
		r.step = "cron " + w.name
		var old *devRemoteCron
		for i := range listed.Jobs {
			if listed.Jobs[i].Name == w.name {
				old = &listed.Jobs[i]
				break
			}
		}
		if old == nil && w.kind == "once" {
			if at, err := time.Parse(time.RFC3339, w.runAt); err == nil && !at.After(r.now()) {
				r.warn("cron %s: run_at 已过期，跳过创建", w.name)
				r.act("skip", "cron", w.name, "run_at 已过期")
				continue
			}
		}
		if old == nil {
			r.act("create", "cron", w.name, w.kind)
			if r.apply {
				created, err := r.c.CreateCronJob(ctx, devCronInput(w))
				if err != nil {
					return err
				}
				var j struct {
					ID string `json:"id"`
				}
				_ = json.Unmarshal(created, &j)
				r.entry.Cron[w.name] = devLockCron{ID: j.ID}
			}
			continue
		}
		if old.TargetKind != "" && old.TargetKind != "function" {
			return &devError{code: "cron_target_mismatch", exit: exitAPI,
				msg: fmt.Sprintf("远端 cron %s 的目标不是云函数（%s），换个任务名", w.name, old.TargetKind)}
		}
		if diff := devCronDiff(*old, w); len(diff) > 0 {
			r.act("update", "cron", w.name, strings.Join(diff, ","))
			if r.apply {
				// update 是全量替换：清单没写 description 时用远端现值，避免被清空。
				if w.description == nil {
					w.description = &old.Description
				}
				if _, err := r.c.UpdateCronJob(ctx, old.ID, devCronInput(w)); err != nil {
					return err
				}
			}
		} else {
			r.act("skip", "cron", w.name, "")
		}
		if r.apply {
			r.entry.Cron[w.name] = devLockCron{ID: old.ID}
		}
	}
	return nil
}

// writeDevEnvHint 生成不含密钥的环境提示文件，业务侧复制需要的行。
func writeDevEnvHint(w *devWorkspace, t devTarget, order []string, ids map[string]string) (string, error) {
	lines := []string{
		"# simplebase dev 生成，不含密钥。SIMPLEBASE_API_KEY 请在该环境的控制台签发后自行填写。",
		"SIMPLEBASE_URL=" + t.URL,
		"SIMPLEBASE_PROJECT_ID=" + t.Project,
	}
	nonAlnum := regexp.MustCompile(`[^A-Za-z0-9]`)
	var known []string
	for _, name := range order {
		if ids[name] != "" {
			known = append(known, name)
		}
	}
	// 不带后缀的 SIMPLEBASE_DATABASE_ID 只在单库时写出；多库时依赖清单顺序容易用错。
	if len(known) == 1 {
		lines = append(lines, "SIMPLEBASE_DATABASE_ID="+ids[known[0]])
	}
	for _, name := range known {
		lines = append(lines, "SIMPLEBASE_DATABASE_ID_"+strings.ToUpper(nonAlnum.ReplaceAllString(name, "_"))+"="+ids[name])
	}
	rel := filepath.ToSlash(filepath.Join(devStateDir, "env."+t.Label))
	err := atomicfile.WriteEnsureDir(filepath.Join(w.root, devStateDir, "env."+t.Label), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	return rel, err
}

// ── 命令 ─────────────────────────────────────────────────

type devFlags struct {
	dir      string
	manifest string
	seed     bool
	yes      bool
}

func parseDevFlags(name string, args []string, allowSeed, allowYes bool) (devFlags, error) {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	f := devFlags{dir: "."}
	set.StringVar(&f.dir, "dir", ".", "")
	set.StringVar(&f.dir, "C", ".", "")
	if allowSeed { // plan / apply
		set.StringVar(&f.manifest, "manifest", "", "")
	}
	if allowSeed {
		set.BoolVar(&f.seed, "seed", false, "")
	}
	if allowYes {
		set.BoolVar(&f.yes, "yes", false, "")
	}
	if err := set.Parse(args); err != nil {
		return f, usagef("%s", err.Error())
	}
	if set.NArg() != 0 {
		return f, usagef("dev %s takes no positional arguments", name)
	}
	return f, nil
}

func devRejectAgent(cfg loadedConfig) error {
	if cfg.Agent {
		return &devError{code: "file_not_allowed", exit: exitUsage,
			msg: "dev commands read the host workspace and are unavailable in agent mode"}
	}
	return nil
}

func cmdDevPlan(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	if err := devRejectAgent(cfg); err != nil {
		return nil, err
	}
	f, err := parseDevFlags("plan", args, true, false)
	if err != nil {
		return nil, err
	}
	return runDev(ctx, c, cfg, f, false)
}

func cmdDevApply(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	if err := devRejectAgent(cfg); err != nil {
		return nil, err
	}
	f, err := parseDevFlags("apply", args, true, true)
	if err != nil {
		return nil, err
	}
	return runDev(ctx, c, cfg, f, true)
}

func runDev(ctx context.Context, c *gosdk.Client, cfg loadedConfig, f devFlags, apply bool) (any, error) {
	w, err := loadDevWorkspaceManifest(f.dir, f.manifest)
	if err != nil {
		return nil, err
	}
	if len(w.errors) > 0 {
		return nil, &devError{code: "workspace_invalid", exit: exitUsage,
			data: map[string]any{"errors": w.errors, "warnings": w.warnings},
			msg:  fmt.Sprintf("工作区校验失败（%d 个错误）：%s", len(w.errors), strings.Join(w.errors, "；"))}
	}
	target := devTargetOf(cfg)
	if w.projectID != "" && w.projectID != target.Project {
		w.warnf("目标 project %s 与清单 project.id %s 不同：业务侧将不能只换 URL", target.Project, w.projectID)
	}
	lock, err := loadDevLock(w)
	if err != nil {
		return nil, err
	}
	result := func(mode string, r *devReplayer) *devResult {
		warns := append(append([]string(nil), w.warnings...), r.warns...)
		return &devResult{Mode: mode, Target: target, Seed: f.seed, Actions: nonNilActions(r.actions),
			Changes: r.changes, Skips: r.skips, Warnings: warns}
	}
	if apply && !target.isLocal() && !f.yes {
		preview := newDevReplayer(w, c, target, lock, false, f.seed)
		if err := preview.run(ctx); err != nil {
			return nil, err
		}
		res := result("plan", preview)
		if preview.changes == 0 {
			return res, nil
		}
		return nil, &devError{code: "confirmation_required", exit: exitConfirm, data: res,
			msg: fmt.Sprintf("目标是远端 %s（%s），共 %d 项变更；确认后加 --yes 重跑 apply", target.Label, target.key(), preview.changes)}
	}
	r := newDevReplayer(w, c, target, lock, apply, f.seed)
	r.checkEngine(ctx, lock)
	runErr := r.run(ctx)
	if apply {
		// 失败路径也保存已完成步骤的记录，下次重跑自然变为 skip。
		if saveErr := saveDevLock(w, lock); saveErr != nil && runErr == nil {
			runErr = &devError{code: "dev_failed", msg: "写入 lock 失败：" + saveErr.Error(), exit: exitUsage}
		}
	}
	if runErr != nil {
		var de *devError
		if errors.As(runErr, &de) && de.data == nil {
			de.data = result("apply", r)
		}
		return nil, runErr
	}
	mode := "plan"
	if apply {
		mode = "apply"
	}
	res := result(mode, r)
	if apply {
		hint, err := writeDevEnvHint(w, target, r.dbOrder, r.dbIDs)
		if err != nil {
			return nil, &devError{code: "dev_failed", msg: "写入环境提示文件失败：" + err.Error(), exit: exitUsage}
		}
		res.EnvHint = hint
	}
	return res, nil
}

func nonNilActions(a []DevAction) []DevAction {
	if a == nil {
		return []DevAction{}
	}
	return a
}

// cmdDevValidate 只读工作区做离线校验，不访问远端，也不需要连接配置。
func cmdDevValidate(_ context.Context, _ *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	if err := devRejectAgent(cfg); err != nil {
		return nil, err
	}
	f, err := parseDevFlags("validate", args, true, false)
	if err != nil {
		return nil, err
	}
	w, err := loadDevWorkspaceManifest(f.dir, f.manifest)
	if err != nil {
		return nil, err
	}
	nMig := 0
	for _, d := range w.databases {
		nMig += len(d.migrations)
	}
	res := map[string]any{
		"errors":      nonNilStrings(w.errors),
		"warnings":    nonNilStrings(w.warnings),
		"databases":   len(w.databases),
		"migrations":  nMig,
		"collections": len(w.colls),
		"functions":   len(w.functions),
		"cron_jobs":   len(w.cronJobs),
	}
	if len(w.errors) > 0 {
		return nil, &devError{code: "workspace_invalid", exit: exitUsage, data: res,
			msg: fmt.Sprintf("工作区校验失败（%d 个错误）：%s", len(w.errors), strings.Join(w.errors, "；"))}
	}
	return res, nil
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func cmdDevStatus(_ context.Context, _ *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	if err := devRejectAgent(cfg); err != nil {
		return nil, err
	}
	f, err := parseDevFlags("status", args, false, false)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(f.dir)
	if err != nil {
		return nil, usagef("%v", err)
	}
	lock, err := loadDevLock(&devWorkspace{root: root})
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(lock.Targets))
	for k := range lock.Targets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	current := ""
	if cfg.Endpoint != "" { // status 不要求连接配置，缺配置时不给 current
		current = devTargetOf(cfg).key()
	}
	return map[string]any{
		"current": current,
		"keys":    keys,
		"targets": lock.Targets,
	}, nil
}
