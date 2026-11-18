# 数据浏览页重写为 BI 看板计划（v6.7）

> 原始起因：`ui/src/pages/DataBrowser.vue`（`/console/data`）重写前是「集合侧栏 + 可编辑表格」，对开发者排查数据分布、趋势、异常值帮助有限。
> 目标：整页重写为 **BI 展示**，图表统一用项目已有的 **ECharts**，表格退居「明细下钻」；顶部提供「输入需求，由 Agent 生成图表」入口，**复用现有数据助手**。
> **2026-11-18 实现复核**：当前工作区已实现集合 / SQL 表看板和 Agent 出图基础链路；下一步不是重新搭建，而是先修复统计与交互正确性，再优化信息层级、图表可读性和确认流程。**当前完成度、问题清单与后续验收以 §12 为准**；§0 保留重写前证据，§1–11 保留原设计与编号，不代表已全部交付。
> 本次只更新计划，不改业务代码、不运行服务、不执行数据库操作。结论来自当前工作区源码及测试文件静态核查，**未做浏览器视觉验收，也未执行构建 / 测试**；“已有测试”不等于“本次通过”。承接 `plan/planv6.7/legacy-plan-consolidation-plan.md`（数据浏览的竞态与导出修复）。
>
> **确认记录**
> 1. 「输入需求后由 Agent 生成」复用现有的数据 Agent，**纳入一期**（§3.4、§4.7、§6 A1–A9）。
> 2. 方向约定：「柱状图」为**纵向**；「条形图」「对比条形图」「堆积条形图」为**横向**（§4.3）。
> 3. 图表类型固定为 9 项：自动、折线图、柱状图、条形图、饼图、表格、对比条形图、堆积条形图、散点图。

## 0. 重写前基线与证据（历史）

> 本节为最初立项时的快照；其中行号、文件大小、注册组件与测试数量均不是当前值。现有实现见 §12.1，不再以“无聚合接口 / SQL 仅展示 SchemaPanel”描述当前产品。

### 0.1 前端

| 事实 | 位置 |
| --- | --- |
| 整页单文件 480 行：库选择 + 集合侧栏 + filter/sort 输入 + 列显隐 + 行内编辑表格 + 分页 | `ui/src/pages/DataBrowser.vue:1-480` |
| 路由 `/console/data`，`name: 'data-browser'`，`meta.titleKey: 'nav.dataBrowser'` | `ui/src/router/index.ts:61-65` |
| `dataModel === 'sql'` 的库不走集合，直接渲染 `SchemaPanel` | `DataBrowser.vue:38-42` |
| 请求竞态防护：`databasesRequest` / `collectionsRequest` / `documentsRequest` 三个计数器 + 当前 project/database/collection 比对 | `DataBrowser.vue:206-208, 253-447` |
| ECharts 已是依赖（`echarts ^6.1.0`），按需引入 `echarts/core`，当前只注册 `BarChart` `LineChart` `GridComponent` `TooltipComponent` `CanvasRenderer` | `ui/package.json:24`、`ui/src/components/TrendChart.vue:53-61` |
| TrendChart 已有可复用做法：读 CSS 变量 `--chart-1..5` / `--border` 等、`ResizeObserver`、监听 `<html class>` 切换暗色 | `TrendChart.vue:89-121, 265-286` |
| 图表色板 token：`--chart-1..5`（亮 / 暗各一套） | `ui/src/style.css:86-90, 143-147` |
| 测试里 echarts 是**整包 mock**，`echarts/charts` 只导出 `BarChart/LineChart`，`echarts/components` 只导出 `GridComponent/TooltipComponent` | `ui/src/test/setup.ts:64-96` |
| 现有测试 8 例（查询/列显隐/行内保存/导出、类型化编辑与分页、事件接线、旧响应竞态、整链刷新、视图只读、SQL 库、真实 Select） | `ui/tests/DataBrowser.test.ts:79-264` |

### 0.2 后端

| 事实 | 位置 |
| --- | --- |
| 集合文档只有行级接口：列表、导出、导入、增删改。**没有任何聚合 / 统计接口** | `internal/api/router.go:479-488` |
| 列表接口 `limit` 上限 200、`offset` 上限 100000，BI 不能靠翻页拉全量再前端聚合 | `internal/api/collection_query.go:73-84` |
| 集合表结构固定为 `id, data(JSON 文本), created_at, updated_at`；字段值要经 `JSONExtract*` 取出，且**随引擎（DuckDB / SQLite）而异** | `data_handler.go:749-751`、`godatabase/dialect.go:12-28` |
| `filter` 语法只认**已登记 schema 的字段**；`created_at` / `updated_at` / `id` 不能出现在 filter 里（`sort` 可以） | `collection_filter.go:279-301`、`collection_query.go:162-164` |
| 视图集合的数据源是 `(<view_query>) AS sb_view`，可被同一套查询逻辑复用 | `collection_query.go:136-141` |
| 列表接口有扫描上限保护（`scanLimit` 默认 50000，只在带 filter 时生效） | `data_handler.go:84-103, 289-300` |
| 策略网关把**非 GET 一律当 write**；只读 Key 调 POST 会被 `key_scope_denied` | `policy_gate.go:45-48` |
| SQL 库已有 `POST /databases/:id/query`（`sqlguard.ReadOnly` + `max_rows` + 超时）可复用于「SQL 数据集」 | `sql_handler.go:154-227` |

### 0.3 编辑与管理入口的去向（重写前必须确认不丢功能）

| 原 DataBrowser 功能 | 重写后去向 |
| --- | --- |
| 行内编辑 / 保存（`api.db.patch`） | **移除**。`Databases.vue` 的 `DocumentListModal` 已覆盖新增、批量、编辑、删除、清空（`DocumentListModal.vue:5-8, 35, 128-193`） |
| Schema 设计器 `CollectionSchemaModal` | 保留，入口移到工具栏「数据操作」 |
| 新建视图 `ViewCollectionModal` | 保留。**全仓只有 DataBrowser 引用它**，不留入口就成了孤儿组件 |
| 导入 `CollectionImportModal` | 保留。同样**只有 DataBrowser 引用** |
| 导出 CSV / JSON | 保留，同样入「数据操作」 |
| SQL 库 `SchemaPanel` | 一期原样保留（见 §6 P2 的 SQL 数据集） |

### 0.4 现有云助手的可复用点（Agent 生成图表的基础）

| 事实 | 位置 | 对本计划的意义 |
| --- | --- | --- |
| 内置「数据」助手：`builtin_key: "database"`、`module: "database"`，内置只读工具 `list_databases` / `list_collections` / `readonly_sql`；`writable` 默认 false | `internal/systemdb/agents.go:775-781` | 直接作为 BI 的 Agent，不新建助手 |
| run 入口 `POST /agent-threads/:threadID/runs`（SSE），权限 `DatabaseRead` + `writeGate`；建会话 `POST /agent-threads` 需 `DatabaseWrite` + `writeGate` | `internal/api/router.go:670, 676` | 不新增路由；**只读角色 / 只读实例无法建会话**，见 Q10 |
| 前端流式层已完备：`useAgentConversation`（phase / 计时 / 重试 / 取消）+ `api.agentThreads.streamRun` | `ui/src/composables/useAgentConversation.ts`、`ui/src/services/http-api-converters.ts:834-857` | 复用，不重写对话层 |
| `streamAgentRun` 只透传 `content / mentions / stream / retry_of_run_id / starter_stage` 五个字段 | `http-api-converters.ts:847-853` | **新字段 `bi_context` 必须加进这里**，否则被静默丢弃 |
| 搭建会话已有同款协议：服务端提示词按阶段追加（`starterPrompt`）+ ProjectEnv 追加 + 末尾 HTML 注释块 `sb:delivery` + 前端「只信结构不信 URL」解析 | `gocloudagent/cloudagent/modules.go:196-262`、`runtime.go:161-190`、`ui/src/utils/starter-delivery.ts` | BI 沿用同一套做法：`biPrompt` + `sb:chart` 块 + 前端严格校验 |
| 技能由 module 决定（`SkillsForModule`），客户端不能传技能；`Writable` 来自助手持久化字段，搭建会话可在运行时覆盖 | `modules.go:187-194`、`agent_run_handler.go:233-238` | BI 运行时强制 `Writable=false`，同一处覆盖 |
| 错误已归一：`llm_not_configured`、`quota_exceeded`；同一 thread 同时只有一个 run（`ClaimThread`） | `agent_run_handler.go:164-185` | 前端复用 `agentErrorText`，并提示 thread 占用冲突 |

## 1. 目标与非目标

**目标**

1. 打开页面即得到「这份数据长什么样」：总量、时间趋势、分类占比、数值分布，零配置。
2. 开发者可自行增删图表、换维度与度量、做简单下钻，配置按「库 + 集合」记住。
3. 图表全部走 ECharts，主题跟随现有 token，亮暗色自适应。
4. 聚合在**服务端**完成，前端不拉全量。
5. 用一句话让数据助手产出图表配置，预览确认后加入看板；助手只出**配置**，数据仍由用户自己的权限经 `aggregate` 接口取。

**非目标**

- 不做行内编辑、不做拖拽布局库（不新增依赖）、不做跨集合 JOIN。
- 不改 `go.mod` / `package.json` / `tsconfig.json` / `vite.config.ts`。
- 不动系统库写保护、不做任何数据库结构变更（无 DDL / 迁移）；看板配置落浏览器 `localStorage`，不入系统库。
- 不改 `TrendChart.vue` 与 Dashboard 页（只新增并行的 composable，不回改，避免连带回归）。
- Agent **不自动写入看板**、**不执行任何写操作**、不新增工具与路由、不新增表；不让助手直接产出 SQL 当作图表数据源。

## 2. 需要修改的代码清单

### 2.1 后端（Go）

| 文件 | 改动 | 说明 |
| --- | --- | --- |
| `godatabase/dialect.go` | `Dialect` 接口新增 `DateBucket(expr, unit string) string` | DuckDB：`strftime(date_trunc(unit, ts), 格式)`；SQLite：`strftime(格式, ts)`；`unit` 白名单 `hour/day/week/month/year`。**godatabase 是独立模块**，须 `GOWORK=off` 编译与测试 |
| `godatabase/dialect_test.go`（或同目录既有测试） | 两引擎的 `DateBucket` 用例 | 含 `week` 在 SQLite 的对齐、非法 unit 拒绝 |
| `internal/api/collection_aggregate.go`（新） | `DataHandler.Aggregate`：取 lease(ReadOnly)、解析、执行、序列化 | 沿用 `ListDocuments` 的 `acquireDatabase` / `h.schema()` / `DialectFor` |
| `internal/api/collection_aggregate_query.go`（新） | 参数解析与 SQL 拼装：`parseAggregateQuery`、`compileMetric`、`compileDimension`、`parsePointsQuery`（散点原始点模式） | 全部走白名单与 schema 校验，**不接受任何用户 SQL 片段** |
| `internal/api/collection_filter.go` | `filterParser.field()` 增加 `created_at` / `updated_at` 作为内置 date 字段（`id` 作为 text） | 时间范围筛选的前提；同时让列表 / 导出 / 聚合三处 filter 同口径。语义对齐 date 字段：`CastTimestamp(created_at)` |
| `internal/api/router.go` | 新增 `p.GET("/databases/:databaseID/data/collections/:collection/aggregate", dh.Aggregate, require(auth.DatabaseRead))`，紧挨 export 路由 | 用 `require` 而非 `delegable`：不扩大云助手可委托集合，避免动 `delegation_route_table_test.go` 的 §3.1 表 |
| `internal/api/data_handler.go` | `DataHandler` 增加 `aggregateTimeout` / `aggregateScan` 字段与 `WithAggregateLimits(timeout, scan)` | 只加字段与方法，聚合逻辑不放进这个 29 KB 的文件 |
| `internal/app/app.go`（装配处，约 `:483`） | `.WithAggregateLimits(...)`，超时取与 `SQLHandler` 相同来源的查询超时 | 不新增配置项；扫描上限先用常量 |
| `gocloudagent/cloudagent/run_types.go` | `RunRequest` 增加 `BI *BIContext`；新增 `BIContext{DatabaseID, Collection, PreferredType, Fields []BIField}` 与 `BIField{Name, Type}` | 无密钥 DTO，与 `StarterStage` 同风格；**gocloudagent 是独立模块**，须 `GOWORK=off` 编译与测试 |
| `gocloudagent/cloudagent/bi.go`（新） | `biPrompt()`、`NormalizeBIContext()`（字段名按 `^[A-Za-z][A-Za-z0-9_]{0,62}$`、类型白名单、字段数 ≤ 64 过滤）、导出 `BIChartTypes` / `BIMetricKinds` 清单 | 提示词与清洗都放模块内，宿主只传原始值；清单导出用于漂移防护（§3.4） |
| `gocloudagent/cloudagent/runtime.go`（`buildInstruction`，约 `:161-190`） | `req.BI != nil` 时在模块层追加 `biPrompt()`，ProjectEnv 追加 `bi_database` / `bi_collection` / `bi_preferred_type` / `bi_fields` | 与 `starterPrompt` 同位置同风格；`bi_context` 不落库，不进 user 消息 |
| `gocloudagent/cloudagent/bi_test.go`（新） | 提示词包含图表清单 / 方向约定 / 度量语法 / 输出格式；`NormalizeBIContext` 丢弃非法字段名、超长截断、非法类型 | 不 import 根模块 |
| `internal/api/agent_run_handler.go` | `createRunBody` 增加 `BIContext *biContextBody \`json:"bi_context"\``；`CreateRun` 校验后写入 `RunRequest.BI`，并强制 `runAgent.Writable = false` | 校验与限权规则见 §3.4 |
| `internal/api/agent_handler.go` | 新增哨兵 `ErrBIContextInvalid`（紧挨 `ErrStarterNotAllowed`，约 `:866-869`） | 遵循「领域错误不裸 `errors.New`」 |
| `internal/api/error.go` | `sentinelErrorMappings` 增加 `ErrBIContextInvalid → 400 bi_context_invalid`（紧挨 `:160-161`） | 聚合侧能复用 `invalidQuery` / `ValidationError` 就不再加哨兵 |
| `internal/api/agent_bi_run_test.go`（新） | 见 §9 | 与既有 `starter_session_test.go` 同风格 |

