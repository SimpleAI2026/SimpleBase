# 控制台数据浏览、事件触发器与运维导航修复计划

> 日期：2026-10-27
> 状态：待实施；本文只包含分析与修复计划，不修改业务代码
> 范围：`ui/`、`internal/api`、`internal/catalog`、相关控制台文档与测试
> 基线：当前工作区源码。工作区已有大量未提交改动，实施时必须逐文件核对并保留现有修改，不回退、不覆盖、不自动提交

## 1. 目标与修复口径

| 原始问题 | 修复目标 | 主要改动层 |
| --- | --- | --- |
| 1. 数据库管理展开数据报 `invalid database name` | 合法项目可正常展开集合或 SQL 表；存储路径错误不再伪装成数据库名称错误 | 后端 catalog/API + 回归测试 |
| 2. 数据浏览命名、间距、数据库选择与导出 | 中文改为「数据浏览」；工具区留白和控件高度统一；数据库可切换且不被旧响应覆盖；界面只提供「导出 CSV / 导出 JSON」 | DataBrowser + 导出 API |
| 3. API 预览归入运维 | 侧栏从「数据」移到「运维」，文档归属同步；保留现有 URL 和功能 | NavMenu + 文档/测试 |
| 4. 事件触发器对齐定时任务 | 使用同样的卡片表格、右上角「刷新 / 新建」、加载/空/错误态和分页 | FunctionTriggers |
| 5. 新建触发器改为统一 Modal | 新建表单迁移到 `SbModal`，字段、校验、保存中和错误反馈与定时任务弹窗一致 | 新 FunctionTriggerModal |
| 6. 边框遮挡与尺寸不一致 | 消除嵌套滚动/边框裁切，统一控件高度、行列间距、窄屏换行和焦点环留白 | 本次涉及页面与弹窗 |

### 明确不做

- 不放宽数据库名称、项目 ID 或数据库路径校验，不重新允许名称代替数据库 UUID。
- 不增加旧数据库布局回退、请求时自动修复 `database_path`、隐式迁移或自动 reset。
- 不改变 `/console/api-preview` 路由、路由名、图标、OpenAPI 生成、试跑和确认逻辑。
- 不把「导出 JSON」做成 NDJSON 改名；JSON 导出必须是合法 JSON 数组。CSV/NDJSON 导入保持现状，既有 NDJSON 导出 API 与 SDK 保持兼容。
- 不为统一样式修改 `components/ui/` 全局风格，不新增依赖，不修改构建配置或覆盖率阈值。
- 不手工编辑 `internal/web/dist`。

## 2. 已确认的现状与根因

### 2.1 展开数据库时的报错不是普通的名称校验问题

前端调用链使用数据库 UUID：

- `ui/src/pages/Databases.vue` 将完整 `DatabaseItem` 传给展开面板。
- `ui/src/components/databases/CollectionPanel.vue:85-90` 使用 `props.database.id` 请求集合。
- `ui/src/components/databases/SchemaPanel.vue:182-185` 使用 `props.database.id` 请求 SQL 表结构。
- `ui/src/services/http-api.ts:987-995` 对 database ID 做 URL 编码，并未把数据库名称拼进路径。

后端真正的差异发生在展开数据时的 Acquire 阶段：

1. `internal/api/data_handler.go:631-661` 先按 database ID 取 catalog 记录，再 Acquire。
2. `godatabase/registry/registry.go:81-88` 在用户库缺少运行时路径时调用 `PathResolver`。
3. `internal/dbbridge/dbbridge.go:57-62` 转到 `catalog.Service.ResolveProjectStorage`。
4. `internal/catalog/service.go:66-72` 从 `sys_projects` 补读并校验 `database_path`。
5. `internal/catalog/project_storage.go:21-29` 将路径错误包装为 `ErrInvalidName`，最终由 `internal/api/error.go:171-173` 映射成 `invalid_database_name`。

因此，在数据库列表已经能加载的前提下，展开时才出现的该错误应优先按**项目存储元数据/路径校验失败**排查，而不是修改前端名称规则。当前本地项目文件位置为规范的 `data/3d14mvm5/database.duckdb`，还需在实施第一步只读核对对应 `sys_projects.database_path` 是否为空、为旧格式或与项目 ID 不一致。

现有测试没有覆盖 `ResolveProjectStorage` 的非法路径与数据展开错误分类，这是回归缺口。

