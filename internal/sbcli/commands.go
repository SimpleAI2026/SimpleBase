// 文件说明：commands.go — internal/sbcli 中 CommandReadOnly、CommandReadOnlyWithArgv 等符号的实现
//
// 用法
//
//	CommandReadOnly(resource, action string) bool  // CommandReadOnly 报告命令是否只读（planv6.0 v2 §3.4）
//	CommandReadOnlyWithArgv(argv []string) bool  // CommandReadOnlyWithArgv 处理带子动作的命令（planv6.0 v2 §3.4）：
//	同包调用：one() → redact.go, run.go；need() → redact.go；dispatch() → run.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sbcli/commands.go
//	         │ 依赖
//	         ▼
//	[下游] packages/go-sdk
//	[同包] collection_transfer.go, config.go, migrations.go, output.go, redact.go 等 6 个
//	[关键引用] one() ← redact.go, run.go
//
// 主要符号
//
//	CommandReadOnly(resource, action string) bool
//	CommandReadOnlyWithArgv(argv []string) bool
//	... 另有 91 个顶层符号

package sbcli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

type commandFunc func(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error)

// readOnlyCommands 是「不改变任何服务端状态」的命令集合（planv6.0 v2 §3.4）。
// 它是助手写权限分级（不可写 Agent 只能执行这里登记的命令）的唯一事实来源：
// gocloudagent 的 isWriteArgv 直接查 CommandReadOnly，而不是再维护一份会漂移的表。
//
// 判定口径：只做 GET/查询类读取，或只影响沙盒内临时环境（sandbox exec/run）
// 而不触碰项目持久数据。凡不在表内的命令一律视为写命令（fail-closed）。
//
// 带子动作的命令（function versions / sandbox files / log retention）不在此表，
// 由 CommandReadOnlyWithArgv 按第三段细分。
var readOnlyCommands = map[string]bool{
	"database list":         true,
	"database get":          true,
	"sql query":             true,
	"schema show":           true,
	"schema rows":           true,
	"collection list":       true,
	"collection schema-get": true,
	"document list":         true,
	"document get":          true,
	"document export":       true,
	"object list":           true,
	"object presign":        true,
	"function list":         true,
	"function get":          true,
	"function test":         true,
	"cron list":             true,
	"cron get":              true,
	"cron runs":             true,
	"sandbox capabilities":  true,
	"sandbox list":          true,
	"sandbox get":           true,
	"log search":            true,
	"user list":             true,
	"user get":              true,
	"apikey list":           true,
	"project list":          true,
	"quota get":             true,
	"audit list":            true,
	"backup list":           true,
	"migrate status":        true,
	"migrate plan":          true,
	"settings get":          true,
}

// CommandReadOnly 报告命令是否只读（planv6.0 v2 §3.4）。
// 未知命令返回 false（fail-closed）：新增命令必须显式登记才会被放行。
func CommandReadOnly(resource, action string) bool {
	return readOnlyCommands[resource+" "+action]
}

// CommandReadOnlyWithArgv 处理带子动作的命令（planv6.0 v2 §3.4）：
//   - log retention get|set
//   - function versions list|get|create|activate
//
// 其余命令退化为 CommandReadOnly（只看前两段）。沙盒的 files 子命令已删除。
func CommandReadOnlyWithArgv(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	resource := argv[0]
	action := ""
	if len(argv) > 1 {
		action = argv[1]
	}
	sub := ""
	if len(argv) > 2 {
		sub = argv[2]
	}
	switch resource + " " + action {
	case "log retention":
		return sub == "get"
	case "function versions":
		return sub == "list" || sub == "get"
	}
	return CommandReadOnly(resource, action)
}

