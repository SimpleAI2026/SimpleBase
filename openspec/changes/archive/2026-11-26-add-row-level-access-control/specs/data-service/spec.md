# Spec Delta

## ADDED Requirements

### Requirement: 集合文档的行级过滤

启用行级规则的集合，读接口（列表、按 id、聚合、导出）MUST 在服务端注入 owner 过滤，分页与总数 MUST 基于过滤后的集合计算。`owner_write` 模式下写接口（更新、补丁、删除、批量、导入）MUST 校验目标行 owner；`owner_read` 模式只限制读，写仍按项目权限。任何启用模式下创建 MUST 写入当前主体的 owner 值。

#### Scenario: 列表只返回本人数据
- **WHEN** 普通成员列出启用了 `owner_read` 的集合
- **THEN** 只返回 owner 为自己的文档，分页总数也只统计这些行

#### Scenario: 创建时写入 owner
- **WHEN** 普通成员在启用规则的集合中创建文档且未提供 owner 字段
- **THEN** 服务端写入当前主体标识，客户端不能伪造为他人

#### Scenario: 更新他人文档
- **WHEN** 普通成员尝试更新或删除 owner 为其他成员的文档
- **THEN** 返回 403 `forbidden_row`，数据不变

### Requirement: 行级规则不适用于 SQL 通道

行级规则 MUST 只约束集合文档 API；SQL 查询与执行仍按项目权限判定。系统 MUST 在文档与错误提示中明确该限制，不得让用户误以为 SQL 也受 owner 过滤。

#### Scenario: 通过 SQL 读取集合
- **WHEN** 普通成员以项目读权限对集合底层表执行 SELECT
- **THEN** 请求按 SQL 权限放行且不注入 owner 过滤，文档明确说明该行为
