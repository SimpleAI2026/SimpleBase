package sbcli

import "regexp"

var sensitiveKey = regexp.MustCompile(`(?i)(^|_)(password|passwd|secret|api_key|access_key|token|credential|dsn|authorization)($|_)`)

func sensitiveName(name string) bool {
	return sensitiveKey.MatchString(name)
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
