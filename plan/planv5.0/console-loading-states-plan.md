# SimpleBase v5.0 控制台加载态

## 原始需求

> 控制台里数据还在加载时，页面、表格、面板上看不到任何加载指示，直到数据回来之前就像空的。希望在数据尚未加载完成的地方，都有统一、好看的加载态。

---

> 日期：2026-10-09
> 范围：仅 `ui/` 控制台（页面、表格、详情面板、弹窗、页签、大盘、文档正文）。不含后端、不含 `packages/js-sdk`。
> 状态：**待实施**（本文件只做分析与方案，不含代码改动）
> 方法：对照 `main` @ `707b7ae` 通读 `ui/src/pages`、`ui/src/components`、`ui/src/stores`、`ui/src/composables`，以及 `ui/src/test/helpers.ts` 的组件桩。
> 编号：§1 现有原语与取数方式，§2 逐面盘点，§3 加载 / 空 / 错误三分，§4 组件与状态机，§5 分期与文件，§6 测试，§7 范围外。

---

## 0. 总览

数据加载并不是「全站零指示」。`Skeleton`、`Spinner`、`useAsyncAction` 和一批列表页的三行灰条已经在。缺的是一条所有数据区都遵守的规则，所以慢请求时用户看到的仍是空白、一行灰，或直接是「还没有数据」。

| # | 现象 | 代表位置 | 优先级 |
| --- | --- | --- | --- |
| 1 | 请求进行中稳定渲染空态文案 | 云沙盒列表后半段、云函数版本弹窗、云助手左栏与会话历史、定时任务选云函数、文档正文、厂商「未配置」 | **P0** |
| 2 | 有指示，但只是整行一条 `h-8` 灰条，或一行「加载表结构」 | 数据库 / 云函数 / 定时任务 / 用户 / 日志 / 对象存储 / 文档列表 / 大盘资源表 | **P1** |
| 3 | 刷新时表格保持旧数据，只有页头按钮转圈；个别刷新按钮连转圈都没有 | 各列表页刷新；云沙盒刷新、运行记录刷新 | **P1** |
| 4 | 失败被 toast 掉，区域落成空列表，看起来像「真的没有数据」 | `useAsyncAction` 的 `error` 无人渲染；版本弹窗 catch 后仍是「暂无版本」 | **P1** |
| 5 | 提交按钮已有 `Spinner`，保持即可 | `SbModal` 的 `confirm-loading`、登录、SQL 执行按钮、云函数保存 | 不改交互，只补缺口 |

**建议顺序**：共享状态机和骨架组件 → 会误报空态的 P0 表面 → 已有灰条的列表换成同一骨架并补错误态 → 缺转圈的提交按钮。

不新增依赖，不改 `package.json` / `vite.config.ts`。样式继续写 Tailwind class，颜色只用现有 token（`bg-muted`、`bg-background`），深色模式跟着主题走。

---

## 1. 现有原语与取数方式

### 1.1 已经有的加载原语

| 原语 | 位置 | 实际能力 |
| --- | --- | --- |
| `Skeleton` | `ui/src/components/ui/skeleton/Skeleton.vue` | 一块 `bg-muted animate-pulse`。无 `aria-busy`，无文案。深色下 `bg-muted` 会跟着主题变。 |
| `Spinner` | `ui/src/components/ui/spinner/Spinner.vue` | `Loader2Icon`，`role="status"`，`aria-label="Loading"`（英文）。`size-4 animate-spin`。 |
| `SbModal` | `confirm-loading` | 确定按钮里塞 `Spinner` 并禁用。这是提交态，不是数据区加载态。 |
| `Button` | `ui/src/components/ui/button/Button.vue` | **没有** `loading` prop。各页手写 `<Spinner v-if>`。`ui/` 视为本地 fork，本次不给 Button 加 prop。 |
| `useAsyncAction` | `ui/src/composables/useAsyncAction.ts` | `loading` 初值 `false`，只在 `run()` 里置 `true`。失败写入 `error` 并 `toast.error`，调用方几乎都不渲染 `error`。 |
| `SbEmptyState` | `ui/src/components/SbEmptyState.vue` | 空态。默认标题「暂无数据」。它不区分「还没请求完」。 |
| `Alert` | `ui/src/components/ui/alert` | 文档缺失、SQL 批量失败已在用。加载失败应复用它，而不是再做一个错误插画。 |
| 测试桩 | `ui/src/test/helpers.ts` | `Skeleton` → `<div class="skeleton" />`，`Spinner` → `<span class="spinner" />`。页面测试可以靠这两个 class 断言「占位出现了」。 |
| 减弱动效 | `ui/src/style.css` `@media (prefers-reduced-motion: reduce)` | 全局把动画压到约 0.01ms。骨架的 `animate-pulse` 会变成静止色块，仍然可见，不必再写一套动画。 |

`stores/` 里只有 `project.loading`（项目列表）带加载标志。`auth` 的 `bootstrap()`、`settings` 没有页面级 loading。业务列表的 `loading` 都是组件内 `ref`，没有跨页缓存。

