// 文件说明：dev_workspace.go — `simplebase dev` 的工作区清单读取与离线校验（planv6.4 P0b）
//
// 与 skills/simplebase/scripts/sbdev.py 的 Workspace 行为一一对应：同一份清单、
// 同一组 fixture（testdata/dev）在两边得到相同的校验结论。
//
// 主要符号
//
//	loadDevWorkspace(dir string) (*devWorkspace, error)
//	type devWorkspace / devDatabase / devCollection / devFunction / devCron

package sbcli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	devManifestName    = "simplebase.json"
	devStateDir        = ".simplebase"
	devLockName        = "lock.json"
	devMigrationsTable = "_migrations"
	devMaxBatch        = 100 // limits.max_batch_statements 默认值（含记账语句）
	devMaxSource       = 256 * 1024
	devMaxFunctions    = 100
	devMaxCronInput    = 8 * 1024
)

var (
	devFuncName   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)
	devCronName   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,62}$`)
	devProjectID  = regexp.MustCompile(`^[A-Za-z0-9-]{8}$`)
	devExportFunc = regexp.MustCompile(`(?m)^func\s+([A-Z][A-Za-z0-9_]*)\s*\(`)
	devImportOne  = regexp.MustCompile(`(?m)^\s*import\s+(?:\w+\s+)?"([^"]+)"`)
	devImportBlk  = regexp.MustCompile(`(?ms)^\s*import\s*\((.*?)\)`)
	devQuoted     = regexp.MustCompile(`"([^"]+)"`)
	devFirstKw    = regexp.MustCompile(`^\s*\(?\s*([A-Za-z_]+)`)
	devReserved   = regexp.MustCompile(`(?i)\b(__sb_|__ducklake_metadata_|sqlite_)\w*`)
	devNoDuckLake = regexp.MustCompile(`(?i)\b(PRIMARY\s+KEY|UNIQUE|CREATE\s+(UNIQUE\s+)?INDEX|CREATE\s+SEQUENCE)\b`)
	devMigTable   = regexp.MustCompile(`(?i)\b` + devMigrationsTable + `\b`)

	devAllowedImports = map[string]bool{
		"fmt": true, "strings": true, "math": true, "time": true, "regexp": true, "strconv": true,
		"errors": true, "bytes": true, "io": true, "io/ioutil": true, "sync/atomic": true,
		"net/http": true, "encoding/json": true, "encoding/base64": true, "encoding/binary": true,
		"github.com/json-iterator/go": true,
	}
	devDeniedKeywords = map[string]bool{
		"ATTACH": true, "DETACH": true, "INSTALL": true, "LOAD": true, "LOAD_EXTENSION": true,
		"COPY": true, "EXPORT": true, "IMPORT": true, "SET": true, "PRAGMA": true, "CHECKPOINT": true,
		"CALL": true, "USE": true, "VACUUM": true, "PREPARE": true, "EXECUTE": true, "DEALLOCATE": true,
	}
	devSecretPatterns = []struct {
		kind string
		re   *regexp.Regexp
	}{
		{"私钥", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
		{"OpenAI 风格 key", regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`)},
		{"腾讯云 SecretId", regexp.MustCompile(`\bAKID[A-Za-z0-9]{13,}`)},
		{"AWS AccessKey", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
		{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`)},
		{"SimpleBase API Key", regexp.MustCompile(`\bsb_live_[0-9a-f]{32}\b`)},
	}
	// 不进入重放的清单字段：值是忽略原因。
	devIgnoredKeys = map[string]string{
		"kv":       "kv 属于数据，P1 再纳入 --seed",
		"objects":  "对象属于数据，P1 再纳入 --seed",
		"api_keys": "签发 Key 会产出密钥，请在控制台操作",
		"agents":   "助手不在重放范围",
		"requires": "仅 examples 使用",
		"function_triggers": "事件触发器只存在于系统库，由 examples 初始化脚本幂等同步",
	}
)

type devManifest struct {
	Project struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
	Databases   []devDBDecl         `json:"databases"`
	Collections []devCollectionDecl `json:"collections"`
	Functions   []devFuncDecl       `json:"functions"`
	CronJobs    []devCronDecl       `json:"cron_jobs"`
}