另一个架构边界是当前版本每个普通项目最多一个默认数据库。此次只修复选择器的加载、切换和竞态逻辑，不恢复多默认数据库能力；两个以上数据库仅作为组件兼容/回归输入，真实环境验收不得为此绕过 `HasDefaultDatabase` 限制。

### 2.2 数据浏览页的加载链路存在状态和竞态缺口

`ui/src/pages/DataBrowser.vue` 当前有三个 immediate watch：

- `[projectId, databaseId, reloadToken]` 加载集合；
- `[projectId, collection]` 加载文档；
- `projectId` 加载数据库。

问题：

- 数据库、集合、文档请求各自独立，没有旧响应失效机制；快速切换数据库或项目时，较慢的旧响应可覆盖新选择。
- `loadDatabases` / `loadCollections` 没有完整错误状态，选择器失败时难以区分「无数据库」和「加载失败」。
- 数据库列表包含未 ready 的集合库，选择后可能直接请求不可用资源。
- 工具栏「刷新」只刷新文档，不刷新数据库和集合，无法反映新建或变更的资源。
- 测试中的 Select 桩固定发出 `info`，没有真实选择两个数据库、乱序响应和项目切换的断言，因此现有测试无法防住该问题。

### 2.3 JSON 导出需要后端新增格式，而非只改文案

- 页面当前按钮为 CSV / NDJSON：`ui/src/pages/DataBrowser.vue:13-14`。
- UI 契约只接受 `'csv' | 'ndjson'`：`ui/src/services/types.ts:1193`。
- 后端 `internal/api/collection_transfer.go:383-416` 也只支持这两种格式。

为保留兼容性，采用「后端新增 `json`，前端默认展示 CSV / JSON」而非删除 NDJSON 能力。

### 2.4 API 预览不需要移动业务代码

API 预览由 `ui/src/pages/ApiPreview.vue`、`api.preview.*` 和 `/v1/openapi.json` 构成，彼此不依赖侧栏分组。只需：

- `ui/src/components/NavMenu.vue` 将 `api-preview` 从 data 组移到 ops 组；
- 文档把 API 预览从数据概览移到运维概览；
- 更新菜单顺序测试。

路由 `ui/src/router/index.ts`、页面、服务层、后端 OpenAPI 代码均保持原处。

### 2.5 事件触发器与定时任务的差异

定时任务 `ui/src/pages/CronJobs.vue` 已有：`ProjectScope → PageContainer → Card`、卡片标题与数量、右上角操作、表格、`useLoadState`、`usePagination`、`SbAsyncRegion`、底部 `TablePager`。

事件触发器 `ui/src/pages/FunctionTriggers.vue` 当前是裸工具栏 + 页内创建表单 + 表格，且：

- 云函数只在 `onMounted` 加载，项目切换时可能保留旧项目函数；
- 创建、启停、调用、删除的 pending 与错误处理不完整；
- admin 项目没有像定时任务一样提供明确的不支持空态，可能直接展示后端 403；
- 没有统一 Modal 和分页。

后端 `internal/api/function_trigger_handler.go` 已提供列表、创建、更新、删除、调用及系统项目保护，本需求无需改触发器 API。

## 3. 分步实施方案

### P0：先补复现与表征，锁定数据库展开问题

1. 在不动数据的前提下复现并记录失败请求：集合库对应 `GET .../data/collections`，SQL/系统库对应 `GET .../schema`；保存 HTTP 状态和 request ID，不记录 SQL 参数或数据库物理路径。
2. 只读核对失败项目的 `sys_projects.database_path` 与实际项目文件是否同属 `{projectID}/database.duckdb|sqlite`。若实际报错并非该链路，先根据 request ID 回到 handler 重新定位，不直接套后续修复。
3. 在 `internal/catalog` 增加服务层用例：合法 DuckDB/SQLite 路径可解析；空路径、跨项目路径、旧格式路径 fail closed；系统项目空路径仍合法。
4. 在 `internal/api` 增加数据展开回归：合法项目能够进入 Acquire；非法项目存储元数据不返回 `invalid_database_name`，且不泄漏物理路径。
5. 增加领域错误（建议 `catalog.ErrInvalidDatabasePath`）并在 `ResolveProjectStorage` 的路径校验边界转换，API 映射为明确的 `invalid_database_path`。不要全局修改 `wrapInvalid`，以免同时改变创建项目时的引擎/名称错误语义。
6. 保持 `godatabase` 的路径规则和磁盘契约不变，不在 registry/filedb 增加 fallback。
7. 若当前开发实例确实残留非法元数据，按项目既有备份、重建/reset 流程单独处理；执行任何破坏性操作前另行确认。本需求不自动修改或清除现有数据。