### 1.2 数据怎么取

- 页面在 `onMounted` 里自己请求，并用 `watch(projectId)` 在切换项目时重拉。Pinia 不保管这些列表。
- 用 `useAsyncAction` 的页面：`Databases.vue`、`GoFunctions.vue`、`CronJobs.vue`、`Users.vue`。其余页面手写 `loading = ref(false)` + `try/finally`。
- 子面板自己请求：`CollectionPanel`、`SchemaPanel`、`KvPanel`、五个 KV 编辑器、`DocsArticle`、`SandboxDrawer`、各业务弹窗。
- 文档目录 `docCatalog` 来自构建期 `import.meta.glob`，进页面时已经在内存里。真正异步的是单篇 Markdown（`loadMarkdown`）和搜索索引（`searchDocs`）。

### 1.3 列表页现在的共同写法

`Databases.vue`、`GoFunctions.vue`、`CronJobs.vue`、`Users.vue`、`Logs.vue`、`S3Manager.vue` 都是：

```text
v-if="loading && !rows.length"  →  3 行，每行一个横跨全表的 Skeleton h-8
v-else-if="!paged.length"       →  SbEmptyState
```

页头刷新按钮在 `loading` 时换成 `Spinner` 并禁用。这能挡住「加载中点第二次」，但盖不住两件事：

1. `loading` 初值是 `false`。`onMounted` 里如果先 `await` 别的请求再置 `loading`，空态会在整个前半段都在。即便同步置位，空态条件也没要求「这次请求已经结束」。
2. 灰条是 `colspan` 里的一根横条，不像表格行，窄屏上尤其像空白。刷新且已有数据时，条件 `loading && !rows.length` 为假，表格完全静止，只剩按钮上的转圈。

`KvPanel.vue` 是同一模式的较好版本：`watch(projectId, { immediate: true })` 在 setup 里同步把 `loading` 置 true，骨架 5 行。空态仍是 `v-else-if="!rows.length"`，逻辑上依赖「loading 已经先为 true」。

---

## 2. 逐面盘点

图例：

- **误报空**：请求未结束时，稳定渲染「没有数据 / 未配置 / 没有正文」一类文案。
- **空白**：不渲染空态，也不渲染占位，区域是空的。
- **弱占位**：有字、转圈或一根灰条，但不占住内容高度，也不像最终布局。
- **按钮转圈**：仅操作按钮有 `Spinner`。
- **空态闸门正确**：空态写在 `v-else`（或 `!loading &&`）里，且 `loading` 在第一次渲染前已同步置 true。

### 2.1 路由页面

