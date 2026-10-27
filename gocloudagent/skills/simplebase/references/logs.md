# logs
<!-- allow: log -->
<!-- readonly: log search -->

命令：`log search`。检索当前项目的结构化日志（按时间倒序）。

## search

```
simplebase log search --q "timeout" --level error --limit 50
```

可选 flag：`--q`（关键词）、`--level`（`error`/`warn`/`info`/`debug`）、`--limit`（默认 100，上限 500，超限回落到 100）。返回 `{"events":[...]}`，每条含时间、级别、logger、消息、`request_id`。

- 先用较窄的 `--level` 与 `--limit` 查，再按 `request_id` 用 `--q` 追同一请求的日志。
- 日志里的密钥与令牌已脱敏；不要尝试还原，也不要向用户转述疑似凭据的内容。
- 没命中返回空 `events`，如实说明，不要编造日志。

## 保留期

`log retention get|set` 不对助手开放，后端返回 `forbidden`。用户要查看或调整保留天数，请其在控制台「日志」页操作（设置需要 `project:admin`）。
