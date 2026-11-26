# Spec Delta

## ADDED Requirements

### Requirement: 站点 CLI 与 SDK

`simplebase` CLI SHALL 提供 `file site enable|disable|status` 与 `file public <key> [--folder] --on|--off`；JavaScript SDK SHALL 提供 `s3.site.get`、`s3.site.set` 与 `s3.setPublic`。两种入口 MUST 与 HTTP API 使用同一语义与错误码。

#### Scenario: CLI 启用站点
- **WHEN** 管理员执行 `simplebase file site enable`
- **THEN** 站点开启，`status` 返回可发布状态与访问地址

#### Scenario: SDK 设置公开访问
- **WHEN** 调用 `s3.setPublic(key, true)`
- **THEN** 对象可经公开链接访问，返回结果与 HTTP API 一致

### Requirement: 部署清理需确认

`site deploy --prune` MUST 先列出将被删除的对象并要求显式确认；非交互模式 MUST 以退出码 3 结束且不删除任何对象。清理范围 MUST 限制在本次发布前缀内，MUST NOT 删除前缀外对象。

#### Scenario: 非交互清理
- **WHEN** 在 CI 中执行 `site deploy --prune` 而未提供确认
- **THEN** 输出待删清单并以退出码 3 结束，对象保持不变

#### Scenario: 确认清理
- **WHEN** 用户确认后执行
- **THEN** 只删除前缀内不属于本次产物的对象，结果列出删除数量
