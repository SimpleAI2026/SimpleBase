// 文件说明：collection_transfer.go — internal/sbcli 的内部实现（cmdDocumentImport、cmdDocumentExport 等）
//
// 用法
//
//	同包调用：cmdDocumentImport() → commands.go；cmdDocumentExport() → commands.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sbcli/collection_transfer.go
//	         │ 依赖
//	         ▼
//	[下游] packages/go-sdk
//	[同包] commands.go, config.go, migrations.go, output.go, redact.go 等 6 个
//	[关键引用] cmdDocumentImport() ← commands.go
//
// 主要符号
//
//	cmdDocumentImport(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args ...
//	cmdDocumentExport(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args ...

package sbcli

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"strings"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func cmdDocumentImport(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error) {
	fs := flag.NewFlagSet("document import", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	database := fs.String("db", "", "database ID")
	collection := fs.String("collection", "", "collection name")
	format := fs.String("format", "ndjson", "csv or ndjson")
	file := fs.String("file", "", "input file")
	dryRun := fs.Bool("dry-run", false, "validate without writing")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *database == "" || *collection == "" || fs.NArg() != 0 {
		return nil, usagef("document import requires --db and --collection")
	}
	body := stdin
	var opened *os.File
	if *file != "" && *file != "-" {
		if cfg.Agent {
			return nil, usagef("file_not_allowed")
		}
		var err error
		opened, err = os.Open(*file)
		if err != nil {
			return nil, err
		}
		defer opened.Close()
		body = opened
	}
	return c.Database(*database).ImportCollection(ctx, *collection, strings.ToLower(*format), *dryRun, body)
}

func cmdDocumentExport(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	fs := flag.NewFlagSet("document export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	database := fs.String("db", "", "database ID")
	collection := fs.String("collection", "", "collection name")
	format := fs.String("format", "ndjson", "csv or ndjson")
	filter := fs.String("filter", "", "record filter")
	sort := fs.String("sort", "", "record sort")
	fields := fs.String("fields", "", "comma-separated fields")
	file := fs.String("file", "", "output file")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *database == "" || *collection == "" || fs.NArg() != 0 {
		return nil, usagef("document export requires --db and --collection")
	}
	if cfg.Agent {
		return nil, usagef("agent_raw_export_disabled")
	}
	query := gosdk.ExportQuery{Format: strings.ToLower(*format), Filter: *filter, Sort: *sort}
	if *fields != "" {
		query.Fields = strings.Split(*fields, ",")
	}
	var output bytes.Buffer
	if err := c.Database(*database).ExportCollection(ctx, *collection, query, &output); err != nil {
		return nil, err
	}
	if *file != "" && *file != "-" {
		if err := os.WriteFile(*file, output.Bytes(), 0o600); err != nil {
			return nil, err
		}
		return map[string]any{"file": *file, "bytes": output.Len()}, nil
	}
	return rawOutput{data: output.Bytes()}, nil
}
