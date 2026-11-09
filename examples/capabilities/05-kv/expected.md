# 预期结果

| 步骤 | 通过条件 |
|---|---|
| SET | `data` 为 `OK` |
| GET | `data` 为 `blue` |
| INCR | `data` 为 `1` |
| HSET | `data` 为 `1`（新增 1 个字段） |
| HGET | `data` 为 `alice` |
| DEL | `data` 为 `1` |
| GET 已删除的键 | `data` 为 JSON `null` |

回复就是 Redis 值本身，CLI 再包一层 `{ok, data}`，没有额外的 `result` 字段。