func dispatch(resource, action string) commandFunc {
	switch resource + " " + action {
	case "database list":
		return cmdDatabaseList
	case "database get":
		return cmdDatabaseGet
	case "database create":
		return cmdDatabaseCreate
	case "database delete":
		return cmdDatabaseDelete
	case "sql query":
		return cmdSQLQuery
	case "sql exec":
		return cmdSQLExec
	case "sql batch":
		return cmdSQLBatch
	case "schema show":
		return cmdSchemaShow
	case "schema rows":
		return cmdSchemaRows
	case "schema create-table":
		return cmdSchemaCreate
	case "schema add-column":
		return cmdSchemaAddColumn
	case "collection list":
		return cmdCollectionList
	case "collection create":
		return cmdCollectionCreate
	case "collection truncate":
		return cmdCollectionTruncate
	case "collection schema-get":
		return cmdCollectionSchemaGet
	case "collection schema-put":
		return cmdCollectionSchemaPut
	case "document list":
		return cmdDocumentList
	case "document get":
		return cmdDocumentGet
	case "document import":
		return cmdDocumentImport
	case "document export":
		return cmdDocumentExport
	case "document insert":
		return cmdDocumentInsert
	case "document update":
		return cmdDocumentUpdate
	case "document patch":
		return cmdDocumentPatch
	case "document batch":
		return cmdDocumentBatch
	case "document delete":
		return cmdDocumentDelete
	case "backup list":
		return cmdBackupList
	case "backup create":
		return cmdBackupCreate
	case "backup restore":
		return cmdBackupRestore
	case "backup delete":
		return cmdBackupDelete
	case "migrate status":
		return cmdMigrateStatus
	case "migrate plan":
		return cmdMigratePlan
	case "kv exec":
		return cmdKV
	case "object list":
		return cmdObjectList
	case "object upload":
		return cmdObjectUpload
	case "object delete":
		return cmdObjectDelete
	case "object presign":
		return cmdObjectPresign
	case "function list":
		return cmdFunctionList
	case "function get":
		return cmdFunctionGet
	case "function create":
		return cmdFunctionCreate
	case "function update":
		return cmdFunctionUpdate
	case "function delete":
		return cmdFunctionDelete
	case "function version":
		return cmdFunctionVersions
	case "function test":
		return cmdFunctionTest
	case "function invoke":
		return cmdFunctionInvoke
	case "function activate":
		return cmdFunctionActivate
	case "cron list":
		return cmdCronList
	case "cron get":
		return cmdCronGet
	case "cron create":
		return cmdCronCreate
	case "cron update":
		return cmdCronUpdate
	case "cron delete":
		return cmdCronDelete
	case "cron runs":
		return cmdCronRuns
	case "cron trigger":
		return cmdCronTrigger
	case "sandbox capabilities":
		return cmdSandboxCaps
	case "sandbox list":
		return cmdSandboxList
	case "sandbox get":
		return cmdSandboxGet
	case "sandbox create":
		return cmdSandboxCreate
	case "sandbox delete":
		return cmdSandboxDelete
	case "sandbox connect":
		return cmdSandboxConnect
	case "sandbox pause":
		return cmdSandboxPause
	case "log search":
		return cmdLogSearch
	case "log retention":
		return cmdLogRetention
	case "user list":
		return cmdUserList
	case "user get":
		return cmdUserGet
	case "user create":
		return cmdUserCreate
	case "user update":
		return cmdUserUpdate
	case "user disable":
		return cmdUserDisable
	case "user delete":
		return cmdUserDelete
	case "project list":
		return cmdProjectList
	case "project create":
		return cmdProjectCreate
	case "apikey list":
		return cmdAPIKeyList
	case "apikey create":
		return cmdAPIKeyCreate
	case "apikey revoke":
		return cmdAPIKeyRevoke
	case "settings get":
		return cmdSettingsGet
	case "settings set":
		return cmdSettingsSet
	case "quota get":
		return cmdQuotaGet
	case "audit list":
		return cmdAuditList
	default:
		return dispatchNested(resource, action)
	}
}

func dispatchNested(resource, action string) commandFunc {
	switch resource + " " + action {
	case "function versions":
		return cmdFunctionVersions
	default:
		return nil
	}
}

