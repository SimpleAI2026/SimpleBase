package api

import (
	"fmt"
	"strings"
)

const maxSchemaColumns = 32

// schemaColumnType 是允许出现在 CREATE/ALTER 里的列类型。键为大写。
var schemaColumnTypes = map[string]struct{}{
	"BOOLEAN": {}, "TINYINT": {}, "SMALLINT": {}, "INTEGER": {}, "BIGINT": {},
	"UTINYINT": {}, "USMALLINT": {}, "UINTEGER": {}, "UBIGINT": {},
	"FLOAT": {}, "DOUBLE": {}, "DECIMAL": {},
	"VARCHAR": {}, "DATE": {}, "TIME": {}, "TIMESTAMP": {},
	"BLOB": {}, "JSON": {}, "UUID": {},
}

type schemaColumnSpec struct {
	Name     string
	Type     string
	Nullable bool
}

func normalizeSchemaType(raw string) (string, error) {
	typ := strings.ToUpper(strings.TrimSpace(raw))
	if _, ok := schemaColumnTypes[typ]; !ok {
		return "", fmt.Errorf("invalid column type %q", raw)
	}
	return typ, nil
}

func normalizeSchemaIdent(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if !collectionNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid identifier %q", raw)
	}
	return name, nil
}

func buildCreateTableSQL(table string, cols []schemaColumnSpec) (string, error) {
	table, err := normalizeSchemaIdent(table)
	if err != nil {
		return "", err
	}
	if len(cols) == 0 || len(cols) > maxSchemaColumns {
		return "", fmt.Errorf("column count must be 1..%d", maxSchemaColumns)
	}
	seen := map[string]struct{}{}
	defs := make([]string, 0, len(cols))
	for _, col := range cols {
		name, err := normalizeSchemaIdent(col.Name)
		if err != nil {
			return "", err
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return "", fmt.Errorf("duplicate column %q", name)
		}
		seen[key] = struct{}{}
		typ, err := normalizeSchemaType(col.Type)
		if err != nil {
			return "", err
		}
		def := quoteIdentifier(name) + " " + typ
		if !col.Nullable {
			def += " NOT NULL"
		}
		defs = append(defs, def)
	}
	return "CREATE TABLE " + quoteIdentifier(table) + " (" + strings.Join(defs, ", ") + ")", nil
}

func buildAddColumnSQL(table string, col schemaColumnSpec) (string, error) {
	table, err := normalizeSchemaIdent(table)
	if err != nil {
		return "", err
	}
	name, err := normalizeSchemaIdent(col.Name)
	if err != nil {
		return "", err
	}
	typ, err := normalizeSchemaType(col.Type)
	if err != nil {
		return "", err
	}
	sql := "ALTER TABLE " + quoteIdentifier(table) + " ADD COLUMN " + quoteIdentifier(name) + " " + typ
	if !col.Nullable {
		sql += " NOT NULL"
	}
	return sql, nil
}
