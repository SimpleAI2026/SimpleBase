# 定时任务

先部署一个已生效的云函数，再创建关闭状态的 interval 任务，手动触发，轮询运行记录，最后删除任务。

`--func-file` 是云函数名，不是磁盘路径。`interval-seconds` 最小 60。触发是异步的。

## 运行

```bash
export CASE=08-cron
./examples/capabilities/08-cron/run.sh
```

## 命令

```bash
simplebase function create --name "tick$RUN_ID" --file tick.go --description "cron target"
simplebase cron create --name "tick-$RUN_ID" --schedule-kind interval --interval-seconds 3600 \
  --func-file "tick$RUN_ID" --func-export Tick --input '{"n":1}' --enabled false
simplebase cron get --id "$JOB"
simplebase cron trigger --id "$JOB"
simplebase cron runs --id "$JOB"
simplebase cron delete --id "$JOB"
```

`cron trigger` 和 `cron delete` 在云助手里需要确认。调度器没启动时触发返回 `cron_scheduler_unavailable`。