func mustFlags(args []string, names ...string) (map[string]*string, *flag.FlagSet, error) {
	set := flag.NewFlagSet("simplebase", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	out := map[string]*string{}
	for _, n := range names {
		out[n] = set.String(n, "", "")
	}
	if err := set.Parse(args); err != nil {
		return nil, nil, usagef("%s", err.Error())
	}
	return out, set, nil
}

func need(flags map[string]*string, name string) (string, error) {
	v := strings.TrimSpace(*flags[name])
	if v == "" {
		return "", usagef("--%s is required", name)
	}
	return v, nil
}

func readFileOrDash(path string, stdin io.Reader) (string, error) {
	if path == "-" {
		b, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
		return string(b), err
	}
	b, err := os.ReadFile(path)
	return string(b), err
}

// readSource 在 readFileOrDash 基础上执行助手模式文件隔离（planv6.0 D3）：
// Agent 运行时（SIMPLEBASE_AGENT_RUN=1）只接受 stdin（"-"）或显式 base64 内联
// （sourceB64 非空），传入宿主机路径一律拒绝，杜绝助手读取宿主机任意文件。
// 人类操作者行为不变。
func readSource(path, sourceB64 string, stdin io.Reader, agent bool) (string, error) {
	if sourceB64 != "" {
		b, err := base64.StdEncoding.DecodeString(sourceB64)
		if err != nil {
			return "", usagef("--source-b64: invalid base64")
		}
		if len(b) > 256<<10 {
			return "", usagef("--source-b64: exceeds 256KiB limit")
		}
		return string(b), nil
	}
	if agent && path != "-" {
		return "", usagef("file_not_allowed: agent mode requires --file - (stdin) or --source-b64")
	}
	return readFileOrDash(path, stdin)
}

func decodeRaw(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{"ok": true}
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	return v
}

func stripAgentSecrets(data any) any {
	m, ok := data.(map[string]any)
	if !ok {
		return data
	}
	if _, has := m["secret"]; has {
		delete(m, "secret")
		m["secret_withheld"] = true
	}
	if url, ok := m["url"].(string); ok && strings.Contains(url, "://") {
		delete(m, "url")
		if _, ok := m["expires_at"]; !ok {
			m["expires_at"] = time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339)
		}
	}
	return m
}

func cmdDatabaseList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "limit", "cursor")
	if err != nil {
		return nil, err
	}
	limit, _ := strconv.Atoi(*f["limit"])
	return c.ListDatabases(ctx, limit, *f["cursor"])
}

func cmdDatabaseGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "id")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	return c.GetDatabase(ctx, id)
}

func cmdDatabaseCreate(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "data-model", "init-sql")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	in := gosdk.CreateDatabaseInput{Name: name, DataModel: *f["data-model"]}
	if p := strings.TrimSpace(*f["init-sql"]); p != "" {
		// planv6.0 D3：助手模式 --init-sql 只接受 "-"（stdin）。
		if cfg.Agent && p != "-" {
			return nil, usagef("file_not_allowed: agent mode requires --init-sql - (stdin)")
		}
		sql, err := readFileOrDash(p, stdin)
		if err != nil {
			return nil, err
		}
		in.InitSQL = sql
	}
	return c.CreateDatabaseWith(ctx, in)
}

func cmdDatabaseDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "id")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	return c.DeleteDatabase(ctx, id)
}

func cmdSQLQuery(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, stdin io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "statement", "param", "max-rows")
	if err != nil {
		return nil, err
	}
	return runSQL(ctx, c, f, stdin, false)
}

func cmdSQLExec(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, stdin io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "statement", "param", "max-rows")
	if err != nil {
		return nil, err
	}
	return runSQL(ctx, c, f, stdin, true)
}

func runSQL(ctx context.Context, c *gosdk.Client, f map[string]*string, stdin io.Reader, exec bool) (any, error) {
	db, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	stmt, err := need(f, "statement")
	if err != nil {
		return nil, err
	}
	if stmt == "-" {
		b, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
		if err != nil {
			return nil, err
		}
		stmt = string(b)
	}
	var params []any
	if p := strings.TrimSpace(*f["param"]); p != "" {
		if err := json.Unmarshal([]byte(p), &params); err != nil {
			return nil, usagef("--param must be a JSON array")
		}
	}
	scoped := c.Database(db)
	if exec {
		return scoped.Execute(ctx, stmt, params)
	}
	maxRows, _ := strconv.Atoi(*f["max-rows"])
	return scoped.Query(ctx, stmt, params, maxRows)
}

