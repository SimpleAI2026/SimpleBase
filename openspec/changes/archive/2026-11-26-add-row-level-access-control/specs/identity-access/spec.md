# Spec Delta

## ADDED Requirements

### Requirement: 集合行级访问规则

项目 SHALL 能为每个集合声明行级访问规则：`owner_field`（默认 `owner_id`）与模式 `off`、`owner_read`、`owner_write`。`owner_read` 只限制读，`owner_write` 同时限制读写。规则默认 `off`，存量项目行为不变。规则变更 MUST 由项目管理员发起并写审计（只记规则摘要）。

#### Scenario: 关闭规则
- **WHEN** 集合规则为 `off`
- **THEN** 项目成员按原有项目权限读写，不注入 owner 过滤

#### Scenario: 修改规则
- **WHEN** 项目管理员把规则从 `off` 改为 `owner_read`
- **THEN** 规则持久化成功并写审计，后续读请求按 owner 过滤

### Requirement: 行级越权拒绝

启用行级规则后，非管理员主体访问不属于自己的文档 MUST 返回 403 `forbidden_row`，且响应与日志不得泄露该行是否存在或包含哪些字段。管理员与系统内部主体 SHALL 绕过行级规则。

#### Scenario: 读取他人文档
- **WHEN** 普通成员按 id 读取一条 owner 为其他成员的文档
- **THEN** 返回 403 `forbidden_row`，不返回文档内容

#### Scenario: 管理员读取
- **WHEN** 项目管理员读取任意成员的文档
- **THEN** 正常返回文档内容

#### Scenario: 终端用户越权
- **WHEN** 终端用户尝试读取其他终端用户的文档
- **THEN** 返回 403 `forbidden_row`，其身份不被提升为项目成员
