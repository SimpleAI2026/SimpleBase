# 物联网遥测

项目 `ex-iot01`；不需要 BFF。运行 `./examples/init.sh --only iot-telemetry` 后在控制台切换项目。

## 验证清单

- 数据库 `telemetry`：`devices`、`readings`、`rollup_5m`、`processed_runs` 和 `_migrations`；`SELECT * FROM devices` 可见设备。
- KV：`online:device-01`（120 秒 TTL）、`events:device-01`、`counter:readings`。
- 文件：`exports/readings.csv`；函数：`rollup.go` 已激活；任务：`iot_rollup` 与 `iot_export_once` 均有手动运行记录。

```bash
set -a; . examples/iot-telemetry/.env; set +a
bash examples/iot-telemetry/scripts/curl.sh
node examples/iot-telemetry/scripts/simulate.mjs
node examples/iot-telemetry/scripts/worker.mjs
```

`worker.mjs` 重复执行不会重复处理已消费的 run；积压达到 100 条或截断时停止并告警。局限：G1、G2、G6、G8、G11、G12（见 `plan/planv4.0/examples-business-cases-plan.md` §6）。
