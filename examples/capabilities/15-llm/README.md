# 可选：OpenAI 兼容接口

验证项目的 OpenAI 兼容出口（`/v1/projects/:projectID/v1/...`）：

1. `GET /models`：返回 OpenAI 列表形状，至少一个模型；
2. `POST /chat/completions`（非流式）：`chat.completion` 形状、`choices[0].message.content` 非空、
   `usage.total_tokens > 0`；
3. `POST /chat/completions`（`stream=true`）：SSE `chat.completion.chunk` 分块、聚合内容非空、
   以 `data: [DONE]` 结束；
4. 不带 `Authorization` 调用返回 401 `unauthenticated`。

## 前置与 SKIP

LLM 依赖服务端配置的 provider 与外部网络；以下情况整体 SKIP 并退出 0，不计 FAIL：

- 路由未挂载（`/models` 返回 404）；
- token 无 `llm:invoke`（403）；
- 模型列表为空（provider 未配置）；
- 上游暂不可用（502/504 或 `llm_upstream_error` / `llm_provider_unavailable`）。

SKIP 时 jsonl 写一条 `ok=true, detail="skipped: ..."` 记录。

## 运行

```bash
export CASE=15-llm
./examples/capabilities/15-llm/run.sh
```

## 接口

```bash
BASE="$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/v1"

curl "$BASE/models" -H "Authorization: Bearer $SIMPLEBASE_TOKEN"

curl -X POST "$BASE/chat/completions" \
  -H "Authorization: Bearer $SIMPLEBASE_TOKEN" -H 'content-type: application/json' \
  -d '{"model":"<model>","messages":[{"role":"user","content":"Reply with exactly: pong"}],"max_tokens":32}'

curl -N -X POST "$BASE/chat/completions" \
  -H "Authorization: Bearer $SIMPLEBASE_TOKEN" -H 'content-type: application/json' \
  -d '{"model":"<model>","messages":[{"role":"user","content":"Reply with exactly: pong"}],"max_tokens":32,"stream":true}'
```

注意路径里有两段 `/v1`：前一段是 SimpleBase API 版本，后一段是 OpenAI SDK 约定的资源前缀
（OpenAI SDK 的 `base_url` 指向 `.../projects/<id>/v1`）。

## 断言点

- `/models`：`object=list`，`data` 至少 1 项且 `data[0].id` 非空。
- 非流式：`object=chat.completion`、`choices[0].message.role=assistant`、content 非空、
  `usage.total_tokens>0`；最多重试 3 次。
- 流式：HTTP 200、至少一个 chunk、每个 chunk `object=chat.completion.chunk`、
  聚合 delta 内容非空、出现 `data: [DONE]`。
- 无鉴权：HTTP 401 且 `error.code=unauthenticated`。

## 已知边界

- 本用例**不需要数据库**，只产生 LLM 用量记录（不写项目文件）。
- 模型名取 `/models` 的第一项；provider 的模型白名单/默认模型由服务端配置决定。
- 上游回答内容不做确定性断言，只要求非空，避免模型措辞变化导致用例抖动。
- 流式输出可能被 provider 以非 SSE 错误体返回；此时按外部依赖不可用 SKIP。
