# Identity Access Specification

## Purpose

定义用户、API Key、会话、委托令牌与项目上下文的身份与授权契约，确保所有入口使用同一套权限判定并保持最小权限。

## Requirements

### Requirement: 权限层级与唯一判定入口

系统 SHALL 采用 `database:admin ⊃ database:write` 与 `database:admin ⊃ database:read` 的层级，且 `database:write` 不隐含 `database:read`。所有授权判定 MUST 经 `auth.Principal.HasPermission` 或 `auth.PermissionImplied`，不得直接读取权限映射。

#### Scenario: 仅写权限的凭据读取数据
- **WHEN** 调用方只持有 `database:write`
- **THEN** 读接口被拒绝，写接口按项目范围放行

#### Scenario: 管理员权限
- **WHEN** 调用方持有 `database:admin`
- **THEN** 读写接口均按项目范围放行，无需额外授予 write 或 read

### Requirement: API Key 的签发与吊销

API Key SHALL 只授予签发者自身权限的子集，并精确携带被授予的权限。Key 明文 MUST 只在创建响应中出现一次，服务端只保存哈希；吊销 SHALL 立即生效。

#### Scenario: 签发越权 Key
- **WHEN** 签发者请求的权限超出自身权限集合
- **THEN** 请求被拒绝，不创建 Key

#### Scenario: 使用已吊销 Key
- **WHEN** 客户端携带已吊销的 API Key 访问任意受保护接口
- **THEN** 返回 401，且响应与日志不包含 Key 明文

### Requirement: 登录、会话与初始改密

交互式登录 SHALL 签发短期访问 JWT 与可轮换 refresh token；引导账号或重置后的初始密码 MUST 在首次登录后强制修改。重复失败登录 SHALL 按维度软锁定，而不是固定 IP+用户名频率阈值。

#### Scenario: 初始密码未修改
- **WHEN** 用户以初始密码登录后调用其他业务接口
- **THEN** 请求被拒绝并提示必须改密，改密成功后恢复

#### Scenario: 访问令牌过期
- **WHEN** 访问 JWT 过期但 refresh token 有效
- **THEN** 客户端可静默轮换并重放原请求；refresh 失效时返回 401 并回到登录态

### Requirement: 委托令牌的最小权限

委托令牌 SHALL 只写入调用方已按 writable 降权后的权限，SHALL 绑定单个项目，并在策略缺失或项目不匹配时 fail-closed。令牌 MUST NOT 因继承关系拿回写或管理权限。

#### Scenario: 只读降级后委托
- **WHEN** 云助手以只读身份为子任务签发委托令牌
- **THEN** 令牌只含读权限，任何写请求返回拒绝

#### Scenario: 跨项目使用
- **WHEN** 委托令牌被用于另一个项目
- **THEN** 返回 `project_mismatch` 并拒绝执行

### Requirement: 项目上下文与终端用户隔离

所有包含 `:projectID` 的路由 SHALL 先经过项目上下文中间件完成鉴权并注入项目；终端用户凭据 SHALL 使用独立鉴权通道，MUST NOT 获得控制台管理员或项目管理员能力。

#### Scenario: 跨项目访问
- **WHEN** 用户 A 使用自己的凭据访问用户 B 的项目资源
- **THEN** 在进入 handler 之前返回 403/404，不泄露目标项目是否存在

#### Scenario: 终端用户访问控制台接口
- **WHEN** 终端用户凭据调用项目管理员接口
- **THEN** 返回 401/403，权限不被提升

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
