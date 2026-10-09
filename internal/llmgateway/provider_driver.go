package llmgateway

import "strings"

// litellmDriver 把设置页的厂商标识映射到 litellm 已注册驱动。
// 未映射的名字返回 false，调用方保持原名（未知供应商仍创建失败）。
func litellmDriver(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "openai", "anthropic", "gemini", "deepseek", "openrouter", "qwen", "glm", "bedrock":
		return strings.ToLower(strings.TrimSpace(name)), true
	case "google":
		return "gemini", true
	case "dashscope":
		return "qwen", true
	case "zhipu":
		return "glm", true
	case "azure_openai", "custom_openai", "moonshot":
		// 这些预设走 OpenAI Chat Completions，靠 base_url 指向兼容端点。
		return "openai", true
	default:
		return "", false
	}
}