### P1：实现 JSON 导出并保持 NDJSON 兼容

1. `internal/api/collection_transfer.go` 接受 `format=json`，默认无 format 仍为 NDJSON；非法格式提示同步包含 json。
2. 新增标准 JSON 数组 writer：流式写 `[`、逗号分隔的文档、`]`，空结果输出 `[]`，不一次性把全表收集到内存。
3. 复用现有行扫描、`flattenDocument`、filter、sort、fields 和 hidden 字段脱敏规则，不另造一套文档语义。
4. 响应使用 `application/json; charset=utf-8`，`Content-Disposition` 文件扩展名为 `.json`。
5. DuckDB 与 SQLite 测试均断言：合法 JSON 数组、顺序、字段选择、过滤、隐藏字段不泄漏、空数组、响应头；既有 NDJSON/CSV 用例保持不变。
6. 前端 `Api.db.exportCollection` 的导出 format 联合类型增加 `'json'`，页面调用与下载文件名使用 json；import 的 format 联合类型不变。

### P2：修复数据浏览页状态、布局与导出

1. 文案：仅将 `nav.dataBrowser` 中文改为「数据浏览」，英文保留 `Data Browser`；路由名、URL 和其他 key 不变。
2. 将初始化与刷新整理为明确的「项目 → 数据库 → 集合 → 文档」链路：
   - 项目变化先清空旧项目状态，再加载 ready 且 `dataModel=collection` 的数据库；
   - 有可选数据库时保留仍存在的当前选择，否则选择第一项；无则清空集合并展示明确空态；
   - 数据库变化清空旧集合、schema、行、草稿、列显隐和分页，再加载集合并默认选择第一项；
   - 集合变化才加载 schema 和文档。
3. 用请求序号或等价上下文校验丢弃旧项目、旧数据库、旧集合的迟到响应；响应写回前同时核对 project/database/collection 上下文。
4. 数据库选择使用有类型的 update handler，值始终是 database ID，显示名称为 name；加载中禁用，失败可重试，空列表不渲染假选项。
5. 「刷新」重新加载当前项目的资源链，而不只重新查询当前页文档；保留当前仍可用的 database/collection 选择，刷新期间不清空已有内容造成闪烁。
6. 为数据库、集合和文档分别提供可理解的加载/空/错误反馈，复用 `SbAsyncRegion` / `SbErrorState` / `SbEmptyState`，不用未捕获 Promise 或 toast 代替区域状态。
7. 布局改为与全站列表页一致的卡片工具区：
   - 工具区使用 Card/CardHeader 或等价的 `p-4 sm:p-5` 容器，数据库选择、刷新、Schema、视图、导入、导出之间统一 `gap-3`；工具区与下方内容保持 `gap-4 sm:gap-5`；
   - SelectTrigger 与按钮统一 default 尺寸（现有均为 `h-8.5`），不要混用 sm/default 造成高度不齐；
   - 数据库选择窄屏 `w-full`、较宽屏 `sm:w-64`，工具组允许 `flex-wrap`，文本长时以 `min-w-0` 防撑破；
   - 外层页面内边距仍只由 `--page-pad` 提供，不在页面复制全局 padding。
8. 集合侧栏、查询区、列显隐区和表格使用一致卡片边框与内容 padding；集合为空时显示专门空态，而不是空白侧栏。
9. 表格只保留一个横向滚动责任方：优先使用 `Table` 自带容器，去掉外层重复的 `overflow-x-auto`；卡片承载圆角和外边框，避免双重边框及焦点环裁切。
10. 按钮改为「导出 CSV」「导出 JSON」，继续沿用当前 filter/sort；i18n 中文和英文同步更新，移除不再使用的 `exportNDJSON` key。

### P3：API 预览移动到运维

1. `NavMenu.vue` 调整为：data = databases、data-browser、key-value、s3；ops = api-preview、logs、users。
2. `docs/data/index.md` 删除 API 预览条目，同时将「数据浏览器」改成「数据浏览」，导入导出说明改成「CSV/NDJSON 导入，CSV/JSON 导出」。
3. `docs/ops/index.md` 增加 API 预览入口、URL 和既有写请求确认说明；不新增重复文档页，不改变六大文档模块。
4. 更新 `ui/tests/simple-components.test.ts` 的菜单顺序和分组断言；运行文档内容与顺序测试，确认链接和模块归属一致。