> 不需要改：`api_key_policy.go`、`policy_gate.go`（路径含 `/data/collections`，会自动解析为 `collection:<name>` 且 GET → read）、`route_meta.go`（`/aggregate` 自动出现在 OpenAPI，`Tryable` 为 true）、`router.go` 的 agent 路由（Agent 部分不新增路由）、`internal/systemdb`（不新增表，不新增内置助手）。

### 2.2 前端（Vue / TS）

| 文件 | 改动 |
| --- | --- |
| `ui/src/pages/DataBrowser.vue` | **整页重写**，只做编排与布局，目标 ≤ 250 行 |
| `ui/src/components/bi/BiToolbar.vue`（新） | 「数据源」库 / 集合下拉、刷新、自动刷新、「数据操作」Popover |
| `ui/src/components/bi/BiAgentPanel.vue`（新） | 顶部 Agent 输入框 + 运行状态 + 待确认的生成结果区（§4.7） |
| `ui/src/components/bi/BiFilterBar.vue`（新） | 时间范围、filter 表达式、联动筛选 chip |
| `ui/src/components/bi/BiKpiRow.vue`（新） | 指标卡行 |
| `ui/src/components/bi/BiWidgetCard.vue`（新） | 单个图表卡：标题、类型切换、三态、更多菜单（编辑 / 复制 / 删除 / 全屏 / 导出）；支持 `preview` 模式（Agent 结果，显示 [加入看板][调整][丢弃]） |
| `ui/src/components/bi/BiWidgetEditor.vue`（新） | 图表配置弹窗（`SbModal`） |
| `ui/src/components/bi/BiChart.vue`（新） | ECharts 容器：init / resize / 主题监听 / dispose / 点击事件透传 |
| `ui/src/components/bi/BiDetailTable.vue`（新） | 只读明细表 + 分页 + 列显隐 + 下钻后的结果 |
| `ui/src/composables/useBiDashboard.ts`（新） | 看板状态：widgets、布局、`localStorage` 持久化、自动生成默认看板 |
| `ui/src/composables/useBiQuery.ts`（新） | 单个聚合请求的生命周期（pending / error / 竞态令牌 / 并发上限） |
| `ui/src/composables/useBiAgent.ts`（新） | Agent 会话与运行编排：定位数据助手、会话读 / 建 / 重建、发送、取消、重试、解析结果、epoch 令牌丢弃过期回调（§4.7） |
| `ui/src/utils/bi-chart.ts`（新） | **纯函数**：`AggregateResult` + `BiWidget` → `EChartsOption`；时间桶补零；Top N 合并「其他」；直方图分箱 |
| `ui/src/utils/bi-defaults.ts`（新） | 按 schema 字段类型生成默认 widget 列表 |
| `ui/src/utils/bi-auto.ts`（新） | 「自动」类型推断：入参为字段类型、维度形态、度量、上次结果类目数，出参为具体 `BiChartType`（规则见 §4.3.1） |
| `ui/src/utils/bi-compat.ts`（新） | **纯函数**：`BiWidget` 配置 × 类型 → `{ ok, reason }`。类型选择器置灰、编辑器校验、Agent 结果校验**共用这一份**，保证三处口径一致 |
| `ui/src/utils/bi-agent-block.ts`（新） | `parseChartBlocks` / `stripChartBlocks` / `validateAgentWidget`，对标 `starter-delivery.ts`（§4.7） |
| `ui/src/components/bi/BiChartTypePicker.vue`（新） | 图表类型选择器：`自动` + 8 种类型，用 `ToggleGroup` 单选；不兼容当前字段配置的类型置灰并带原因提示 |
| `ui/src/services/types.ts` | 新增 `AggregateQuery` / `AggregateResult` / `BiWidget` / `BiDashboardConfig` / `BiAgentContext` / `BiAgentField`（均用 `interface`），`AgentRunRequest` 增加可选 `bi_context`，并在 `db` 契约里加 `aggregate` |
| `ui/src/services/http-api.ts` | `db.aggregate`：路径复用 `dataCollectionsPath(...) + '/aggregate'`，snake_case → camelCase 映射函数 `toAggregateResult` |
| `ui/src/services/http-api-converters.ts` | `streamAgentRun` 的请求体增加 `...(req.bi_context ? { bi_context: req.bi_context } : {})`（`:847-853`） |
| `ui/src/composables/useAgentConversation.ts` | `start()` 的 `body` 类型增加可选 `bi_context`，原样透传给 `streamRun` |
| `ui/src/utils/agent-errors.ts` | 增加错误码 `bi_context_invalid` → `error.bi_context_invalid` |
| `ui/src/test/api-mock.ts` | `api.db.aggregate: vi.fn()`，并在 `resetApiMocks` 里给默认值 |
| `ui/src/test/setup.ts` | **必改**：`echarts/charts` 增补 `PieChart` `ScatterChart`，`echarts/components` 增补 `LegendComponent` `DatasetComponent` `DataZoomComponent` `ToolboxComponent`（见 §4.6，否则 mock 缺导出会让用例直接抛错） |
| `ui/src/locales/zh-CN.ts` / `en-US.ts` | 重写 `pages.dataBrowser.*`，新增 `components.bi.*`（含 `components.bi.agent.*`）与 `error.bi_context_invalid`，两树 key 必须一致 |
| `ui/tests/DataBrowser.test.ts` | 重写；新增 `ui/tests/bi-chart.test.ts`、`bi-auto.test.ts`、`bi-compat.test.ts`、`bi-defaults.test.ts`、`bi-agent-block.test.ts`、`useBiAgent.test.ts`、`BiAgentPanel.test.ts`、`BiChartTypePicker.test.ts`、`BiWidgetEditor.test.ts`、`useBiDashboard.test.ts`，`http-api.test.ts`（补 `aggregate` 与 `bi_context` 透传） |

### 2.3 文档

- `docs/data/index.md:30` 的「数据浏览」描述改为看板能力（须过 `ui/tests/docs-content.test.ts` 的链接 / 锚点 / frontmatter 校验）。
- `docs/data/` 增补聚合 API 小节（参数、示例、限制）。
- `docs/workbench/agent.md` 增补「在数据看板里用数据助手生成图表」一节（入口、权限要求、结果需确认）。
- `README.md:21`、`ui/README.md:54`、`ui/AGENTS.md`（目录职责表加入 `components/bi/`）同步。
- 页面命名是否改为「数据看板」见 §8 待确认项。

## 3. 后端设计

### 3.1 聚合接口

```
GET /v1/projects/:projectID/databases/:databaseID/data/collections/:collection/aggregate
权限：require(auth.DatabaseRead)；只读 lease；不挂 writeGate
```

| 参数 | 说明 | 约束 |
| --- | --- | --- |
| `metrics` | 逗号分隔：`count`、`sum:f`、`avg:f`、`min:f`、`max:f`、`distinct:f` | 1–6 个；`sum/avg` 仅 number 字段；`min/max` 仅 number/date；`distinct` 非 json 字段 |
| `group_by` | 主维度：`field`、`field:day`（时间桶）、`field:bin=10`（数值分箱） | 可选；`field` 为已登记字段或 `created_at` / `updated_at`；时间桶仅 date 类；分箱仅 number 类 |
| `series` | 次维度（堆叠 / 多折线） | 可选；仅普通字段，不允许时间桶 |
| `filter` | 复用现有 filter 语法 | 复用 `compileFilter`，长度 / `;` / NUL 校验不变 |
| `sort` | 度量别名或维度，`-` 前缀降序 | 只允许出现在本次 `metrics` / 维度里的别名，默认主维度升序（时间桶）或第一个度量降序 |
| `limit` | 返回行数 | 1–500，默认 100；超出返回 `truncated: true` |

度量别名固定生成：`count`、`sum_<f>`、`avg_<f>`、`min_<f>`、`max_<f>`、`distinct_<f>`。

**散点图的原始点模式**（`mode=points`，同一个端点，不聚合）

| 参数 | 说明 | 约束 |
| --- | --- | --- |
| `mode` | 固定 `points`；缺省为聚合模式 | 与 `metrics` / `group_by` 互斥，同传返回 400 |
| `x` / `y` | 两个 number 字段 | 必填；必须是已登记的 number 字段，`NULL` 行在 SQL 里过滤掉 |
| `size` | 点大小字段 | 可选；number 字段 |
| `series` | 颜色分组字段 | 可选；text / select / bool 字段；分组数上限 12，超出的归入「其他」 |
| `filter` | 同聚合模式 | 同 |
| `limit` | 点数 | 1–2000，默认 1000；`ORDER BY` 固定为 `created_at DESC, id DESC`，保证同参数结果稳定 |

响应列固定为 `x` / `y` / `size`（可选）/ `series`（可选），`truncated` 表示总点数超过 `limit`。取最新 N 个点而非随机抽样，目的是结果可复现；若后续需要均匀抽样，再单独评审。

**响应**

```json
{
  "columns": ["x", "series", "count", "sum_amount"],
  "rows": [["2026-11-01", "paid", 12, 3400.5]],
  "truncated": false,
  "scanned": 8231,
  "duration_ms": 14,
  "request_id": "..."
}
```

行值经 `godatabase.SerializeRows`（与 `/query` 同一条序列化路径）输出，避免 DuckDB 时间 / Decimal 类型直出出错。主维度列固定叫 `x`，次维度列固定叫 `series`，前端不用猜列名。

### 3.2 SQL 生成与安全

- 所有标识符来自 **schema 登记字段**，再经 `fieldNamePattern` 校验；表达式只由 `Dialect.JSONExtract*` / `CastTimestamp` / `DateBucket` 与固定的 `COUNT/SUM/AVG/MIN/MAX/COUNT(DISTINCT …)` 模板拼出。没有任何一处拼接用户原文。
- 数值分箱步长 `bin` 以**参数绑定**传入（`FLOOR(x / ?) * ?`），且限制 `> 0`、有限数。
- 未登记 schema 的集合：只放行 `count`、`group_by=created_at:<unit>`；其余返回既有 `schema_not_registered`（400）。前端据此提示「去 Schema 设计器登记字段」。
- 扫描保护：带 filter 或不带都先 `SELECT COUNT(*)`，超过 `aggregateScan`（先定常量 100 万）返回 `filter_scan_limit_exceeded`（复用 `scanLimitError`），提示缩小时间范围。
- 超时：`context.WithTimeout`，到时按既有错误映射返回，不留悬挂查询。
- 系统库：与 `ListDocuments` 同口径，只读允许。新增用例确认聚合走 `godatabase.ReadOnly` lease，不会触发写路径。
- 日志 / 审计：只记元数据，不记 filter 参数值（遵守「不输出 SQL 参数」）。

### 3.3 为什么用 GET

策略网关对非 GET 一律判 `write`（`policy_gate.go:45-48`）。若做成 `POST /aggregate` 批量接口，持只读 Key 的使用者会被拒。因此：单次聚合用 GET；看板打开时前端并发 ≤ 4 个 GET，不做服务端批量。若后续确需批量，同步把 `/aggregate` 加入 `policyGate` 的读动作判定，再单独评审。

### 3.4 Agent 生成图表（后端）

复用内置 `database` 助手与现有 run 链路，**不新增路由、不新增工具、不新增表**。

