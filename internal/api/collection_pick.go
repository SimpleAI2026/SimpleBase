// 文件说明：collection_pick.go — internal/api 的内部实现（selectedFieldPattern、parseFields 等）
//
// 用法
//
//	同包调用：parseFields() → collection_query.go, collection_transfer.go；hideDocumentFields() → collection_transfer.go, data_handler.go；pickDocument() → data_handler.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/collection_pick.go
//	         │ 依赖
//	         ▼
//	[下游] internal/collection
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] parseFields() ← collection_query.go, collection_transfer.go
//
// 主要符号
//
//	var selectedFieldPattern
//	parseFields(raw string, schema *collection.Collection) ([]string, error)
//	pickDocument(doc map[string]any, fields []string) map[string]any
//	nestedValue(doc map[string]any, parts []string) (any, bool)
//	hideDocumentFields(doc map[string]any, hidden []string) map[string]any

package api

import (
	"regexp"
	"strings"

	"github.com/linkxzhou/SimpleBase/internal/collection"
)

var selectedFieldPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}(?:\.[A-Za-z][A-Za-z0-9_]{0,62})*$`)

func parseFields(raw string, schema *collection.Collection) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	registered := map[string]collection.Field{}
	if schema != nil {
		for _, f := range schema.Fields {
			registered[f.Name] = f
		}
	}
	seen := map[string]struct{}{"id": {}}
	fields := []string{"id"}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if !selectedFieldPattern.MatchString(name) {
			return nil, &collection.ValidationError{Code: "invalid_fields", Field: name, Message: "invalid field selection"}
		}
		root := strings.SplitN(name, ".", 2)[0]
		if schema != nil {
			field, ok := registered[root]
			if (!ok || strings.Contains(name, ".") && field.Type != "json") && root != "created_at" && root != "updated_at" && root != "id" {
				return nil, &collection.ValidationError{Code: "unknown_field", Field: root, Message: "field is not registered or not selectable"}
			}
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			fields = append(fields, name)
		}
	}
	return fields, nil
}

func pickDocument(doc map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return doc
	}
	out := map[string]any{}
	for _, name := range fields {
		if name == "id" || name == "created_at" || name == "updated_at" {
			continue
		}
		parts := strings.Split(name, ".")
		value, ok := nestedValue(doc, parts)
		if !ok {
			continue
		}
		target := out
		for _, part := range parts[:len(parts)-1] {
			nested, ok := target[part].(map[string]any)
			if !ok {
				nested = map[string]any{}
				target[part] = nested
			}
			target = nested
		}
		target[parts[len(parts)-1]] = value
	}
	return out
}

func nestedValue(doc map[string]any, parts []string) (any, bool) {
	var current any = doc
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func hideDocumentFields(doc map[string]any, hidden []string) map[string]any {
	if len(hidden) == 0 {
		return doc
	}
	out := make(map[string]any, len(doc))
	for k, v := range doc {
		out[k] = v
	}
	for _, name := range hidden {
		delete(out, name)
	}
	return out
}