type devDBDecl struct {
	Name       string `json:"name"`
	DataModel  string `json:"data_model"`
	Migrations string `json:"migrations"`
	Seed       string `json:"seed"`
}

type devCollectionDecl struct {
	Database string           `json:"database"`
	Name     string           `json:"name"`
	Seed     []map[string]any `json:"seed"`
}

type devFuncDecl struct {
	Name        string  `json:"name"`
	File        string  `json:"file"`
	Description *string `json:"description"`
	Test        *struct {
		FunctionName string          `json:"function_name"`
		Body         json.RawMessage `json:"body"`
	} `json:"test"`
}

type devCronDecl struct {
	Name            string          `json:"name"`
	Description     *string         `json:"description"`
	ScheduleKind    string          `json:"schedule_kind"`
	CronExpr        string          `json:"cron_expr"`
	IntervalSeconds *int64          `json:"interval_seconds"`
	RunAt           string          `json:"run_at"`
	FuncFile        string          `json:"func_file"`
	FuncExport      string          `json:"func_export"`
	InputJSON       string          `json:"input_json"`
	Enabled         *bool           `json:"enabled"`
	TriggerOnce     json.RawMessage `json:"trigger_once"`
}

// devSQLFile 是一个迁移或种子文件：marker 是写入 _migrations 的记账名。
type devSQLFile struct {
	marker string
	path   string
	stmts  []string
	sha    string
}

type devDatabase struct {
	name       string
	dataModel  string
	migrations []devSQLFile
	seed       *devSQLFile
}

type devCollection struct {
	database string
	name     string
	seed     []map[string]any
}

type devFuncTest struct {
	export string
	body   string
}

type devFunction struct {
	name        string
	source      string
	sha         string
	description *string
	test        *devFuncTest
	exports     []string
}

type devCron struct {
	name        string
	kind        string
	cronExpr    string
	interval    int64
	runAt       string
	funcFile    string
	funcExport  string
	inputJSON   string
	enabled     bool
	description *string // nil 表示清单没写，保留远端值；空串表示显式清空
}

// devWorkspace 是校验后的工作区：路径已解析，文件内容已读入。
type devWorkspace struct {
	root      string
	rootReal  string
	projectID string
	databases []devDatabase
	colls     []devCollection
	functions []devFunction
	cronJobs  []devCron
	errors    []string
	warnings  []string
}

func (w *devWorkspace) errf(format string, args ...any) {
	w.errors = append(w.errors, fmt.Sprintf(format, args...))
}

func (w *devWorkspace) warnf(format string, args ...any) {
	w.warnings = append(w.warnings, fmt.Sprintf(format, args...))
}

// loadDevWorkspace 读取并校验工作区。清单缺失或不是合法 JSON 直接返回错误；
// 其余问题累积在 errors/warnings 里，由调用方决定是否拒绝。
func loadDevWorkspace(dir string) (*devWorkspace, error) {
	return loadDevWorkspaceManifest(dir, "")
}

