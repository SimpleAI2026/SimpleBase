# SimpleBase 前端约束（ui/）

Vue 3 + TypeScript + Vite 控制台。技术栈固定：Pinia、vue-router、Tailwind CSS v4、reka-ui（shadcn-vue 风格）、vue-sonner、axios、ECharts（图表）、Monaco Editor（SQL/Go 编辑器）、marked + DOMPurify（文档渲染）。面向使用者的说明见 [README.md](./README.md)。

## 目录职责

| 目录 | 职责 | 约束 |
| --- | --- | --- |
| `src/pages/` | 路由页面（Home / Dashboard / Databases / DataBrowser / ApiPreview / KeyValue / S3Manager / GoFunctions / FunctionTriggers / CronJobs / AgentManager / Logs / Users / DocsWiki） | 页面只做数据编排与布局；新页面必须在 `router/index.ts` 注册并配 `meta.titleKey`，需角色控制的配 `meta.requiresRole`（如 Users 的 `superadminl1`）。全局设置不是全页，见 `SettingsModal` |
| `src/components/ui/` | 基础组件库（button / table / dialog / select 等，shadcn 风格） | **视为本地 fork 的库代码**：只增删组件不改风格约定；删除组件前必须全局确认零引用 |
| `src/components/` | 业务组件：`databases/` `modal/` `ai/` `agent/` `chat/` `cronjob/` `sandbox/` `editor/` `settings/` `docs/` 分域，加顶层通用件（ConfirmAction / NavMenu / PageContainer / TablePager / SettingsModal / GlobalProjectSwitcher / TrendChart / SbAsyncRegion / SbEmptyState / SbErrorState / SbTabsPanel / SbCodeBlock / UserMenu 等） | 命名 `PascalCase.vue`；业务组件不得反向被 `ui/` 依赖 |
| `src/services/` | API 层：`api.ts`（固定导出 `httpApi`）→ `http-api.ts`（实现）→ `http.ts`（axios 实例、token 存取、401 处理）→ `types.ts`（契约） | 所有请求只经 `api.*`；页面禁止直接 import axios 或 `http-api` |
| `src/stores/` | Pinia store（auth 登录态与角色 / project 项目切换 / settings） | 跨页面状态才进 store；组件内状态用 `ref` |
| `src/composables/` | 组合函数（usePagination / useAsyncAction / useAiChat / useAgentConversation / useLoadState / useLocale） | 在组件外取文案用 `@/locales` 导出的 `t()`，不要在 composable 里调 `useI18n()` |
| `src/locales/` | i18n 文案源（`zh-CN.ts` 权威树、`en-US.ts` 同构、`index.ts` 实例与 `setI18nLocale`） | 唯一文案来源；不得在别处新建语言包 |
| `src/layouts/` | DefaultLayout（`/console` 控制台）/ DocsLayout（`/docs` 文档站）；Home 页独立于两布局 | — |
| `src/docs/` | 文档目录、按需加载 / 搜索、Markdown 安全渲染 | 文档正文来自仓库根 `docs/`；新增文档遵循 `docs/README.md` 并通过 `ui/tests/docs-content.test.ts` 的链接、锚点与 frontmatter 校验；大型上游镜像不要进首页 chunk |

## API 层规则

- 接口契约集中在 `services/types.ts`；后端字段用 snake_case 转 camelCase 的映射函数（`toProjectItem` 风格），不透传 raw。
- 路径构造函数模式：`xxxPath(projectId, …)` 统一 `encodeURIComponent`。
- 运行时 mock 层（`mock.js` / `mock-kv.js` / `VITE_USE_MOCK` / `isMock` 徽标）已于 v4.0 退役，**不得重新引入**；新增接口只需改 `types.ts` + `http-api.ts`，单测桩统一走 `src/test/api-mock.ts`。

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
- 以下**不属于**待迁移文案：代码注释、`services/http-api.ts`、`constants/llmProviders.ts` 的服务商名、`src/docs/*`（文档正文）、`src/components/ui/**`（fork 库代码，如 `DialogContent` 的 `Close`）。
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
  - 右上角顺序：使用文档 → 项目切换器（`w-56`）→ 设置 → 刷新，间距 `gap-4 sm:gap-5`；
  - 页面标题只用 PageContainer 的 subtitle 单行（不加 h2 大标题）。
