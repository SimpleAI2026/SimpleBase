// 文件说明：fieldtypes.go — internal/collection 中 ValidateSchema、ValidateDocument 等符号的实现
//
// 用法
//
//	ValidateSchema(schema Collection) error
//	ValidateDocument(schema *Collection, input map[string]any, applyDefaults bool) ...
//	同包调用：ValidateSchema() → repository.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/collection/fieldtypes.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] model.go, repository.go
//	[关键引用] ValidateSchema() ← repository.go
//
// 主要符号
//
//	ValidateSchema(schema Collection) error
//	ValidateDocument(schema *Collection, input map[string]any, applyDefaults bool) ...
//	ValidateStoredDocument(schema *Collection, input map[string]any, applyDefaults ...
//	ValidateField(f Field, value any) *ValidationError
//	... 另有 5 个顶层符号

package collection

import (
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var fieldNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

func ValidateSchema(schema Collection) error {
	if !fieldNamePattern.MatchString(schema.Name) {
		return &ValidationError{Code: "invalid_request", Message: "invalid collection name"}
	}
	if schema.SchemaMode == "" {
		schema.SchemaMode = "json"
	}
	if schema.SchemaMode != "json" && schema.SchemaMode != "view" {
		return &ValidationError{Code: "invalid_request", Message: "schema_mode must be json or view"}
	}
	if schema.SchemaMode == "view" && strings.TrimSpace(schema.ViewQuery) == "" || schema.SchemaMode == "json" && strings.TrimSpace(schema.ViewQuery) != "" {
		return &ValidationError{Code: "invalid_view_query", Message: "view_query is required only for view collections"}
	}
	seen := map[string]struct{}{}
	for i := range schema.Fields {
		f := &schema.Fields[i]
		if !fieldNamePattern.MatchString(f.Name) || f.Name == "id" || f.Name == "created_at" || f.Name == "updated_at" {
			return &ValidationError{Code: "invalid_field", Field: f.Name, Message: "invalid or reserved field name"}
		}
		if _, ok := seen[f.Name]; ok {
			return &ValidationError{Code: "invalid_field", Field: f.Name, Message: "duplicate field"}
		}
		seen[f.Name] = struct{}{}
		switch f.Type {
		case "text", "number", "bool", "date", "email", "url", "select", "json":
		default:
			return &ValidationError{Code: "invalid_field_type", Field: f.Name, Message: "unsupported field type"}
		}
		if f.Options.Pattern != "" {
			if _, err := regexp.Compile(f.Options.Pattern); err != nil {
				return &ValidationError{Code: "invalid_field_options", Field: f.Name, Message: "invalid pattern"}
			}
		}
		if f.Type == "select" && len(f.Options.Values) == 0 {
			return &ValidationError{Code: "invalid_field_options", Field: f.Name, Message: "select values required"}
		}
		if f.Type == "date" {
			for _, value := range []any{f.Options.Min, f.Options.Max} {
				if value == nil {
					continue
				}
				text, ok := value.(string)
				if !ok {
					return &ValidationError{Code: "invalid_field_options", Field: f.Name, Message: "date bounds must be RFC3339"}
				}
				if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
					return &ValidationError{Code: "invalid_field_options", Field: f.Name, Message: "date bounds must be RFC3339"}
				}
			}
		} else if f.Type != "select" && f.Type != "bool" && f.Type != "json" {
			min, minOK := numericOption(f.Options.Min)
			max, maxOK := numericOption(f.Options.Max)
			if f.Options.Min != nil && !minOK || f.Options.Max != nil && !maxOK || minOK && maxOK && min > max {
				return &ValidationError{Code: "invalid_field_options", Field: f.Name, Message: "invalid numeric bounds"}
			}
		}
		if f.Default != nil {
			if err := ValidateField(*f, f.Default); err != nil {
				return &ValidationError{Code: "invalid_field_options", Field: f.Name, Message: "default does not match field"}
			}
		}
	}
	return nil
}

func ValidateDocument(schema *Collection, input map[string]any, applyDefaults bool) (map[string]any, *ValidationError) {
	return validateDocument(schema, input, applyDefaults, false)
}

func ValidateStoredDocument(schema *Collection, input map[string]any, applyDefaults bool) (map[string]any, *ValidationError) {
	return validateDocument(schema, input, applyDefaults, true)
}

