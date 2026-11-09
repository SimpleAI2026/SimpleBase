// 文件说明：redact.go — internal/sbcli 的内部实现（sensitiveName、redact 等）
//
// 用法
//
//	同包调用：redact() → collection_transfer.go, commands.go 等
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sbcli/redact.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/mask
//	[同包] collection_transfer.go, commands.go, config.go, migrations.go, output.go 等 6 个
//	[关键引用] redact() ← collection_transfer.go, commands.go, config.go
//
// 主要符号
//
//	sensitiveName(name string) bool
//	redact(v any) any
//	redactSQLRows(cols, rows []any) []any

package sbcli

import "github.com/linkxzhou/SimpleBase/internal/utils/mask"

func sensitiveName(name string) bool {
	return mask.IsSensitiveKey(name)
}

// redact walks JSON-like values. A top-level API key secret is kept for human
// output and replaced for agent output by the caller before redact.
func redact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		cols, _ := t["columns"].([]any)
		rows, _ := t["rows"].([]any)
		if cols != nil && rows != nil {
			out["columns"] = cols
			out["rows"] = redactSQLRows(cols, rows)
			for k, val := range t {
				if k == "columns" || k == "rows" {
					continue
				}
				out[k] = redact(val)
			}
			return out
		}
		_, apiKeyShape := t["permissions"]
		for k, val := range t {
			if sensitiveName(k) {
				// API key create returns the one-time secret next to permissions.
				// Agent mode removes it before redact; humans still need to see it.
				if k == "secret" && apiKeyShape {
					out[k] = val
					continue
				}
				out[k] = "***"
				continue
			}
			out[k] = redact(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redact(val)
		}
		return out
	default:
		return v
	}
}

func redactSQLRows(cols, rows []any) []any {
	out := make([]any, len(rows))
	for i, row := range rows {
		cells, ok := row.([]any)
		if !ok {
			out[i] = redact(row)
			continue
		}
		next := make([]any, len(cells))
		for j, cell := range cells {
			name := ""
			if j < len(cols) {
				name, _ = cols[j].(string)
			}
			if sensitiveName(name) {
				next[j] = "***"
			} else {
				next[j] = cell
			}
		}
		out[i] = next
	}
	return out
}
