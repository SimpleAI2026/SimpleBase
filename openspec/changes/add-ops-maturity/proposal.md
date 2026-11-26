# Proposal

## Why

生产就绪还缺四块：日志与指标的保留策略只有配置、没有执行者；`sys_metric_rollups_hourly` 建表后零读写；没有 HTTP 就绪端点（只有内置 doctor）；告警没有外部通知通道。这四项都在 legacy plan §3-A 与 §3-H 登记，属于上线前必须收口的能力。

## What Changes

- 保留清理：按 `keep_days` 删除过期日志、指标与审计行，挂到既有维护流程（`systemdb/maintenance.go`），不新起无序 goroutine；清理失败不阻断启动。
- 小时聚合：为 `sys_metric_rollups_hourly` 接线（长周期看板唯一读取点）或移除该死表，二选一，消除零读写表。
- 就绪端点：新增 `GET /v1/system/readiness`，返回各关键依赖的 ready 状态与原因；无副作用，200/503 两态。
- 外部通知：至少支持一种外部通道（SMTP 或 Webhook），告警可按配置推送；密钥不落日志与审计。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `ops-governance`: 增加保留清理、小时聚合唯一读写点、HTTP 就绪端点与外部通知。

## Impact

- 受影响代码：`internal/systemdb`（logs/metrics/audit 清理、维护入口）、`internal/api/opshttp`（readiness）、`internal/alerts`（通知通道）、`internal/config`（通道配置，密钥只走环境变量）、`docs/ops`。
- 不改变：现有 `/health/live`、`/health/ready` 与 `/metrics` 行为；不新增第三方依赖（SMTP 用标准库 `net/smtp`，Webhook 用 `net/http`）。
- 数据迁移：只做删除过期行，不改表结构。