func validateDocument(schema *Collection, input map[string]any, applyDefaults, allowUnknown bool) (map[string]any, *ValidationError) {
	if schema == nil {
		return input, nil
	}
	out := make(map[string]any, len(input)+len(schema.Fields))
	for k, v := range input {
		out[k] = v
	}
	fields := map[string]Field{}
	for _, f := range schema.Fields {
		fields[f.Name] = f
	}
	for name := range out {
		if name == "id" || name == "created_at" || name == "updated_at" {
			delete(out, name)
			continue
		}
		if _, ok := fields[name]; !ok && !allowUnknown {
			return nil, &ValidationError{Code: "unknown_field", Field: name, Message: "field is not registered"}
		}
	}
	for _, f := range schema.Fields {
		value, ok := out[f.Name]
		if (!ok || value == nil) && applyDefaults && f.Default != nil {
			value, ok = f.Default, true
			out[f.Name] = value
		}
		if !ok || value == nil {
			if f.Required {
				return nil, &ValidationError{Code: "required_field", Field: f.Name, Message: "field is required"}
			}
			continue
		}
		if err := ValidateField(f, value); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func ValidateField(f Field, value any) *ValidationError {
	fail := func(code, message string) *ValidationError {
		return &ValidationError{Code: code, Field: f.Name, Message: message}
	}
	switch f.Type {
	case "text", "email", "url":
		s, ok := value.(string)
		if !ok {
			return fail("field_type_mismatch", "expected string")
		}
		min, hasMin := numericOption(f.Options.Min)
		max, hasMax := numericOption(f.Options.Max)
		if hasMin && float64(utf8.RuneCountInString(s)) < min {
			return fail("field_too_short", "string shorter than minimum")
		}
		if !hasMax {
			max = 5000.0
		}
		if float64(utf8.RuneCountInString(s)) > max {
			return fail("field_too_long", "string longer than maximum")
		}
		if f.Options.Pattern != "" {
			matched, err := regexp.MatchString(f.Options.Pattern, s)
			if err != nil || !matched {
				return fail("field_pattern_mismatch", "string does not match pattern")
			}
		}
		if f.Type == "email" {
			address, err := mail.ParseAddress(s)
			if err != nil || address.Address != s || !strings.Contains(s, "@") {
				return fail("field_format_mismatch", "invalid email")
			}
			if !domainAllowed(strings.Split(s, "@")[1], f.Options) {
				return fail("field_domain_denied", "email domain denied")
			}
		}
		if f.Type == "url" {
			u, err := url.ParseRequestURI(s)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
				return fail("field_format_mismatch", "invalid URL")
			}
			if !domainAllowed(u.Hostname(), f.Options) {
				return fail("field_domain_denied", "URL domain denied")
			}
		}
	case "number":
		n, ok := value.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return fail("field_type_mismatch", "expected number")
		}
		if f.Options.OnlyInt && n != math.Trunc(n) {
			return fail("field_type_mismatch", "expected integer")
		}
		min, hasMin := numericOption(f.Options.Min)
		max, hasMax := numericOption(f.Options.Max)
		if hasMin && n < min || hasMax && n > max {
			return fail("field_out_of_range", "number out of range")
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fail("field_type_mismatch", "expected boolean")
		}
	case "date":
		s, ok := value.(string)
		if !ok {
			return fail("field_type_mismatch", "expected RFC3339 string")
		}
		parsed, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return fail("field_format_mismatch", "invalid RFC3339 date")
		}
		if !dateWithin(parsed, f.Options) {
			return fail("field_out_of_range", "date out of range")
		}
	case "select":
		values := []any{value}
		if list, ok := value.([]any); ok {
			values = list
			if f.Options.MaxSelect > 0 && len(list) > f.Options.MaxSelect {
				return fail("field_too_many_values", "too many selected values")
			}
		}
		allowed := map[string]struct{}{}
		for _, item := range f.Options.Values {
			allowed[item] = struct{}{}
		}
		for _, item := range values {
			s, ok := item.(string)
			if !ok {
				return fail("field_type_mismatch", "expected select string")
			}
			if _, ok := allowed[s]; !ok {
				return fail("field_value_denied", "select value denied")
			}
		}
	case "json":
		body, err := json.Marshal(value)
		if err != nil {
			return fail("field_type_mismatch", "invalid JSON")
		}
		max := f.Options.MaxSize
		if max <= 0 {
			max = 65536
		}
		if len(body) > max {
			return fail("field_too_large", fmt.Sprintf("JSON exceeds %d bytes", max))
		}
	}
	return nil
}

func domainAllowed(domain string, options FieldOptions) bool {
	domain = strings.ToLower(domain)
	for _, blocked := range options.ExceptDomains {
		if strings.EqualFold(domain, blocked) {
			return false
		}
	}
	if len(options.OnlyDomains) == 0 {
		return true
	}
	for _, allowed := range options.OnlyDomains {
		if strings.EqualFold(domain, allowed) {
			return true
		}
	}
	return false
}

func numericOption(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, !math.IsNaN(n) && !math.IsInf(n, 0)
	case *float64:
		if n == nil {
			return 0, false
		}
		return *n, !math.IsNaN(*n) && !math.IsInf(*n, 0)
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		v, err := n.Float64()
		return v, err == nil
	default:
		return 0, false
	}
}

func dateWithin(value time.Time, options FieldOptions) bool {
	for _, bound := range []struct {
		raw   any
		after bool
	}{{options.Min, true}, {options.Max, false}} {
		if bound.raw == nil {
			continue
		}
		text, ok := bound.raw.(string)
		limit, err := time.Parse(time.RFC3339Nano, text)
		if !ok || err != nil || bound.after && value.Before(limit) || !bound.after && value.After(limit) {
			return false
		}
	}
	return true
}
