// 文件说明：commands_resources.go — internal/sbcli 各资源命令的具体实现
// （database/sql/schema/collection/document/backup/agent/sandbox/log/user/project/apikey/settings/quota/audit）
//
// 用法
//
//	func cmdDatabaseList / cmdSQLQuery / cmdSchemaCreate / cmdAgentRun / cmdSandboxConnect ...（包内使用，经 dispatch 表注册）
//	同包调用：cmd* → commands.go(dispatch)；one() → redact.go, run.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sbcli/commands_resources.go
//	         │ 依赖
//	         ▼
//	[下游] packages/go-sdk
//	[同包] commands.go, collection_transfer.go, config.go, output.go, redact.go 等 7 个
//	[关键引用] cmd* ← commands.go(dispatch 表)
//
// 主要符号
//
//	cmdDatabaseList, cmdSQLQuery, cmdSchemaShow, cmdCollectionList, cmdDocumentGet,
//	cmdBackupList, cmdAgentRun, cmdSandboxConnect, cmdUserCreate, cmdProjectCreate 等

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
	f, _, err := mustFlags(args, "name", "file", "description", "source-b64", "activate")
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
	activate := true
	if s := strings.TrimSpace(*f["activate"]); s != "" {
		activate, err = strconv.ParseBool(s)
		if err != nil {
			return nil, usagef("--activate must be true or false")
		}
	}
	src, err := readSource(path, *f["source-b64"], stdin, cfg.Agent)
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateFunctionWith(ctx, name, string(src), *f["description"], activate)
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
	f, _, err := mustFlags(args, "name", "engine", "id")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateProjectWithID(ctx, strings.TrimSpace(*f["id"]), name, strings.TrimSpace(*f["engine"]))
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