**请求**：`POST /agent-threads/:threadID/runs` 的请求体新增可选字段 `bi_context`。

```json
{
  "content": "近 30 天各状态订单的金额，用堆积条形图",
  "mentions": [{ "agent_id": "<database 助手 id>" }],
  "stream": true,
  "bi_context": {
    "database_id": "<uuid>",
    "collection": "orders",
    "preferred_type": "stacked_bar",
    "fields": [
      { "name": "amount", "type": "number" },
      { "name": "status", "type": "select" },
      { "name": "created_at", "type": "date" }
    ]
  }
}
```

**校验（`CreateRun` 内，失败统一返回 400 `bi_context_invalid`）**

| 规则 | 说明 |
| --- | --- |
| 与搭建会话互斥 | `bi_context` 与 `starter_stage` / 搭建会话（`thread.StarterTemplate != ""`）不能同时出现 |
| 助手 module | 仅 `database` 与 `general`；其余 module 拒绝 |
| `database_id` | 必须是合法 UUID（对象与内部标识一律 UUID） |
| `collection` | 必须符合 `fieldNamePattern` |
| `preferred_type` | 缺省或 `auto` 表示不限；否则必须在 `BIChartTypes` 内 |
| `fields` | 逐项经 `NormalizeBIContext`：名称符合 `fieldNamePattern`、类型在字段类型白名单内，不合规项**丢弃**，总数截断到 64；`database_id` / `collection` 不合规则整体拒绝 |

**限权与注入**

- `runAgent.Writable = false` **强制**（即使该助手被用户设为可写）：BI 生成永远不写。CLI 技能模式下写命令在确认之前就被 `write_not_allowed` 拒绝。覆盖点与搭建会话覆盖 `Writable=true` 是同一处（`agent_run_handler.go:233-238`）。
- 技能仍由 `SkillsForModule(agent.Module)` 决定，不接受客户端传技能；权限仍是 `DatabaseRead` + `writeGate`，委托令牌仍是调用者权限的子集，不放大。
- `bi_context` **不落库**：不写进 user 消息，不写进系统库；只用于本轮提示词与 ProjectEnv。重试（`retry_of_run_id`）时前端重新携带。
- 日志与审计：不记录 `fields` 与提示词正文，沿用既有 `agent.trace_log` 的脱敏边界。
- `ProjectEnv` 追加的字段名已经过正则过滤，不会成为提示词注入通道。

**提示词要点（`biPrompt`，模块层，与 `starterPrompt` 同位置追加）**

1. 目标：围绕 env 指定的**单个集合**产出 1–4 个图表配置。
2. 只可使用 env 列出的字段；`created_at` / `updated_at` 是 date 字段，可用于时间桶；不得编造字段。
3. 可用只读工具抽样确认取值分布，**禁止写入**。
4. 图表类型白名单与方向约定：`line` `column`（纵向）`bar`（横向）`pie` `table` `grouped_bar`（横向）`stacked_bar`（横向）`scatter`；`auto` 表示交给前端推断；KPI 指标卡用「无 `group_by` + 1 个度量」表达，`type` 写 `auto`。
5. 度量与聚合语法同 §3.1：`sum` / `avg` 仅 number 字段；`stacked_bar` 只能用 `count` / `sum`；`scatter` 需要两个 number 字段 `x` / `y`，且不带 `metrics` / `group_by`。
6. 输出格式：先给一段简短说明（用用户的语言），再在末尾为每个图表输出**一行** `<!-- sb:chart {单行 JSON} -->`；不输出 `id` / 布局；`title` ≤ 40 字。
7. 数据不适合或字段不存在时，只说明原因，不输出块。
8. 配置里没有 SQL 字段，不输出 SQL 充当数据源；不输出任何密钥。

**`sb:chart` 块协议**

| key | 类型 | 说明 |
| --- | --- | --- |
| `version` | number | 固定 `1` |
| `title` | string | 图表标题，纯文本 |
| `type` | string | `auto` 或 8 种具体类型之一 |
| `metrics` | string[] | 同 §3.1，聚合模式必填；散点不填 |
| `group_by` | string | 同 §3.1 |
| `series` | string | 同 §3.1 |
| `x` / `y` / `size` | string | 仅散点 |
| `filter` | string | 同全局 filter 语法，与看板全局条件取 AND |
| `sort` | string | 同 §3.1 |
| `limit` | number | 前端夹到后端允许范围 |
| `area` / `donut` | boolean | 折线面积 / 饼图环形 |

限制：单块 ≤ 4 KB；单条消息最多取 4 块；未知 key 忽略；JSON 解析失败的块跳过。

**漂移防护**：`cloudagent` 导出 `BIChartTypes`、`BIMetricKinds`。`internal/api` 增加一条测试，遍历 `BIMetricKinds` 逐项喂给 `parseAggregateQuery`，保证提示词承诺的语法服务端都接受。图表类型清单前后端各一份，各自用快照测试固定，变更时同步（见 §8.2）。

## 4. 前端设计

### 4.1 页面布局

布局对齐产品原型：顶部从上到下依次为「数据源」、「图表类型」、「Agent 输入框」。

```
┌ PageContainer subtitle（单行，不加 h2）────────────────────────────────┐
│ 数据源 [数据库 ▾] / [集合 ▾]                       [刷新][自动▾][数据操作▾] │  BiToolbar
│ 图表类型 [自动][折线图][柱状图][条形图][饼图][表格][对比条形图][堆积条形图][散点图] │  BiChartTypePicker
│ ┌ 输入图表搭建需求，发送后由 Agent 生成 ──────────────────────────── ➤ ┐ │  BiAgentPanel
│ │ （运行状态 / 生成结果预览卡，每张带 [加入看板][调整][丢弃]）          │ │
│ └──────────────────────────────────────────────────────────────────┘ │
│ [时间字段 ▾][近1h|24h|7d|30d|全部]  filter: [ status="open" ] [应用] ⟨status=open ✕⟩ │  BiFilterBar
├────────────────────────────────────────────────────────────────────────┤
│ ┌ 总记录 ┐ ┌ 今日新增 ┐ ┌ 金额合计 ┐ ┌ 平均金额 ┐                         │  BiKpiRow
├────────────────────────────────────────────────────────────────────────┤
│ ┌ 记录趋势（折线/面积，按天）──────────────┐ ┌ status 分布（饼图/环形）─┐ │
│ │                                          │ │                          │ │  BiWidgetCard 栅格
│ └──────────────────────────────────────────┘ └──────────────────────────┘ │
│ ┌ priority 分布（柱状图+分箱）┐ ┌ category Top10（条形图）┐ ┌ + 添加图表 ┐ │
├────────────────────────────────────────────────────────────────────────┤
│ 明细（只读表）  列显隐▾                                          分页   │  BiDetailTable
└────────────────────────────────────────────────────────────────────────┘
```

「数据源」只有一个 ready 用户库时隐藏库下拉（每个项目只有一个用户库）；多库（如 admin 项目）时两个下拉并排。**实现已扩展**：SQL 库（`dataModel === 'sql'`）也进入看板，读取真实表 / 列，使用表聚合与表明细接口；Agent 上下文通过 `source: 'table'` 区分来源。没有时间列时隐藏时间范围，不伪造 `created_at`。集合与表配置按来源类型隔离；这不等于 F21 的任意只读 SELECT 数据集。上述布局图仍是原型，当前没有独立 `BiKpiRow`，后续布局调整见 §12.3。

### 4.2 状态与数据流

1. 保留原有三个竞态计数器思路（库 → 集合 → 数据），抽到 `useBiDashboard`。
2. 选定集合后：取 schema → 读 `localStorage` 配置 → 无配置则 `bi-defaults.ts` 生成默认 widgets。
3. 每个 widget 持有独立的 `useBiQuery`：请求带「全局 filter + 时间范围 + 联动 chip」合成的 `filter`；自带令牌防旧响应覆盖；全局并发上限 4。
4. 联动筛选：点击柱 / 扇区 / 折线点 → 追加 `field="value"`（或时间桶区间）为 chip → 所有 widget 与明细表按新 filter 重取；chip 可单独移除。
5. 持久化键：`sb_bi_v1:<projectId>:<databaseId>:<collection>`；字段已不存在的 widget 加载时丢弃并 toast 提示。
6. 切换项目 / 库 / 集合：清空 widgets、chip 与 Agent 待确认区，作废在途请求（沿用原 `watch` 链）。

### 4.3 图表类型（ECharts 映射）

图表类型选择器固定为 **9 项**（与产品原型一致）：`自动` + 8 种具体类型。类型值用字符串联合别名 `BiChartType`：`auto | line | column | bar | pie | table | grouped_bar | stacked_bar | scatter`。

| 选项 | 值 | 方向 | ECharts 实现 | 数据来源 | 备注 |
| --- | --- | --- | --- | --- | --- |
| 自动 | `auto` | 随解析结果 | 由 §4.3.1 规则在运行时解析为下列某一种 | 按所选字段类型与基数推断 | 默认选项；编辑器里显示「已推断为：xx」，点其他类型即固定 |
| 折线图 | `line` | 横轴为类目 / 时间 | `line`，`smooth: 0.25`；可选「面积填充」（`areaStyle`）；有 `series` 时多折线 | 主维度为时间桶（`created_at:day` 等）或有序分类，1–3 个度量 | 面积不单列类型，作为折线图的样式开关 |
| 柱状图 | `column` | **纵向** | `bar`，`xAxis` 为类目轴、`yAxis` 为数值轴 | 分类 / 时间桶 / 数值分箱维度 + 1 个度量 | **直方图并入这里**：维度选数值字段并设分箱步长即为直方图 |
| 条形图 | `bar` | **横向** | `bar`，`yAxis` 为类目轴、`xAxis` 为数值轴，按度量降序 | 分类维度 + 1 个度量，Top N | 类目名较长或类目多时优先 |
| 饼图 | `pie` | — | `pie`，可选「环形」（`radius: ['55%','75%']`） | 分类维度 + 1 个度量，Top N 之外合并「其他」 | 类目数建议 ≤ 8，超出时编辑器提示改用条形图 |
| 表格 | `table` | — | 非 ECharts，复用 `ui/table` | 两种模式：**聚合**（维度 + 多度量，走 `aggregate`）与**明细**（选字段列，走 `documents`） | 兜底类型；支持分页与列显隐 |
| 对比条形图 | `grouped_bar` | **横向** | `bar`，`yAxis` 为类目轴，多个 series 并排（`barGap: '10%'`，不设 `stack`） | 分类维度 + 2–3 个度量，**或**分类维度 + `series` 次维度 + 1 个度量 | 用于同一类目下多指标 / 多分组的横向对比 |
| 堆积条形图 | `stacked_bar` | **横向** | `bar`，`yAxis` 为类目轴，所有 series 同一个 `stack: 'total'` | 分类维度 + `series` 次维度 + 1 个可累加度量（`count` / `sum`） | `avg` / `min` / `max` / `distinct` 不可累加，编辑器禁用并提示 |
| 散点图 | `scatter` | — | `scatter`，`symbolSize` 固定或映射第三个数值字段；`series` 对应颜色分组 | **原始点**：X（number）、Y（number）、可选大小字段、可选分组字段；经 §3.1 的 `mode=points` | 不聚合，服务端限量取最新点，超出返回 `truncated` |

KPI 指标卡**不属于**图表类型选择器：它是页面固定的 `BiKpiRow`，同时也是「自动」在「无维度 + 1 个度量」时的退化形式。漏斗、仪表盘、热力图不在本期范围。

**方向约定（已确认）**：「柱状图」纵向；「条形图」「对比条形图」「堆积条形图」横向。实现上，横向图用 `yAxis: { type: 'category', inverse: true }` 让第一项排在最上面，`xAxis` 为数值轴；`bi-chart.test.ts` 固定断言各类型的轴类型，防止回退。

#### 4.3.1 「自动」推断规则（`utils/bi-auto.ts`，纯函数）

按从上到下第一条命中的规则：

| # | 条件 | 解析为 |
| --- | --- | --- |
| 1 | 选了两个 number 字段做 X / Y，且未选度量 | `scatter` |
| 2 | 无维度，仅 1 个度量 | KPI 指标卡 |
| 3 | 主维度为时间桶 | `line` |
| 4 | 主维度为数值分箱 | `column` |
| 5 | 主维度为分类 + `series` + 1 个可累加度量 | `stacked_bar` |
| 6 | 主维度为分类 + `series` + 1 个不可累加度量，或分类 + 2–3 个度量 | `grouped_bar` |
| 7 | 主维度为分类 + 1 度量，类目数 ≤ 6 且 `select` / `bool` 类型 | `pie` |
| 8 | 主维度为分类 + 1 度量，类目数 ≤ 12 且标签平均长度 ≤ 8 | `column` |
| 9 | 主维度为分类 + 1 度量，其余情况 | `bar`（Top 10 / 20） |
| 10 | 度量数 > 3、或无法判断 | `table` |

