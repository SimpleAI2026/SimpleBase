# 预期结果

| 步骤 | 通过条件 |
|---|---|
| 创建函数 | `ok=true`，可被定时任务调用 |
| 创建任务 | `data.id` 非空，`data.enabled` 为 false，`data.func_export` 为 `Tick` |
| get | 同一个 id |
| trigger | `data.triggered` 为 true（HTTP 202） |
| runs | 约 20 秒内出现一条记录，`status` 最终为 `success`，`response_json` 含 `"n":2` |
| delete | `ok=true` |

任务创建时是关闭的；手动触发不依赖下一次 interval。
