# SimpleBase 前端约束（ui/）

Vue 3 + TypeScript + Vite 控制台。技术栈固定：Pinia、vue-router、Tailwind CSS v4、reka-ui（shadcn-vue 风格）、vue-sonner、axios、ECharts（图表）、Monaco Editor（SQL/Go 编辑器）、marked + DOMPurify（文档渲染）。面向使用者的说明见 [README.md](./README.md)。

## 目录职责

| 目录 | 职责 | 约束 |
| --- | --- | --- |
| `src/pages/` | 路由页面（Home / Dashboard / Databases / DataBrowser / ApiPreview / KeyValue / S3Manager / GoFunctions / FunctionTriggers / CronJobs / AgentManager / Logs / Users / DocsWiki） | 页面只做数据编排与布局；新页面必须在 `router/index.ts` 注册并配 `meta.titleKey`，需角色控制的配 `meta.requiresRole`（如 Users 的 `superadminl1`）。全局设置不是全页，见 `SettingsModal` |
| `src/components/ui/` | 基础组件库（button / table / dialog / select 等，shadcn 风格） | **视为本地 fork 的库代码**：只增删组件不改风格约定；删除组件前必须全局确认零引用 |
| `src/components/` | 业务组件：`databases/` `modal/` `ai/` `agent/` `chat/` `cronjob/` `sandbox/` `editor/` `settings/` `docs/` `bi/` 分域，加顶层通用件（ConfirmAction / NavMenu / PageContainer / TablePager / SettingsModal / GlobalProjectSwitcher / TrendChart / SbAsyncRegion / SbEmptyState / SbErrorState / SbTabsPanel / SbCodeBlock / UserMenu 等） | 命名 `PascalCase.vue`；业务组件不得反向被 `ui/` 依赖。`bi/` 是数据看板（planv6.7）：**只有 `BiChart.vue` 直接使用 echarts**，其余组件只传 option 或渲染描述；图表构造、类型兼容、自动推断、筛选合成全在 `utils/bi-*.ts` 纯函数里 |
| `src/services/` | API 层：`api.ts`（固定导出 `httpApi`）→ `http-api.ts`（按域组装 `http-api/{platform,data,agent}.ts`）→ `http.ts`（axios 实例、token 存取、401 处理）→ `types.ts`（契约） | 所有请求只经 `api.*`；页面禁止直接 import axios 或 `http-api` |
| `src/stores/` | Pinia store（auth 登录态与角色 / project 项目切换 / settings） | 跨页面状态才进 store；组件内状态用 `ref` |
| `src/composables/` | 组合函数（usePagination / useLoadState / useLocale / useBiDashboard / useBiQuery / useBiTheme / useBiDataSource / useBiDetail / useAgentConversation / useAgentCollaboration / useAgentWorkspace / useActiveRunsPoll / useAgentTagEditor / useStarterThread / useThreadNavigation / useAgentDirectory / useAgentSender / useAgentRunLayer / useHumanTasks / useThreadCapabilities / useWorkspaceView / useQueryPatch / useStarterLaunch / useApiParamOptions / useCanWrite / useProjectReload / useFilterChips / useKvEditor） | 在组件外取文案用 `@/locales` 导出的 `t()`，不要在 composable 里调 `useI18n()` |
| `src/locales/` | i18n 文案源（`zh-CN.ts` 权威树、`en-US.ts` 同构、`index.ts` 实例与 `setI18nLocale`） | 唯一文案来源；不得在别处新建语言包 |
| `src/layouts/` | DefaultLayout（`/console` 控制台）/ DocsLayout（`/docs` 文档站）；Home 页独立于两布局 | — |
| `src/docs/` | 文档目录、按需加载 / 搜索、Markdown 安全渲染 | 文档正文来自仓库根 `docs/`；新增文档遵循 `docs/README.md` 并通过 `ui/tests/docs-content.test.ts` 的链接、锚点与 frontmatter 校验；大型上游镜像不要进首页 chunk |

## API 层规则