「类目数」取自上一次聚合结果；首次请求前按字段类型预判，结果回来后若与预判不符会自动重新解析，不重复请求。

### 4.4 默认看板生成规则（`bi-defaults.ts`）

| 条件 | 生成 widget |
| --- | --- |
| 恒有 | KPI「总记录」`count`；折线「记录趋势」`created_at:day` + `count` |
| 有 number 字段（取前 2 个）| KPI `sum` / `avg`；柱状图 + 数值分箱（直方图） |
| 有 select / bool 字段（取前 2 个）| 饼图（环形），Top 8 |
| 有 text / email / url 字段（取前 1 个）| 条形图 Top 10（按 count 降序）|
| 有 date 字段（非 `created_at`，取前 1 个）| 折线 `date:day` |
| schema 未登记 | 仅「总记录」与「记录趋势」，另给「登记 Schema」提示卡 |
| 总数上限 | 8 个，避免首屏请求风暴 |

### 4.5 组件拆分约束

- 页面只做编排；每个 `.vue` 控制在 300 行内，逻辑放 composable 与 `utils/bi-chart.ts`（纯函数便于覆盖率）。
- ECharts 初始化逻辑只在 `BiChart.vue` 一处，其余组件不直接碰 `echarts`。
- 新增 composable 在组件外取文案用 `@/locales` 的 `t()`，不调 `useI18n()`。
- 不使用 `any`；对象形状全用 `interface`；`BiWidget` 的图表类型用字符串联合别名即可（非对象形状）。

### 4.6 ECharts 按需注册与测试 mock

`BiChart.vue` 需要注册（仅在该文件 `echarts.use([...])`）：

- charts：`BarChart` `LineChart` `PieChart` `ScatterChart`
- components：`GridComponent` `TooltipComponent` `LegendComponent` `DatasetComponent` `DataZoomComponent` `ToolboxComponent`（`saveAsImage` 导出 PNG）
- renderer：`CanvasRenderer`

`ui/src/test/setup.ts:80-82` 现在把这三个模块 mock 成固定导出集合，新增名字不补进去，vitest 会报「mock 上没有该导出」。所以 `setup.ts` 必须同步扩充；`BiChart` 的断言打在 `setOption` 收到的 option 上，与 `TrendChart.test.ts` 同一做法。

### 4.7 Agent 生成图表（前端）

**流程**

1. **入口**：`BiAgentPanel`，数据源已选且 schema 已加载时可用。禁用条件与原因提示：无集合；`isAdmin`（系统项目）；`!auth.canWrite`；实例只读（建会话需 `database:write` + `writeGate`，见 Q10）。视图集合可用（字段来自其 schema）。
2. **定位助手**：`api.agents.list` 取 `builtin_key === 'database'` 的助手；缺失（被用户删除）时面板提示「数据助手不存在，请到云助手页恢复」，**不自动创建**。
3. **会话**：每个「项目 + 库 + 集合」一个会话，id 存 `localStorage`（`sb_bi_thread_v1:<projectId>:<databaseId>:<collection>`）；读到 404 就重建（`agentThreads.create(projectId, 'BI · <集合名>', agentId)`）；面板提供「新对话」按钮强制重建。
4. **发送**：Enter 发送、Shift+Enter 换行。调用 `useAgentConversation.start({ content, mentions: [{ agent_id }], module: 'database', bi_context })`。`bi_context.fields` 来自已加载 schema（`hidden` 字段不传，另补 `created_at` / `updated_at` 两个 date 字段）；`preferred_type` 取类型选择器当前值，`auto` 不传。
5. **运行状态**：复用 `statusText` / phase；运行中发送按钮变「停止」（`cancel`）；失败用 `agentErrorText` 展示（`llm_not_configured` / `quota_exceeded` / `bi_context_invalid` 等），带「重试」（`retry_of_run_id`，重新携带 `bi_context`）。
6. **解析**：run 结束后 `parseChartBlocks(content)` 取至多 4 块 → `validateAgentWidget`（与类型选择器同一份 `bi-compat.ts`）→ 有效的转成 `BiWidget`（分配 `id`，`span` 按类型取默认值）进「待确认区」；无效的在原位显示原因，**不进入看板**。去掉块后的说明文字用 `AgentMarkdown` 渲染。
7. **预览**：待确认区的每个 widget 用 `BiWidgetCard` 的 `preview` 模式**真实查询渲染**（走 `aggregate`，权限是用户自己），带全局时间范围与联动条件，与看板内 widget 同口径。按钮：[加入看板]（才写入 `useBiDashboard` 与 `localStorage`）、[调整]（打开 `BiWidgetEditor` 预填）、[丢弃]。
8. **追问与修正**：同一会话支持多轮（「改成条形图」「再加一个按状态的占比」）；预览查询失败（如 filter 被后端拒绝）时卡片进入错误态，提供「让助手修正」——自动发送一条携带后端错误文本的跟进消息。
9. **前端只信结构**（对标 `starter-delivery.ts`）：`title` 纯文本渲染，不用 `v-html`；所有字段名必须在 schema 内；`filter` 前端不解析，原样交后端 `compileFilter`；`limit` 夹到后端范围；未知 key 丢弃；超长 / 非法 JSON 的块丢弃。
10. **并发与切换**：切项目 / 库 / 集合时用 epoch 令牌丢弃过期回调（同 `useStarterRun`）；同一 thread 同时只有一个 run，若与云助手页正在使用的同一会话冲突，提示「该会话正在运行」。

**`utils/bi-agent-block.ts`**

- `parseChartBlocks(content): AgentChartBlock[]`：至多 4 块、单块 ≤ 4 KB、JSON 失败跳过。
- `stripChartBlocks(content): string`：去掉块注释，剩余作为 Markdown 展示。
- `validateAgentWidget(block, schema): { ok: true; widget: BiWidget } | { ok: false; reason: string }`。

### 4.8 `BiWidget` 数据结构（`services/types.ts`）

```ts
export type BiChartType =
  | 'auto' | 'line' | 'column' | 'bar' | 'pie'
  | 'table' | 'grouped_bar' | 'stacked_bar' | 'scatter'

export interface BiWidget {
  id: string
  title: string
  type: BiChartType
  /** 聚合模式的度量，如 ['count', 'sum:amount']；散点为空 */
  metrics: string[]
  groupBy?: string
  series?: string
  /** 仅散点 */
  x?: string
  y?: string
  size?: string
  filter?: string
  sort?: string
  limit?: number
  /** 折线面积 / 饼图环形 */
  area?: boolean
  donut?: boolean
  /** 栅格宽度档位 */
  span: 4 | 6 | 8 | 12
}

export interface BiAgentField {
  name: string
  type: CollectionField['type']
}

export interface BiAgentContext {
  database_id: string
  collection: string
  preferred_type?: BiChartType
  fields: BiAgentField[]
}
```

`sb:chart` 块的 snake_case 键（`group_by`）在 `validateAgentWidget` 内映射为 camelCase（`groupBy`），`id` 与 `span` 由前端分配，块里不接受。

## 5. 样式清单

遵循 `ui/AGENTS.md`，不写自定义 CSS 文件，全部 Tailwind class。

| 项 | 规范 |
| --- | --- |
| 页面外壳 | `ProjectScope` + `PageContainer`，仅 subtitle 单行，铺满全宽（不加 `max-w-*`），内边距只走 `--page-pad` |
| 卡片 | `Card` / `CardContent`，`p-4 sm:p-5`，与现有页面一致 |
| 栅格 | `grid grid-cols-1 gap-4 sm:gap-5 md:grid-cols-12`；widget 宽度 `md:col-span-4/6/8/12` 四档，窄屏一律单列 |
| 图表高度 | 折线 / 柱 / 条 / 饼 / 散点统一 `h-64 sm:h-72`；横向条形图类目多时按类目数增高（每类目约 28px，封顶 `h-[28rem]`）；KPI 卡高度自适应；全屏弹窗 `h-[70svh]` |
| 色板 | 读 `--chart-1..5`；系列超过 5 个时在前 5 色上按 0.65 / 0.4 透明度循环，不再引入新的色值字面量 |
| 暗色 | `BiChart` 监听 `<html class>`（同 `TrendChart.vue:271`），重算 option；tooltip 背景 `--card`、边框 `--border`、文字 `--foreground` |
| 文字 | 轴标签 `12px`，颜色 `--muted-foreground`；数值用 `tabular-nums`；长数字走 `compact`（1.2k / 3.4M） |
| tooltip | 与 TrendChart 同款：圆角 8、`padding [8,12]`、`valueFormatter` 用 `toLocaleString()` |
| 三态 | 一律 `SbAsyncRegion`（`block="chart"` 骨架）；错误只用 `SbErrorState`，不手写 Alert |
| 空态 | `SbEmptyState`，重试 / 登记 Schema 显式传 `actionIcon`（不用默认 `PlusIcon`） |
| 交互组件 | 时间范围与图表类型用 `ToggleGroup`；下拉用 `ui/select`；弹窗用 `SbModal`（`maxWidth` 通道）；确认用 `ConfirmAction`；提示用 `toast` |
| 图表类型 chips | 对齐原型：小号描边按钮，选中态主色底白字；窄屏 `flex-wrap`；不兼容项 `data-disabled` 置灰并带 tooltip 原因 |
| Agent 输入框 | 多行 `Textarea`（`min-h` 单行高度，随内容增高，最高 4 行），右下角圆形发送按钮（`SendIcon`），运行中变停止按钮；边框沿用输入框 token，聚焦环用 `ring-3` 既有写法；占位文案走 i18n |
| Agent 结果区 | 输入框下方折叠区：说明文字用 `AgentMarkdown`，生成的预览卡用 `BiWidgetCard`（`preview`）；卡片右上角三个按钮（加入看板 / 调整 / 丢弃）；加载、空、错误同样走 `SbAsyncRegion` |
| 状态类 | 只写 `data-[state=…]:`，不写裸 `data-checked` 等 |
| 弹窗内并排行 | 父级 `flex flex-wrap`，主输入 `min-w-0 flex-1`（`ui/tests/modal-input-layout.test.ts` 会卡） |
| 图标 | `@lucide/vue`，按钮内图标加 `data-icon="inline-start"` |
| 不引入 | 已删除的 `dropdown-menu` 等组件不得重新引入；「数据操作」与 widget 的「更多」菜单用 `Popover` 内放按钮列表实现 |

## 6. 功能清单

### P0（MVP，一期）

| # | 功能 | 说明 |
| --- | --- | --- |
| F1 | 数据源选择 | 「数据源」标签后接库 / 集合两个下拉（只有一个 ready 库时隐藏库下拉，替代侧栏）；保留旧页竞态防护与「集合文档数 / VIEW」标识 |
| F2 | 默认看板 | 按 §4.4 规则自动生成，零配置可看 |
| F3 | KPI 行 | 总记录、新增、sum / avg（按字段） |
| F4 | 图表类型 | 选择器 9 项：自动、折线图、柱状图（纵向）、条形图（横向）、饼图、表格、对比条形图（横向）、堆积条形图（横向）、散点图（映射与规则见 §4.3；面积、环形、直方图是对应类型的样式 / 分箱选项，不单列） |
| F4a | 自动类型 | 「自动」按 §4.3.1 规则推断，选择器内显示推断结果；用户点选具体类型后固定，不再自动变更 |
| F4b | 类型兼容校验 | 当前字段配置不满足某类型时置灰并给原因（如堆积条形图要求可累加度量、散点图要求两个 number 字段）；与 Agent 结果校验共用 `bi-compat.ts` |
| F5 | 时间范围 | 近 1h / 24h / 7d / 30d / 全部，作用于所选时间字段（默认 `created_at`），转成 `filter` |
| F6 | 全局 filter | 沿用现有语法，保留 Enter 提交与最近 5 条历史；字段名快捷追加改为字段 chip |
| F7 | 联动下钻 | 点击图元追加筛选 chip，可单独移除，所有 widget 与明细表同步刷新 |
| F8 | 明细表 | 只读、分页（`TablePager`）、列显隐；随全局 filter 变化；不含编辑 |
| F9 | 图表编辑 | 添加 / 编辑 / 复制 / 删除 widget；类型、维度、度量、系列、Top N、排序、宽度 |
| F10 | 配置持久化 | `localStorage` 按「项目 + 库 + 集合」记忆；「恢复默认」按钮 |
| F11 | 数据操作 | Schema 设计器、新建视图、导入、导出 CSV / JSON（原有能力保留） |
| F12 | 刷新 | 手动刷新 + 自动刷新（关 / 30s / 1m / 5m），页面隐藏时暂停 |
| F13 | 权限与只读 | 看板为只读视图；`isAdmin` / `!canWrite` / 视图集合时隐藏「Schema / 导入 / 新建视图」 |
| F14 | 空与错 | 无库 / 无集合 / 未登记 Schema / 聚合超限 / 加载失败，各有明确提示 |