### P4：事件触发器列表对齐定时任务

1. 页面结构改成与 CronJobs 相同的 `ProjectScope → PageContainer → Card`。
2. CardHeader 包含「触发器列表」、数量和说明；CardAction 右侧顺序固定为「刷新」后「新建触发器」，使用 RefreshCw/Plus/Spinner 和相同 size/gap。
3. 表格列建议为：事件类型（含 events）、目标（含 filter 摘要）、云函数、启用、操作；长标识符使用等宽字体和必要截断，操作列按钮风格与 CronJobs 一致。
4. 接入 `useLoadState`、`usePagination`、`SbAsyncRegion` 和 footer 版 `TablePager`；空态提供新建入口，错误态使用统一重试。
5. admin 项目显示「系统项目不支持事件触发器」空态，隐藏新建、启停、调用和删除，不发送必然 403 的列表请求；普通项目写入口同时受 `auth.canWrite` 控制。
6. 启停、调用、删除补齐 try/catch、行级 pending 与 toast；失败不回滚成错误视觉状态，成功后按现有语义刷新或局部更新。
7. 项目切换重新加载触发器，并使旧项目响应失效。

### P5：新建触发器迁移到统一 Modal

1. 新增 `ui/src/components/modal/FunctionTriggerModal.vue`，页面只维护 `modalOpen` 和 saved 后刷新；删除页内创建表单。
2. 使用 `SbModal`、`maxWidth=720`、标准 footer、确认 loading 和不可提交禁用逻辑；正文用 `FieldGroup` / `Field` / `FieldLabel` / `FieldDescription` / `FieldError`。
3. 保留现有输入契约：kind、target、events、funcFile、funcExport、filter、enabled。每次打开重置为默认值，取消不保存，成功后关闭、toast 并刷新。
4. 根据 kind 处理字段：HTTP 清空并隐藏/禁用 target；record/object 必须填写合法 target；events 去空格并移除空项；filter 可选但保留后端校验错误。
5. 弹窗打开时按当前 project 加载云函数，只列出有活动版本的可执行函数；函数变化联动 export 选项。加载中、失败重试、无已发布函数均沿用 CronJobModal 的反馈方式。
6. 项目切换或快速重复打开时忽略旧函数列表响应，不能把其他项目函数提交给后端。
7. 保留 `SbModal` 正文的 `px-1 -mx-1` 焦点环空间；弹窗内并排字段使用 `flex flex-wrap`、主输入 `min-w-0 flex-1`，不添加第二层不必要的横向滚动。
8. 将新建表单的 i18n 放到 `components.functionTriggerModal`，中英文 key 同构；列表新增文案放在 `pages.functionTriggers`。

## 4. 样式专项检查（对应问题 6）

实施时逐页检查，不用一次全局替换：

- **边框**：卡片只有一层外边框；表格外不再叠加同尺寸圆角边框；分隔线使用 CardHeader border-b / TablePager border-t。
- **滚动**：横向滚动只由 Table 容器负责；Modal 纵向滚动只由 SbModal body 负责；避免父级 `overflow-hidden` 裁切 Select 内容、focus ring 或错误提示。
- **高度**：同一工具行的 Select、Input、Button 使用同一尺寸档位；表头/单元格沿用全局 h-11、px-4/py-3，不在页面另造行高。
- **间距**：页面块级间距、卡片 padding、工具组 gap 各自统一；相邻组件上下不重叠，左右操作组基线一致。
- **响应式**：375/768/1024/1440px 检查长数据库名、函数名、filter 和按钮组；需要换行的容器必须有 `flex-wrap` 和 `min-w-0`，不得出现横向页面溢出。
- **交互状态**：键盘 focus-visible 环完整可见；loading/disabled 不改变控件高度；浅色/深色下边框、选中、错误文字均可辨识。

## 5. 测试计划

### 后端

- `internal/catalog`：路径解析合法/非法矩阵、系统项目例外、错误类型。
- `internal/api`：展开数据不再把存储路径错误报告为名称错误；合法集合与 SQL 展开不被新错误边界影响。
- `collection_transfer_test.go`：DuckDB/SQLite 的 JSON 数组导出、空结果、filter/sort/fields、hidden 字段与响应头；CSV/NDJSON 原用例继续通过。
- `error_test.go`：新领域错误的 HTTP/code/message 映射，且不包含物理路径。