- **状态样式必须匹配 reka-ui 2.10.5 的真实属性**：它输出 `data-state`（`checked`/`unchecked`、`active`/`inactive`、`open`/`closed`）与 `data-disabled`/`data-highlighted`/`data-selected` 等，**没有**裸 `data-checked`/`data-unchecked`/`data-open`/`data-closed`。写 `data-[state=checked]:…`，不要写 `data-checked:…`；后者编译出的选择器永不命中，开关会「点了没反应」且无报错（`Switch` / `Dialog*` / `PopoverContent` / `SelectContent` / `Sheet*` / `TooltipContent` / `Combobox*` 都在这个坑里，见 planv5.0 §4.1）。
- **`data-active` 是唯一例外**：reka 的 `RovingFocusItem` 会给选中项渲染**空属性** `data-active`（`node_modules/reka-ui/dist/RovingFocus/RovingFocusItem.js`），所以 `TabsTrigger` 上写 `data-active:` 本来就能命中（`ui/tests/ui-state-attributes.test.ts` 已断言选中项 `data-active === ''`）。新代码仍统一写 `data-[state=active]:` 保持一致性；**不要**据此认为 `data-active:` 失效。
- 组件测试**不要桩掉 `src/components/ui/` 的交互组件**（尤其 `Switch`）。`src/test/helpers.ts` 的 `Switch` 桩用的是废弃的 `checked`/`update:checked`，会把失效的开关测成通过（planv5.0 §7.3）。
- 图标用 `@lucide/vue`；按钮内图标加 `data-icon="inline-start"`（或 inline-end）配合样式钩子。
- 提示统一 `vue-sonner` 的 `toast`；确认操作统一 `ConfirmAction` 组件。
- 弹窗用 `SbModal`；空态用 `SbEmptyState`；分页用 `TablePager`（配合 `usePagination`）。
- `SbModal` 宽度只有一条通道：`maxWidth`（`width` 是历史别名，二者等价），都换算成 CSS 变量 `--sb-modal-max-w` / `--sb-modal-min-w`，不再走 `sm:max-w-5xl` 之类的分档类。`okButtonProps` 是 `SbModalOkButtonProps`（`variant` / `size` / `class` / `disabled`），会整体透传给确认按钮；`okText` / `cancelText` 缺省走 `common.ok` / `common.cancel`。
- `SbModal` 正文容器带 `px-1 -mx-1`：它有 `overflow-x-hidden`，不留这 4px 会把输入框的 `ring-3` 焦点环裁掉；内边距与负外边距相互抵消，视觉位置不变。改正文容器时不要去掉。
- **弹窗内「输入框 + 其他元素」并排行**：父容器必须 `flex flex-wrap`；主输入框用 `min-w-0 flex-1`（或 `min-w-32 flex-1`）；固定宽度输入只给短输入（数字、分数），并加 `shrink-0`；需要宽屏保持固定宽度而窄屏撑满的，写 `min-w-0 flex-1 sm:w-48 sm:flex-none`。不要在不换行的 `flex` 行里给 `Input` / `SelectTrigger` 写裸 `w-*`——会把输入框撑出弹窗。该约定由 `ui/tests/modal-input-layout.test.ts` 按源码断言守住。
- 下拉优先用 `ui/select`（可搜索输入用 `ui/combobox`），新代码不写原生 `<select>` / `<option>`。`SandboxCreateModal`、`Sandboxes` 的过滤器已迁移；`databases/SchemaPanel.vue` 的列类型下拉与 `modal/AgentFormModal.vue` 的 `datalist` 模型输入仍是原生写法，属于遗留项，改动这两处时顺手迁移。
- 加载/空/错误三态经 `SbAsyncRegion`；**错误态只有一个实现**，即 `SbErrorState`（`variant: 'inline'` 给表格 `tbody`、`'block'` 给普通区域），不得再在页面里手写 `Alert + 重试按钮`。
- **空态归属**：区域自己就是列表/内容的渲染方时（如 `ConversationView` 的消息流），把空态交给它的 `empty` 插槽，并给 `SbAsyncRegion` 传 `:show-empty="false"`。否则 `SbAsyncRegion` 会在空数据时接管整个区域，插槽内容（消息、列表项）根本不挂载——云助手首条消息看不见就是这个原因（planv6.0 §0.1）。
- **「资源未建立」不等于「加载失败」**：后端用 404 + 登记过的错误码表示「这个项目还没有这份存储」（如 `kv_store_not_found`）。这类错误经 `utils/api-errors.ts` 的 `isEmptyResourceError` 判定，传给 `useLoadState({ emptyOn })`，命中时按空态处理、不 toast。**只有明确登记进 `EMPTY_RESOURCE_CODES` 的码才算**，否则会把真正的失败吞掉。
- 多页签面板用 `SbTabsPanel`（`items` + 每个 `value` 一个具名 slot），不要在业务侧重复拼 `Tabs` + `TabsList variant="line"` + 滚动容器。
- 表单字段用 `ui/field` 的 `Field` / `FieldGroup` / `FieldLabel` / `FieldError`，不手写 `<label>` + 裸 `Input` 组合。
- `SbEmptyState` 的操作按钮图标由 `actionIcon` 决定（默认 `PlusIcon` 仅适合「新建」语义；「重试/刷新/查看」必须显式传图标）。
- **页面内边距只走 `--page-pad`**（`style.css` 定义，`DefaultLayout` 施加在 `router-view` 容器上，带窄屏/大屏断点）。需要按视口算高度的页面（如云助手的对话区 `h-[calc(100svh-var(--header-height)-var(--page-pad)-var(--page-pad))]`）必须引用这个变量，**不要写 `6rem` 之类的字面量**——那等于把布局的内边距复制到页面里，改一处就会让另一处对不上。
- **确认交互**：删除、覆盖一类破坏性操作统一用 `ConfirmAction`（`AlertDialog`）。唯一的例外是**云助手对话中的运行确认**——它用 `components/agent/AgentConfirmCard.vue` 行内卡片，因为模态会遮住上下文，而用户需要一边看工具调用一边决定是否放行（planv6.0 §3.6 #1）。删除助手/会话仍用 `ConfirmAction`。行内卡片出现时聚焦「确认执行」，`Esc` 等于取消，**不绑定 Enter 确认**（避免与输入框的 Enter 发送冲突）。
- 全局设置：右上角齿轮打开 `SettingsModal`（`SbModal` + `:max-width="720"` + `hide-footer`），不是全页、不是 Sheet。页签为 `connection` / `models` / `storage` / `backups`，由 `SbTabsPanel`（`variant="line"`）承载；`SettingsTab` 类型在 `stores/auth.ts`，`DefaultLayout.vue` 的 `SETTINGS_TABS` 白名单必须同步。模型连接（API Key / Base URL）与默认参数同属「模型」页，写同一个 `sys_llm_settings` 行。界面语言切换在顶栏（设置按钮旁的 `Select`），窄屏（<sm）顶栏整组隐藏时回落到侧栏底部那组入口，与文档/主题/设置一致。侧栏常规菜单不放「设置」。`/settings` 仅作 deep-link redirect。