- 接口契约集中在 `services/types.ts`（桶文件，实体按域拆在 `services/types/{platform,data,agent,api}.ts`，全仓仍从桶文件引入）；后端字段用 snake_case 转 camelCase 的映射函数（`toProjectItem` 风格，位于 `services/converters/`），不透传 raw。项目级路径只经 `services/paths.ts` 的 `projectPath()` 构造，列表读取经 `services/wire.ts` 的 `listOf()`。
- 路径构造函数模式：`xxxPath(projectId, …)` 统一 `encodeURIComponent`。
- 运行时 mock 层（`mock.js` / `mock-kv.js` / `VITE_USE_MOCK` / `isMock` 徽标）已于 v4.0 退役，**不得重新引入**；新增接口只需改 `types.ts` + 对应域的 `http-api/{platform,data,agent}.ts`，单测桩统一走 `src/test/api-mock.ts`。

## admin（系统）项目规则

`stores/project.ts` 中 `ADMIN_PROJECT_ID`（= 后端 `ReservedSystemProjectID`）、`isAdmin` getter 是唯一判定入口：

- `Databases.vue`：admin 下**隐藏**新建数据库按钮与删除按钮，**保留**「查看数据」（展开集合）与「SQL」（只读查询）入口。
- `CollectionPanel` / `DocumentListModal` / `SqlWorkModal` 通过 `readonly` prop 进入只读模式：隐藏写按钮、SQL 仅显示「查询」模式。
- 前端只读是**体验层**的；真正的保护在后端（`ErrSystemProtected`），前端不得假设后端不校验。

## 登录态与角色（login-auth-plan）

- `stores/auth.ts` 是登录态唯一入口：JWT access/refresh 自动轮换（`services/http.ts` 存取）；未登录可继续用 API Key（SettingsModal 设置）。
- 角色 `superadminl1` > `admin`（只读管理员）> `user`；判定一律用 getter（`isSuper` / `isAdminRole` / `isUser` / `canWrite` / `canManageUsers` / `canViewUsers`），不得散落比较 role 字符串。
- `canWrite`：未登录（API Key 通道）沿旧行为可写；登录态下仅 superadminl1 与 user 可写，admin 只读。
- 401 统一由 auth store 记录 `lastUnauthorizedAt` 并打开 `LoginModal`（含 `must_change_password` 强制改密流程）。
- 路由级角色控制用 `meta.requiresRole`（Users 页 `superadminl1`）。

## 多语言（i18n）

- 依赖为 `vue-i18n`（Composition API，`legacy: false`）。语言包是 `.ts` 静态导入，**不做懒加载**、不引入 `@intlify/unplugin-vue-i18n`。
- 语言只有 `zh-CN` 与 `en-US` 两种，**默认中文且不做浏览器探测**；语言状态存在 `stores/settings.ts` 的 `locale` 字段（与 `theme` 同一份 localStorage，`sb_settings_v1`），`setLocale()` 同步 `document.documentElement.lang`。
- 组件内用 `useI18n()`；**组件外**（`services/http.ts`、`utils/*`、`composables/*`、Pinia getter）一律用 `@/locales` 导出的 `t()`（即 `i18n.global.t`）—— `useI18n()` 在无组件上下文处会抛错。
- 「key 或已是文案」的兜底入口（`useLoadState({ fallback })`、`errorMessage(e, fallback)`）传 i18n key 即可，内部经 `resolveText()` 解析；传后端裸文案则原样保留。
- 带插值的文案用命名参数（`t('agentPhase.done', { secs, tokens, calls })`），不要在代码里拼接中文/英文片段。
- 以下**不属于**待迁移文案：代码注释、`services/http-api/*.ts`、`constants/llmProviders.ts` 的服务商名、`src/docs/*`（文档正文）、`src/components/ui/**`（fork 库代码，如 `DialogContent` 的 `Close`）。
- key 用命名空间点号：`common.*` / `nav.*` / `status.*` / `logLevel.*` / `cron.*` / `error.*` / `pages.<page>.*` / `components.<component>.*`。新增文案必须**同时**补 `zh-CN.ts` 与 `en-US.ts`，两棵树 key 集合必须完全一致（`ui/tests/i18n.test.ts` 会卡）。
- 路由标题走 `meta.titleKey`（不是 `meta.title`），由 `NavMenu` 与 `DefaultLayout` 的 `t()` 消费。
- 禁止在 `v-for` 模板里现拼 key；列表项文案用 `computed` 预算或绑定常量 key。
- 组件默认值（`SbModal` 的 `okText` / `cancelText`、`ConfirmAction` 的 `description`、`SbEmptyState` 的 `title` / `description`）走 `common.*`，但**必须保留 props 覆盖能力**（已翻译文案仍可由调用方传入）。
- 后端返回的自由文本不翻译：`agent-errors.ts` 只把已知错误码映射到 `error.<code>`，未知码回退后端原始 `message`。
- 不为 i18n 改 `vite.config.ts` / `tsconfig.json`，也不下调覆盖率阈值。

