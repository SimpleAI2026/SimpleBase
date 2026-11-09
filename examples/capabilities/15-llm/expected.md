# 预期结果

| 步骤 | 通过条件 |
|---|---|
| 前置探测 | `/models` 返回 200 且至少 1 个模型；404/403/空列表 → 整体 SKIP，退出码 0，jsonl `ok=true` + `detail=skipped: ...` |
| models | `object=list`、`data` 非空、`data[0].id` 非空 |
| chat-completion | HTTP 200；`object=chat.completion`、assistant 消息 content 非空、`usage.total_tokens>0` |
| chat-stream | HTTP 200；至少一个 `chat.completion.chunk`、聚合 delta 非空、以 `data: [DONE]` 结束 |
| no-auth-401 | HTTP 401 且 `error.code=unauthenticated` |

> 上游 502/504 或 `llm_upstream_error` → SKIP（外部依赖），不计 FAIL。
> 本机实例已配置 provider 时完整流程应为 4 行 PASS；未配置 provider 时只有前置 SKIP 行。
