# Data Service Specification

## Purpose

定义 SQL、集合文档、KV 与实时变更的对外行为与安全边界，包括系统库只读、敏感列脱敏与查询限额。

## Requirements

### Requirement: SQL 守卫与查询限额

所有用户 SQL SHALL 经 `godatabase/sqlguard` 校验：只读通道 MUST 拒绝写语句，任何通道 MUST 拒绝多语句与 NUL 字节。执行 MUST 受并发信号量、返回行数与请求体上限约束；审计只记录 SQL 的 SHA-256 与元数据，不得记录 SQL 文本或参数。

#### Scenario: 只读通道执行写语句
- **WHEN** 客户端在查询接口提交 `INSERT`、`UPDATE`、`DELETE` 或 DDL
- **THEN** 返回 400 且不连接数据库执行

#### Scenario: 多语句注入
- **WHEN** SQL 中包含分号分隔的第二条语句或 NUL 字符
- **THEN** 请求被拒绝，审计记录哈希而不记录原文

### Requirement: 集合文档与过滤语言约束

集合名 MUST 匹配 `^[A-Za-z][A-Za-z0-9_]{0,62}$`。过滤条件长度 MUST ≤ 2KB，单次扫描行数 MUST ≤ 50000，`in` 元素 MUST ≤ 32，展开上限 MUST ≤ 1024；分页游标 SHALL 使用 `base64({c,i})` 且按 `(created_at, id)` 稳定排序。调用方已指定 `databaseId` 时 MUST NOT 静默改用项目第一个库。

#### Scenario: 超限过滤
- **WHEN** 过滤条件超过 2KB 或扫描超过 5 万行
- **THEN** 返回可识别的限制错误，不返回部分结果冒充全量

#### Scenario: 指定不存在的数据库
- **WHEN** 请求携带 `databaseId` 但该库不属于当前项目
- **THEN** 返回 404/400，不回退到第一个库

### Requirement: KV 缺失键语义

KV MUST 区分「键不存在」与错误：`GET` 返回 `null`，`TTL` 返回 `-2`（无该键）或 `-1`（无过期），`TYPE` 返回 `none`，`DEL` 返回 `0`，`SCAN` 游标结束返回字符串 `"0"`。KV 数据 MUST 存放在用户库的 `__` 前缀内部表中。

#### Scenario: 读取不存在的键
- **WHEN** 客户端对不存在的键执行 `GET`
- **THEN** 返回 `null` 而不是 404

#### Scenario: 删除不存在的键
- **WHEN** 客户端删除不存在的键
- **THEN** 返回 `0` 且不报错

### Requirement: 系统库只读与敏感列脱敏

系统库 MUST 只允许 SELECT，禁止写入与删除，且不得经用户库 registry 打开。查询系统库时 MUST 先做标识符扫描：命中 `password_hash`、`key_hash`、`refresh_token_hash`、`access_jti`、`credentials_json`、`credential_ref` 时返回 403 `system_column_redacted` 且不执行；结果中的敏感列 SHALL 置 `null` 并在 `redacted_columns` 中返回列名。行浏览 MUST 默认 50 行、上限 200，`offset` 上限 100000。

#### Scenario: 查询敏感列
- **WHEN** 客户端显式 SELECT 任一敏感列
- **THEN** 返回 403 `system_column_redacted`，SQL 不执行

#### Scenario: 浏览系统表行
- **WHEN** 客户端浏览系统表且未指定 limit
- **THEN** 最多返回 50 行，且 `__` 前缀内部表不可见

### Requirement: 变更事件的发布顺序

数据变更 SHALL 按「变更 → 提交 → NotifyWrite → 索引 → 领域事件 → 审计」的顺序传播；事件只在提交成功后发布，订阅方不得在回滚路径收到事件。

#### Scenario: 写入回滚
- **WHEN** 数据写入在提交前失败并回滚
- **THEN** 不发布变更事件，实时订阅者收不到该次变更

#### Scenario: 成功写入
- **WHEN** 文档写入提交成功
- **THEN** 实时订阅与触发器按登记顺序收到携带集合与文档标识的事件

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
