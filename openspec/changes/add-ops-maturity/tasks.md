# Tasks

## 1. 保留清理（legacy A1）

- [ ] 1.1 在 `systemdb/maintenance.go` 增加按 `keep_days` 分批清理日志 / 指标 / 审计
- [ ] 1.2 清理跳过 `__` 前缀内部表；失败只告警不阻断启动
- [ ] 1.3 用例：写入过期行后清理生效；清理失败时服务仍可启动

## 2. 小时聚合表（legacy A2）

- [x] 2.1 复核 `sys_metric_rollups_hourly` 是否仍零读写，记录证据
- [ ] 2.2 默认移除该表并同步 `schema.go`、`SystemTableCount` 与发布说明；若决定接线，改为写入 + 唯一读取点
- [ ] 2.3 用例：系统表清单与断言一致，无零读写表

## 3. 就绪端点（legacy H1）

- [ ] 3.1 复用 doctor 检查器实现 `GET /v1/system/readiness`，要求管理员权限
- [ ] 3.2 依赖就绪返回 200，任一失败返回 503 与原因；1–2 秒短 TTL 缓存
- [ ] 3.3 用例：两态各一；探针无写副作用；响应不含 DSN / 凭据

## 4. 外部通知（legacy H6）

- [ ] 4.1 定义 `alerts.Notifier` 与有界发送队列，满时丢弃并计数
- [ ] 4.2 实现 SMTP 或 Webhook 之一（标准库），凭据只从环境变量读取
- [ ] 4.3 用例：告警触发后通道收到脱敏内容；通道失败不影响告警记录与主流程

## 5. 验证

- [ ] 5.1 `go test ./internal/systemdb ./internal/alerts ./internal/api ./internal/app` 通过
- [ ] 5.2 `TestArchitectureGate` 通过；新增配置项不写入 YAML 密钥
- [ ] 5.3 文档更新：保留策略、readiness 端点、通知配置与回滚开关

## 完成情况核对（2026-11-26）

- 2.1 已完成：`sys_metric_rollups_hourly` 只在 `internal/systemdb/schema.go` 建表，全仓无读写点，确认仍是死表。
- 其余未开始：保留清理、就绪端点、外部通知均无实现。
