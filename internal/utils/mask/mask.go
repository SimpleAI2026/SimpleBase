// 文件说明：mask.go — 敏感值掩码与敏感键名判定
//
// 用法
//
//	IsSensitiveKey(name string) bool  // IsSensitiveKey 判断字段名是否是敏感字段（password / token / api_key 等）
//	Secret(v string) string  // Secret 掩码展示一个密钥：保留前 3 后 4 字符，长度不足 8 时全部遮蔽
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/sbcli, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/utils/mask/mask.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	IsSensitiveKey(name string) bool
//	Secret(v string) string
//	... 另有 1 个顶层符号

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