## 样式与 UI 约定

- Tailwind v4，样式写在 template 的 class 中；不写自定义 CSS 文件（全局仅 `style.css` 的主题 token）。
- 已定稿的规范不得回退：
  - 侧边栏菜单项 `h-11`（sm 下 `h-9`）、间距 `gap-1.5`；
  - 主内容区铺满全宽（不加 `max-w-*` 限制）；
  - 右上角顺序：使用文档 → 项目切换器（`w-56`）→ 设置（planv6.7 R4 已移除顶栏「刷新页面」，刷新只在各列表卡片头 `SbRefreshButton`）；
  - 页面标题只用 PageContainer 的 subtitle 单行（不加 h2 大标题）。
- **状态样式必须匹配 reka-ui 2.10.5 的真实属性**：它输出 `data-state`（`checked`/`unchecked`、`active`/`inactive`、`open`/`closed`）与 `data-disabled`/`data-highlighted`/`data-selected` 等，**没有**裸 `data-checked`/`data-unchecked`/`data-open`/`data-closed`。写 `data-[state=checked]:…`，不要写 `data-checked:…`；后者编译出的选择器永不命中，开关会「点了没反应」且无报错（`Switch` / `Dialog*` / `PopoverContent` / `SelectContent` / `Sheet*` / `TooltipContent` / `Combobox*` 都在这个坑里，见 planv5.0 §4.1）。
- **`data-active` 是唯一例外**：reka 的 `RovingFocusItem` 会给选中项渲染**空属性** `data-active`（`node_modules/reka-ui/dist/RovingFocus/RovingFocusItem.js`），所以 `TabsTrigger` 上写 `data-active:` 本来就能命中（`ui/tests/ui-state-attributes.test.ts` 已断言选中项 `data-active === ''`）。新代码仍统一写 `data-[state=active]:` 保持一致性；**不要**据此认为 `data-active:` 失效。
- 组件测试**不要桩掉 `src/components/ui/` 的 reka 交互组件**：`Select / Switch / Tabs / Popover / Command / Combobox / ToggleGroup / Slider / Dialog / Sheet / AlertDialog` 在 `src/test/helpers.ts` 里**已全部移除桩**，`uiStubs` 只保留展示类与布局类桩（Card / Table / Field / Sidebar 等）和应用层壳（`SbModal` / `ConfirmAction` / `SbEmptyState` / `ProjectScope` / `PageContainer` / `TablePager`）。原因：桩能发出真实组件发不出的值（如不存在的选项），也会无条件渲染弹层内容，让失效的下拉 / 菜单 / 开关假通过。
  - 需要真实弹层时，在测试文件顶层调用 `installJsdomPolyfills()`（补 `scrollIntoView` / pointer capture / `ResizeObserver`，并在每条用例后自动卸载包装器），用 `attachTo: document.body` 挂载，再合入对应的 stub 集合：`realPopoverStubs`（Popover + Command）、`realSelectStubs`、`realDialogStubs`（SbModal + Dialog）、`realAlertDialogStubs`（ConfirmAction + AlertDialog）、`realToggleStubs`。
  - 交互辅助都在 `helpers.ts`：`clickTab` / `openPopover` / `chooseFromPopover` / `clickInPopover` / `popoverEl` / `openSelect` / `pickSelectOption` / `selectOptions` / `selectTriggers` / `closeSelect` / `alertDialogEl` / `clickAlertDialog` / `dialogEl` / `closeDialog`。reka 的 `TabsTrigger` 响应 `mousedown`、`SelectItem` 响应 `pointerup`/键盘 Enter（jsdom 的 PointerEvent 不完整，用键盘路径），不要用 `.trigger('click')` 去碰它们。
  - 真实弹层关闭时**不渲染**内容：断言「选项里有什么 / 菜单里有什么」必须先打开再读 `document.body`，不要读 wrapper 文本。
  - 已知问题（真实组件才暴露，`tests/ConfirmAction.test.ts` 已锁定现状）：`ConfirmAction` 点「取消」会发两次 `cancel`、受控模式点「确定」会先多发一次 `cancel`。全仓没有 `@cancel` 消费者，故暂无实际后果。