### P0-Agent（一期，复用现有数据助手）

| # | 功能 | 说明 |
| --- | --- | --- |
| A1 | Agent 输入框 | 原型顶部输入框，占位文案「输入图表搭建需求，发送后由 Agent 生成」；Enter 发送、Shift+Enter 换行、发送按钮；运行中变「停止」 |
| A2 | 类型偏好 | 类型选择器非「自动」时作为 `preferred_type` 传给助手；「自动」由助手决定，缺省时前端按 `bi-auto` 推断 |
| A3 | 数据源约束 | 助手只对当前「数据源」出图；字段清单随 schema 传入，助手不得使用清单外字段 |
| A4 | 结果预览与确认 | 助手产出的每个图表先在待确认区**真实查询预览**，用户点「加入看板」才写入；可「调整」后再加入或「丢弃」；一次最多 4 个 |
| A5 | 追问与修正 | 同一会话多轮追问；预览失败时「让助手修正」自动带上后端错误文本跟进 |
| A6 | 新对话 | 一键重建会话，清空上下文与待确认区 |
| A7 | 状态与错误 | 思考 / 调用工具 / 生成中状态、取消、重试；`llm_not_configured` / `quota_exceeded` / 运行失败各有提示（复用 `agentErrorText`） |
| A8 | 权限与禁用 | 系统项目、只读角色、只读实例禁用并说明原因；数据助手被删除时提示恢复 |
| A9 | 安全边界 | 助手运行强制只读；块里只有配置，没有 SQL；前端只信结构；`bi_context` 不落库 |

### P1（二期）

| # | 功能 |
| --- | --- |
| F15 | 单图导出 PNG（ECharts `saveAsImage`）与 CSV（由聚合结果客户端生成） |
| F16 | 图表全屏查看 |
| F17 | 看板配置导出 / 导入 JSON（跨环境复用） |
| F18 | 布局调整：左右移动 + 宽度档位（不引入拖拽库；原生 DnD 作为可选） |
| F19 | KPI 环比（与上一个同长度时间窗对比，需额外一次聚合） |
| F20 | 时间桶粒度切换（小时 / 天 / 周 / 月）与缩放 `DataZoom` |
| F25 | Agent 对「已有 widget」的修改（「把第二个图改成条形图」直接指向 widget id，而不是新增） |

### P2（三期，需单独评审）

| # | 功能 |
| --- | --- |
| F21 | **SQL 数据集**：SQL 库与集合库都可写只读 `SELECT`，经现有 `POST /query` 出结果，列映射到图表；复用 `sqlguard.ReadOnly` 与 `max_rows` |
| F22 | 漏斗、仪表盘、热力图（散点图已提前到一期） |
| F23 | 看板服务端保存与项目内共享（涉及存储位置与迁移，**按约定不自动执行 DDL，需用户手动执行**） |
| F24 | `packages/js-sdk` 增加 `aggregate` 方法与文档 |
| F26 | 只读角色的 Agent 通道（需要一条不需要建会话的只读 run 入口，或放宽建会话权限，涉及权限模型，需单独评审，见 Q10） |

## 7. 分期与验收

| 阶段 | 内容 | 依赖 | 预估 |
| --- | --- | --- | --- |
| S1 后端聚合 | `DateBucket` + filter 支持内置时间字段 + `Aggregate` + 路由 + 装配 + 测试 | 无 | 2–3d |
| S1b 后端 Agent 接入 | `RunRequest.BI` + `bi.go`（提示词 / 清洗 / 清单导出）+ `buildInstruction` 追加 + `CreateRun` 校验与强制只读 + 哨兵与映射 + 测试 | S1 的语法定稿（提示词描述 §3.1 的语法） | 1–1.5d |
| S2 前端基础 | `types` / `http-api` / `http-api-converters` / `api-mock` / `setup.ts` / `BiChart` / `bi-chart.ts` / `bi-compat.ts` / i18n 骨架 | S1 接口契约（可先按契约写，用 mock 联调） | 2d |
| S3 看板与交互 | 工具栏、过滤、KPI、widget 卡、默认看板、联动、明细表、持久化 | S2 | 3–4d |
| S3b 前端 Agent | `bi-agent-block.ts` / `useBiAgent` / `BiAgentPanel` / `BiWidgetCard` 预览模式 / 错误码与文案 / `useAgentConversation` 透传 | S1b、S3（预览卡依赖 widget 卡） | 2d |
| S4 图表编辑器与收尾 | `BiWidgetEditor`、数据操作菜单迁移、空 / 错态、文档同步、老页测试迁移 | S3、S3b | 2d |
| S5 二期 | F15–F20、F25 | S4 | 2–3d |

**验收标准**

1. 任意已登记 Schema 的集合，打开页面首屏 ≤ 8 个请求，全部成功即渲染默认看板。
2. 同一份数据，DuckDB 与 SQLite 两种引擎的聚合结果一致（后端用例覆盖）。
3. 点击图元联动筛选后，widget 与明细表数据都落在筛选范围内；移除 chip 恢复。
4. 快速切库 / 切集合，旧响应不覆盖新选择（沿用并迁移原竞态用例）。
5. 视图集合、系统项目、只读角色下无写入口；后端对越权请求仍由原有机制拒绝。
6. 亮 / 暗主题切换后图表颜色同步；窄屏单列无横向溢出。
7. `yarn build`、`yarn test`、`yarn typecheck` 通过；覆盖率不低于现状基线（新增文件计入覆盖，不下调阈值）。
8. 方向约定：柱状图的类目轴是 `xAxis`；条形图、对比条形图、堆积条形图的类目轴是 `yAxis`（测试断言）。
9. 输入「近 30 天各状态订单金额，用堆积条形图」，助手产出的配置经校验后出现在待确认区并成功渲染；点「加入看板」前看板与 `localStorage` 均无变化。
10. 助手输出不合法字段 / 不兼容类型 / 非法 JSON 时，对应块被拒并显示原因，看板不受影响；任何情况下助手运行都不产生写操作（含助手被设为可写时）。

## 8. 风险与待确认

### 8.1 待确认（建议值已给出）

| # | 问题 | 建议 |
| --- | --- | --- |
| Q1 | 页面是否改名 | 建议中文「数据看板」、英文 `Data Insights`，路由 `/console/data` 与 `nav.dataBrowser` key 不变；会连带改 `README.md:21`、`docs/data/index.md:30` 及相关文档测试 |
| Q2 | 明细表是否保留行内编辑 | 建议**不保留**，编辑统一走数据管理页的 `DocumentListModal` |
| Q3 | 聚合扫描上限是否暴露为配置 | 建议一期用常量 100 万，不动 `config.yaml` 结构；有需要再单独开项 |
| Q4 | SQL 表与 SQL 数据集的边界 | SQL 单表结构化聚合已实现；F21 任意只读 SELECT 数据集仍需单独评审，不再计划回退为 `SchemaPanel` |
| Q5 | `aggregate` 是否开放给云助手委托 | 建议一期不开放（`require` 而非 `delegable`），避免改 §3.1 委托白名单测试 |
| Q8 | Agent 会话落点 | 建议每「项目 + 库 + 集合」一个会话，标题 `BI · <集合名>`，会出现在云助手页的数据助手会话列表里（可在那里删除）；不新增隔离存储 |
| Q9 | 助手产出是否可免确认自动加入看板 | 建议**不可**，必须经预览确认（已写入 A4 / 验收 9） |
| Q10 | 只读角色 / 只读实例的 Agent | 建议一期接受「禁用并说明原因」：建会话需 `database:write` + `writeGate`，放宽会改权限模型。若业务上必须让只读角色用，需 F26 单独评审新增只读 run 通道 |

**已确认**

| # | 结论 |
| --- | --- |
| Q6 | 「输入需求后由 Agent 生成」**纳入一期**，**复用现有数据 Agent**（内置 `database` 助手），不新建助手、工具、路由、表 |
| Q7 | 「柱状图」纵向；「条形图」「对比条形图」「堆积条形图」横向 |

### 8.2 风险

| 风险 | 影响 | 对策 |
| --- | --- | --- |
| 内置时间字段进入 filter 会改变列表 / 导出的可用语法 | 老用户 filter 不受影响（只新增可用字段），但需防 `created_at` 以文本比较的精度问题 | 统一走 `CastTimestamp`，补两引擎用例；`created_at` 存的是 RFC3339 文本 |
| 集合数据是 JSON 文本，聚合要逐行 `json_extract` | 大表聚合慢 | 扫描上限 + 超时 + 前端提示缩小时间范围；默认看板最多 8 个请求且并发 ≤ 4 |
| `Dialect` 接口新增方法 | 所有实现（目前仅 DuckDB / SQLite 两个）必须同步，否则 godatabase 编译失败 | S1 先改接口与两个实现，再改调用方；`GOWORK=off` 单独验证该模块 |
| 测试里 echarts 为整包 mock | 新增图表 / 组件不补 mock 则整套用例失败 | S2 先改 `setup.ts`，并在 `BiChart` 测试断言 `setOption` 的 option |
| 一次重写丢掉旧功能 | 孤儿组件、导入 / 视图入口消失 | §0.3 对照表逐项落地；验收时手动核对「数据操作」菜单 |
| 默认看板请求风暴 | 首屏慢、占用只读 lease | widget ≤ 8、并发 ≤ 4、可见才请求（`IntersectionObserver` 为二期优化） |
| 覆盖率阈值 | 新增大量 UI 代码拉低 functions / branches | 逻辑下沉到纯函数与 composable 并同步补测；不桩掉 `ui/` 下的 `Switch` / `Tabs` 等交互组件 |
| `streamAgentRun` 只透传白名单字段 | `bi_context` 被静默丢弃，助手拿不到字段清单，生成结果「看起来能跑但全是瞎猜」 | S2 就改 `http-api-converters.ts`，`http-api.test.ts` 断言请求体含 `bi_context` |
| 助手输出不可信（幻觉字段、非法 JSON、越界 limit、恶意 title） | 空图 / 报错 / XSS | 前端严格校验（§4.7 第 9 条）、标题纯文本渲染、`filter` 交后端校验、块只是配置不含 SQL |
| 提示词与后端语法漂移 | 助手给出服务端不接受的度量 / 类型 | 导出 `BIMetricKinds` 并在 `internal/api` 测试逐项过 `parseAggregateQuery`；图表类型清单前后端各一份快照测试，变更需同步 |
| 小模型输出格式不稳定 | 块缺失或残缺 | 解析失败时展示助手原文 + 「让助手重新输出」按钮；提示词要求单行 JSON；前端容忍块前后有文字 |
| 同一会话在云助手页同时使用 | `ClaimThread` 冲突，发送失败 | 捕获冲突并提示「该会话正在运行」；「新对话」可绕开 |
| 助手被用户设为可写 | BI 运行可能触发写命令 | 运行时强制 `Writable=false`（§3.4），并为此补用例 |
| 助手调用只读工具抽样耗时 / 耗 token | 生成慢、配额消耗 | 沿用全局 `agent_max_iterations` / `agent_run_timeout`，不为 BI 放宽；`quota_exceeded` 有明确提示 |
| BI 会话污染云助手会话列表 | 用户困惑 | 标题固定前缀 `BI ·`；Q8 已说明可在云助手页删除 |

## 9. 测试计划

**后端（`go test ./...`，另 `(cd godatabase && GOWORK=off go test ./...)`、`(cd gocloudagent && GOWORK=off go test ./...)`）**

