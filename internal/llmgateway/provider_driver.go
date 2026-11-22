// 文件说明：provider_driver.go — internal/llmgateway 的内部实现（litellmDriver 等）
//
// 用法
//
//	同包调用：litellmDriver() → service.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/llmgateway/provider_driver.go
//	         │
//	         ▼
//	[下游] internal/utils/strutil
//	[同包] catalog_resolver.go, fallback_resolver.go, project_cred_resolver.go, service.go, streaming.go
//	[关键引用] litellmDriver() ← service.go
//
// 主要符号
//
//	litellmDriver(name string) (string, bool)

package llmgateway

import "github.com/linkxzhou/SimpleBase/internal/utils/strutil"

// litellmDriver 把设置页的厂商标识映射到 litellm 已注册驱动。
// 未映射的名字返回 false，调用方保持原名（未知供应商仍创建失败）。
func litellmDriver(name string) (string, bool) {
	switch strutil.Normalize(name) {
	case "openai", "anthropic", "gemini", "deepseek", "openrouter", "qwen", "glm", "bedrock":
		return strutil.Normalize(name), true
	case "google":
		return "gemini", true
	case "dashscope":
		return "qwen", true
	case "zhipu":
		return "glm", true
	case "azure_openai", "custom_openai", "moonshot", "codex":
		// 这些预设走 OpenAI Chat Completions，靠 base_url 指向兼容端点。
		// codex 是 Codex CLI 的订阅端点，协议与 OpenAI 一致。
		return "openai", true
	case "claude":
		// claude 是 Claude Code 的订阅端点，消息协议与 anthropic 一致。
		return "anthropic", true
	default:
		return "", false
	}
}
