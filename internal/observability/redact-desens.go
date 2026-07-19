package observability

import (
	"strings"
)

// redactedReplacements 是必须从日志/错误中抹除的敏感字段名。
// 判断时统一小写比较。
var redactedFields = map[string]struct{}{
	"api_key":          {},
	"apikey":           {},
	"authorization":    {},
	"password":         {},
	"secret":           {},
	"secret_key":       {},
	"secretkey":        {},
	"access_key":       {},
	"accesskey":        {},
	"access_key_id":    {},
	"token":            {},
	"bearer":           {},
	"dsn":              {},
	"credential":       {},
	"credential_ref":   {},
	"private_key":      {},
	"session_token":    {},
	"prompt":           {},
	"messages":         {},
	"sql_args":         {},
}

// RedactString 对疑似敏感字符串值进行脱敏。仅保留长度信息。
func RedactString(s string) string {
	if s == "" {
		return ""
	}
	return strings.Repeat("*", min(len(s), 8))
}

// IsSensitiveField 判断字段名是否敏感。
func IsSensitiveField(name string) bool {
	_, ok := redactedFields[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// RedactMap 返回 map 的脱敏副本，敏感字段值替换为 "***"。
// 用于审计 metadata、日志 fields、错误 detail。
func RedactMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if IsSensitiveField(k) {
			out[k] = "***"
			continue
		}
		switch vv := v.(type) {
		case map[string]any:
			out[k] = RedactMap(vv)
		case string:
			// 长字符串疑似 token/key 时只保留摘要
			if len(vv) > 64 {
				out[k] = vv[:6] + "...(redacted)"
			} else {
				out[k] = vv
			}
		default:
			out[k] = v
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