- `DateBucket`：DuckDB / SQLite 的 hour / day / week / month / year；非法 unit 拒绝。
- `parseAggregateQuery`：每个度量与维度的合法 / 非法组合（number 才能 `sum`、date 才能时间桶、json 字段拒绝、未知字段 `unknown_field`、未登记 Schema 的降级、`limit` 越界、度量数超限、`;` / NUL）。
- 注入防护：字段名与 `bin` 的非法输入不进入 SQL 文本。
- `Aggregate` handler：按天计数、`series` 堆叠、`filter` 联动、视图集合、`truncated`、扫描超限、超时、只读 lease、系统库只读可读。
- 散点 `mode=points`：x / y 必须是 number 字段、与 `metrics` / `group_by` 互斥、`NULL` 行被过滤、`limit` 上限 2000、分组超 12 归入「其他」、结果顺序稳定（`created_at DESC, id DESC`）。
- filter 增量：`created_at` / `updated_at` 范围筛选在列表、导出、聚合三处一致。
- 路由：`/aggregate` 要求 `database:read`；只读 scoped Key 可 GET；委托令牌不可用（保持 `delegation_route_table_test.go` 不变并通过）。
- `cloudagent/bi_test.go`：提示词含图表类型清单与方向约定、度量语法、`sb:chart` 输出格式、「禁止写入」；`NormalizeBIContext` 丢弃非法字段名 / 非法类型、超 64 个截断、非法 `collection` 整体拒绝；`buildInstruction` 在 `req.BI != nil` 时追加提示词与 env 行，`nil` 时与现状逐字一致（回归）。
- `internal/api/agent_bi_run_test.go`：
  - `database` / `general` 助手接受 `bi_context`，其他 module 返回 400 `bi_context_invalid`；
  - 与 `starter_stage` / 搭建会话同传返回 400；
  - `database_id` 非 UUID、`collection` 非法返回 400；
  - 助手被设为 `writable=true` 时，传给 runtime 的 `RunRequest.Agent.Writable` 仍为 `false`；
  - `bi_context` 不出现在落库的 user 消息里；
  - 不带 `bi_context` 的既有请求行为不变；
  - 系统项目、只读实例按既有规则（建会话阶段）被拒。
- 漂移防护：遍历 `cloudagent.BIMetricKinds`，逐项构造聚合参数交给 `parseAggregateQuery`，全部被接受。

**前端（`yarn test`）**

- `bi-chart.test.ts`：8 种具体类型的 option 构造（折线含面积开关、**柱状图类目轴为 `xAxis`**、**条形 / 对比条形 / 堆积条形类目轴为 `yAxis` 且 `inverse`**、条形降序、饼图含环形与「其他」合并、对比条形无 `stack`、堆积条形同一 `stack`、散点含大小与分组、表格不产出 ECharts option）、时间桶补零、空数据、compact 格式。
- `bi-auto.test.ts`：§4.3.1 十条规则逐条命中与顺序优先级、类目数边界（6 / 12）、结果回来后重新解析。
- `bi-compat.test.ts`：每种类型的兼容 / 不兼容组合与原因文案。
- `BiChartTypePicker.test.ts`：9 个选项渲染、不兼容置灰与原因、点选固定后不再自动变更（打在真实 `ToggleGroup` 的 `data-state` 上，不桩组件）。
- `bi-defaults.test.ts`：§4.4 每条规则与 8 个上限、未登记 Schema 的降级。
- `useBiDashboard.test.ts`：持久化读写、字段失效丢弃、恢复默认、项目 / 库 / 集合切换清空、竞态。
- `BiWidgetEditor.test.ts`：类型与度量联动、非法组合禁用、保存回调。
- `bi-agent-block.test.ts`：多块解析与 4 块上限、单块 4 KB 上限、非法 JSON 跳过、未知 key 丢弃、`stripChartBlocks`、`validateAgentWidget` 对未知字段 / 不兼容类型 / 越界 `limit` 的拒绝与原因、`title` 里的 HTML 不被解释；图表类型白名单快照与 `BiChartType` 一致。
- `useBiAgent.test.ts`：定位 `builtin_key === 'database'` 助手、助手缺失的提示、会话 id 持久化与 404 重建、`bi_context` 内容（隐藏字段不传、补 `created_at` / `updated_at`、`auto` 不传 `preferred_type`）、发送 / 取消 / 重试（重试重新携带 `bi_context`）、epoch 令牌丢弃过期回调、切换集合清空待确认区、`ClaimThread` 冲突提示。
- `BiAgentPanel.test.ts`：Enter 发送与 Shift+Enter 换行、运行中变停止、禁用条件（系统项目 / `!canWrite` / 无集合）与原因提示、待确认区三个按钮、「加入看板」前 `localStorage` 无写入、「让助手修正」发送带错误文本的跟进消息。
- `DataBrowser*.test.ts`：保留旧响应竞态、视图只读、真实 Select 与数据操作覆盖；刷新改为验证保留当前分析上下文。SQL 库验证真实表看板、表聚合 / 明细接口分流、真实时间列 / 无时间列降级、Agent 的 `source: 'table'` 与集合 / 表配置隔离，不再断言回退 `SchemaPanel` 或隐藏 Agent / 类型选择器。新增修复回归见 §12.5。
- `http-api.test.ts`：`db.aggregate` 路径与 snake → camel 映射；`streamAgentRun` 请求体包含 `bi_context`，缺省时不带该键。
- `i18n.test.ts`：两棵语言树 key 一致。

## 10. 验证命令

```bash
go build ./internal/... ./cmd/... && go test ./internal/api/ ./internal/app/ ./internal/auth/
(cd godatabase && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./... -timeout 900s)
(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...)
gofmt -l internal godatabase gocloudagent
cd ui && yarn typecheck && yarn test && yarn build
./examples/run_all.sh --start --skip-build --skip-agent --api-port 18087 --only 01-project   # 端到端冒烟（可选）
```

## 11. 约束自查

- 不新增依赖、不改 `go.mod` / `package.json` / `tsconfig.json` / `vite.config.ts`。
- 不做任何数据库结构变更；不自动执行 `init_db`；Agent 部分不新增表、不新增内置助手。
- 新接口为只读，不碰系统库写保护；错误统一走 `WriteError` + 领域错误（`ErrBIContextInvalid` 进 `sentinelErrorMappings`）。
- Agent 运行强制只读；技能与权限不接受客户端输入；`bi_context` 不落库；日志与审计不含字段清单与提示词正文。
- 前端对象形状一律 `interface`，无 `any`；文案只进 `ui/src/locales/`；状态类用 reka 真实属性；助手输出只信结构，标题纯文本渲染。
- `godatabase` 与 `gocloudagent` 不 import 根模块；`Dialect` 改动只在 godatabase 内完成，提示词与清洗只在 `cloudagent` 内完成，宿主只传原始值。

## 12. 实现复核与功能 / 视觉优化计划（2026-11-18）

### 12.1 当前交付状态（静态核查）

**结论：基础能力已成形，优先把“图表能显示”提升为“统计可信、操作可恢复、重点看得清”。不建议再增加图表类型或做整页装饰性重写。**

本节以当前工作区（含未提交实现）为基线。下文“已实现”仅表示存在完整代码路径；“待修复”来自静态逻辑核查；视觉效果与运行结果仍须按 §12.5 验收。原 S1–S5 是初始估算，不再作为剩余工作量；后续按 §12.4 执行。

| 原编号 / 能力 | 当前状态 | 依据与剩余边界 |
| --- | --- | --- |
| F1 / F2 / F4 / F10 | 基础已实现 | `useBiDataSource.ts`、`useBiDashboard.ts`、`bi-defaults.ts`、`bi-auto.ts`：集合 / SQL 表选源、默认最多 8 卡、9 项类型选择、本地配置及恢复默认均存在；正确性问题见 BI-F01–F04 |
| SQL 表看板 | 已实现，超出原一期范围 | `internal/api/schema_aggregate.go`、`aggregate_env.go`、`useBiDetail.ts`、`DataBrowserSql.test.ts`：真实表 / 列、只读结构化聚合及明细、表来源 Agent；无时间列不补系统时间字段。**F21 任意 SELECT 数据集仍未实现** |
| F3 KPI 行 | 部分完成 | 无维度单指标可显示 KPI，默认含 count / sum / avg；尚无独立 `BiKpiRow`，没有默认“今日新增”与环比。KPI 不应再作为新图表类型开发 |
| F5–F8 / F12 | 基础已实现，需修正交互 | 时间范围、表达式 / 历史、图元下钻、明细分页 / 列显隐、自动刷新及隐藏页面暂停已有；手动刷新、滚动时间窗口、空值下钻等见 BI-F02 / F05–F07 |
| F9 / F18 | 部分完成 | `BiWidgetEditor.vue` 已有增改、字段 / 度量 / 系列、分箱、limit、面积 / 环形、4 档宽度；复制 / 删除已有。排序 UI、移动顺序、容量反馈未完成，不能把宽度调整重新列为未开发 |
| F11 / F13 / F14 | 已有入口，不能视为验收完成 | 集合 Schema / 视图 / 导入 / 导出入口及权限处理已有；SQL 表不沿用集合写操作。Schema 失败恢复、实例只读提示需完善；浏览器内修改看板配置与数据库写权限是不同概念 |
| A1–A9 | 主链路已实现，部分边界未闭合 | `agent_bi.go`、`cloudagent/bi.go`、`useBiAgent.ts`、`bi-agent-block.ts`：BI 上下文、运行强制只读、出块、预览、加入 / 调整 / 丢弃、多轮与重试已有；准备期互斥、切源恢复、调整后确认和非法块反馈见 BI-F09–F12 |
| F15–F17 / F19 / F20 | 尚未完整实现 | PNG / 单图 CSV、全屏、配置导入导出、环比未见用户入口；hour / day / week / month / year 编辑已有，但补桶仅 day / month / year，DataZoom 未配置。**注册 Toolbox / DataZoom 不等于功能已交付** |
| 样式与测试基础 | 已存在，不从零重做 | Tailwind 栅格、主题 token、ECharts resize / dispose、三态、真实 Select、输入法 Enter 防误发送已有；已有聚合双引擎及前端测试，但本次未执行 |

**需要同步修正的旧契约理解：**

- `useBiDataSource` 负责来源 / 字段，`useBiDetail` 负责明细，`useBiDashboard` 负责配置；实际不是把所有状态放进一个 composable。`BiWidgetRunner` 负责单卡取数与渲染编排，后续继续沿用。
- `scanned` 当前来自同 filter 的 `COUNT(*)`，代表**符合条件 / 参与统计的行数**，不是物理扫描量；points 还排除 X / Y 为 NULL 的行。默认 100 万是参与行上限，COUNT 本身仍可能扫描大量数据，超时保护不可替代。
- 当前有维度聚合缺省 `x ASC`，不等于指标 Top N；输出 limit 截断的是分组 / 点结果，并非先截断聚合输入。只调整前端排序无法补回服务端已截掉的类别。
- SQL 散点排序目前按常见时间列名、id、第一列回退，尚无唯一排序键证明；仅集合可沿用“按 created_at / id 取最新点”的语义，不承诺所有 SQL 表都是“最新且稳定的抽样”。
- 页面底部 `BiDetailTable` 有明细分页 / 列显隐；卡片内 `type=table` 是聚合 / points 结果展示，**不等于 §4.3 原设计的独立明细型 widget**。

### 12.2 功能正确性与操作体验待办

优先级独立于 §6 的历史分期：**P0 为会误导统计或绕过明确确认的问题；P1 为主流程与恢复性；P2 为增强项。** 下方各条的「证据」是 2026-11-18 复核时的原始问题描述，保留作历史；**实施状态以本表为准**。

**R1 / R2 实施状态（2026-11-18，前端 + 聚合接口；浏览器验收未做）**

