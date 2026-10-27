# cron
<!-- allow: cron -->
<!-- readonly: cron list,cron get,cron runs -->

命令：`cron list|get|create|update|delete|runs|trigger`。定时任务按计划调用一个**已生效的云函数导出函数**，不是执行 shell 命令。

前置：目标云函数必须存在且有生效版本，`--func-export` 必须在该版本的导出函数内（先用 `function get` 确认）。

## create

```
simplebase cron create --name cleanup --schedule-kind cron --cron "0 3 * * *" --func-file hello --func-export Hello --input '{"days":7}'
simplebase cron create --name poll --schedule-kind interval --interval-seconds 300 --func-file hello --func-export Hello
simplebase cron create --name once --schedule-kind once --run-at 2026-12-31T23:00:00Z --func-file hello --func-export Hello
```

- 必填：`--name`（字母开头，字母数字 `_-`，最多 63 字符）、`--schedule-kind`（`cron`/`interval`/`once`）、`--func-file`（云函数名）、`--func-export`。
- 调度字段按 kind 选一：`cron` 用 `--cron`（5 段：分 时 日 月 周）；`interval` 用 `--interval-seconds`（60 到 2592000）；`once` 用 `--run-at`（RFC3339）。
- 可选：`--description`、`--input`（JSON，最多 8KB，默认 `{}`）、`--enabled true|false`。

## update

```
simplebase cron update --id <任务ID> --enabled false   ← 只传 enabled 即纯启停
simplebase cron update --id <任务ID> --schedule-kind interval --interval-seconds 600 --func-file hello --func-export Hello
```

任务名不可改。改调度时要把 `--schedule-kind`、对应调度字段、`--func-file`、`--func-export` 一并传全；缺字段返回 `invalid_request`。

## 查看

```
simplebase cron list
simplebase cron get --id <任务ID>
simplebase cron runs --id <任务ID>
```

`runs[].status` 只有 `running`、`completed`、`failed`、`canceled`；成功是 `completed`，没有 `success`。

`list` 里的 `target_missing:true` 表示目标函数或导出已不可用，需要修复函数或改任务。

## 触发与删除

```
simplebase cron trigger --id <任务ID>   ← 需要确认
simplebase cron delete --id <任务ID>    ← 需要确认
```

两者都需要界面确认；headless 返回 `confirmation_unavailable`，不要重试。`trigger` 异步执行，结果用 `cron runs` 查看；任务正在运行时返回 `cron_job_running`，稍后再看。

## 常见错误码

`invalid_cron_expression`、`invalid_interval`、`invalid_run_at`、`invalid_cron_job_name`（按提示改字段）；`gofunction_not_found`、`no_active_version`、`function_not_exported`（目标函数问题，先修函数）；`cron_job_exists`（换名）；`cron_job_not_found`（核对 `--id`）；`cron_scheduler_unavailable`/`cron_scheduler_stopped`（调度器未运行，如实告知）；`readonly_mode`/`writer_unavailable`（实例只读）。
