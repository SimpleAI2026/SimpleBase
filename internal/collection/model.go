// 文件说明：model.go — internal/collection 中 ErrNotFound、ErrUnavailable 等符号的实现
//
// 用法
//
//	(e *ValidationError) Error() string
//	MergePatch(target, patch map[string]any) map[string]any
//	同包调用：Collection → fieldtypes.go, repository.go；Field → fieldtypes.go, repository.go；FieldOptions → fieldtypes.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/collection/model.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] fieldtypes.go, repository.go
//	[关键引用] Collection ← fieldtypes.go, repository.go
//
// 主要符号
//
//	var ErrNotFound
//	var ErrUnavailable
//	type Collection struct {…}
//	type Field struct {…}
//	type FieldOptions struct {…}
//	type ValidationError struct {…}
//	... 另有 3 个顶层符号

package collection

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound    = errors.New("schema_not_registered")
	ErrUnavailable = errors.New("collection_schema_unavailable")
)

type Collection struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Name       string    `json:"name"`
	SchemaMode string    `json:"schema_mode"`
	ViewQuery  string    `json:"view_query,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Fields     []Field   `json:"fields"`
}

type Field struct {
	Name       string       `json:"name"`
	Type       string       `json:"type"`
	Required   bool         `json:"required"`
	Unique     bool         `json:"unique"`
	Default    any          `json:"default,omitempty"`
	Options    FieldOptions `json:"options"`
	Hidden     bool         `json:"hidden"`
	Readonly   bool         `json:"readonly"`
	OrderIndex int          `json:"order_index,omitempty"`
}

type FieldOptions struct {
	Min           any      `json:"min,omitempty"`
	Max           any      `json:"max,omitempty"`
	Pattern       string   `json:"pattern,omitempty"`
	Values        []string `json:"values,omitempty"`
	MaxSelect     int      `json:"max_select,omitempty"`
	MaxSize       int      `json:"max_size,omitempty"`
	OnlyInt       bool     `json:"only_int,omitempty"`
	ExceptDomains []string `json:"except_domains,omitempty"`
	OnlyDomains   []string `json:"only_domains,omitempty"`
}

type ValidationError struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Index   *int   `json:"failed_index,omitempty"`
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Field)
}

func MergePatch(target, patch map[string]any) map[string]any {
	out := make(map[string]any, len(target)+len(patch))
	for k, v := range target {
		out[k] = v
	}
	for k, v := range patch {
		if v == nil {
			delete(out, k)
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			if base, ok := out[k].(map[string]any); ok {
				out[k] = MergePatch(base, nested)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func ChangedFields(before, after map[string]any) []string {
	changed := make([]string, 0)
	for k, v := range after {
		old, ok := before[k]
		a, _ := json.Marshal(old)
		b, _ := json.Marshal(v)
		if !ok || string(a) != string(b) {
			changed = append(changed, k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			changed = append(changed, k)
		}
	}
	return changed
}
