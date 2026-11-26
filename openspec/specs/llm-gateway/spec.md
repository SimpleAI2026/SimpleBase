# LLM Gateway Specification

## Purpose

定义多供应商 LLM 网关与 OpenAI 兼容出口的协议、凭据与追踪契约，保证外部 SDK 可用且密钥不泄露。

## Requirements

### Requirement: 供应商配置与凭据

LLM 供应商 SHALL 由配置声明（openai、anthropic、azure_openai、google、deepseek、moonshot、glm、qwen、openrouter、custom_openai 等），密钥 MUST NOT 从环境变量自动发现，也不得在模型列表接口返回。LLM 未启用时模型列表 SHALL 返回 200 与空列表，而不是错误。

#### Scenario: 查询模型列表
- **WHEN** 客户端调用项目模型列表接口
- **THEN** 只返回模型名称，不返回 `base_url`、`api_key` 或 `credential_ref`

#### Scenario: 未配置任何供应商
- **WHEN** 项目未启用 LLM
- **THEN** 模型列表返回 200 与空数组，聊天接口返回明确的未配置错误

### Requirement: OpenAI Chat Completions 兼容

兼容出口 MUST 使用两段 `v1` 路径：`POST /v1/projects/:projectID/v1/chat/completions`、`POST .../v1/responses`、`GET .../v1/models`。该路由组内错误 MUST 使用 `{"error":{"message","type","param","code"}}` 信封，这是 `WriteError` 的唯一例外；其余 handler MUST NOT 使用该信封。Chat 流 MUST 以 `data: [DONE]` 结束，首帧 `delta.role="assistant"`，工具调用增量按 `index` 聚合。

#### Scenario: OpenAI SDK 调用
- **WHEN** 客户端以 `base_url=".../projects/<id>/v1"` 调用 `/chat/completions`
- **THEN** 请求按 OpenAI 形状被接受并返回兼容响应或流

#### Scenario: 兼容组内错误
- **WHEN** 请求参数非法或模型不可用
- **THEN** 返回 OpenAI 错误信封，其中 `code` 仍来自统一错误映射

### Requirement: Responses API 是有状态子集

`/v1/responses` MUST 作为 chat 之上的协议转换实现，不得直接调用供应商专有的 Responses 接口。仅支持无状态子集；`previous_response_id`、`conversation`、`background`、结构化输出与内置工具 MUST 返回 400，不得静默忽略。其流式响应没有 `[DONE]`，MUST 以 `response.completed`、`response.incomplete` 或 `response.failed` 收尾。

#### Scenario: 使用不支持的有状态参数
- **WHEN** 请求携带 `previous_response_id` 或 `background`
- **THEN** 返回 400，不降级为无状态执行

#### Scenario: 流式执行失败
- **WHEN** 流开始后上游报错
- **THEN** 发送 `response.failed` 事件并结束流

### Requirement: LLM 追踪与脱敏

LLM 调用默认只记录元数据（标识、长度、SHA-256、耗时、token 数）。仅当 `agent.trace_log` 打开时，提示词与正文才可经脱敏与截断写入 zap 日志；MUST NOT 写入系统库 logs 表，MUST NOT 记录密钥、SQL 参数或完整正文。

#### Scenario: 默认追踪
- **WHEN** `agent.trace_log` 未开启
- **THEN** 日志与审计只有元数据，无法还原提示词或回答正文

#### Scenario: 开启追踪后的敏感内容
- **WHEN** 提示词包含密钥形态的字符串
- **THEN** 写入日志前被脱敏并截断，原文不出现在系统库或日志正文