| 表面 | 文件 | 请求 | 加载中 | 空态会不会抢跑 |
| --- | --- | --- | --- | --- |
| 首页 | `pages/Home.vue` | 无 | 静态营销页 | 不适用 |
| 监控大盘 · 四张计数卡 | `pages/Dashboard.vue` | `load()`：`Promise.allSettled` 八个接口 | `loading` 时卡片内是图标位 + 两行骨架，这是全站最完整的一处 | `loading` 在 `load()` 入口同步置 true。首帧竞态外，不会把 `-` 画成空态 |
| 大盘 · 分位数 | 同上 | `metrics.summary` | 无样本且 `loading` 时三块 `h-16` 骨架 | 否则「— 暂无近 24 小时接口样本」。闸门是对的 |
| 大盘 · 趋势图 | 同上 + `TrendChart.vue` | `metrics.trend` | 无点且 `loading` 时一块 `h-44` 骨架 | 失败走 `trendFailed`，文案是「趋势加载失败」，这是全站唯一把失败和空数据分开的区域。其它页面应向它对齐 |
| 大盘 · 资源表 | 同上 | 五个 list | 3 行、每行一根 `h-8` 灰条 | `v-else` 直接渲染计数。加载中不会写「暂无」 |
| 数据库列表 | `pages/Databases.vue` | `api.databases.list` | 桌面 3 根灰条；`md:hidden` 三张 `h-28` 卡片骨架。刷新按钮有转圈 | 空态是 `v-else-if="!paged.length"`，未要求 settled。已有数据时刷新只转按钮 |
| 集合页签 | `components/databases/DataTabs.vue` | 不请求，只包 `CollectionPanel` | 无自己的状态 | 见集合面板 |
| 集合列表 | `components/databases/CollectionPanel.vue` | `api.db.collections`，`watch` immediate | 三根 100% / 66% / 50% 宽的骨架，表格先不渲染 | 空态在 `v-else` 的表里。闸门正确。骨架仍不像两列表格 |
| 表结构 | `components/databases/SchemaPanel.vue` | `api.databases.schema`。`loading` 初值 **true** | 一行 `text-sm`：「加载表结构」，没有省略号，没有骨架 | 空态「还没有表」在 `v-else`。不误报。视觉太弱，表单区在加载完之前整块消失，高度会跳 |
| SQL 结果 | `components/modal/SqlWorkModal.vue` | `api.sql.query` / `execute` / `batch` | 页脚按钮有 `Spinner`。结果区在 `running` 时仍是「执行后结果将显示在这里」 | 这句是闲置提示，不是「0 行」。0 行成功后才是「暂时未查询到数据」。加载中结果区没有占位 |
| 文档行 | `components/modal/DocumentListModal.vue` | `api.db.rows` | `loading && !paged.length` 时一行里两根骨架。刷新按钮有转圈 | 否则 `TableEmpty`「暂时未查询到数据」。与列表页同一类条件 |
| Key-Value 页 | `pages/KeyValue.vue` | 不直接请求 | 刷新按钮绑定 `KvPanel` 冒泡的 `loading` | 见 `KvPanel` |
| KV 列表 | `components/databases/kv/KvPanel.vue` | `SCAN` + `execBatch`，`watch` immediate | 5 行 `h-5` 灰条。「加载更多」按钮有转圈 | 空态「暂无 Key」在 `v-else-if`。首屏闸门靠 immediate。已有行时刷新不换骨架 |
| KV 详情元数据 | `components/databases/kv/KvDetailModal.vue` | `TYPE` / `PTTL` / 长度 | 徽标 `v-if="meta"`，meta 到达前头部是空的 | 编辑器自己有转圈。头部没有占位 |
| KV 编辑器 string / hash / list / set / zset | `components/databases/kv/editors/*.vue` | `GET` / `HGETALL` / `LRANGE` / `SMEMBERS` / `ZRANGE` | 居中 `Spinner`，内容在 `v-else` | 「暂无字段 / 列表为空 / 集合为空 / 暂无成员」不会在加载中出现。转圈只有 `py-6`，面板中间一大块空白 |
| 对象存储 | `pages/S3Manager.vue` | `api.s3.list`；上传另有 `uploading` 与 `Progress` | 3 根灰条。刷新、上传按钮有转圈 | 「暂无对象」走 `v-else-if="!paged.length"` |
| 云函数列表 | `pages/GoFunctions.vue` | `api.gofunctions.list` | 3 根灰条，刷新按钮有转圈 | 「还没有云函数」/「系统项目不支持云函数」走同一 `v-else-if` |
| 云函数版本 | `components/modal/GoFuncVersionsModal.vue` | `listVersions`，打开时 `watch` | **没有 `loading`**。列表为空就渲染「暂无版本」 | **误报空**，持续整个请求。失败同样 toast 后留着「暂无版本」（`GoFuncTestModal.test.ts` 已把失败断言成这句）。「设为生效」只 `:disabled="activating"`，按钮上没有转圈 |
| 云函数测试 · 版本栏 | `components/modal/GoFuncTestModal.vue` | 打开时 `listVersions` | 左栏在返回前没有任何按钮，也没有占位 | 空白，不是空态文案。发送按钮已有 `Spinner` |
| 云函数编辑器保存 | `components/modal/GoFunctionModal.vue` | create / saveVersion | 保存按钮有 `Spinner` | 不拉列表 |
| 定时任务列表 | `pages/CronJobs.vue` | `api.cronjobs.list` | 3 根灰条，刷新按钮有转圈 | 「还没有定时任务」走 `v-else-if` |
| 定时任务 · 选云函数 | `components/modal/CronJobModal.vue` | 打开时 `loadFunctions()`，无 loading 标志 | 下拉为空 | `gofunctions.length === 0` 时说明是「项目内还没有云函数，先去创建」。**加载期间误报空** |
| 运行记录 | `components/modal/CronJobRunsModal.vue` | `api.cronjobs.runs` | `loading && !runs.length` 时居中 `Spinner` | 空态在 `v-else-if`。闸门正确。刷新按钮只有 `RefreshCwIcon`，没有 `Spinner`。过滤无匹配是真空态（「没有匹配的记录」），应保留 |
| 云沙盒列表 | `pages/Sandboxes.vue` | 先 `capabilities`，再 `list` | `loading && !capabilities` 时一行「正在加载云沙盒…」。刷新按钮**没有** `Spinner`，只是禁用 | capabilities 返回且 `available` 之后、`list` 返回之前：`loading` 仍为 true，但模板已离开第一支，`records` 仍是 `[]`，**稳定渲染「还没有云沙盒」**。`available === false` 的「未配置云沙盒」是真空态，必须留在 settled 之后 |
| 云沙盒抽屉 · 文件 | `components/sandbox/SandboxDrawer.vue` | `files.list` / `read` | **没有 loading**。`entries` 为空就 `SbEmptyState`「目录为空」 | **误报空**。读文件时文本域保持上一次内容或空白，无占位。执行按钮把字改成「冷启动中…」，没有 `Spinner`。保存有 `saving` 禁用，没有转圈 |
| 云助手 · Agent 卡片 | `pages/AgentManager.vue` | `bootstrap()`：`loadModules` → `loadAgents` → `ensureThread` → `loadSchedules` | 空态条件是 `!loading && !agents.length`，加载中左栏是**空白**，没有骨架。刷新按钮有转圈 | `loading` 要等 `loadModules()` 结束、进入 `loadAgents()` 才置 true。模块请求期间 `loading === false` 且 `agents === []`，**稳定显示「还没有 Agent」** |
| 云助手 · 会话列表 | 同上 + `components/agent/ThreadSwitcher.vue` | `agentThreads.page` | 下拉在线程返回前只有 placeholder「选择会话」 | 无骨架。新建会话按钮不反映 `ensureThread` 进行中 |
| 云助手 · 历史消息 | 同上 + `ConversationView.vue` | `agentThreads.messages`。`selectThread` 先把 `chatMessages` 置 `[]` 再 await | 右栏立刻走空插槽 | **误报空**：「用 @ 点名左侧 Agent…」。流式发送本身已有 `statusText`（思考中 / 执行工具）和光标 `▍`，那是生成态，不要换成骨架 |
| 云助手定时 · 执行历史 | `components/ai/AgentScheduleModal.vue` | `loadRuns()`，无 loading 标志 | `!runs.length` 即「暂无执行记录」 | **误报空**。保存 / 立即执行按钮已有 `Spinner`。单条 `status === 'running'` 已有小转圈 |
| 日志 | `pages/Logs.vue` | `logs.list`，可选 `getRetention` | 3 根灰条。查询按钮有转圈。自动刷新走同一 `load()` | 「暂无匹配日志」走 `v-else-if`。有筛选时的「清除筛选」是真空态 |
| 用户 | `pages/Users.vue` | `api.users.list` | 3 根灰条，刷新按钮有转圈 | 「暂无用户」走 `v-else-if="!filtered.length"`。加载中若本地过滤结果为空，会先出骨架而不是空态，这是可接受的 |
| 使用文档 · 目录 | `pages/DocsWiki.vue` | 构建期目录，无请求 | 目录为空时是「未能加载文档」Alert | 这是构建失败，不是加载中。不要加骨架 |
| 使用文档 · 正文 | `components/docs/DocsArticle.vue` | `loadMarkdown` | **只有** `isLargeDoc(filePath)` 才把 `loading` 置 true，然后一块 `h-40` 骨架 | 普通文档 `loading` 保持 false，`raw` 在返回前是 `null`，`html` 为 `''`，模板走到「这篇文档没有正文」。**误报空** |
| 使用文档 · 搜索 | `components/docs/DocsSearch.vue` | `searchDocs` | `pending` 时「正在索引文档…」 | 「没有匹配的文档」在 `v-else-if="query.trim()"` 且非 pending。闸门正确。可把纯文字换成同一 `Spinner`，非必须 |
| 设置 · 模型 | `components/settings/SettingsPanel.vue` | `llmSettings.get`，`onMounted` | 无占位。表单立刻绑定 `settings.defaultsFor` 的本地默认值 | 不是空白，但是在服务端返回前把本地默认画成已保存的值 |
| 设置 · 厂商 | 同上 | `llmProviderCreds.list`。`credsLoading` **只写不读** | 卡片立刻渲染 | 未返回前 `configured()` 为 false，徽标是「未配置」。**误报状态**。保存按钮已有 `Spinner` |
| 设置 · 连接 | `components/settings/ConnectionPanel.vue` | 重置时 `apiKeys.list/create/revoke` | 保存 / 重置按钮无 `Spinner` | 不展示远端列表，无空态闪烁 |
| 项目切换器 | `components/GlobalProjectSwitcher.vue` | `project.loadProjects` | `CommandEmpty` 在 `store.loading` 时写「加载项目…」 | 「没有匹配的项目」只在非 loading。闸门正确。触发器在列表返回前显示 localStorage 里的当前 id，可接受 |
| 未选项目 | `components/ProjectScope.vue` | 不请求 | — | 「请先在右上角选择或创建一个项目」是真的未选项目，不是加载态 |
| 顶栏账号 | `components/UserMenu.vue`、`layouts/DefaultLayout.vue` | `auth.bootstrap()` → `auth.me` | 未登录不渲染菜单 | 无数据区。登录弹窗提交已有 `Spinner` |
| 登录 / 新建类弹窗 | `LoginModal`、`CreateProjectModal`、`CreateCollectionModal`、`UserFormModal`、`SandboxCreateModal`、`KvCreateKeyModal`、`KvTtlModal`、`DocumentKvModal` | 仅提交 | `confirm-loading` 或按钮内 `Spinner` | 无列表可闪 |

