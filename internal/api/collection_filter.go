// 文件说明：collection_filter.go — internal/api 的内部实现（filterToken、filterParser 等）
//
// 用法
//
//	type filterToken（包内使用）
//	type filterParser（包内使用）
//	同包调用：value() → api_key_policy.go, collection_meta_handler.go 等；field() → api_key_policy.go, collection_meta_handler.go 等；compileFilter() → collection_query.go, collection_transfer.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/collection_filter.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, internal/collection
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] value() ← api_key_policy.go, collection_meta_handler.go, collection_pick.go
//
// 主要符号
//
//	type filterToken struct {…}
//	type filterParser struct {…}
//	compileFilter(raw string, schema *collection.Collection, dialect ...
//	invalidFilter(message string) error
//	lexFilter(raw string) ([]filterToken, error)
//	(p *filterParser) parseOr() (string, error)
//	... 另有 8 个顶层符号

package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

type filterToken struct {
	kind string
	text string
}

type filterParser struct {
	tokens  []filterToken
	pos     int
	schema  *collection.Collection
	dialect godatabase.Dialect
	args    []any
}

func compileFilter(raw string, schema *collection.Collection, dialect godatabase.Dialect) (string, []any, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil, nil
	}
	if len(raw) > 2048 || strings.ContainsRune(raw, 0) || strings.Contains(raw, ";") {
		return "", nil, invalidFilter("invalid filter")
	}
	tokens, err := lexFilter(raw)
	if err != nil {
		return "", nil, err
	}
	p := &filterParser{tokens: tokens, schema: schema, dialect: dialect}
	sql, err := p.parseOr()
	if err != nil {
		return "", nil, err
	}
	if p.pos != len(p.tokens) {
		return "", nil, invalidFilter("unexpected token")
	}
	return sql, p.args, nil
}

func invalidFilter(message string) error {
	return &collection.ValidationError{Code: "invalid_filter", Message: message}
}

