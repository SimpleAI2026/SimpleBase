package sbcli

import (
	"context"
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
	case "document list":
		return cmdDocumentList
	case "document insert":
		return cmdDocumentInsert
	case "document update":
		return cmdDocumentUpdate
	case "document delete":
		return cmdDocumentDelete
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
	case "sandbox start":
		return cmdSandboxStart
	case "sandbox stop":
		return cmdSandboxStop
	case "sandbox exec":
		return cmdSandboxExec
	case "sandbox update":
		return cmdSandboxUpdate
	case "sandbox run":
		return cmdSandboxRun
	case "sandbox files":
		return cmdSandboxFiles
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

func fs(args []string) (*flag.FlagSet, error) {
	set := flag.NewFlagSet("simplebase", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	if err := set.Parse(args); err != nil {
		return nil, usagef("%s", err.Error())
	}
	return set, nil
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

func cmdDatabaseCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, stdin io.Reader) (any, error) {
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

func cmdSQLBatch(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	set := flag.NewFlagSet("batch", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	db := set.String("database", "", "")
	file := set.String("file", "", "")
	transactional := set.Bool("transactional", false, "")
	if err := set.Parse(args); err != nil {
		return nil, usagef("%s", err.Error())
	}
	if strings.TrimSpace(*db) == "" || strings.TrimSpace(*file) == "" {
		return nil, usagef("--database and --file are required")
	}
	b, err := os.ReadFile(*file)
	if err != nil {
		return nil, err
	}
	var stmts []gosdk.SQLStatement
	if err := json.Unmarshal(b, &stmts); err != nil {
		return nil, usagef("--file must be a JSON array of {sql, args}")
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
	f, _, err := mustFlags(args, "database", "collection")
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
	return c.Database(id).ListDocuments(ctx, name)
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
	if kind == "update" {
		id, err := need(f, "id")
		if err != nil {
			return nil, err
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

func cmdObjectUpload(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "key", "file")
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
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return c.UploadObject(ctx, key, fh, path, "")
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

func cmdFunctionCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "file", "description")
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
	src, err := os.ReadFile(path)
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

func cmdFunctionVersions(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	// function versions list|get|create  --name --version --file
	if len(args) == 0 {
		return nil, usagef("usage: function versions <list|get|create>")
	}
	sub := args[0]
	f, _, err := mustFlags(args[1:], "name", "version", "file", "note")
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
		src, err := os.ReadFile(path)
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

func cmdSandboxList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	return c.ListSandboxes(ctx, "", "", 0)
}

func cmdSandboxGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	return c.GetSandbox(ctx, id, false)
}

func cmdSandboxCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "image", "name")
	if err != nil {
		return nil, err
	}
	return c.CreateSandbox(ctx, gosdk.CreateSandboxInput{Image: *f["image"], Name: *f["name"]})
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

func cmdSandboxStart(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	return c.StartSandbox(ctx, id)
}

func cmdSandboxStop(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	return c.StopSandbox(ctx, id)
}

func cmdSandboxExec(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "id", "command")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	command, err := need(f, "command")
	if err != nil {
		return nil, err
	}
	return c.ExecSandbox(ctx, id, gosdk.SandboxExecInput{Command: command})
}

func cmdSandboxUpdate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "id", "name")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	var name *string
	if s := strings.TrimSpace(*f["name"]); s != "" {
		name = &s
	}
	return c.UpdateSandbox(ctx, id, gosdk.UpdateSandboxInput{Name: name})
}

func cmdSandboxRun(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "command", "image")
	if err != nil {
		return nil, err
	}
	command, err := need(f, "command")
	if err != nil {
		return nil, err
	}
	return c.RunSandbox(ctx, gosdk.SandboxRunInput{SandboxExecInput: gosdk.SandboxExecInput{Command: command}, Image: *f["image"]})
}

func cmdSandboxFiles(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	if len(args) == 0 {
		return nil, usagef("usage: sandbox files <list|read|write|delete>")
	}
	sub := args[0]
	f, _, err := mustFlags(args[1:], "id", "path", "content")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	path, err := need(f, "path")
	if err != nil {
		return nil, err
	}
	switch sub {
	case "list":
		return c.ListSandboxFiles(ctx, id, path)
	case "read":
		return c.ReadSandboxFile(ctx, id, path)
	case "write":
		if err := c.WriteSandboxFile(ctx, id, path, *f["content"]); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "delete":
		if err := c.DeleteSandboxFile(ctx, id, path); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	default:
		return nil, usagef("unknown sandbox files action")
	}
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
	name, err := one(args, "name")
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateProject(ctx, name)
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
