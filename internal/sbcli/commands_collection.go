// 文件说明：internal/sbcli collection / document 命令实现（原 commands_resources.go 拆出）
package sbcli

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

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