// loadDevWorkspaceManifest 与 loadDevWorkspace 相同，但清单文件可由 manifest 指定
// （相对路径按 dir 解析，也可以是绝对路径）；清单里引用的迁移、函数等文件始终相对 dir。
// examples 的初始化脚本用它传入一份派生清单，而不改动仓库里的 simplebase.json。
func loadDevWorkspaceManifest(dir, manifest string) (*devWorkspace, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, &devError{code: "workspace_invalid", msg: err.Error(), exit: exitUsage}
	}
	w := &devWorkspace{root: root, rootReal: root}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		w.rootReal = real
	}
	mpath := filepath.Join(root, devManifestName)
	if manifest != "" {
		mpath = manifest
		if !filepath.IsAbs(mpath) {
			mpath = filepath.Join(root, manifest)
		}
	}
	raw, err := os.ReadFile(mpath)
	if err != nil {
		return nil, &devError{code: "workspace_invalid", msg: fmt.Sprintf("%s 不存在：先运行 sbdev.py init 或创建清单", mpath), exit: exitUsage}
	}
	text := string(raw)
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, &devError{code: "workspace_invalid", msg: fmt.Sprintf("%s 不是合法 JSON 对象：%v", devManifestName, err), exit: exitUsage}
	}
	var m devManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, &devError{code: "workspace_invalid", msg: fmt.Sprintf("%s 字段类型不正确：%v", devManifestName, err), exit: exitUsage}
	}
	w.scanSecrets(devManifestName, text)
	keys := make([]string, 0, len(devIgnoredKeys))
	for k := range devIgnoredKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, ok := top[k]; ok {
			w.warnf("忽略 `%s`：%s", k, devIgnoredKeys[k])
		}
	}
	w.projectID = m.Project.ID
	if w.projectID != "" && !devProjectID.MatchString(w.projectID) {
		w.errf("project.id 必须是 8 位字母、数字或 -（当前 %q）", w.projectID)
	}
	withCollections := map[string]bool{}
	for _, c := range m.Collections {
		withCollections[c.Database] = true
	}
	w.checkDatabases(m.Databases, withCollections)
	w.checkCollections(m.Collections)
	w.checkFunctions(m.Functions)
	w.checkCron(m.CronJobs)
	return w, nil
}

func (w *devWorkspace) scanSecrets(label, text string) {
	for _, p := range devSecretPatterns {
		if p.re.MatchString(text) {
			w.errf("%s: 疑似包含%s，密钥不能进入工作区产物", label, p.kind)
		}
	}
}

func devWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (w *devWorkspace) safePath(rel, what string) (string, bool) {
	if strings.TrimSpace(rel) == "" {
		w.errf("%s: 路径为空", what)
		return "", false
	}
	if filepath.IsAbs(rel) {
		w.errf("%s: 必须是工作区内的相对路径（%s）", what, rel)
		return "", false
	}
	full := filepath.Join(w.root, rel)
	if !devWithin(w.root, full) {
		w.errf("%s: 路径越出工作区（%s）", what, rel)
		return "", false
	}
	if real, err := filepath.EvalSymlinks(full); err == nil && !devWithin(w.rootReal, real) {
		w.errf("%s: 路径越出工作区（%s）", what, rel)
		return "", false
	}
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/") {
		if strings.HasPrefix(part, ".env") {
			w.errf("%s: 不允许引用 .env 文件（%s）", what, rel)
			return "", false
		}
	}
	return full, true
}

// devSplitSQL 按 `;` 拆分语句。能识别单/双引号、`$` 美元引号、`--` 行注释与
// `/* */` 块注释，引号里的 `;` 不会拆错；注释会被去掉。
func devSplitSQL(text string) []string { return devSQLStatements(text, false) }

// devMaskSQL 与 devSplitSQL 拆分结果一一对应，但把字符串与美元引号的内容替换成空格，
// 关键字、保留前缀这类检查用它，避免字面量里的文本造成误判。
func devMaskSQL(text string) []string { return devSQLStatements(text, true) }