`ProjectScope` 用 `:key="store.id"` 包住页面。切换项目会整页重挂，加载态会再走一遍，不能靠「上次的行留着」跨项目。

### 2.2 不需要当成数据加载的东西

- 流式回复：`useAgentConversation` 的 `statusText` 与 `ConversationView` 的光标。
- 上传进度：`S3Manager` 的 `Progress`。
- Sonner 的 `loading-icon`（`ui/sonner/Sonner.vue`）是 toast 自己的图标。
- 路由 `sb-fade`（`DefaultLayout.vue`）只是页面切换淡入，不表示数据就绪。

---

## 3. 加载、空、错误必须分开

每个数据区只允许下面一种可见状态。判断放在一个组件里，页面不再各自写 `loading && !rows`。

| 状态 | 条件 | 画什么 | 禁止 |
| --- | --- | --- | --- |
| 安静等待 | 首次请求已发出，尚未超过延迟，且没有可展示的数据 | 区域 `min-h` 占住最终骨架的高度，`aria-busy="true"`，不放空态文案 | 不要渲染 `SbEmptyState` |
| 首次加载 | 超过延迟仍无数据 | 骨架（表 / 卡片 / 正文）或面板中央的 `Spinner` | 不要换成空态，也不要把旧项目的行留到新项目（`ProjectScope` 会重挂） |
| 刷新 | 已有成功数据，又一次请求进行中 | **留下当前行**。按钮 `Spinner` + 禁用。区域 `aria-busy="true"`，内容 `opacity-80` | 不要撤掉行再铺骨架，避免高度跳、数据闪没 |
| 空 | 请求已结束，无错误，长度为 0 | 现有 `SbEmptyState`，文案不动 | 不要在 pending 时出现 |
| 错误 | 请求已结束，失败，且没有可留的数据 | `Alert` + 错误文案 + 「重试」 | 不要画成「暂无版本 / 还没有云沙盒」。toast 可以保留，但不能是唯一反馈 |
| 错误但有旧数据 | 刷新失败 | 留下旧数据，只 toast | 不要清空列表来表示失败 |
| 闲置 | 用户还没触发（SQL 未执行、搜索框未输入） | 保持现在的提示句 | 闲置不是空，也不是加载 |