func cmdSQLBatch(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error) {
	set := flag.NewFlagSet("batch", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	db := set.String("database", "", "")
	file := set.String("file", "", "")
	statement := set.String("statement", "", "")
	transactional := set.Bool("transactional", false, "")
	if err := set.Parse(args); err != nil {
		return nil, usagef("%s", err.Error())
	}
	if strings.TrimSpace(*db) == "" || (strings.TrimSpace(*file) == "" && strings.TrimSpace(*statement) == "") {
		return nil, usagef("--database and (--file | --statement) are required")
	}
	// planv6.0 D3：助手模式 --file 只接受 "-"（stdin JSON）；人类可用路径或
	// --statement 内联。
	var b []byte
	if strings.TrimSpace(*statement) != "" {
		b = []byte(*statement)
	} else {
		if cfg.Agent && *file != "-" {
			return nil, usagef("file_not_allowed: agent mode requires --file - (stdin) or --statement")
		}
		var r io.Reader
		if *file == "-" {
			r = io.LimitReader(stdin, 1<<20)
		} else {
			fh, err := os.Open(*file)
			if err != nil {
				return nil, err
			}
			defer fh.Close()
			r = fh
		}
		var err error
		b, err = io.ReadAll(r)
		if err != nil {
			return nil, err
		}
	}
	var stmts []gosdk.SQLStatement
	if err := json.Unmarshal(b, &stmts); err != nil {
		return nil, usagef("batch input must be a JSON array of {sql, args}")
	}
	return c.Database(*db).Batch(ctx, stmts, *transactional)
}

func cmdSchemaShow(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	return c.DatabaseSchema(ctx, id)
}

func cmdSchemaRows(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "table", "limit")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	table, err := need(f, "table")
	if err != nil {
		return nil, err
	}
	limit, _ := strconv.Atoi(*f["limit"])
	return c.ListTableRows(ctx, id, table, limit)
}

func cmdSchemaCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "name", "columns")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	cols, err := need(f, "columns")
	if err != nil {
		return nil, err
	}
	var columns []gosdk.SchemaColumn
	if err := json.Unmarshal([]byte(cols), &columns); err != nil {
		return nil, usagef("--columns must be a JSON array")
	}
	return c.CreateTable(ctx, id, gosdk.SchemaTable{Name: name, Columns: columns})
}

func cmdSchemaAddColumn(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "table", "column")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	table, err := need(f, "table")
	if err != nil {
		return nil, err
	}
	raw, err := need(f, "column")
	if err != nil {
		return nil, err
	}
	var col gosdk.SchemaColumn
	if err := json.Unmarshal([]byte(raw), &col); err != nil {
		return nil, usagef("--column must be JSON")
	}
	return c.AddColumn(ctx, id, table, col)
}

func cmdCollectionList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := databaseFlag(args)
	if err != nil {
		return nil, err
	}
	return c.Database(id).ListCollections(ctx)
}

func cmdCollectionCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "name")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	if err := c.Database(id).CreateCollection(ctx, name); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "name": name}, nil
}

func cmdDocumentList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "collection", "limit", "cursor", "offset", "filter", "sort", "fields", "count")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "collection")
	if err != nil {
		return nil, err
	}
	query := gosdk.DocumentQuery{Cursor: *f["cursor"], Filter: *f["filter"], Sort: *f["sort"], Count: *f["count"] == "1" || *f["count"] == "true"}
	if *f["fields"] != "" {
		query.Fields = strings.Split(*f["fields"], ",")
	}
	for name, target := range map[string]*int{"limit": &query.Limit, "offset": &query.Offset} {
		if *f[name] == "" {
			continue
		}
		value, err := strconv.Atoi(*f[name])
		if err != nil {
			return nil, usagef("--%s must be an integer", name)
		}
		*target = value
	}
	return c.Database(id).ListDocumentsWithOptions(ctx, name, query)
}

func cmdDocumentInsert(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	return docWrite(ctx, c, args, "insert")
}

func cmdDocumentUpdate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	return docWrite(ctx, c, args, "update")
}

func cmdDocumentDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "collection", "id")
	if err != nil {
		return nil, err
	}
	db, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	col, err := need(f, "collection")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	if err := c.Database(db).DeleteDocument(ctx, col, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdCollectionTruncate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	db, col, err := documentCollectionFlags(args)
	if err != nil {
		return nil, err
	}
	if err := c.Database(db).TruncateCollection(ctx, col); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdCollectionSchemaGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	db, col, err := documentCollectionFlags(args)
	if err != nil {
		return nil, err
	}
	return c.Database(db).GetCollectionSchema(ctx, col)
}