## 拆分后的目录约定

- `components/cronjob/`：`useCronJobForm.ts`（表单状态、校验、保存）+ `CronJobScheduleFields.vue`（调度字段）；`CronJobModal.vue` 只做布局编排并原样暴露 composable 字段，以兼容既有测试的 `vm.form` 访问。
- `components/modal/SqlResultPanel.vue`：`SqlWorkModal` 的结果区，**分页状态由它自己持有**，宿主不再暴露 `queryPage`；测试走 `.pager-next` 之类的分页桩交互。
- `components/settings/`：`ModelDefaultsPanel.vue` 是「模型」页唯一实现（默认供应商 / API Key / Base URL / 采样参数），`ConnectionPanel.vue` 是「连接」页。早期按 `section` 转发的 `SettingsPanel.vue` 薄壳与 `ProviderKeysPanel.vue`（按厂商配 Key）、`LanguagePanel.vue`（面板式语言切换）已下线，不要重新引入。
- 新拆出的组件、composable 都会计入 coverage（只有 `src/components/ui/**` 被排除），拆分时同步补测，别让 `functions` 阈值再掉回 95% 以下。

## 工程规则

- 构建：`yarn build` 必须通过（改完跑一遍）；本地开发 `yarn dev`（推荐仓库根 `./build.sh dev` 一键起前后端）；测试 `yarn test`（coverage 95% 阈值）。
- `yarn typecheck`（`tsc --noEmit`）当前**不干净**：`src/vite-env.d.ts` 缺 `*.vue` 模块 shim，导致 `tests/**` 里 241 处 TS2307；改前端时以 `yarn build` + `yarn test` 为准，类型检查现状见 planv5.0 §7.1。
- 不新增依赖、不升级依赖版本、不改 `package.json` / `tsconfig.json` / `vite.config.ts`，除非用户明确要求。
- 类型：不使用 `any` 落盘新代码；跨层契约必须走 `types.ts`；对象形状一律用 `interface` 定义（不用 `type`）。
- 已删除的零引用组件（checkbox / drawer / dropdown-menu / pagination / radio-group）不得重新引入——对应能力分别由 switch、sheet、原生方案、TablePager、toggle/radio 内联实现。
- 路由 redirect：`/console` 与控制台入口（首页「进入控制台」、文档站「返回控制台」）落到 `agents`（`/console/agents`）；监控大盘是 `/console/dashboard`。旧路径仍是 `/databases` 等八个 → `/console/*`，`/sql` → `/console/databases`，`/data` → `/console/data`（真实数据浏览器），`/llm` → `/console/agents`，`/settings` → `/console/agents?settings=1`。
