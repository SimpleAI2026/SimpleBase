// 文件说明：internal/sbcli database / sql / schema 命令实现（原 commands_resources.go 拆出）
//
// 单 ID 的 database get / delete、schema show 直接在 commands.go 的命令表里用 idResult 适配，不在本文件重复。
package sbcli

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"strconv"
	"strings"

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

// sqlCommand 是 `sql query`（exec=false）与 `sql exec`（exec=true）共用的入口：flag 集合只在这里声明一次。
func sqlCommand(exec bool) commandFunc {
	return func(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, stdin io.Reader) (any, error) {
		f, _, err := mustFlags(args, "database", "statement", "param", "max-rows")
		if err != nil {
			return nil, err
		}
		return runSQL(ctx, c, f, stdin, exec)
	}
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