「筛选后 0 条」（日志、运行记录、沙盒状态过滤、用户角色过滤）是空，不是加载。条件是：请求已结束，且过滤结果为空。

延迟：占位**出现**延迟 160ms，**消失**不延迟。160ms 内返回的请求不闪骨架，也不闪空态。超过 160ms 才铺骨架。实现用 `setTimeout`，settle 时 `clearTimeout`。不要把空态也推迟 160ms，否则测试和慢眼睛都会看到空白后突然空态。

`useAsyncAction` 保持原样给提交动作用（成功 toast / 失败 toast）。列表加载改走下面的 `useLoadState`，避免一边改 composable 一边打断现有调用方。

---

## 4. 方案

### 4.1 为什么是这一套

全站已经是 shadcn 风格的 `Skeleton` + `Spinner` + `SbEmptyState`。再引入进度条库，或只在顶栏加一条 nprogress，都不会让表格在数据回来之前看起来像表格。根因是每个表面自己拼布尔条件。

所以只加三件业务组件（放在 `ui/src/components/`，不放进 `ui/src/components/ui/`），加一个 composable。视觉继续用 `bg-muted` 脉冲和现有转圈，深色模式不用新颜色。

### 4.2 `useLoadState`

`ui/src/composables/useLoadState.ts`。对象形状用 `interface`，不写 `any`。

```text
interface LoadState {
  pending: Ref<boolean>
  settled: Ref<boolean>
  error: Ref<string>
  hasData: Ref<boolean>
  showSkeleton: ComputedRef<boolean>   // pending && !hasData && 已过 160ms
  showEmpty: ComputedRef<boolean>      // settled && !pending && !error && !hasData
  showError: ComputedRef<boolean>      // settled && !pending && !!error && !hasData
  refreshing: ComputedRef<boolean>     // pending && hasData
  run: (task: () => Promise<boolean>) => Promise<void>
}
```

`run` 的 `task` 返回「这次是否有可展示数据」。`task` 自己负责写入行数组。`run` 负责 `pending` / `settled` / `error`，并用 `errorMessage(e, fallback)` 填 `error`。页面在 `setup` 里立刻 `void state.run(...)`（不要只放 `onMounted`），这样第一次渲染就是 pending，而不是空。

`showSkeleton` 的延迟常量导出为 `LOAD_PLACEHOLDER_DELAY_MS = 160`，测试用假定时器。

### 4.3 `SbTableSkeleton`

`ui/src/components/SbTableSkeleton.vue`。

- props：`rows`（默认 6）、`columns`（必填）。
- 渲染真实的 `TableRow` / `TableCell`，单元格里是 `Skeleton`，宽度按列循环 `92%` / `64%` / `40%`，高度 `h-4`，行距与数据行一致（约 `h-10`）。
- 表头由调用方保留。骨架只替换 `TableBody` 的数据行，列数对齐，加载完不会横向跳动。
- 骨架节点 `aria-hidden="true"`。忙状态由外层区域宣告，避免每一根灰条都被读出来。

这替换现在「一根 `h-8` 横跨 `colspan`」的写法。`Databases.vue` 的移动端卡片继续用三块圆角骨架，抽成同一组件的 `variant="cards"` 或调用方保留那三块 `Skeleton`，但必须套进同一套 `showSkeleton` 闸门。

### 4.4 `SbBlockSkeleton`

`ui/src/components/SbBlockSkeleton.vue`。给非表格：

