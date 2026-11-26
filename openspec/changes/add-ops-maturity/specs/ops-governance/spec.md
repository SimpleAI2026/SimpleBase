# Spec Delta

## ADDED Requirements

### Requirement: 保留策略可执行

日志、指标与审计的保留配置 SHALL 由既有维护流程按期执行删除，不得只保存 `keep_days` 而无执行者。清理失败 MUST NOT 阻断服务启动，MUST NOT 删除 `__` 前缀内部表，且每次清理 SHALL 记录删除行数与耗时（不含内容）。

#### Scenario: 过期数据清理
- **WHEN** 维护任务运行且存在超过 `keep_days` 的日志行
- **THEN** 过期行被删除，内部表与当前窗口数据保留

#### Scenario: 清理失败
- **WHEN** 清理语句执行失败
- **THEN** 记录告警并跳过本次清理，服务继续运行

### Requirement: 不存在零读写系统表

系统库中的指标聚合表 SHALL 有明确读写方：要么由写入方维护并被至少一个查询读取，要么从最终态建表中移除。MUST NOT 保留建表后零读写的死表。

#### Scenario: 小时聚合表
- **WHEN** 任一系统表被声明为聚合表
- **THEN** 存在写入点与读取点，或该表已从 `schema.go` 移除并有迁移说明

### Requirement: HTTP 就绪端点

系统 SHALL 提供 `GET /v1/system/readiness`，返回每个关键依赖的 ready 状态与失败原因。该端点 MUST 无写副作用，依赖全就绪时返回 200，否则返回 503。

#### Scenario: 依赖全部就绪
- **WHEN** 管理员调用就绪端点且系统库等依赖可用
- **THEN** 返回 200 与结构化 ready 列表

#### Scenario: 依赖不可用
- **WHEN** 任一依赖检查失败
- **THEN** 返回 503，指出失败依赖与原因，不泄露凭据或 DSN

### Requirement: 告警外部通知

告警 SHALL 支持至少一种外部通知通道（SMTP 或 Webhook），通道由配置启用，凭据 MUST 只从环境变量读取。发送失败 MUST 记录元数据并重试或降级，不得阻塞告警产生路径；通知内容 MUST NOT 包含密钥或完整请求体。

#### Scenario: 触发告警
- **WHEN** 磁盘或依赖告警达到阈值
- **THEN** 已启用的外部通道收到通知，系统内告警记录同时存在

#### Scenario: 通道不可用
- **WHEN** SMTP/Webhook 连接失败
- **THEN** 告警仍被记录，失败原因写日志，不影响主流程
