# 物联网遥测

项目 `ex-iot01`。运行 `./examples/init.sh --only iot-telemetry` 后，两种方式查看：

```bash
# 可视化页面（含 KV、对象、定时任务、触发器审计）
set -a; . examples/iot-telemetry/.env; set +a
go run ./examples/iot-telemetry/server      # http://127.0.0.1:8098

# 原有脚本（设备模拟、worker、curl）
bash examples/iot-telemetry/scripts/curl.sh
node examples/iot-telemetry/scripts/simulate.mjs
node examples/iot-telemetry/scripts/worker.mjs
```

## 能力覆盖

| 能力 | 落在哪 |
| --- | --- |
| 数据库（SQL） | `devices`、`readings`、`rollup_5m`、`processed_runs`、`audit_log` |
| 集合文档 | `device_events`（设备事件流，驱动 record_change 触发器） |
| KV | `online:device-01`（120s TTL）、`events:device-01`（list）、`counter:readings`（计数器） |
| 对象存储 | `exports/readings.csv`，`exports/` 下的上传走对象事件触发器 |
| 云函数 | `rollup.Window`（5 分钟窗口）、`rollup.Aggregate`（均值/峰值/样本数）、`audit.*` |
| 定时任务 | `iot_rollup`（每 300s）与 `iot_export_once`（once，2099 年），各有运行记录 |
| 事件触发器 | `record_change:device_events`、`object_event:exports/`、`http` |

页面上的动作按钮对应：

| 按钮 | 链路 |
| --- | --- |
| 写一条读数 | `INSERT readings` + KV `INCR`/`SET EX`/`LPUSH` + 写 `device_events` → 触发器回调 |
| 按 5 分钟窗口聚合 | 取最近读数 → 调 `rollup.Aggregate` → 写 `rollup_5m` |
| 上传一份 CSV | 上传 `exports/*.csv` → 对象事件触发器回调 |
| 立即触发 iot_rollup | `POST /cron-jobs/<id>/trigger` |
| 写一条设备事件 / invoke HTTP | `record_change` 与 `http` 触发器 |

## 验证清单

- 数据库 `telemetry`：`devices`（2 台）、`readings`、`rollup_5m`、`processed_runs`、`audit_log` 与 `_migrations`。
- 集合 `device_events` 有种子文档 `evt-demo-0001`。
- KV：`online:device-01`、`events:device-01`、`counter:readings`。
- 文件：`exports/readings.csv`（以及页面上传的 CSV）。
- 云函数：`rollup`（`Window`/`Aggregate`）、`audit`（三个导出）。
- 定时任务：`iot_rollup`（interval 300s）、`iot_export_once`（once 2099-01-01）。
- 触发器：三条，`GET /function-triggers` 需 `project:admin`；运行时读 `.simplebase/triggers.json` 快照。

`worker.mjs` 重复执行不会重复处理已消费的 run；积压达到 100 条或截断时停止并告警。
局限：G1、G2、G6、G8、G11、G12（见 `plan/planv4.0/examples-business-cases-plan.md` §6）。