| variant | 用途 | 形状 |
| --- | --- | --- |
| `lines` | 表结构、文档正文、设置表单 | 4–6 行不同宽度的 `h-4`，文档用 `min-h-40` |
| `cards` | 云助手左栏、厂商卡片、大盘计数卡 | 3 张 `h-24` 或调用方指定 `count` |
| `chart` | 趋势图 | 一块 `h-44 rounded-xl`，与现在大盘一致 |
| `spinner` | KV 编辑器、运行记录、小面板 | 区域 `min-h-28` 中央一枚 `Spinner`，比现在的 `py-6` 更高，避免「空盒子里一个小图标」 |

### 4.5 `SbAsyncRegion`

`ui/src/components/SbAsyncRegion.vue`。这是闸门，页面不允许再手写空态条件。

```text
props: showSkeleton, showEmpty, showError, refreshing, error, emptyTitle?, emptyDescription?, emptyActionText?
emit: retry, empty-action
slots: default（数据）, skeleton, empty（可选，默认 SbEmptyState）
```

渲染顺序：`showError` → `Alert`；`showSkeleton` → skeleton 槽；`showEmpty` → 空态槽；否则 default。`refreshing` 时仍渲染 default，根节点加 `aria-busy="true"` 和 `opacity-80`。

根节点：

- `aria-busy="true"` 当 `showSkeleton || refreshing`，否则 `false`。
- 骨架可见时加一条 `sr-only`「正在加载」。
- 不使用会反复朗读的 `aria-live`。

`Spinner` 的 `aria-label` 从 `Loading` 改为 `正在加载`（`ui/spinner/Spinner.vue` 一处）。这是无障碍文案，不改组件外观约定。

### 4.6 按钮

提交和刷新继续在按钮里放 `Spinner`，并 `:disabled`。不给 `Button` 加 `loading` prop。刷新在 `refreshing` 时也转，不只在首次加载时转。

### 4.7 布局与深色

- 骨架行数固定，区域 `min-h` 等于骨架高度，安静等待的 160ms 里也不把空态放出来，高度不跳。
- 只使用 `bg-muted`、`text-muted-foreground`、`bg-background/80`。不写新的 CSS 文件。
- `prefers-reduced-motion` 已由 `style.css` 处理。

---

## 5. 分期与文件

每一期都要能单独合并：共享组件先落地并有测试，页面按优先级接上。接上的页面删掉本地的 `loading && !rows` 分支，改为 `useLoadState` + `SbAsyncRegion`。

### 阶段 0 — 共享件

| 文件 | 动作 |
| --- | --- |
| `ui/src/composables/useLoadState.ts` | 新增 |
| `ui/src/components/SbTableSkeleton.vue` | 新增 |
| `ui/src/components/SbBlockSkeleton.vue` | 新增 |
| `ui/src/components/SbAsyncRegion.vue` | 新增 |
| `ui/src/components/ui/spinner/Spinner.vue` | `aria-label` 改为「正在加载」 |
| `ui/tests/useLoadState.test.ts` | 新增 |
| `ui/tests/loading-region.test.ts` | 新增，覆盖三个组件 |

### 阶段 1 — 会误报空或整段空白的表面（P0）

| 文件 | 改什么 |
| --- | --- |
| `ui/src/pages/Sandboxes.vue` | capabilities 与 list 分两段状态。list 未返回前是表骨架，不是「还没有云沙盒」。「未配置」只在 capabilities 已 settled 且 `available === false`。刷新按钮加 `Spinner` |
| `ui/src/pages/AgentManager.vue` | `bootstrap` 一开始就进入 pending。左栏用 `cards` 骨架。`selectThread` 在历史返回前不要把空文案露出来（保留旧消息，或显示 `lines` 骨架；切换中不要先 `chatMessages = []`） |
| `ui/src/components/agent/ConversationView.vue` | 增加 `historyLoading`（或由父级用 `SbAsyncRegion` 包住空插槽）。流式 `sending` 不动 |
| `ui/src/components/agent/ThreadSwitcher.vue` | 线程未 settled 时触发器显示「正在加载会话」，不要只留「选择会话」 |
| `ui/src/components/modal/GoFuncVersionsModal.vue` | 版本列表走 `SbAsyncRegion`。失败是 Alert + 重试，不再是「暂无版本」。`activating` 时按钮加 `Spinner` |
| `ui/src/components/ai/AgentScheduleModal.vue` | `loadRuns` 期间历史区是骨架或 `spinner`，不是「暂无执行记录」 |
| `ui/src/components/sandbox/SandboxDrawer.vue` | 文件列表、读文件内容有 `spinner` / `lines`。执行按钮加 `Spinner` |
| `ui/src/components/modal/CronJobModal.vue` | `loadFunctions` 完成前不要渲染「还没有云函数」 |
| `ui/src/components/modal/GoFuncTestModal.vue` | 版本栏在 `listVersions` 期间用 `lines` 骨架 |
| `ui/src/components/docs/DocsArticle.vue` | 任意文档在 `raw === null` 且无 `loadError` 时都算加载（去掉「只有大文档才 loading」）。普通文档不再闪「这篇文档没有正文」 |
| `ui/src/components/settings/SettingsPanel.vue` | 读 `credsLoading`：厂商卡在列表返回前用 `cards` 骨架，徽标不要先写「未配置」。模型表单在首次 `get` 返回前用 `lines` 骨架，不要把本地默认值画成已保存 |
| `ui/src/components/modal/SqlWorkModal.vue` | `running` 时结果区换 `SbTableSkeleton`（查询）或 `spinner`（执行 / 批量）。闲置句和「暂时未查询到数据」留在对应状态 |

