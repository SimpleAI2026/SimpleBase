# Spec Delta

## ADDED Requirements

### Requirement: 分阶段自动执行

搭建会话 SHALL 由前端按 `plan → data → func → cron → site → verify` 顺序自动发送下一阶段，仅在上一阶段成功结束后推进。任一阶段失败 MUST 停在原阶段并提供「重试本阶段 / 跳过 / 我来接手」；用户手动发送消息 MUST 退出自动模式。

#### Scenario: 阶段成功
- **WHEN** `data` 阶段以成功 end 事件结束
- **THEN** 前端自动发送 `func` 阶段请求

#### Scenario: 阶段失败
- **WHEN** `func` 阶段失败
- **THEN** 不进入 `cron`，界面展示重试 / 跳过 / 接手三个动作

#### Scenario: 用户接管
- **WHEN** 自动执行期间用户手动发送消息
- **THEN** 自动模式停止，后续阶段不再自动发送

### Requirement: 交付卡片渲染

前端 SHALL 解析搭建会话最后一条助手消息中的交付块并渲染交付卡片，包含站点入口、函数地址、数据对象、定时任务与下一步。`site_url` MUST 与后端返回的站点基址同源，否则丢弃并使用后端值；函数地址 MUST 由前端按当前 origin 与项目 ID 重新拼接。字段缺失时逐项隐藏。

#### Scenario: 站点地址不一致
- **WHEN** 交付块中的 `site_url` 与后端站点基址不同源
- **THEN** 卡片使用后端返回的地址，不渲染模型给出的 URL

#### Scenario: 部分字段缺失
- **WHEN** 交付块只有 tables 与 next
- **THEN** 卡片只渲染这两项，其余区域隐藏
