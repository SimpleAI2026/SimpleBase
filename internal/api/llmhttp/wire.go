// 文件说明：wire.go — OpenAI 兼容响应的线上形态别名（导出仅供路由级测试解码断言）
//
// 用法
//
//	var out llmhttp.ChatResponse     // 解码 chat/completions 响应
//	var ch llmhttp.ChatChunk        // 解码流式增量帧
//	var env llmhttp.ErrorEnvelope   // 解码错误信封
//	var list llmhttp.ModelList       // 解码 models 响应
//
// 调用架构
//
//	[上游] internal/api 的路由级测试
//	         │ 使用本包导出类型
//	         ▼
//	[本文件] internal/api/llmhttp/wire.go
//	         │ 依赖
//	         ▼
//	[下游] 仅本包
//
// 主要符号
//
//	type ChatResponse = oaiChatResponse
//	type ChatChunk = oaiChunk
//	type ErrorEnvelope = oaiErrorBody
//	type ModelList = oaiModelList

package llmhttp

type (
	// ChatResponse 是 chat/completions 的非流式响应形态。
	ChatResponse = oaiChatResponse
	// ChatChunk 是 chat/completions 的流式增量帧形态。
	ChatChunk = oaiChunk
	// ErrorEnvelope 是 OpenAI 错误信封 {"error":{"message","type","param","code"}}。
	ErrorEnvelope = oaiErrorBody
	// ModelList 是 models 列表的响应形态。
	ModelList = oaiModelList
)
