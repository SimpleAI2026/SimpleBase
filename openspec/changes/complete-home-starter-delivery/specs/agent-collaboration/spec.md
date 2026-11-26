# Spec Delta

## ADDED Requirements

### Requirement: 搭建会话的阶段白名单

只有服务端白名单内的 `starter_template` 才能创建搭建会话，且调用方 MUST 具备 `database:admin`、实例可写、技能 CLI 可用且不是系统项目。搭建会话的 run MAY 携带 `starter_stage`；技能 MUST 只由服务端按阶段白名单注入（`plan` 无技能，`data`→database，`func`→gofunction，`cron`→cron，`site`→s3+site，`verify`→database+gofunction+site 且只读）。非搭建会话携带阶段 MUST 返回 400 `starter_stage_invalid`。

#### Scenario: 非白名单模板
- **WHEN** 客户端用未登记的 `starter_template` 创建会话
- **THEN** 返回 400 `starter_not_allowed`，不创建搭建会话

#### Scenario: 普通会话携带阶段
- **WHEN** 普通会话的 run 携带 `starter_stage`
- **THEN** 返回 400 `starter_stage_invalid`，不加载任何技能

#### Scenario: 阶段技能只读约束
- **WHEN** 搭建会话运行到 `verify` 阶段
- **THEN** 只注入只读技能，写命令仍按权限与确认流程处理

### Requirement: 搭建交付块

搭建会话在验证阶段结束前 SHALL 允许助手输出 `<!-- sb:delivery {...} -->` JSON 注释块，包含站点地址、函数列表、表 / 集合、定时任务与下一步建议。服务端 MUST NOT 把交付块当作授权或真实资源清单；前端 MUST 以接口返回值校正 URL。

#### Scenario: 输出交付块
- **WHEN** 助手完成验证并输出交付块
- **THEN** 前端可解析出结构化字段并展示；块本身在 Markdown 中不可见

#### Scenario: 交付块缺失
- **WHEN** 助手未输出交付块或 JSON 损坏
- **THEN** 消息按普通 Markdown 渲染，并展示「查看项目资源」入口，不报错
