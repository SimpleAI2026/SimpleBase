---
title: OpenAI 兼容接口
order: 4
description: 用官方 OpenAI SDK 或任意兼容客户端，直接调用 SimpleBase 项目的默认模型。
---

# OpenAI 兼容接口

SimpleBase 为每个项目提供一个 OpenAI Chat Completions 兼容出口。开发者不必自己申请供应商 Key，把 `base_url` 指向项目、`api_key` 填项目的 API Key，就能用官方 SDK 调用项目的默认模型。

> **注意** 该出口取代了旧的 `/llm/chat`、`/llm/stream`、`/llm/providers`，旧端点已删除，不保留兼容期。设计背景见[LLM 出口统一计划（v6.0）](https://github.com/linkxzhou/SimpleBase/blob/main/plan/planv6.0/openai-compatible-llm-api-plan.md)。

## 连接信息

| 项 | 值 |
|---|---|
| API 地址（`base_url`） | `{BASE}/v1/projects/{PROJECT}/v1` |
| API Key | 项目 API Key，在控制台「设置 → 连接」签发与重置 |
| 默认模型 | 项目「设置 → 模型」中的默认模型，也可由 `GET /models` 查询 |
| Token 额度 | 项目配额中 LLM token 上限为 0（默认）时不限；设置了上限则按周期累计，超出后返回 429 |

地址里有两段 `/v1`：第一段是 SimpleBase 自身的 API 版本，第二段是 OpenAI SDK 约定的资源前缀。保留尾部的 `/v1`，SDK 才能原样拼出 `/chat/completions`。

调用方需要 `llm:invoke` 权限，内置的三类角色都已具备。控制台「设置 → 模型」顶部的绿色卡片会直接展示当前项目的地址和 Key，并提供复制按钮。

## 快速调用

设置环境变量，沿用[5 分钟上手](/docs/getting-started/quickstart)中的 `BASE`、`PROJECT` 与 `SB_KEY`：

```bash
curl -sS "$BASE/v1/projects/$PROJECT/v1/chat/completions" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"model":"<默认模型>","messages":[{"role":"user","content":"你好"}]}'
```

响应是标准的 `chat.completion` 对象，文本在 `choices[0].message.content`。

## 使用 OpenAI SDK

Python：

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8080/v1/projects/<PROJECT>/v1",
    api_key="<SB_KEY>",
)
resp = client.chat.completions.create(
    model="<默认模型>",
    messages=[{"role": "user", "content": "你好"}],
)
print(resp.choices[0].message.content)
```

Node.js：

```javascript
import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: 'http://127.0.0.1:8080/v1/projects/<PROJECT>/v1',
  apiKey: '<SB_KEY>',
})
const resp = await client.chat.completions.create({
  model: '<默认模型>',
  messages: [{ role: 'user', content: '你好' }],
})
console.log(resp.choices[0].message.content)
```

## 查询可用模型

```bash
curl -sS "$BASE/v1/projects/$PROJECT/v1/models" \
  -H "Authorization: Bearer $SB_KEY"
```

返回 OpenAI 的模型列表形状：`{"object":"list","data":[{"id":"…","object":"model"}]}`。

## 流式输出

请求里加 `"stream": true`，响应是 SSE，每帧是 `chat.completion.chunk`，以 `data: [DONE]` 结束：

```bash
curl -N "$BASE/v1/projects/$PROJECT/v1/chat/completions" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"model":"<默认模型>","stream":true,"messages":[{"role":"user","content":"写一首四行短诗"}]}'
```

需要用量时再加 `"stream_options":{"include_usage":true}`，最后一帧会带 `usage`。

## 工具调用

请求体支持标准的 `tools`（仅 `type: "function"`），响应的 `choices[0].message.tool_calls` 是 `arguments` 为 JSON 字符串的标准形状。执行工具后，用 `role: "tool"` 与 `tool_call_id` 把结果回灌即可。

`tool_choice` 支持 `"auto"`、`"none"`、`"required"`，以及指定函数的对象 `{"type":"function","function":{"name":"…"}}`。未声明 `tools` 时指定 `auto` 或 `required` 会返回 400。

## Responses API

同一个地址下还提供 OpenAI 新版的 Responses API，官方 SDK 的 `client.responses.create` 可直接使用：

```bash
curl -sS "$BASE/v1/projects/$PROJECT/v1/responses" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"model":"<默认模型>","instructions":"回答要简短","input":"你好"}'
```

```python
resp = client.responses.create(model="<默认模型>", input="你好")
print(resp.output_text)
```

**支持的范围**

| 能力 | 说明 |
|---|---|
| `input` | 字符串，或由 `message`、`function_call`、`function_call_output` 条目组成的数组；`reasoning` 条目会被忽略 |
| `instructions`、`max_output_tokens`、`temperature` | 与官方一致 |
| `tools`、`tool_choice` | 仅 `type: "function"`，使用 Responses 的平铺形状 `{"type":"function","name":"…","parameters":{…}}` |
| `stream` | `true` 时返回事件流，以 `response.completed`（长度截断时为 `response.incomplete`）收尾，没有 `[DONE]` |

流式事件包括 `response.created`、`response.output_item.added`、`response.output_text.delta`、`response.function_call_arguments.delta` 等，每个事件带递增的 `sequence_number`。流开始后上游出错时，会发出标准的 `response.failed` 事件。

**不支持，返回 400**

- `previous_response_id`、`conversation`：本端点无状态，请在 `input` 里重发完整对话。
- `background`，以及 `text.format` 里的结构化输出（`json_schema`、`json_object`）。
- 内置工具（`web_search`、`file_search`、`code_interpreter`、`mcp` 等）和非文本内容（图片、文件）。

响应不会被保存，`store` 参数会被接受但忽略，也没有 `GET /responses/{id}`。

它在服务端是把请求转换为对话请求再转换回来，所以任何已配置的供应商都能使用，不要求上游本身支持 Responses API。

## 错误形状

该出口的错误使用 OpenAI 的格式，官方 SDK 能直接解析：

```json
{"error": {"message": "…", "type": "invalid_request_error", "param": null, "code": "model_not_found"}}
```

| HTTP | `error.type` | 常见原因 |
|---|---|---|
| 400 | `invalid_request_error` | `messages` 为空、模型未配置或不在允许列表 |
| 401 | `authentication_error` | Key 缺失、无效或已吊销 |
| 403 | `permission_error` | 无 `llm:invoke` 权限，或访问了其它项目 |
| 429 | `rate_limit_error` | 触发项目配额限制 |
| 5xx | `api_error` | 上游供应商错误或超时 |

错误信息不含密钥、SQL 参数或对话正文。项目与 Key 的权限边界见[核心概念](/docs/getting-started/concepts)。

## 常见问题

| 现象 | 可能原因 | 处理 |
|---|---|---|
| SDK 报 404 | `base_url` 少写了尾部 `/v1` | 地址应以 `/v1/projects/<PROJECT>/v1` 结尾 |
| 返回 401 | Key 无效或已重置 | 在「设置 → 连接」确认当前 Key |
| 提示模型未配置 | 项目没有设置默认模型 | 在「设置 → 模型」选择默认模型，或请求里显式传 `model` |
