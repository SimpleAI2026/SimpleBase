// 文件说明：pick.go — 文档字段投影：fields 选择、隐藏字段剔除与嵌套取值
//
// 用法
//
//	FieldNamePattern                                             // 字段名形态
//	InvalidQuery(message string) error                           // 查询参数不合法
//	ParseFields(raw string, schema *Collection) ([]string, error)   // 解析 fields= 参数（逗号分隔，支持 a.b.c），只允许已登记且可选的字段
//	PickDocument(doc map[string]any, fields []string) map[string]any // 按解析结果投影文档
//	HideDocumentFields(doc map[string]any, hidden []string) map[string]any // 剔除隐藏字段（响应形态）
//
// 调用架构
//
//	[上游] internal/api（集合文档查询 / 导入导出）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/collection/pick.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] model.go, fieldtypes.go, repository.go, events.go
//
// 主要符号
//
//	ParseFields(raw string, schema *Collection) ([]string, error)
//	PickDocument(doc map[string]any, fields []string) map[string]any
//	NestedValue(doc map[string]any, parts []string) (any, bool)
//	HideDocumentFields(doc map[string]any, hidden []string) map[string]any
//	FieldNamePattern                                         // 字段名形态（字母开头，最长 63）
//	InvalidQuery(message string) error                       // 查询参数不合法（code=invalid_request）
//
// 归属（internal-ddd-hexagonal-implementation-plan §5.1）：字段投影是集合的领域规则，不依赖 HTTP 与方言，
// 自 internal/api 迁入；错误仍是 *ValidationError（入口按既有映射写出）。

package collection

import (
	"regexp"
	"strings"
)

var selectedFieldPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}(?:\.[A-Za-z][A-Za-z0-9_]{0,62})*$`)

func ParseFields(raw string, schema *Collection) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	registered := map[string]Field{}
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
			return nil, &ValidationError{Code: "invalid_fields", Field: name, Message: "invalid field selection"}
		}
		root := strings.SplitN(name, ".", 2)[0]
		if schema != nil {
			field, ok := registered[root]
			if (!ok || strings.Contains(name, ".") && field.Type != "json") && root != "created_at" && root != "updated_at" && root != "id" {
				return nil, &ValidationError{Code: "unknown_field", Field: root, Message: "field is not registered or not selectable"}
			}
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			fields = append(fields, name)
		}
	}
	return fields, nil
}

func PickDocument(doc map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return doc
	}
	out := map[string]any{}
	for _, name := range fields {
		if name == "id" || name == "created_at" || name == "updated_at" {
			continue
		}
		parts := strings.Split(name, ".")
		value, ok := NestedValue(doc, parts)
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

func NestedValue(doc map[string]any, parts []string) (any, bool) {
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

func HideDocumentFields(doc map[string]any, hidden []string) map[string]any {
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

// FieldNamePattern 是字段名的形态约束：字母开头，仅字母 / 数字 / 下划线，最长 63 个字符。
// 查询、排序、聚合与投影共用这一条规则（SQL 里字段名只会以参数或已校验的标识符出现）。
var FieldNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

// InvalidQuery 构造「查询参数不合法」的校验错误（code=invalid_request）；入口按既有映射写出。
func InvalidQuery(message string) error {
	return &ValidationError{Code: "invalid_request", Message: message}
}
