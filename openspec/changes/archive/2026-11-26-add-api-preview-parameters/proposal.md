# Proposal

## Why

API 预览在切换端点时把所有路径参数清空为 `''`，用户必须手抄项目 ID、数据库 ID、集合名与助手 ID 才能试跑；`resolvedPath` 还只替换每个参数的第一次出现。这是控制台里最影响试用效率的摩擦点（legacy APV-1~4）。

## What Changes

- OpenAPI 的路径参数增加来源元数据（项目、数据库、助手、集合等），作为扩展字段输出，不改变现有消费者可见的 schema。
- API 预览在选中端点时自动填充：`projectID` 取当前项目，`databaseID` / `agentID` / `collection` 等从当前项目资源列表选择（下拉），资源不存在时保持空并提示。
- 路径替换改为替换全部同名占位符；切换端点时先清空旧值再注入默认值。
- 资源列表加载失败时不影响预览与手填，只把下拉退化为输入框。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `console-ui`: 增加 API 预览的参数来源、默认值与下拉选择规则。

## Impact

- 受影响代码：`internal/api/route_meta.go`（OpenAPI 扩展字段）、`ui/src/pages/ApiPreview.vue`、`ui/src/services/{types,http-api}.ts`、`ui/src/locales/`。
- 不改变：路由集合、OpenAPI 既有字段与路径、鉴权与请求行为。
- 兼容性：扩展字段以下划线或 `x-` 前缀输出，旧客户端忽略即可。