### 阶段 2 — 已有弱灰条的列表（P1）

把正文换成 `SbTableSkeleton`，空态和错误改走 `SbAsyncRegion`。刷新时留住已有行。

| 文件 |
| --- |
| `ui/src/pages/Databases.vue`（含移动端卡片骨架） |
| `ui/src/components/databases/CollectionPanel.vue` |
| `ui/src/components/databases/SchemaPanel.vue`（「加载表结构」换成 `lines`，高度含下方表单占位） |
| `ui/src/components/modal/DocumentListModal.vue` |
| `ui/src/components/databases/kv/KvPanel.vue` |
| `ui/src/components/databases/kv/KvDetailModal.vue`（头部徽标位一条骨架） |
| `ui/src/components/databases/kv/editors/KvStringEditor.vue` |
| `ui/src/components/databases/kv/editors/KvHashEditor.vue` |
| `ui/src/components/databases/kv/editors/KvListEditor.vue` |
| `ui/src/components/databases/kv/editors/KvSetEditor.vue` |
| `ui/src/components/databases/kv/editors/KvZSetEditor.vue` |
| `ui/src/pages/Dashboard.vue`（资源表改列对齐骨架；卡片和趋势已合格，只换闸门，避免首帧 `loading === false` 时画出 0 和「暂无数据」） |
| `ui/src/pages/GoFunctions.vue` |
| `ui/src/pages/CronJobs.vue` |
| `ui/src/components/modal/CronJobRunsModal.vue`（刷新按钮补 `Spinner`） |
| `ui/src/pages/S3Manager.vue` |
| `ui/src/pages/Logs.vue` |
| `ui/src/pages/Users.vue` |
| `ui/src/pages/KeyValue.vue`（按钮已接 `KvPanel`，确认 `refreshing` 时也转） |

这些页面里现有的 `useAsyncAction` 可以留着做删除 / 创建的 toast，列表加载本身改 `useLoadState`。不要为了统一把 `useAsyncAction` 的签名改掉。

### 阶段 3 — 只缺按钮转圈的提交

数据区没有列表。只补 `Spinner` + 禁用。

| 文件 | 动作 |
| --- | --- |
| `ui/src/pages/Sandboxes.vue` | 启动 / 停止 / 删除进行中，该行按钮转圈（删除仍走 `ConfirmAction`） |
| `ui/src/components/settings/ConnectionPanel.vue` | 保存、重置 |
| `ui/src/components/modal/CronJobRunsModal.vue` | 「立即执行」进行中（若阶段 2 未补） |

### 阶段之间的依赖

阶段 1–3 只依赖阶段 0。阶段 1 与阶段 2 可以分 PR，但不要在阶段 2 之前让页面各自再写一套 `loading && !rows`。

---

## 6. 测试

命令：`cd ui && yarn test`。覆盖率阈值仍是语句 / 行 / 分支 / 函数各 95%（`ui/vite.config.ts`）。`src/components/ui/**` 不计入覆盖，因此 `Spinner.vue` 的文案改动不进分母。新建的 composable 和 `Sb*` 组件**会计入**，必须有测试。

当前基线：`plan/planv5.0/code-and-style-optimization-plan.md` §2.4 记录 functions **94.24%**，`yarn test` 已因阈值非零退出。本功能不负责把历史缺口补到 95%。验收是：相对实施前的基线，statements / branches / functions **不下降**；新文件自身四项都 ≥ 95%。

`helpers.ts` 会把 `Skeleton` / `Spinner` 桩成 `.skeleton` / `.spinner`。页面测试断言这两个 class 即可。`SbTableSkeleton` 自己的测试不要桩掉 `Skeleton`，确认每列都有 `data-slot="skeleton"`（或桩存在时的 `.skeleton` 数量等于 `rows * columns`）。不要桩 `Switch`。

### 6.1 状态机（必须）

`ui/tests/useLoadState.test.ts`，使用假定时器：

1. `run` 开始后、160ms 前：`showSkeleton === false`，`showEmpty === false`，`showError === false`。
2. 假时间推进到 160ms 且 promise 未完成：`showSkeleton === true`。
3. 在 160ms 之前 resolve 空数组：`showSkeleton` 从未为 true，`showEmpty === true`。
4. resolve 非空：`showEmpty === false`，`hasData === true`。
5. reject 且无旧数据：`showError === true`，`showEmpty === false`。
6. 已有数据时再次 `run`：`refreshing === true`，`showSkeleton === false`。
7. 刷新失败：`hasData` 仍为 true，`showError === false`（错误留给 toast / 调用方）。

### 6.2 区域组件