func lexFilter(raw string) ([]filterToken, error) {
	var out []filterToken
	for i := 0; i < len(raw); {
		r := rune(raw[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if strings.ContainsRune("(),[]", r) {
			out = append(out, filterToken{kind: string(r), text: string(r)})
			i++
			continue
		}
		if raw[i] == '"' {
			j := i + 1
			escaped := false
			for ; j < len(raw); j++ {
				if raw[j] == '"' && !escaped {
					break
				}
				if raw[j] == '\\' && !escaped {
					escaped = true
				} else {
					escaped = false
				}
			}
			if j >= len(raw) {
				return nil, invalidFilter("unterminated string")
			}
			out = append(out, filterToken{kind: "value", text: raw[i : j+1]})
			i = j + 1
			continue
		}
		if strings.ContainsRune("=!<>~", r) {
			j := i + 1
			if j < len(raw) && strings.ContainsRune("=~", rune(raw[j])) {
				j++
			}
			op := raw[i:j]
			switch op {
			case "=", "!=", ">", ">=", "<", "<=", "~", "!~":
			default:
				return nil, invalidFilter("invalid operator")
			}
			out = append(out, filterToken{kind: "op", text: op})
			i = j
			continue
		}
		j := i
		for j < len(raw) && !unicode.IsSpace(rune(raw[j])) && !strings.ContainsRune("(),[]=!<>~", rune(raw[j])) {
			j++
		}
		word := raw[i:j]
		if word == "" {
			return nil, invalidFilter("invalid token")
		}
		switch word {
		case "&&", "||", "in", "is", "not", "null", "true", "false":
			out = append(out, filterToken{kind: word, text: word})
		default:
			if _, err := strconv.ParseFloat(word, 64); err == nil {
				out = append(out, filterToken{kind: "value", text: word})
			} else if fieldNamePattern.MatchString(word) {
				out = append(out, filterToken{kind: "field", text: word})
			} else {
				return nil, invalidFilter("invalid token")
			}
		}
		i = j
	}
	return out, nil
}

func (p *filterParser) parseOr() (string, error) {
	left, err := p.parseAnd()
	if err != nil {
		return "", err
	}
	for p.take("||") {
		right, err := p.parseAnd()
		if err != nil {
			return "", err
		}
		left = "(" + left + " OR " + right + ")"
	}
	return left, nil
}

func (p *filterParser) parseAnd() (string, error) {
	left, err := p.parseTerm()
	if err != nil {
		return "", err
	}
	for p.take("&&") {
		right, err := p.parseTerm()
		if err != nil {
			return "", err
		}
		left = "(" + left + " AND " + right + ")"
	}
	return left, nil
}

func (p *filterParser) parseTerm() (string, error) {
	if p.take("(") {
		expr, err := p.parseOr()
		if err != nil {
			return "", err
		}
		if !p.take(")") {
			return "", invalidFilter("missing closing parenthesis")
		}
		return "(" + expr + ")", nil
	}
	field, err := p.expect("field")
	if err != nil {
		return "", err
	}
	f, expr, err := p.field(field.text)
	if err != nil {
		return "", err
	}
	if p.take("is") {
		not := p.take("not")
		if !p.take("null") {
			return "", invalidFilter("expected null")
		}
		if not {
			return expr + " IS NOT NULL", nil
		}
		return expr + " IS NULL", nil
	}
	op, err := p.expect("op")
	if err != nil && p.take("in") {
		op = filterToken{kind: "op", text: "in"}
		err = nil
	}
	if err != nil {
		return "", err
	}
	if op.text == "in" {
		values, err := p.array()
		if err != nil || len(values) == 0 || len(values) > 32 {
			return "", invalidFilter("in requires 1..32 values")
		}
		args := make([]any, len(values))
		placeholders := make([]string, len(values))
		for i, value := range values {
			arg, err := filterArg(f, value)
			if err != nil {
				return "", err
			}
			args[i], placeholders[i] = arg, "?"
		}
		p.args = append(p.args, args...)
		return expr + " IN (" + strings.Join(placeholders, ",") + ")", nil
	}
	value, err := p.value()
	if err != nil {
		return "", err
	}
	if value == nil {
		if op.text == "=" {
			return expr + " IS NULL", nil
		}
		if op.text == "!=" {
			return expr + " IS NOT NULL", nil
		}
	}
	if (op.text == "~" || op.text == "!~") && (f.Type == "number" || f.Type == "bool" || f.Type == "date" || f.Type == "json") {
		return "", invalidFilter("contains requires text field")
	}
	if f.Type == "json" {
		return "", invalidFilter("json fields only support null checks")
	}
	arg, err := filterArg(f, value)
	if err != nil {
		return "", err
	}
	p.args = append(p.args, arg)
	if op.text == "~" || op.text == "!~" {
		s, ok := arg.(string)
		if !ok {
			return "", invalidFilter("contains requires string")
		}
		s = strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(s)
		p.args[len(p.args)-1] = "%" + s + "%"
		negate := ""
		if op.text == "!~" {
			negate = "NOT "
		}
		return expr + " " + negate + "LIKE ? ESCAPE '\\'", nil
	}
	return expr + " " + op.text + " ?", nil
}

func (p *filterParser) field(name string) (collection.Field, string, error) {
	if p.schema == nil {
		return collection.Field{}, "", &collection.ValidationError{Code: "schema_not_registered", Field: name, Message: "schema required for filter"}
	}
	for _, f := range p.schema.Fields {
		if f.Name == name {
			path := "$." + name
			switch f.Type {
			case "number":
				return f, p.dialect.JSONExtractNumber(path), nil
			case "bool":
				return f, p.dialect.JSONExtractBool(path), nil
			case "date":
				return f, p.dialect.CastTimestamp(p.dialect.JSONExtractText(path)), nil
			case "json":
				return f, p.dialect.JSONExtractText(path), nil
			default:
				return f, p.dialect.JSONExtractText(path), nil
			}
		}
	}
	return collection.Field{}, "", &collection.ValidationError{Code: "unknown_field", Field: name, Message: "field is not registered"}
}

func filterArg(f collection.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch f.Type {
	case "number":
		if _, ok := value.(float64); !ok {
			return nil, invalidFilter("number value required")
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return nil, invalidFilter("boolean value required")
		}
	case "date":
		s, ok := value.(string)
		if !ok {
			return nil, invalidFilter("date string required")
		}
		return s, nil
	default:
		if _, ok := value.(string); !ok {
			return nil, invalidFilter("string value required")
		}
	}
	return value, nil
}

func (p *filterParser) value() (any, error) {
	if p.take("null") {
		return nil, nil
	}
	if p.take("true") {
		return true, nil
	}
	if p.take("false") {
		return false, nil
	}
	token, err := p.expect("value")
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal([]byte(token.text), &value); err != nil {
		return nil, invalidFilter("invalid value")
	}
	return value, nil
}

func (p *filterParser) array() ([]any, error) {
	if !p.take("[") {
		return nil, invalidFilter("expected array")
	}
	var values []any
	if p.take("]") {
		return values, nil
	}
	for {
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		if p.take("]") {
			return values, nil
		}
		if !p.take(",") {
			return nil, invalidFilter("expected comma")
		}
	}
}

func (p *filterParser) take(kind string) bool {
	if p.pos < len(p.tokens) && p.tokens[p.pos].kind == kind {
		p.pos++
		return true
	}
	return false
}

func (p *filterParser) expect(kind string) (filterToken, error) {
	if p.pos >= len(p.tokens) || p.tokens[p.pos].kind != kind {
		return filterToken{}, invalidFilter(fmt.Sprintf("expected %s", kind))
	}
	token := p.tokens[p.pos]
	p.pos++
	return token, nil
}
