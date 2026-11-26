# Design

## 保留清理

- 入口：`systemdb/maintenance.go` 的既有维护循环增加 `pruneExpired(ctx)`，按表读取 `keep_days`，分批删除（每批 1000 行）避免长事务。
- 表范围：`sys_log_events`、`sys_metric_samples`、`sys_audit_events`；`__` 前缀内部表永不进入清理。
- 失败：记录 zap warn（表名、错误、耗时），下一轮重试；不阻断启动。

## 小时聚合决策

优先「移除」而不是「接线」：当前没有任何长周期看板依赖该表，接线会新增写入放大与维护成本。移除需同步 `systemTables` 与 `SystemTableCount` 断言，并在发布说明标注（旧库 reset 后消失）。若后续看板需要长周期趋势，再以独立变更引入。

## 就绪端点

- 路由：`GET /v1/system/readiness`，放在系统管理组，要求项目管理员或超管权限。
- 复用 `app/doctor.go` 的检查器，构造只读检查列表；响应 `{ready: bool, checks: [{name, ready, reason}]}`。
- 结果做 1–2 秒短 TTL 缓存，失败不缓存，避免探针高频打系统库。

## 外部通知

- 抽象 `alerts.Notifier`（`Notify(ctx, event) error`），实现 `smtpNotifier` 与 `webhookNotifier`；配置选中一个即可。
- 凭据只从 `SIMPLEBASE_ALERT_SMTP_PASSWORD` / `SIMPLEBASE_ALERT_WEBHOOK_TOKEN` 读取，YAML 只放地址与开关。
- 发送异步化 + 有界队列（容量 64），满时丢弃并计数；内容经现有脱敏函数处理。

## 回滚

- 清理与通知均可通过配置关闭；readiness 端点可独立回滚路由。
- 移除聚合表属破坏性变更：先在一个版本停止写入，再在下一个版本删表，避免旧进程写已删表。