func cmdCollectionSchemaPut(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "collection", "json")
	if err != nil {
		return nil, err
	}
	db, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	col, err := need(f, "collection")
	if err != nil {
		return nil, err
	}
	raw, err := need(f, "json")
	if err != nil {
		return nil, err
	}
	var schema gosdk.CollectionSchema
	if err := json.Unmarshal([]byte(raw), &schema); err != nil {
		return nil, usagef("--json must be a collection schema")
	}
	schema.Name = col
	return c.Database(db).PutCollectionSchema(ctx, schema)
}

func documentCollectionFlags(args []string) (string, string, error) {
	f, _, err := mustFlags(args, "database", "collection")
	if err != nil {
		return "", "", err
	}
	db, err := need(f, "database")
	if err != nil {
		return "", "", err
	}
	col, err := need(f, "collection")
	return db, col, err
}

func cmdDocumentGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "collection", "id")
	if err != nil {
		return nil, err
	}
	db, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	col, err := need(f, "collection")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	return c.Database(db).GetDocument(ctx, col, id)
}

func cmdDocumentPatch(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	return docWrite(ctx, c, args, "patch")
}

func cmdDocumentBatch(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "database", "collection", "json")
	if err != nil {
		return nil, err
	}
	db, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	col, err := need(f, "collection")
	if err != nil {
		return nil, err
	}
	raw, err := need(f, "json")
	if err != nil {
		return nil, err
	}
	var documents []gosdk.Document
	if err := json.Unmarshal([]byte(raw), &documents); err != nil || len(documents) == 0 {
		return nil, usagef("--json must be a non-empty document array")
	}
	items, err := c.Database(db).BatchDocuments(ctx, col, documents)
	return map[string]any{"items": items}, err
}

func cmdBackupList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	items, err := c.ListBackups(ctx)
	return map[string]any{"items": items}, err
}

func cmdBackupCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	return c.CreateBackup(ctx)
}

func cmdBackupRestore(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	return backupIDCommand(ctx, args, c.RestoreBackup)
}

func cmdBackupDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	return backupIDCommand(ctx, args, c.DeleteBackup)
}

func backupIDCommand(ctx context.Context, args []string, action func(context.Context, string) error) (any, error) {
	f, _, err := mustFlags(args, "id")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	if err := action(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func docWrite(ctx context.Context, c *gosdk.Client, args []string, kind string) (any, error) {
	f, _, err := mustFlags(args, "database", "collection", "id", "json")
	if err != nil {
		return nil, err
	}
	db, err := need(f, "database")
	if err != nil {
		return nil, err
	}
	col, err := need(f, "collection")
	if err != nil {
		return nil, err
	}
	raw, err := need(f, "json")
	if err != nil {
		return nil, err
	}
	var doc gosdk.Document
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, usagef("--json must be an object")
	}
	if kind == "update" || kind == "patch" {
		id, err := need(f, "id")
		if err != nil {
			return nil, err
		}
		if kind == "patch" {
			return c.Database(db).PatchDocument(ctx, col, id, doc)
		}
		return c.Database(db).UpdateDocument(ctx, col, id, doc)
	}
	return c.Database(db).InsertDocument(ctx, col, doc)
}

func databaseFlag(args []string) (string, error) {
	f, _, err := mustFlags(args, "database")
	if err != nil {
		return "", err
	}
	return need(f, "database")
}

func cmdKV(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	set := flag.NewFlagSet("kv", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	command := set.String("command", "", "")
	var argv []string
	set.Func("arg", "", func(s string) error {
		argv = append(argv, s)
		return nil
	})
	if err := set.Parse(args); err != nil {
		return nil, usagef("%s", err.Error())
	}
	if strings.TrimSpace(*command) == "" {
		return nil, usagef("--command is required")
	}
	raw, err := c.KVExec(ctx, append([]string{*command}, argv...))
	if err != nil {
		return nil, err
	}
	return decodeRaw(raw), nil
}

func cmdObjectList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "prefix")
	if err != nil {
		return nil, err
	}
	return c.ListObjects(ctx, *f["prefix"], false)
}

