// Package mask 提供敏感值掩码与敏感键名判定。
//
// 只允许 import 标准库；不得 import 根模块任何业务包（见 internal/utils/README.md）。
package mask

import "regexp"

// sensitiveKey 匹配敏感字段名（大小写无关）。原先在 internal/sbcli 与
// internal/agentbridge 各有一份完全相同的正则字面量。
var sensitiveKey = regexp.MustCompile(`(?i)(^|_)(password|passwd|secret|api_key|access_key|token|credential|dsn|authorization)($|_)`)

// IsSensitiveKey 判断字段名是否是敏感字段（password / token / api_key 等）。
func IsSensitiveKey(name string) bool {
	return sensitiveKey.MatchString(name)
}

// Secret 掩码展示一个密钥：保留前 3 后 4 字符，长度不足 8 时全部遮蔽。
//
// 语义与 internal/systemdb.MaskSecretValue 一致，用于 API 响应里的 api_key_masked。
func Secret(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 8 {
		return "••••••••"
	}
	return v[:3] + "..." + v[len(v)-4:]
}

// KeepEnds 保留首尾各 keep 个字符，中间用小星星等量替换（按字节计）。
//
// 语义与 internal/audit.Redact 一致：长度不超过 2*keep 时全部遮蔽。
func KeepEnds(s string, keep int) string {
	if keep < 0 {
		keep = 0
	}
	if len(s) <= 2*keep {
		return repeated(len(s))
	}
	return s[:keep] + repeated(len(s)-2*keep) + s[len(s)-keep:]
}

func repeated(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = '*'
	}
	return string(out)
}