`ui/tests/loading-region.test.ts`：

- `showSkeleton` 时能找到骨架，文本不含「暂无数据」。
- `showEmpty` 时出现 `SbEmptyState` 的描述，找不到骨架。
- `showError` 时出现 `Alert` 与重试按钮，点击发出 `retry`，不出现空态标题。
- `refreshing` 时默认槽仍在，根节点 `aria-busy="true"`。
- 骨架槽内装饰节点 `aria-hidden="true"`，区域有「正在加载」。

### 6.3 页面（每个 P0 表面一条）

把对应 `api.*` mock 成「返回前不 resolve」的 Promise，挂载后断言：

| 用例 | 加载中必须看到 | 加载中不能看到 | resolve 空之后才看到 |
| --- | --- | --- | --- |
| `Sandboxes.test.ts` | 表骨架或「正在加载」 | 「还没有云沙盒」「未配置云沙盒」 | list 为空才「还没有云沙盒」；`available:false` 才「未配置」 |
| `AgentManager.test.ts` | 左栏骨架 | 「还没有 Agent」 | 空列表才出现该句 |
| 会话切换 | 历史骨架或保留上一线程，直到返回 | 「用 @ 点名」在 `selectThread` 的 await 期间 | 返回 `[]` 之后 |
| `GoFuncVersionsModal` | 骨架 | 「暂无版本」 | 空列表；失败是 Alert，不是「暂无版本」 |
| `AgentScheduleModal` | 历史占位 | 「暂无执行记录」 | 空 runs |
| `SandboxDrawer` | 文件占位 | 「目录为空」 | 空目录 |
| `CronJobModal` | 云函数下拉占位或禁用 | 「还没有云函数」 | 真的没有函数 |
| `GoFuncTestModal` | 版本骨架 | 空白侧栏被当成「没有版本」 | — |
| `DocsArticle` | 骨架（含非 large 文档） | 「这篇文档没有正文」 | 正文为空的文件 |
| `SettingsPanel` | 厂商骨架 | 「未配置」 | 列表返回且该厂商不在其中 |
| `SqlWorkModal` | 结果区骨架，按钮 `.spinner` | 加载中不出现「暂时未查询到数据」 | 0 行才出现 |

阶段 2 每个列表页加一条同样的「挡住 promise → 有 `.skeleton`、无空文案 → resolve `[]` → 才有空文案」。已有 `flushPromises()` 之后断言「暂无 / 还没有」的用例应继续通过：mock 立即 resolve，空态在 settle 后出现。若某用例在挂载的同一拍、请求尚未结束时就断言空文案，改成先挡住 promise，再断言占位。

阶段 3 断言按钮在 pending 时包含 `.spinner` 且 `disabled`。

### 6.4 回归

- `cd ui && yarn test`：相对基线，三项覆盖率不下降。
- `cd ui && yarn build` 通过。
- 不改 `vite.config.ts` 的阈值来「放行」。
- 浏览器里目视（实施阶段，不是本计划）：数据库列表、云沙盒、云助手、文档正文、设置厂商。慢网速下先看到骨架，快网速下不闪空态。深色下骨架是 `muted` 而不是一条白带。刷新已有数据时行不消失。

---

## 7. 范围外

- `pages/Home.vue` 静态首页。
- `ProjectScope` 的「请先选择项目」、文档目录的「未能加载文档」（构建期没有 markdown）、项目切换器已有的「加载项目…」。
- 登录态 `bootstrap`、全屏启动画面、路由进度条。不引入 nprogress 或任何新依赖。
- 云助手流式生成的「思考中 / 执行工具」和光标。
- S3 上传百分比（已有 `Progress`）。
- Monaco / Go 编辑器初始化。
- 空态文案、按钮文案、信息架构。本计划不改「还没有云函数」这类句子，只改它们何时出现。
- `Button` 增加 `loading` prop，以及 `ui/src/components/ui/` 里除 `Spinner` 的 `aria-label` 以外的改动。
- 把历史 functions 94.24% 补到 95%（那是 `code-and-style-optimization-plan.md` §2.4 的事）。
- 后端延迟、接口协议、`useAsyncAction` 的调用方签名。
- `DataTabs.vue` 只有一个页签，不单独做加载态。

---

## 8. 验收

1. §2 里标成 **误报空** 或 **空白** 的表面，在请求 settle 之前看不到对应空文案。
2. 表格首次加载使用列对齐的 `SbTableSkeleton`，不是一根横跨全表的灰条。
3. 已有数据的刷新不撤行；按钮有 `Spinner`。
4. 首次失败显示 `Alert` 和重试，不显示 `SbEmptyState`。
5. 160ms 内返回不出现骨架闪烁；超过 160ms 必有占位。
6. 区域在加载和刷新时 `aria-busy="true"`，骨架 `aria-hidden`，读屏文案是「正在加载」。
7. 深色模式下占位使用 `bg-muted`。`prefers-reduced-motion` 下占位仍在，只是不脉冲。
8. `yarn build` 通过；`yarn test` 的三项覆盖率不低于实施前基线。