func cmdObjectUpload(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error) {
	f, _, err := mustFlags(args, "key", "file", "content-base64")
	if err != nil {
		return nil, err
	}
	key, err := need(f, "key")
	if err != nil {
		return nil, err
	}
	path, err := need(f, "file")
	if err != nil {
		return nil, err
	}
	// planv6.0 D3：助手模式宿主机文件不可读；内容经 --content-base64
	// 内联或 --file -（stdin）传入。人类操作者路径行为不变。
	if cfg.Agent && path != "-" && strings.TrimSpace(*f["content-base64"]) == "" {
		return nil, usagef("file_not_allowed: agent mode requires --file - (stdin) or --content-base64")
	}
	if b64 := strings.TrimSpace(*f["content-base64"]); b64 != "" {
		b, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, usagef("--content-base64: invalid base64")
		}
		return c.UploadObject(ctx, key, bytes.NewReader(b), path, "")
	}
	var r io.Reader
	if path == "-" {
		r = io.LimitReader(stdin, 50<<20)
	} else {
		fh, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer fh.Close()
		r = fh
	}
	return c.UploadObject(ctx, key, r, path, "")
}

func cmdObjectDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "key")
	if err != nil {
		return nil, err
	}
	key, err := need(f, "key")
	if err != nil {
		return nil, err
	}
	return c.DeleteObject(ctx, key)
}

func cmdObjectPresign(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "key")
	if err != nil {
		return nil, err
	}
	key, err := need(f, "key")
	if err != nil {
		return nil, err
	}
	res, err := c.PresignObject(ctx, key)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"key": key, "expires_at": time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339)}
	if !cfg.Agent {
		out["url"] = res.URL
	}
	return out, nil
}

func cmdFunctionList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.ListFunctions(ctx)
	return decodeRaw(raw), err
}

func cmdFunctionGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	name, err := one(args, "name")
	if err != nil {
		return nil, err
	}
	raw, err := c.GetFunction(ctx, name)
	return decodeRaw(raw), err
}

func cmdFunctionCreate(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "file", "description", "source-b64")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	path, err := need(f, "file")
	if err != nil {
		return nil, err
	}
	src, err := readSource(path, *f["source-b64"], stdin, cfg.Agent)
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateFunction(ctx, name, string(src), *f["description"])
	return decodeRaw(raw), err
}

func cmdFunctionUpdate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "description")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	raw, err := c.UpdateFunction(ctx, name, *f["description"])
	return decodeRaw(raw), err
}

func cmdFunctionDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	name, err := one(args, "name")
	if err != nil {
		return nil, err
	}
	if err := c.DeleteFunction(ctx, name); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdFunctionVersions(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error) {
	// function versions list|get|create  --name --version --file
	if len(args) == 0 {
		return nil, usagef("usage: function versions <list|get|create>")
	}
	sub := args[0]
	f, _, err := mustFlags(args[1:], "name", "version", "file", "note", "source-b64")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	switch sub {
	case "list":
		raw, err := c.ListFunctionVersions(ctx, name)
		return decodeRaw(raw), err
	case "get":
		ver, err := need(f, "version")
		if err != nil {
			return nil, err
		}
		raw, err := c.GetFunctionVersion(ctx, name, ver)
		return decodeRaw(raw), err
	case "create":
		path, err := need(f, "file")
		if err != nil {
			return nil, err
		}
		var b64 string
		if f["source-b64"] != nil {
			b64 = *f["source-b64"]
		}
		src, err := readSource(path, b64, stdin, cfg.Agent)
		if err != nil {
			return nil, err
		}
		raw, err := c.CreateFunctionVersion(ctx, name, string(src), *f["note"], false)
		return decodeRaw(raw), err
	default:
		return nil, usagef("unknown function versions action")
	}
}

func cmdFunctionActivate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "version")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	ver, err := need(f, "version")
	if err != nil {
		return nil, err
	}
	raw, err := c.ActivateFunctionVersion(ctx, name, ver)
	return decodeRaw(raw), err
}

