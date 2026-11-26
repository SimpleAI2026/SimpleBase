# Console UI Specification

## Purpose

定义 SimpleBase 控制台的加载与错误状态、国际化文案、组件状态钩子、路由与只读权限，以及云助手工作台、数据看板与首页引导的交互契约，保证跨页面体验一致、可测试且不误导用户。

## Requirements

### Requirement: 加载、空与错误三态

异步区域 SHALL 经统一组件呈现加载、空与错误三态；加载指示 MUST 延迟 160ms 出现、消失不延迟，避免闪烁。错误态 MUST 只有一个实现，表格内使用 inline 变体，普通区域使用 block 变体。

#### Scenario: 快速返回
- **WHEN** 请求在 160ms 内完成
- **THEN** 不显示加载态，直接呈现数据或空态

#### Scenario: 请求失败
- **WHEN** 列表请求返回错误
- **THEN** 区域显示统一错误组件与重试入口，不残留旧数据

### Requirement: 国际化单一来源

所有界面文案 MUST 来自 `ui/src/locales/`（`zh-CN.ts` 权威树与 `en-US.ts` 同构）。新增文案 MUST 同时补齐两棵树，key 集合完全一致；组件内使用 `useI18n()`，组件外使用统一导出的 `t()`。文案中的裸 `@` MUST 写成 `{'@'}`。

#### Scenario: 只补一棵语言树
- **WHEN** 开发者只在一个语言文件新增 key
- **THEN** i18n 测试失败并指出缺失 key

#### Scenario: 文案包含邮箱
- **WHEN** 文案中出现 `user@example.com`
- **THEN** 按 vue-i18n 规则转义，渲染不报 `Invalid linked format`

### Requirement: 组件状态钩子匹配 reka-ui

状态样式 MUST 使用 reka-ui 实际输出的 `data-state`（`checked`/`unchecked`、`active`/`inactive`、`open`/`closed`）与 `data-disabled`、`data-highlighted`、`data-selected`；MUST NOT 使用裸 `data-checked`、`data-unchecked`、`data-open`、`data-closed`。`data-active` 仅作为 `TabsTrigger` 的已知例外。

#### Scenario: 开关选中样式
- **WHEN** 用户打开一个 Switch
- **THEN** 选中样式由 `data-[state=checked]:` 命中，关闭时由 `data-[state=unchecked]:` 命中

#### Scenario: 新增页签样式
- **WHEN** 开发者编写页签选中样式
- **THEN** 使用 `data-[state=active]:`，不写裸 `data-active:`

### Requirement: 路由与只读体验

路由 redirect MUST 保持既有映射（`/sql`→`/databases`、`/llm`→`/agents`、`/settings`→`/?settings=1` 等）。新页面 MUST 在 `router/index.ts` 注册并配置 `meta.titleKey`，需要角色控制时配置 `meta.requiresRole`。系统项目的只读判定 MUST 只经 `stores/project.ts` 的 `isAdmin`，且前端只读只是体验层。

#### Scenario: 访问旧路径
- **WHEN** 用户打开 `/sql`
- **THEN** 重定向到 `/databases`，页面标题正确

#### Scenario: 系统项目
- **WHEN** 用户进入 admin 项目
- **THEN** 写按钮隐藏、SQL 进入只读查询模式，后端仍独立校验

### Requirement: 云助手工作台

工作台 MUST 以「群 = 协作会话」组织：群可展开多个会话，可添加已有助手或新建助手，支持群级路由提示词与多助手分工；人工待办 MUST 在消息流中可见并可回复。计划确认 MUST 以整份计划为单位展示，未确认前不执行写操作。

#### Scenario: 群内新增会话
- **WHEN** 用户在群行点击 `+` 并选择新建会话
- **THEN** 新会话出现在该群下，历史消息彼此隔离

#### Scenario: 确认计划
- **WHEN** 助手产出计划决策
- **THEN** 工作台展示整份计划与确认入口；确认后才继续执行

### Requirement: 数据看板与首页引导

数据浏览页 SHALL 支持集合与 SQL 表看板、只读聚合、筛选、时间范围与明细查看；统计结果 MUST 区分「参与统计行数」与物理扫描量，截断结果不得冒充全量占比。首页提示词 SHALL 支持一键创建项目并进入对应助手会话：点击提示词先打开确认弹窗，确认后创建项目并自动发送一次种子提示词（这是「点击只填不发」的显式例外）。

#### Scenario: 饼图 Top N
- **WHEN** 分类数量超过展示上限且结果被截断
- **THEN** 卡片标注部分分组，不声称展示全量占比

#### Scenario: 首页提示词
- **WHEN** 用户点击一个首页提示词并在确认弹窗中确认
- **THEN** 创建或复用项目、打开对应助手会话，并自动发送一次种子提示词；重复进入同一项目不重复发送

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