| 编号 | 状态 | 落地内容 | 仍待办 |
| --- | --- | --- | --- |
| BI-F01 | 已完成 | 前端：饼图 / 条形图按首个度量降序取数，取数上限（≥100，封顶 500）与展示 Top N 分开；截断结果或不可累加度量不再合并「其他」。后端：新增只读参数 `with_total=1`，分组 + 单度量 `count` / `sum` 时额外返回同口径总量 `total`；饼图用 `total − 前 N 类` 得到真实「其他」，拿不到总量时退回「结果不完整」提示 | 负值 / 全零饼图仍直接用扇区表达（未改） |
| BI-F02 | 已完成（主维度） | 图元带下钻状态：合并「其他」/ 空值 / 展示名重名时不生成 chip，并 toast 说明；NULL 显示为「（空）」，不再冒充「其他」 | 系列级下钻（只筛主维度）；bool / 时间桶的真实浏览器点击验收 |
| BI-F03 | 已完成 | 日期 min / max：KPI 保留日期文本、有维度时兼容规则禁用图表且自动推断退表格；时间维度 + series + 多度量：兼容规则拒绝、自动推断退表格；分箱 + 多度量 / series 同样退表格 | 日期 KPI 的专用格式化 / 时区显示 |
| BI-F04 | 已完成 | `Frame` 记录每个系列的缺口值：count / sum 补 0，avg / min / max 缺桶与 NULL 留空（`null`）而非 0；补桶覆盖 hour（`YYYY-MM-DD HH:00`，含跨天）与 week（周一对齐，+7 天） | 补桶仍以返回的桶范围为界，不推测服务端截断之外的时间窗 |
| BI-F05 | 已完成 | 手动刷新只重取数据并保留筛选 / 时间 / chip / 明细页（字段已就绪时不再重载来源）；相对时间窗口使用每轮统一的时间锚点，自动刷新时同步前进，同卡每轮只请求一次；明细页只在条件变化时回到第一页 | 「最近成功更新时间」展示 |
| BI-F06 | 已完成 | 除前一轮内容外：卡片把请求结果（成败 + 错误码）上报页面，条件类错误（`invalid_filter` / `invalid_request` / `filter_scan_limit_exceeded`）在筛选区集中提示一次；输入与已应用不一致时提示「有未应用的筛选修改」；切源 / 改条件后旧错误立即作废 | 表达式语法仍未做本地高亮 / 行内定位（后端为准） |
| BI-F07 | 已完成 | `documentTable` 不再经数据补回 hidden 字段，系统列以文档元数据为准；明细表把 NULL 显示为破折号（`data-null` + 提示「空值」）、与空串 / 0 / false 区分；超长值点击展开、可复制（成功 / 失败都有 toast）；列显隐按「项目 + 库 + 数据源」持久化（`sb_bi_cols_v1:*`） | — |
| BI-F08 | 已完成 | `add` 返回成败，满 24 张时新增 / 复制 / Agent 加入都提示上限并保留预览；编辑器新增「排序」字段 + 方向（候选只有 x / series / 各度量别名），`bi-compat` 增加 `sort_unknown` 校验（三处共用），陈旧排序在打开时自愈；limit / 宽度 / 样式收进「展示」分组 | 拖动排序仍未做（属 F18） |
| BI-F09 | 已完成 | Agent「调整」保存只更新待确认项并重新预览，必须在预览成功后点「加入看板」才落配置 | — |
| BI-F10 | 已完成 | 准备阶段计入忙状态、连发只建一个会话；`reset` / 新对话清空失败 / 计时 / 重试 id；新增按数据源暂存现场（`park` / `sync`）：切走再回来恢复历史与待确认区，仍在跑的 run 用 `attach` 续接；暂存按「会话所属数据源」记账，不会把 A 的结果挂到 B 名下 | 同一会话在云助手页并发使用时的冲突提示沿用既有报错文案 |
| BI-F11 | 已完成 | 解析阶段的坏块（坏 JSON / 超长 / 非对象）逐项给原因；`filter` / `sort` 等超长字符串拒绝整块，不再静默丢弃放宽筛选 | `limit` 夹取与缺省 `version` 的兼容策略仍是「静默修正」，待与验收口径对齐 |
| BI-F12 | 已完成（可预判部分见右列） | 文案改为「参与统计 N 行」，分组截断与点集截断分开提示；`sqlTableOrder` 明确承诺边界（无唯一键时截断边界不保证可复现，不称「最新」或「随机样本」），`docs/data/index.md` 同步；只读实例建会话被拒（`writer_unavailable`）时展示「当前实例为只读模式，无法创建助手会话」 | 前端仍无实例 writable 的读取接口，无法在入口**前置**禁用——按计划退化为「准确的拒绝反馈」；若要做前置禁用需新增只读能力探测接口 |

**本轮验证（实际执行）**：`yarn typecheck` 0 错误；`yarn build` 通过；前端全量 1487 例通过（排除 `docs-content`），覆盖率 statements 98.58% / branches 94.86% / functions 94.26%（branches / functions 仍低于 95% 阈值，与改动前基线 94.92% / 94.1% 同档）。后端 `go build ./internal/... ./cmd/...`、`go test ./internal/api/ ./internal/app/ ./internal/auth/` 通过，`gofmt -l` 无输出；聚合测试覆盖 DuckDB 与 SQLite 两个真实引擎（含 `with_total`、filter 同口径、非累加度量不返回 total）。`tests/docs-content.test.ts` 仍有 1 项失败，来自工作区里未提交的 `docs/getting-started/openai-compatible.md` 链接到 `plan/` 目录（文档站不含该目录），与 BI 无关。**未做**：真实浏览器视觉 / 交互验收、BI-V01–V06 视觉整理（R3）、R4 增强。

#### BI-F01 · P0 · Top N 与占比必须有正确分母

- **证据**：`bi-defaults.ts` 的分类饼图 `limit: 8` 未给指标降序；`collection_aggregate_query.go` 默认 `x ASC`；`BiWidgetRunner.vue` 将 limit 原样交后端；`bi-chart.ts` 的 `buildPie` / `foldTopN` 只能合并已经返回的行。同一个 limit 同时承担取数上限与展示 Top N，真实链路拿不到尾部数据。
- **修复**：区分“取数上限”和“展示 Top N”，分类排名在服务端显式按度量排序。截断结果在卡头标注“部分分组”，不能称为全量占比。短期没有准确分母时改用排名条形图；若要保留全量占比，需后端给出同口径总量 / 剩余量，另行评审只读接口扩展，不靠拉全量明细。
- **语义**：只有可加且适合占比的非负度量可合并“其他”；avg / min / max / distinct 不能相加冒充总体，负值也不强行做饼图。
- **验收**：12 个类别、名称排序与数量排序不同，确认真正 Top 8；覆盖截断、负值、全零、NULL 与非可加度量。真实请求链路测试不能仅给 `foldTopN` 一份完整假数据。

#### BI-F02 · P0 · 图元身份与显示标签分离

- **证据**：`bi-chart.ts` 把合并类及部分空值显示为“其他”；`BiWidgetRunner.vue` 点击处理用 `payload.name` 拼 filter。翻译标签不是原始字段值，真实值“其他”、NULL、空字符串与合并项会混淆。
- **修复**：图元携带原始值、值类型、分组身份 / 桶边界，展示文本只负责显示。合并“其他”无准确筛选表达式时禁用下钻并解释；不能生成 `field="其他"` 假装选中尾部类。系列级下钻明确是只选主维度还是同时选系列。
- **验收**：上述四类值、bool / number 类目、时间桶、分箱分别点击，图表和明细范围一致；切换中英文不改变查询语义。

#### BI-F03 · P0 · 日期指标与多度量不能静默变形

- **证据**：编辑器允许 date 的 min / max；`bi-chart.ts` 的 KPI / `toFrame` 通过 `num()` 把不可转换日期变成 0。有 `series` 时 `toFrame` 只读第一个度量，但折线兼容规则允许 1–3 个度量。
- **修复**：按字段 / 度量结果类型格式化日期 KPI；不支持的图形明确禁用或退为表格。`series × metrics` 要么完整展开且控制系列数，要么在编辑器、自动推断及 Agent 校验里统一拒绝，不静默丢指标。
- **验收**：日期 min / max 展示为日期或表格；“日期＋状态＋count / sum”不得只画 count；NULL 与真实 0 区分。

#### BI-F04 · P0 · 时间补桶保持指标语义

- **证据**：`bi-chart.ts` 的 `fillTimeBuckets` 对所有度量统一补 0，目前只处理 day / month / year；缺失平均值被画成 0 会制造下跌趋势。
- **修复**：定义缺桶策略：count 可补 0；sum 按明确口径处理；avg / min / max 无观测应留空，不插造测量值。补齐 hour / week 时与后端 UTC / 周起始规则一致，并限制总桶数；未实现前不要宣称全粒度连续补桶。
- **验收**：同一缺日数据的 count 与 avg 分别显示 0 / 断点；覆盖跨月、跨年周、小时边界及长跨度，截断结果不得因补桶伪装成完整时间窗。

#### BI-F05 · P1 · 刷新保留分析上下文，相对时间随时钟前进

- **证据**：`DataBrowser.vue` 手动刷新调用 `ds.refresh`，重载来源会走 `clearBelowSource` 清条件 / Agent / 明细；自动刷新只增加 `refreshTick`。`globalFilter` 的 `rangeExpr()` 使用当前时间，但 computed 不依赖刷新时钟，窗口下界会停在首次计算时。
- **修复**：区分“刷新数据”与“重载来源结构”，默认刷新保留来源、图表、筛选、明细页与待确认项；真正切源或来源失效再清理。每轮取数固定一个时间锚点，更新相对窗口，同轮卡片 / 明细共用；避免 filter watch 与 tick 造成双发。显示最近成功更新时间、刷新中 / 已暂停 / 部分失败，失败不能更新成功时间。
- **验收**：近 1h＋表达式＋chip＋明细第 2 页，手动刷新后状态不丢；假时钟前进 30 分钟后边界同步前进。隐藏页不请求，恢复后按策略只补一轮，慢请求不叠加。

#### BI-F06 · P1 · 筛选、Schema 与排队请求各自可恢复

- **证据**：`DataBrowser.vue` 的清除条件不包含时间范围；字段加载失败写到 `detail.error`，明细区重试却只重取 rows；`useBiQuery.ts` 令牌能丢旧响应，但旧排队任务出队仍发请求。
- **修复**：提供明确的“清除全部”语义，区分草稿 / 已应用条件，历史按来源隔离；表达式错误在筛选区集中反馈。字段加载独立 pending / error / retry，未就绪时限制配置入口；过期任务出队前跳过，同卡仅保留最新待执行查询。保留已有并发上限 4 与令牌防护。
- **验收**：只有时间筛选也能清空；Schema 首次 500 后原位重试恢复默认看板；占满 4 槽后连续换源，旧排队任务不调用 API。合法但无结果不是查询错误。

#### BI-F07 · P1 · 明细字段展示与值类型准确

- **证据**：`bi-source.ts` 的 `documentTable` 过滤 hidden 后又从 data 补入全部键；对象展开顺序允许业务同名字段覆盖系统 id / 时间。现有 `bi-source.test.ts` 中相关用例不能证明元数据得到保护。
- **修复**：隐藏字段不因补列重新出现；系统元数据与同名业务字段明确分开或制定不可覆盖的命名规则。NULL、空字符串、0、false 各自呈现；长 JSON / 文本提供展开与复制，列偏好按来源保存。hidden 是展示约定，不作为鉴权边界。
- **验收**：包含隐藏字段及业务 id / created_at 的文档不会污染系统元数据；数值右对齐，时间明确时区，复制取得原值。

#### BI-F08 · P1 · 编辑器补足排序与容量反馈

- **证据**：`BiWidgetEditor.vue` 已有宽度 / 粒度等配置但无排序入口；`useBiDashboard.ts` 达 24 卡时 `add()` 静默返回，页面仍可能关闭编辑器或提示 Agent 加入成功。
- **修复**：增加排序字段 / 方向及排序与度量联动校验；将 Top N 与返回上限的语义说明清楚。新增 / 复制 / Agent 加入共享可判定的成功结果，满额时保留草稿 / 预览并提示上限。编辑按“数据 → 图形 → 展示”分组，P2 再增加防抖查询预览，不在每次输入时请求。
- **验收**：第 25 张卡不假成功、不丢预览；删除排序依赖字段后提示或重置 sort；调整 span 不重取数据。

#### BI-F09 · P0 · Agent 调整后仍需重新预览确认

- **证据**：`BiWidgetCard.vue` 的直接加入会在 pending / error 时禁用；但 `DataBrowser.vue` 的 `saveWidget()` 遇到 `editingPendingId` 直接 add 并移除预览，绕过修改后的查询确认。
- **修复**：“调整 → 保存”只更新待确认配置，重跑同口径预览；成功后显式“加入看板”才落配置。预览配置 / 筛选变化即使保留旧图，也不能把旧成功状态当成新配置可加入的凭据。
- **验收**：改成错误 filter 后不能加入；保存调整不写看板存储；重新预览成功、且未达容量上限后，确认只加入一次。

#### BI-F10 · P1 · Agent 生命周期与后台运行可解释

- **证据**：`useBiAgent.ts` 的 busy 只来自运行阶段，查助手 / 建会话期间可重复发送；`reset()` 调 `conv.stop(false)` 不清旧失败 ID / error。持久化会话探测未恢复 messages，切源断流后后台 run 可能继续，回到原源也未 attach。
- **修复**：准备、建会话、运行、取消均进入统一 busy / epoch；新对话和切源清空旧运行错误 / 计时 / retry ID。建议保留“切源只断开、不自动取消后台”策略，并在返回时恢复历史及活动 run、提示占用与提供显式停止；不能把本地清空等同服务端取消。发送准备失败保留可重试的原需求。
- **验收**：延迟建会话时双击只建一次；失败后切源不显示旧错误 / 重试旧 run；生成中切走再回来、刷新页面后追问，都能识别当前会话状态。

#### BI-F11 · P0 · 非法 Agent 配置不能静默放宽筛选

- **证据**：`bi-agent-block.ts` 跳过非法 JSON / 超长块，混合好坏块时缺少逐项拒绝原因；超长 filter 经 `str()` 变 undefined，可能将有条件的图变成无该条件的图。
- **修复**：语义字段校验失败必须拒绝整块；filter 不做静默删除 / 截断。解析返回有界的块级错误，说明哪一块失败；version 缺省和 limit 夹取若保留兼容策略，应明确反馈，不能一处写“夹取”、另一处验收写“拒绝”。错误码补齐两种语言。
- **验收**：好坏块混合仍逐项解释；2048 字符边界前后 filter 不会悄悄消失；无有效块时保留说明并允许重新生成，不写看板。