func cmdFunctionTest(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "version", "export", "input")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	ver, err := need(f, "version")
	if err != nil {
		return nil, err
	}
	export, err := need(f, "export")
	if err != nil {
		return nil, err
	}
	body := json.RawMessage(*f["input"])
	raw, err := c.TestFunction(ctx, name, ver, export, body)
	return decodeRaw(raw), err
}

func cmdFunctionInvoke(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "export", "input")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	export, err := need(f, "export")
	if err != nil {
		return nil, err
	}
	body := json.RawMessage(*f["input"])
	raw, err := c.InvokeFunction(ctx, name, export, body)
	return decodeRaw(raw), err
}

func cronInput(f map[string]*string) (gosdk.CronJobInput, error) {
	in := gosdk.CronJobInput{
		Name: *f["name"], Description: *f["description"], ScheduleKind: *f["schedule-kind"],
		CronExpr: *f["cron"], RunAt: *f["run-at"], FuncFile: *f["func-file"], FuncExport: *f["func-export"],
		InputJSON: *f["input"],
	}
	if s := strings.TrimSpace(*f["interval-seconds"]); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return in, usagef("--interval-seconds must be an integer")
		}
		in.IntervalSeconds = n
	}
	if s := strings.TrimSpace(*f["enabled"]); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return in, usagef("--enabled must be true or false")
		}
		in.Enabled = &b
	}
	return in, nil
}

func cronFlags(args []string) (map[string]*string, error) {
	f, _, err := mustFlags(args, "id", "name", "description", "schedule-kind", "cron", "interval-seconds", "run-at", "func-file", "func-export", "input", "enabled")
	return f, err
}

func cmdCronList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.ListCronJobs(ctx)
	return decodeRaw(raw), err
}

func cmdCronGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	raw, err := c.GetCronJob(ctx, id)
	return decodeRaw(raw), err
}

func cmdCronCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, err := cronFlags(args)
	if err != nil {
		return nil, err
	}
	in, err := cronInput(f)
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateCronJob(ctx, in)
	return decodeRaw(raw), err
}

func cmdCronUpdate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, err := cronFlags(args)
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	in, err := cronInput(f)
	if err != nil {
		return nil, err
	}
	raw, err := c.UpdateCronJob(ctx, id, in)
	return decodeRaw(raw), err
}

func cmdCronDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	if err := c.DeleteCronJob(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdCronRuns(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	raw, err := c.ListCronRuns(ctx, id)
	return decodeRaw(raw), err
}

func cmdCronTrigger(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	raw, err := c.TriggerCronJob(ctx, id)
	return decodeRaw(raw), err
}

func cmdSandboxCaps(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	return c.SandboxCapabilities(ctx)
}

func cmdSandboxList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "state", "limit")
	if err != nil {
		return nil, err
	}
	limit, _ := strconv.Atoi(*f["limit"])
	return c.ListSandboxes(ctx, *f["state"], limit)
}

func cmdSandboxGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	return c.GetSandbox(ctx, id)
}

func cmdSandboxCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "template", "name", "timeout")
	if err != nil {
		return nil, err
	}
	in := gosdk.CreateSandboxInput{TemplateID: *f["template"], Name: *f["name"]}
	if s := strings.TrimSpace(*f["timeout"]); s != "" {
		n, perr := strconv.ParseInt(s, 10, 64)
		if perr != nil {
			return nil, usagef("--timeout must be an integer")
		}
		in.TimeoutS = n
	}
	return c.CreateSandbox(ctx, in)
}

func cmdSandboxDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	if err := c.DeleteSandbox(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdSandboxConnect(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "id", "timeout")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	timeout := int64(0)
	if s := strings.TrimSpace(*f["timeout"]); s != "" {
		n, perr := strconv.ParseInt(s, 10, 64)
		if perr != nil {
			return nil, usagef("--timeout must be an integer")
		}
		timeout = n
	}
	return c.ConnectSandbox(ctx, id, timeout)
}

func cmdSandboxPause(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	if err := c.PauseSandbox(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdLogSearch(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "level", "q", "limit")
	if err != nil {
		return nil, err
	}
	limit, _ := strconv.Atoi(*f["limit"])
	raw, err := c.SearchLogs(ctx, *f["level"], *f["q"], limit)
	return decodeRaw(raw), err
}

func cmdUserList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "limit", "cursor")
	if err != nil {
		return nil, err
	}
	limit, _ := strconv.Atoi(*f["limit"])
	raw, err := c.ListUsers(ctx, limit, *f["cursor"])
	return decodeRaw(raw), err
}

func cmdUserGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	raw, err := c.GetUser(ctx, id)
	return decodeRaw(raw), err
}

func cmdUserCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, stdin io.Reader) (any, error) {
	if strings.Contains(strings.Join(args, " "), "--password") && !strings.Contains(strings.Join(args, " "), "--password-stdin") {
		return nil, usagef("passwords must be passed with --password-stdin")
	}
	f, _, err := mustFlags(args, "username", "role", "display-name", "email")
	if err != nil {
		return nil, err
	}
	user, err := need(f, "username")
	if err != nil {
		return nil, err
	}
	pw, err := io.ReadAll(io.LimitReader(stdin, 4096))
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateUser(ctx, gosdk.UserInput{
		Username: user, Password: strings.TrimSpace(string(pw)), Role: *f["role"],
		DisplayName: *f["display-name"], Email: *f["email"],
	})
	return decodeRaw(raw), err
}

func cmdUserUpdate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "id", "role", "display-name", "email")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	raw, err := c.UpdateUser(ctx, id, gosdk.UserInput{Role: *f["role"], DisplayName: *f["display-name"], Email: *f["email"]})
	return decodeRaw(raw), err
}

func cmdUserDisable(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	st := "disabled"
	raw, err := c.UpdateUser(ctx, id, gosdk.UserInput{Status: &st})
	return decodeRaw(raw), err
}

func cmdUserDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	if err := c.DeleteUser(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdProjectList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.ListProjects(ctx)
	return decodeRaw(raw), err
}

func cmdProjectCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "engine")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateProject(ctx, name, strings.TrimSpace(*f["engine"]))
	return decodeRaw(raw), err
}

func cmdAPIKeyList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.ListAPIKeys(ctx)
	return decodeRaw(raw), err
}

func cmdAPIKeyCreate(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "permissions")
	if err != nil {
		return nil, err
	}
	var perms []string
	if p := strings.TrimSpace(*f["permissions"]); p != "" {
		if err := json.Unmarshal([]byte(p), &perms); err != nil {
			perms = strings.Split(p, ",")
		}
	}
	key, err := c.CreateAPIKey(ctx, perms)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"id": key.ID, "permissions": key.Permissions}
	if cfg.Agent {
		out["secret_withheld"] = true
	} else {
		out["secret"] = key.Secret
	}
	return out, nil
}

func cmdAPIKeyRevoke(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	if err := c.RevokeAPIKey(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdSettingsGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.GetSettings(ctx)
	return decodeRaw(raw), err
}

func cmdSettingsSet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	rawJSON, err := one(args, "json")
	if err != nil {
		return nil, err
	}
	raw, err := c.PutSettings(ctx, json.RawMessage(rawJSON))
	return decodeRaw(raw), err
}

func cmdQuotaGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.GetQuota(ctx)
	return decodeRaw(raw), err
}

func cmdAuditList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "limit")
	if err != nil {
		return nil, err
	}
	limit, _ := strconv.Atoi(*f["limit"])
	raw, err := c.ListAudit(ctx, limit)
	return decodeRaw(raw), err
}

func one(args []string, name string) (string, error) {
	f, _, err := mustFlags(args, name)
	if err != nil {
		return "", err
	}
	return need(f, name)
}

// log retention is two actions under one resource word pair: handled here.
func init() {
	_ = cmdLogRetention
}

func cmdLogRetention(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	if len(args) == 0 {
		return nil, usagef("usage: log retention <get|set>")
	}
	switch args[0] {
	case "get":
		raw, err := c.GetLogRetention(ctx)
		return decodeRaw(raw), err
	case "set":
		f, _, err := mustFlags(args[1:], "days")
		if err != nil {
			return nil, err
		}
		days, err := strconv.Atoi(strings.TrimSpace(*f["days"]))
		if err != nil {
			return nil, usagef("--days is required")
		}
		raw, err := c.SetLogRetention(ctx, days)
		return decodeRaw(raw), err
	default:
		return nil, usagef("unknown log retention action")
	}
}