- 图标用 `@lucide/vue`；按钮内图标加 `data-icon="inline-start"`（或 inline-end）配合样式钩子。
- **按钮尺寸规范**（planv6.7 `ui-simplify-ux-plan.md` S1）：卡片头 `CardAction` 与筛选栏按钮一律 `size="sm"`（次要 `outline`、主动作 `default`，每卡最多一个主动作）；表格行内操作 `variant="ghost" size="sm"`，删除用 `destructiveGhost`；行内图标按钮 `ghost` + `icon-sm` 并带 `aria-label`；chip / 小标签按钮 `size="xs"`。**不要**在 `<Button>` 上再写 `h-* / px-* / size-*` 覆盖尺寸（唯一例外：顶栏 `DefaultLayout` / `UserMenu` 的 `h-8`，为顶栏控件同高，勿改）。列表页「刷新」统一用 `components/SbRefreshButton.vue`，写入口用 `canWrite = !isAdmin && auth.canWrite` 控制。列表卡片骨架统一用 `components/SbListCard.vue`（planv6.7 S5）：`title` / `description` 属性 + `actions` / `toolbar` / 默认插槽，`pager` 属性传入后内嵌 `TablePager`（footer 样式），**不要**再手写 `CardHeader` 换行 class、筛选 `CardContent` 或 `TablePager variant="footer"`。
- 下载 Blob 用 `utils/download.ts` 的 `downloadBlob`（延迟回收 URL），复制文本用 `utils/clipboard.ts` 的 `copyText`，不要再手写锚点 / clipboard 片段。
- 提示统一 `vue-sonner` 的 `toast`；确认操作统一 `ConfirmAction` 组件。
- 弹窗用 `SbModal`；空态用 `SbEmptyState`；分页用 `TablePager`（配合 `usePagination`）。
- `SbModal` 宽度只有一条通道：`maxWidth`，与 `minWidth` 分别换算成 CSS 变量 `--sb-modal-max-w` / `--sb-modal-min-w`，不再走 `sm:max-w-5xl` 之类的分档类；历史 `width` 别名已删除，调用方统一使用 `maxWidth`。`okButtonProps` 是 `SbModalOkButtonProps`（`variant` / `size` / `class` / `disabled`），会整体透传给确认按钮；`okText` / `cancelText` 缺省走 `common.ok` / `common.cancel`。
- `SbModal` 正文容器带 `px-1 -mx-1`：它有 `overflow-x-hidden`，不留这 4px 会把输入框的 `ring-3` 焦点环裁掉；内边距与负外边距相互抵消，视觉位置不变。改正文容器时不要去掉。
- **弹窗内「输入框 + 其他元素」并排行**：父容器必须 `flex flex-wrap`；主输入框用 `min-w-0 flex-1`（或 `min-w-32 flex-1`）；固定宽度输入只给短输入（数字、分数），并加 `shrink-0`；需要宽屏保持固定宽度而窄屏撑满的，写 `min-w-0 flex-1 sm:w-48 sm:flex-none`。不要在不换行的 `flex` 行里给 `Input` / `SelectTrigger` 写裸 `w-*`——会把输入框撑出弹窗。该约定由 `ui/tests/modal-input-layout.test.ts` 按源码断言守住。
- 下拉优先用 `ui/select`（可搜索输入用 `ui/combobox`），新代码不写原生 `<select>` / `<option>`。原生 `<select>` 已清零（`SandboxCreateModal`、`Sandboxes`、`CreateProjectModal`、`AgentTagModal` 均已迁移）。`SelectItem` 的 value 不允许空串（reka 会抛错），表示「无 / 全部」时用 `__none__` / `__all__` 哨兵值，参见 `BiWidgetEditor`、`AgentTagModal`。
- 加载/空/错误三态经 `SbAsyncRegion`；**错误态只有一个实现**，即 `SbErrorState`（`variant: 'inline'` 给表格 `tbody`、`'block'` 给普通区域），不得再在页面里手写 `Alert + 重试按钮`。
- **空态归属**：区域自己就是列表/内容的渲染方时（如 `ConversationView` 的消息流），把空态交给它的 `empty` 插槽，并给 `SbAsyncRegion` 传 `:show-empty="false"`。否则 `SbAsyncRegion` 会在空数据时接管整个区域，插槽内容（消息、列表项）根本不挂载——云助手首条消息看不见就是这个原因（planv6.0 §0.1）。
- **「资源未建立」不等于「加载失败」**：后端用 404 + 登记过的错误码表示「这个项目还没有这份存储」（如 `kv_store_not_found`）。这类错误经 `utils/api-errors.ts` 的 `isEmptyResourceError` 判定，传给 `useLoadState({ emptyOn })`，命中时按空态处理、不 toast。**只有明确登记进 `EMPTY_RESOURCE_CODES` 的码才算**，否则会把真正的失败吞掉。
- 多页签面板用 `SbTabsPanel`（`items` + 每个 `value` 一个具名 slot），不要在业务侧重复拼 `Tabs` + `TabsList variant="line"` + 滚动容器。
- 表单字段用 `ui/field` 的 `Field` / `FieldGroup` / `FieldLabel` / `FieldError`，不手写 `<label>` + 裸 `Input` 组合。
- `SbEmptyState` 的操作按钮图标由 `actionIcon` 决定（默认 `PlusIcon` 仅适合「新建」语义；「重试/刷新/查看」必须显式传图标）。
- **页面内边距只走 `--page-pad`**（`style.css` 定义，`DefaultLayout` 施加在 `router-view` 容器上，带窄屏/大屏断点）。需要按视口算高度的页面（如云助手的对话区 `h-[calc(100svh-var(--header-height)-var(--page-pad)-var(--page-pad))]`）必须引用这个变量，**不要写 `6rem` 之类的字面量**——那等于把布局的内边距复制到页面里，改一处就会让另一处对不上。
- **确认交互**：删除、覆盖一类破坏性操作统一用 `ConfirmAction`（`AlertDialog`）。例外有两个，都在**云助手对话**里：①**工具级运行确认**用 `components/agent/AgentConfirmCard.vue` 行内卡片，因为模态会遮住上下文，而用户需要一边看工具调用一边决定是否放行（planv6.0 §3.6 #1）；②**整份 Plan 的确认**（群轮次执行任何助手之前）用 `components/agent/PlanConfirmDialog.vue`（`SbModal` + `:max-width="720"`，展示 goal / steps / 每个 dispatch，可编辑 message 或移除 dispatch 后批准，或拒绝），因为用户需要看全局分工再决定（planv6.8 §13.3 / §13.9）。两者必须严格区分：行内卡片只放行单次工具调用，弹窗只决定整份计划。群轮次的 Plan 卡是 `components/agent/AgentPlanCard.vue`（目标 / 步骤 / 分工 / 进度条 / 失败重试，`data-plan-*` 与 `data-dispatch-*` 钩子），任务气泡按 `taskId` 绑定（`ChatMsg.taskId`），不要按 `agentId` 找气泡——同一助手在历史里回答过或一轮里有两个任务时会串。删除助手/会话仍用 `ConfirmAction`；替换输入框草稿（提示词）也用 `ConfirmAction` 的受控模式，不用 `window.confirm`。行内卡片出现时聚焦「确认执行」，`Esc` 等于取消，**不绑定 Enter 确认**（避免与输入框的 Enter 发送冲突）。
- 全局设置：右上角齿轮打开 `SettingsModal`（`SbModal` + `:max-width="720"` + `hide-footer`），不是全页、不是 Sheet。页签为 `connection` / `storage` / `backups`（「模型」已迁至 `/console/models` 独立页，planv6.5），由 `SbTabsPanel`（`variant="line"`）承载；`SettingsTab` 类型在 `stores/auth.ts`，`DefaultLayout.vue` 的 `SETTINGS_TABS` 白名单必须同步。模型连接（API Key / Base URL）与默认参数在 `pages/ModelSettings.vue` 复用 `ModelDefaultsPanel`，写同一个 `sys_llm_settings` 行；调用观测与模型合并为同一页的双页签（二期 planv6.5 §11.1）：`?tab=traces` 深链直达 Trace 页签（`components/models/ModelTraceList.vue` 分组列表 + `TraceDetailModal.vue` 详情弹窗（`SbModal` 居中弹窗，非 Sheet），读 `sys_llm_traces` 分组 API），`/console/traces` 仅作 redirect。`?settings=models` deep-link 由路由层落到 `/console/models`。界面语言切换在顶栏（设置按钮旁的 `Select`），窄屏（<sm）顶栏整组隐藏时回落到侧栏底部那组入口，与文档/主题/设置一致。侧栏常规菜单不放「设置」。`/settings` 仅作 deep-link redirect。