#### BI-F12 · P1 · 限制提示与权限语义真实

- **证据**：`BiWidgetCard.vue` 统一显示“扫描 N 行 / 前 N 项”；`aggregate_env.go` 的 SQL 点排序依赖列名回退；`agentDisabledReason` 仅看系统项目 / `auth.canWrite` 等，未预判实例 writable。
- **修复**：区分参与行数、返回分组数、返回点数和耗时；SQL 点集说明排序取样策略，不称随机样本。无唯一键时收窄稳定性承诺，或读取主键信息构造唯一排序。Agent 建会话需要写权限的原因单独说明；能读取实例能力时前置禁用，不能预判 API Key 权限时正确处理服务端拒绝，不放宽现有权限。
- **验收**：只读角色 / 只读实例 / scoped Key 的拒绝原因准确；无 id、重复时间、复合主键的表测试点集策略；修改浏览器本地布局不被误描述为数据库写操作。

### 12.3 视觉与功能美化规格（待实施）

**方向：保持现有控制台的轻量数据工具风格，减少控制区抢占视线，让指标、趋势和异常更容易阅读。** 不增加大标题、渐变背景、装饰图、重阴影或新的设计体系。本节为设计建议，非已截图确认的视觉结论。

#### BI-V01 · P1 · 信息层级与首屏密度

- 保留“数据源 → 图表类型 → Agent 输入”的已确认顺序，但把“图表类型”明确标成**生成偏好**：它目前不负责批量切换已有卡片，避免用户误解。
- 数据源行左侧展示来源名与集合 / 表 / VIEW 小徽标，右侧收纳刷新、自动刷新及数据操作；SQL 无时间列提示为低强调说明，不当成报错横幅。
- 类型偏好与紧凑输入保持可见；字段快捷项、历史和长 Agent 说明改为可展开区域。生成结束后展示“待确认 N 张”，允许收起结果但不能自动加入或丢弃。
- 下方按“已应用筛选摘要 → KPI 概览 → 趋势 / 分布 → 明细”分层。新增图表作为看板区域操作，恢复默认保持次级且确认，避免顶部满屏同级按钮。
- **验收**：常规 1440×900 视口在结果折叠时可看到 KPI 和第一排图表的主要内容；不靠隐藏必要操作实现。用户能分清生成偏好与单卡类型。

#### BI-V02 · P1 · KPI 与图表栅格分层

- 当前所有 widget 在同一 grid 中，短 KPI 与高图表同排易产生留白；独立渲染 KPI 区，图表区继续用 12 列，不新增布局依赖。显示分组不能悄悄重写已保存的 widget 顺序 / 类型。
- KPI 建议单列起步、sm 两列、宽屏 3–4 列；标题 `text-xs/text-sm`，主值 `text-3xl`＋`tabular-nums`，日期型主值另用适合的字号与格式，辅助口径不重复主标题。
- 图表仍为 `h-64 sm:h-72` 基线；中屏优先半宽，4 列窄卡在宽屏才启用。横向条形图按**唯一类目数**决定高度 / 滚动，不用 series 展开后的行数估算；同排绘图区高度协调。
- 默认 8 卡从“按字段顺序塞满”改为兼顾总量 / 趋势 / 分类 / 数值分布；分箱步长不固定对所有数量级用 10。禁止为美化再追加无界请求；今日新增、环比另按 F19 的成本评审。
- **验收**：纯数值、纯分类、无日期、字段很多的来源均有合理布局；KPI 不与趋势图形成大片空白；不改变原有图表方向约定。

#### BI-V03 · P1 · 图表可读性与数值表达

- 轴刻度用紧凑数字，tooltip 保留准确值；单位 / 小数位优先由字段配置表达，不从字段名臆测币种。日期和时区显式一致，原始字段名作为辅助信息，度量标题使用本地化名称。
- 长类目统一截断并提供完整名称；横向图预留合理标签宽度，数值轴基线与网格弱化；密集时间标签自动抽稀，不把全部文字旋转后挤在一起。
- 环形图在分母可信时显示总量 / 指标名，tooltip 同时给值和口径明确的比例；类目较多或截断时优先条形图。图例与外侧标签不能挤出容器。
- 分类颜色按原始类别身份稳定映射，刷新排序后不随索引漂移；继续使用 `--chart-*` token，超过 5 系列增加线型 / 点型区分，不仅靠降低透明度。未知 / 缺失类别的中性色与真实类别区分。
- “部分结果”提示应在卡头可见，统计行数 / 耗时等诊断元数据降为页脚或 tooltip；不能只为了好看隐藏截断信息。
- **验收**：5 / 12 类、多系列、长中英文标签、负值、极大 / 极小数、NULL、亮暗主题均可读；排序及移除类别不会重置其他类别颜色。

#### BI-V04 · P1 · 卡片操作与窄屏适配

- `BiWidgetCard.vue` 预览卡头目前三个按钮放在不可收缩的横排，标题与操作易争宽；改为标题独占一行、预览操作移至卡尾且 `flex-wrap`，窄屏主按钮可全宽。
- 看板卡头仅保留标题、必要状态及“更多”；P2 全屏 / 导出放菜单，不将所有操作常驻。标题允许两行或截断＋完整提示，长 source / chip 同样 `min-w-0`，移除按钮不得被挤掉。
- 保留 `Card`、`p-4 sm:p-5`、`gap-4 sm:gap-5` 与全宽外壳；菜单用现有 `Popover`，不恢复已删除组件。主题选中态用 `primary-foreground`，不硬编码白字适配所有主题。
- **验收**：320 / 375 / 768 / 1440px、中英文、长标题下页面无横向溢出；仅表格自身允许横滚。菜单关闭后焦点回到触发按钮，触屏操作不依赖 hover。

#### BI-V05 · P1 · 筛选与编辑表单降低理解成本

- 时间字段、时间范围、已应用 chip 与表达式的视觉层次分开；高级表达式可折叠但摘要常显，未应用草稿有明确提示。“清除全部”覆盖时间 / 表达式 / 联动，字段快捷项按需展开。
- 编辑器用 `Field` 系列分组展示：数据设置在前、图形选项其次、宽度等展示选项最后；不兼容原因紧挨控件。`sort` / limit / Top N 显示各自含义，自动推断结果用辅助提示，不做第二套类型系统。
- 预览 pending 时展示最后成功结果并标记刷新中，不把页面跳回骨架；首次加载保留目标区域高度。错误重试位于真正失败的区域，统一走 `SbAsyncRegion` / `SbErrorState`，Agent 运行失败区也不再手写第二套错误外观。
- **验收**：用户能区分无数据、未配置、筛选无匹配、部分结果和请求失败；Schema 失败不伪装成明细错误；表单缩窄后输入框及焦点环不被裁切。

#### BI-V06 · P1 · 明细可读性与键盘可用性

- 明细区增加结果数量 / 当前筛选摘要，保留既有分页和列显隐；需要时支持折叠但保留可发现入口。数值右对齐、id 等宽、长 JSON 展开，表头在区域滚动时保持可辨识。
- `FieldLabel` 与输入补 `for/id` 或等效关联；filter 输入不能只有 placeholder。禁用类型的原因需键盘可达；图表除已有 `role="img"` / 名称外，提供查看数据及类别筛选的等价键盘入口。
- 状态更新用适度的 live region；遵循 `prefers-reduced-motion`，不让自动刷新重复播大动画；焦点可见，普通文字对比度目标不低于 4.5:1，不能只凭颜色区别状态。
- **验收**：键盘可完成选源、筛选、添加、预览确认和删除；屏幕阅读器能辨认字段与错误；主题切换不破坏焦点，200% 缩放下仍可完成主要操作。

### 12.4 实施顺序与范围

1. **R1：统计与确认正确性** — BI-F01–F04、BI-F09、BI-F11。先补可失败的回归用例，再修纯函数 / 兼容规则 / 只读聚合契约；未解决准确分母前不做“更漂亮的全量占比”。
2. **R2：主流程可靠性** — BI-F05–F08、BI-F10、BI-F12。统一刷新 / 生命周期 / 错误归属 / 容量反馈，保留已有只读与竞态保护。
3. **R3：视觉整理** — BI-V01–V06。先做信息层级、KPI 分区、窄屏预览按钮，再优化颜色 / 标签 / 明细。每个视觉改动同时覆盖真实交互，而非只断言 class 字符串。
4. **R4：按需增强** — F15–F17 的全屏 / 导出 / 配置迁移、F18 移动、F19 环比、F20 DataZoom 和编辑器实时预览。实现顺序优先“全屏＋查看数据”，之后才是可分享产物；配置导入复用字段 / 类型校验并确认覆盖，CSV / PNG 标注筛选及截断口径。

**主要落点**：`ui/src/pages/DataBrowser.vue` 只编排；布局 / 交互在 `components/bi/`；刷新、队列、会话在 `composables/useBi*.ts`；统计转换、图元身份、默认规则、解析在 `utils/bi-*.ts`；跨层契约在 `services/types.ts`（对象只用 `interface`）；中英文文案同步。后端只在确需准确分母 / 点集排序时调整既有聚合编译及响应，不为美化新增表、依赖或配置来源。

**不纳入本轮**：拖拽布局库、更多图表类型、自定义 SELECT 数据集、看板服务端共享、只读 Agent 新权限通道。不得启动迁移 / `init_db`，不对用户运行库执行 DDL；也不为截图改动用户真实数据。

### 12.5 验收矩阵与验证记录

**新增回归覆盖**（扩展既有测试，避免单为编号新建测试文件）：

- `bi-chart.test.ts`、`bi-auto.test.ts`、`bi-compat.test.ts`、`bi-defaults.test.ts`：Top N 全链路口径、空值身份、日期 KPI、多度量系列、不同度量的缺桶策略、默认布局与分类颜色稳定。
- `BiWidgetRunner.test.ts`、`DataBrowser*.test.ts`、`useBiDataSource.test.ts`、`useBiQuery.test.ts`：图元到真实 filter、刷新保留状态 / 更新时间窗、Schema 重试、失效排队任务不发请求、集合 / 表分流与无时间列降级。
- `bi-source.test.ts`、`useBiDetail.test.ts`、`BiWidgetEditor*.test.ts`、`useBiDashboard.test.ts`：隐藏字段与元数据冲突、NULL / 空串、排序依赖、24 卡容量反馈、显示分区不改保存顺序。
- `bi-agent-block.test.ts`、`useBiAgent.test.ts`、`BiAgentPanel.test.ts`、`DataBrowserAgent.test.ts`：好坏块混合、超长 filter 明确拒绝、准备期防重复、切源清理旧 retry、历史 / 活动 run 恢复、调整重预览、仅成功确认落盘。
- `collection_aggregate_test.go`、`schema_aggregate_engine_test.go`、`agent_bi_run_test.go`：集合与 SQL 表双引擎口径、输出截断与输入参与行上限、确定性排序前提、只读限制；扩展响应时补 `http-api-bi.test.ts` snake_case 映射。

**真实浏览器验收**（尚未执行，mock ECharts 用例不能替代）：

- 矩阵：集合 / SQL 表、空源 / 无时间列 / 多字段；明暗主题 × 中英文；320 / 375 / 768 / 1440px；长标题 / 高基数 / 大数 / NULL；首次加载 / 刷新 / 局部失败 / 部分结果 / Agent 待确认。
- 检查真实 ECharts 标签 / tooltip / 点击身份 / resize / 全屏恢复，预览操作不溢出，卡片状态不造成整页跳动；记录核心断点截图与交互结果，不把源码推断写成截图结论。
- 选源、筛选、预览、确认、清除、重试、键盘操作完整走通；DataZoom 若启用，明确只是缩放已返回数据还是改变查询范围，不能隐式混淆二者。
- 请求预算修正：**默认最多 8 个 widget 聚合请求、聚合并发不超过 4**，不是整个首屏所有接口加起来 ≤ 8；来源 / 字段 / 明细等请求另计，Agent 预览也须受共享聚合队列约束。更新时间边界不能让同卡一轮重取两次。

**验证门禁**：代码实施后按 §10 跑相关包测试及前端 typecheck / test / build，不降低覆盖阈值；后端公共契约变更需验证独立模块。端到端只使用明确隔离的测试环境，不自动执行文档中可能触发真实库初始化的启动命令。

**记录**：首次提交（文档复核）只更新计划，未改代码。随后落地了 R1 / R2 的前端部分，逐项状态与实际验证结果见 §12.2 顶部状态表；BI-V01–V06（R3）、真实浏览器验收、后端 Go 改动与 R4 增强仍为待实施，不能据此勾选整体验收完成。