var devDollarTag = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)?\$`)

func devSQLStatements(text string, mask bool) []string {
	var out []string
	var cur strings.Builder
	blank := func(s string) {
		cur.WriteString(strings.Repeat(" ", len(s)))
	}
	put := func(s string) {
		if mask {
			blank(s)
			return
		}
		cur.WriteString(s)
	}
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	n := len(text)
	for i := 0; i < n; {
		c := text[i]
		switch {
		case c == '\'' || c == '"':
			cur.WriteByte(c)
			i++
			start := i
			for i < n {
				if text[i] == c {
					if i+1 < n && text[i+1] == c { // '' / "" 转义
						i += 2
						continue
					}
					break
				}
				i++
			}
			put(text[start:i])
			if i < n {
				cur.WriteByte(c)
				i++
			}
		case c == '-' && i+1 < n && text[i+1] == '-':
			for i < n && text[i] != '\n' {
				i++
			}
			cur.WriteByte(' ')
		case c == '/' && i+1 < n && text[i+1] == '*':
			i += 2
			for i+1 < n && !(text[i] == '*' && text[i+1] == '/') {
				i++
			}
			if i+1 < n {
				i += 2
			}
			cur.WriteByte(' ')
		case c == '$' && devDollarTag.MatchString(text[i:]):
			tag := devDollarTag.FindString(text[i:])
			cur.WriteString(tag)
			i += len(tag)
			start := i
			for i < n && !strings.HasPrefix(text[i:], tag) {
				i++
			}
			put(text[start:i])
			if strings.HasPrefix(text[i:], tag) {
				cur.WriteString(tag)
				i += len(tag)
			}
		case c == ';':
			flush()
			i++
		default:
			cur.WriteByte(c)
			i++
		}
	}
	flush()
	return out
}

func devFirstKeyword(stmt string) string {
	if m := devFirstKw.FindStringSubmatch(stmt); m != nil {
		return strings.ToUpper(m[1])
	}
	return ""
}

func (w *devWorkspace) sqlFile(rel, what string, isMigration bool) *devSQLFile {
	path, ok := w.safePath(rel, what)
	if !ok {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		w.errf("%s: 文件不存在（%s）", what, rel)
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		w.errf("%s: 读取失败（%s）", what, rel)
		return nil
	}
	text := string(b)
	w.scanSecrets(rel, text)
	stmts := devSplitSQL(text)
	if len(stmts) == 0 {
		w.errf("%s: 没有可执行语句", rel)
	}
	if len(stmts)+1 > devMaxBatch {
		w.errf("%s: %d 条语句超过单批上限 %d，请拆成多个文件", rel, len(stmts), devMaxBatch-1)
	}
	// 关键字与保留前缀用打了码的版本检查，字符串字面量里的同名文本不算。
	masks := devMaskSQL(text)
	for i, s := range stmts {
		n := i + 1
		probe := s
		if i < len(masks) {
			probe = masks[i]
		}
		kw := devFirstKeyword(probe)
		if devDeniedKeywords[kw] {
			w.errf("%s 第 %d 条: %s 被 sqlguard 拒绝", rel, n, kw)
		}
		if devReserved.MatchString(probe) {
			w.errf("%s 第 %d 条: 引用了保留前缀（__sb_ / __ducklake_metadata_ / sqlite_）", rel, n)
		}
		if devMigTable.MatchString(probe) {
			w.errf("%s 第 %d 条: %s 是迁移记账表，不要在迁移里操作", rel, n, devMigrationsTable)
		}
		if isMigration && devNoDuckLake.MatchString(probe) {
			w.warnf("%s 第 %d 条: PRIMARY KEY/UNIQUE/INDEX/SEQUENCE 在 DuckLake 上不受支持，examples 约定不用", rel, n)
		}
		if isMigration && (kw == "DROP" || kw == "DELETE" || kw == "TRUNCATE") {
			w.warnf("%s 第 %d 条: %s 是破坏性语句，云端重放前请确认", rel, n, kw)
		}
	}
	marker := "seed:" + rel
	if isMigration {
		marker = filepath.Base(path)
	}
	return &devSQLFile{marker: marker, path: path, stmts: stmts, sha: devSHA256(text)}
}

func (w *devWorkspace) checkDatabases(items []devDBDecl, withCollections map[string]bool) {
	seen := map[string]bool{}
	for i, d := range items {
		what := fmt.Sprintf("databases[%d]", i)
		name := strings.TrimSpace(d.Name)
		if name == "" || len(name) > 63 {
			w.errf("%s.name 必填且不超过 63 字符", what)
			continue
		}
		if seen[name] {
			w.errf("%s: 库名 %s 重复", what, name)
		}
		seen[name] = true
		model := d.DataModel
		if model == "" {
			// 缺省是 collection（与建库接口一致）；只有声明了迁移/种子、且没有集合引用它时才推断为 sql。
			model = "collection"
			if (d.Migrations != "" || d.Seed != "") && !withCollections[name] {
				model = "sql"
			}
		}
		if model != "sql" && model != "collection" {
			w.errf("%s.data_model 只能是 sql 或 collection", what)
			continue
		}
		// 声明了 migrations/seed 且未写 data_model 时按 sql 处理；显式写 collection 则允许
		// 同一个库既跑迁移 SQL 又存集合文档（examples 的 community / ops-assistant 就是这样）。
		db := devDatabase{name: name, dataModel: model}
		if d.Migrations != "" {
			if mdir, ok := w.safePath(d.Migrations, what+".migrations"); ok {
				entries, err := os.ReadDir(mdir)
				if err != nil {
					w.errf("%s.migrations: 目录不存在（%s）", what, d.Migrations)
				} else {
					for _, e := range entries {
						if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
							continue
						}
						rel := filepath.ToSlash(filepath.Join(d.Migrations, e.Name()))
						if f := w.sqlFile(rel, what+".migrations", true); f != nil {
							db.migrations = append(db.migrations, *f)
						}
					}
					if len(db.migrations) == 0 {
						w.warnf("%s.migrations: 目录里没有 .sql 文件", what)
					}
				}
			}
		}
		if d.Seed != "" {
			db.seed = w.sqlFile(d.Seed, what+".seed", false)
		}
		w.databases = append(w.databases, db)
	}
	if len(w.databases) > 1 {
		w.warnf("databases: 声明了 %d 个库，但每个项目只能有一个用户数据库，多出的库建库时会返回 database_already_exists", len(w.databases))
	}
}

func (w *devWorkspace) checkCollections(items []devCollectionDecl) {
	models := map[string]string{}
	for _, d := range w.databases {
		models[d.name] = d.dataModel
	}
	for i, c := range items {
		what := fmt.Sprintf("collections[%d]", i)
		model, ok := models[c.Database]
		if !ok {
			w.errf("%s.database 必须引用 databases 里的库（%q）", what, c.Database)
			continue
		}
		if model != "collection" {
			w.errf("%s: %s 是 sql 库，不能建集合", what, c.Database)
		}
		name := strings.TrimSpace(c.Name)
		if name == "" {
			w.errf("%s.name 必填", what)
			continue
		}
		for j, doc := range c.Seed {
			if id, _ := doc["id"].(string); id == "" {
				w.errf("%s.seed[%d] 必须是带字符串 id 的对象（用于去重）", what, j)
			}
		}
		if b, err := json.Marshal(c.Seed); err == nil {
			w.scanSecrets(what+".seed", string(b))
		}
		w.colls = append(w.colls, devCollection{database: c.Database, name: name, seed: c.Seed})
	}
}

func devImports(src string) []string {
	var out []string
	for _, m := range devImportOne.FindAllStringSubmatch(src, -1) {
		out = append(out, m[1])
	}
	for _, blk := range devImportBlk.FindAllStringSubmatch(src, -1) {
		for _, m := range devQuoted.FindAllStringSubmatch(blk[1], -1) {
			out = append(out, m[1])
		}
	}
	return out
}

var devPackageMain = regexp.MustCompile(`(?m)^\s*package\s+main\b`)

func (w *devWorkspace) checkFunctions(items []devFuncDecl) {
	if len(items) > devMaxFunctions {
		w.errf("functions 超过每项目 %d 个上限", devMaxFunctions)
	}
	seen := map[string]bool{}
	for i, f := range items {
		what := fmt.Sprintf("functions[%d]", i)
		if !devFuncName.MatchString(f.Name) {
			w.errf("%s.name 须字母开头、仅字母数字下划线、最长 63（%q）", what, f.Name)
			continue
		}
		if seen[f.Name] {
			w.errf("%s: 函数名 %s 重复", what, f.Name)
		}
		seen[f.Name] = true
		path, ok := w.safePath(f.File, what+".file")
		if !ok {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			w.errf("%s.file 不存在（%s）", what, f.File)
			continue
		}
		src := string(b)
		if len(b) > devMaxSource {
			w.errf("%s: 源码超过 256KiB", what)
		}
		if !devPackageMain.MatchString(src) {
			w.errf("%s: 源码必须是 package main", what)
		}
		var exports []string
		for _, m := range devExportFunc.FindAllStringSubmatch(src, -1) {
			exports = append(exports, m[1])
		}
		if len(exports) == 0 {
			w.errf("%s: 没有大写开头的顶层导出函数", what)
		}
		for _, imp := range devImports(src) {
			if !devAllowedImports[imp] {
				w.warnf("%s: import %q 不在已注册包清单里，编译可能失败", what, imp)
			}
		}
		w.scanSecrets(f.File, src)
		fn := devFunction{name: f.Name, source: src, sha: devSHA256(src), description: f.Description, exports: exports}
		if f.Test == nil {
			w.warnf("%s: 没有配置 test，apply 会不经试跑直接生效", what)
		}
		if f.Test != nil {
			switch {
			case f.Test.FunctionName == "":
				w.errf("%s.test 需要 function_name", what)
			case !devContains(exports, f.Test.FunctionName):
				w.errf("%s.test.function_name=%s 不是导出函数", what, f.Test.FunctionName)
			default:
				body := strings.TrimSpace(string(f.Test.Body))
				if body == "" || body == "null" {
					body = "{}"
				}
				fn.test = &devFuncTest{export: f.Test.FunctionName, body: body}
			}
		}
		w.functions = append(w.functions, fn)
	}
}

func devContains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (w *devWorkspace) checkCron(items []devCronDecl) {
	funcs := map[string]*devFunction{}
	for i := range w.functions {
		funcs[w.functions[i].name] = &w.functions[i]
	}
	seen := map[string]bool{}
	for i, j := range items {
		what := fmt.Sprintf("cron_jobs[%d]", i)
		if !devCronName.MatchString(j.Name) {
			w.errf("%s.name 须字母开头、字母数字 _-、最长 63（%q）", what, j.Name)
			continue
		}
		if seen[j.Name] {
			w.errf("%s: 任务名 %s 重复", what, j.Name)
		}
		seen[j.Name] = true
		if len(j.TriggerOnce) > 0 && string(j.TriggerOnce) != "null" {
			w.warnf("%s: 忽略 trigger_once（触发有副作用，不进入自动重放）", what)
		}
		c := devCron{
			name: j.Name, kind: j.ScheduleKind, funcFile: j.FuncFile, funcExport: j.FuncExport,
			inputJSON: j.InputJSON, enabled: true, description: j.Description,
		}
		if c.inputJSON == "" {
			c.inputJSON = "{}"
		}
		if j.Enabled != nil {
			c.enabled = *j.Enabled
		}
		switch j.ScheduleKind {
		case "cron":
			if len(strings.Fields(j.CronExpr)) != 5 {
				w.errf("%s.cron_expr 需要 5 段（分 时 日 月 周）", what)
			}
			c.cronExpr = j.CronExpr
		case "interval":
			if j.IntervalSeconds == nil || *j.IntervalSeconds < 60 || *j.IntervalSeconds > 2592000 {
				w.errf("%s.interval_seconds 必须是 60–2592000 的整数", what)
			} else {
				c.interval = *j.IntervalSeconds
			}
		case "once":
			if _, err := time.Parse(time.RFC3339, j.RunAt); err != nil {
				w.errf("%s.run_at 必须是 RFC3339 时间", what)
			}
			c.runAt = j.RunAt
		default:
			w.errf("%s.schedule_kind 只能是 cron / interval / once", what)
		}
		switch target, ok := funcs[c.funcFile]; {
		case c.funcFile == "" || c.funcExport == "":
			w.errf("%s: func_file 与 func_export 必填", what)
		case !ok:
			w.warnf("%s: func_file=%s 不在清单里，需远端已存在", what, c.funcFile)
		case !devContains(target.exports, c.funcExport):
			w.errf("%s: %s 不是 %s 的导出函数", what, c.funcExport, c.funcFile)
		}
		var probe any
		if json.Unmarshal([]byte(c.inputJSON), &probe) != nil {
			w.errf("%s.input_json 必须是 JSON 字符串", what)
		}
		if len(c.inputJSON) > devMaxCronInput {
			w.errf("%s.input_json 超过 8KB", what)
		}
		w.cronJobs = append(w.cronJobs, c)
	}
}