## 拆分后的目录约定

- `components/cronjob/`：`useCronJobForm.ts`（表单状态、校验、保存）+ `CronJobScheduleFields.vue`（调度字段）；`CronJobModal.vue` 只做布局编排并原样暴露 composable 字段，以兼容既有测试的 `vm.form` 访问。
- `components/modal/SqlResultPanel.vue`：`SqlWorkModal` 的结果区，**分页状态由它自己持有**，宿主不再暴露 `queryPage`；测试走 `TablePager` 桩的 `.pager-next` 或真实分页按钮交互。
- `components/settings/`：`ModelDefaultsPanel.vue` 是「模型」页唯一实现（默认供应商 / API Key / Base URL / 采样参数），`ConnectionPanel.vue` 是「连接」页。早期按 `section` 转发的 `SettingsPanel.vue` 薄壳与 `ProviderKeysPanel.vue`（按厂商配 Key）、`LanguagePanel.vue`（面板式语言切换）已下线，不要重新引入。
- 新拆出的组件、composable 都会计入 coverage（只有 `src/components/ui/**` 被排除），拆分时同步补测，别让 `functions` 阈值再掉回 90% 以下。

## 工程规则

- 构建：`yarn build` 必须通过（改完跑一遍）；本地开发 `yarn dev`（推荐仓库根 `./build.sh dev` 一键起前后端）；测试 `yarn test`（coverage 90% 阈值，以 `vite.config.ts` 为准）。
- `yarn typecheck`（`tsc --noEmit`，含 `tests/`）必须通过，当前基线 0 错误；与 `yarn build`、`yarn test` 一起作为提交门禁。
- 不新增依赖、不升级依赖版本、不改 `package.json` / `tsconfig.json` / `vite.config.ts`，除非用户明确要求。
- 类型：不使用 `any` 落盘新代码；跨层契约必须走 `types.ts`；对象形状一律用 `interface` 定义（不用 `type`）。
- 已删除的零引用组件（checkbox / drawer / dropdown-menu / pagination / radio-group）不得重新引入——对应能力分别由 switch、sheet、原生方案、TablePager、toggle/radio 内联实现。
- 路由 redirect：`/console` 与控制台入口（首页「进入控制台」、文档站「返回控制台」）落到 `agents`（`/console/agents`）；监控大盘是 `/console/dashboard`。旧路径仍是 `/databases` 等八个 → `/console/*`，`/sql` → `/console/databases`，`/data` → `/console/data`（真实数据浏览器），`/llm` → `/console/agents`，`/settings` → `/console/agents?settings=1`。