### 前端

- `DataBrowser.test.ts`：
  - 至少两个数据库之间真实切换，断言后续 collection/documents 请求使用新 ID；
  - 用可控 Promise 让旧请求后返回，断言旧响应不会覆盖当前选择；
  - 项目切换、无 ready 集合库、数据库/集合加载失败与重试；
  - 刷新完整资源链且保留有效选择；
  - CSV 与 JSON 按钮分别调用正确 format、查询参数和 `.csv` / `.json` 文件名，不再出现导出 NDJSON。
- `http-api.test.ts`：json 导出请求参数和 Blob 返回，import 契约不变。
- `FunctionTriggers.test.ts`：卡片头按钮位置/存在性、分页、空态新建、admin 不请求且隐藏写操作、项目切换、启停/调用/删除成功与失败。
- 新 `FunctionTriggerModal.test.ts`：打开重置、kind/target 联动、函数与 export 联动、无函数/加载失败、校验、保存中、取消、成功关闭、失败保留表单、旧项目响应失效。
- `simple-components.test.ts`：API 预览只在运维组，菜单顺序更新。
- `loading-surfaces.test.ts`：事件触发器纳入统一加载/空/错误表征。
- `modal-input-layout.test.ts`：新增触发器弹窗的换行、min-w-0 和焦点环约束；交互测试使用真实 Switch/reka 契约，不用失效桩假通过。
- `i18n.test.ts`、文档内容与 `docs-order.test.ts`：中英文 key、文档链接和模块顺序。

## 6. 验证命令与端到端验收

### 命令

```bash
# 聚焦回归
go test ./internal/catalog ./internal/api
cd ui && yarn test --run tests/DataBrowser.test.ts tests/FunctionTriggers.test.ts tests/FunctionTriggerModal.test.ts tests/simple-components.test.ts tests/http-api.test.ts tests/modal-input-layout.test.ts tests/loading-surfaces.test.ts tests/i18n.test.ts tests/docs-order.test.ts

# 全量门禁
go build ./internal/... ./cmd/...
go test ./...
cd ui && yarn build && yarn test
```

若全量命令被已知、未触及的失败阻塞，必须记录原始失败与聚焦回归结果，不得宣称全量通过。

### 真实场景

1. 普通项目分别展开集合库和 SQL 库，均不再出现 `invalid database name`；非法存储元数据得到明确且脱敏的错误。
2. 数据浏览可选择当前 ready 集合库，刷新后资源状态正确；组件测试中的多数据库快速切换后，集合与文档始终属于最后选择。
3. CSV 可正常打开；JSON 可由标准解析器直接解析为数组，且与当前筛选/排序一致；NDJSON 旧 API 请求仍成功。
4. API 预览只出现在运维，旧 URL 直接访问仍正常，文档入口不重复。
5. 事件触发器与定时任务的卡片头、表格、分页、空/错/加载态一致；刷新和新建位于右上角。
6. 新建触发器在 Modal 内完成 record/object/http 三类配置；取消不保存，失败可修正后重试，成功立即出现在表格。
7. admin 与只读角色没有写入口；项目切换后不会提交旧项目云函数。
8. 四种视口和浅色/深色模式下，无双重边框、边框/焦点环遮挡、控件高度不齐或页面横向溢出。

## 7. 预计文件清单与实施顺序

主要文件：

- 后端：`internal/catalog/errors.go`、`service.go`、相关测试；`internal/api/error.go`、`collection_transfer.go`、相关测试。
- 数据浏览：`ui/src/pages/DataBrowser.vue`、`ui/src/services/types.ts`、`http-api.ts`、语言包与测试。
- 导航/文档：`ui/src/components/NavMenu.vue`、`docs/data/index.md`、`docs/ops/index.md`、相关测试。
- 触发器：`ui/src/pages/FunctionTriggers.vue`、新 `ui/src/components/modal/FunctionTriggerModal.vue`、语言包与测试。

推荐顺序：**P0 数据库根因与测试 → P1 JSON API → P2 数据浏览 → P3 导航/文档 → P4/P5 触发器 → 样式专项与全量验收**。每一步先跑聚焦测试再进入下一步，避免把后端错误、状态竞态和纯样式问题混在一次不可定位的修改中。
