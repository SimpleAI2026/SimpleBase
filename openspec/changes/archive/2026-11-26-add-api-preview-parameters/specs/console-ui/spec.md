# Spec Delta

## ADDED Requirements

### Requirement: OpenAPI 路径参数来源

OpenAPI 文档中的路径参数 SHALL 携带可选来源元数据，标识该参数应取当前项目、数据库、助手、集合或其他项目资源。未知来源 MUST 被忽略，既有字段（`name`、`in`、`required`、`schema`）MUST 保持不变。

#### Scenario: 读取参数来源
- **WHEN** 控制台加载 OpenAPI 文档
- **THEN** 已知来源的路径参数带有来源标记，未知参数按普通字符串处理

### Requirement: API 预览自动填充

API 预览在选中端点时 MUST 为路径参数注入默认值：`projectID` 取当前项目；数据库、助手、集合等参数 MUST 从当前项目的资源列表选择。资源列表不可用时 MUST 退化为手动输入，不得阻塞预览。

#### Scenario: 选择端点
- **WHEN** 用户在项目中选中一个含 `:projectID` 与 `:databaseID` 的端点
- **THEN** `projectID` 自动填入当前项目，`databaseID` 下拉列出当前项目的数据库

#### Scenario: 资源列表失败
- **WHEN** 数据库列表请求失败
- **THEN** 预览仍可打开，参数显示为输入框并提示需手动填写

### Requirement: 路径占位符完整替换

预览请求路径 MUST 替换参数的所有同名占位符，不得只替换第一次出现；参数为空时 MUST 阻止发送并提示缺失参数，而不是发出含 `{}` 的非法路径。

#### Scenario: 同名参数出现两次
- **WHEN** 路径模板包含两个同名占位符
- **THEN** 两处都被替换为同一个值

#### Scenario: 参数缺失
- **WHEN** 用户未填写必填路径参数就发送
- **THEN** 请求被阻止并高亮缺失参数
