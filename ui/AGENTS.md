# SimpleBase 前端约束（ui/）

Vue 3 + TypeScript + Vite 控制台。技术栈固定：Pinia、vue-router、Tailwind CSS v4、reka-ui（shadcn-vue 风格）、vue-sonner、axios、ECharts（图表）、Monaco Editor（SQL/Go 编辑器）、marked + DOMPurify（文档渲染）。面向使用者的说明见 [README.md](./README.md)。

## 目录职责

| 目录 | 职责 | 约束 |
| --- | --- | --- |
| `src/pages/` | 路由页面（Home / Dashboard / Databases / KeyValue / S3Manager / GoFunctions / CronJobs / AgentManager / Logs / Users / DocsWiki） | 页面只做数据编排与布局；新页面必须在 `router/index.ts` 注册并配 `meta.title`，需角色控制的配 `meta.requiresRole`（如 Users 的 `superadminl1`）。全局设置不是全页，见 `SettingsModal` |
| `src/components/ui/` | 基础组件库（button / table / dialog / select 等，shadcn 风格） | **视为本地 fork 的库代码**：只增删组件不改风格约定；删除组件前必须全局确认零引用 |
| `src/components/` | 业务组件：`databases/` `modal/` `ai/` `chat/` `editor/` `settings/` `docs/` 分域，加顶层通用件（ConfirmAction / NavMenu / PageContainer / TablePager / SettingsModal / GlobalProjectSwitcher / TrendChart / SbEmptyState / SbCodeBlock / UserMenu 等） | 命名 `PascalCase.vue`；业务组件不得反向被 `ui/` 依赖 |
| `src/services/` | API 层：`api.ts`（固定导出 `httpApi`）→ `http-api.ts`（实现）→ `http.ts`（axios 实例、token 存取、401 处理）→ `types.ts`（契约） | 所有请求只经 `api.*`；页面禁止直接 import axios 或 `http-api` |
| `src/stores/` | Pinia store（auth 登录态与角色 / project 项目切换 / settings） | 跨页面状态才进 store；组件内状态用 `ref` |
| `src/composables/` | 组合函数（usePagination / useAsyncAction / useAiChat） | — |
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

## 样式与 UI 约定

- Tailwind v4，样式写在 template 的 class 中；不写自定义 CSS 文件（全局仅 `style.css` 的主题 token）。
- 已定稿的规范不得回退：
  - 侧边栏菜单项 `h-11`（sm 下 `h-9`）、间距 `gap-1.5`；
  - 主内容区铺满全宽（不加 `max-w-*` 限制）；
  - 右上角顺序：使用文档 → 项目切换器（`w-56`）→ 设置 → 刷新，间距 `gap-4 sm:gap-5`；
  - 页面标题只用 PageContainer 的 subtitle 单行（不加 h2 大标题）。
- 图标用 `@lucide/vue`；按钮内图标加 `data-icon="inline-start"`（或 inline-end）配合样式钩子。
- 提示统一 `vue-sonner` 的 `toast`；确认操作统一 `ConfirmAction` 组件。
- 弹窗用 `SbModal`；空态用 `SbEmptyState`；分页用 `TablePager`（配合 `usePagination`）。
- 全局设置：右上角齿轮打开 `SettingsModal`（`SbModal` + `maxWidth={900}` + `hideFooter`），不是全页、不是 Sheet。侧栏不放「设置」。`/settings` 仅作 deep-link redirect。

## 工程规则

- 构建：`yarn build` 必须通过（改完跑一遍）；本地开发 `yarn dev`（推荐仓库根 `./build.sh dev` 一键起前后端）；测试 `yarn test`（coverage 95% 阈值）。
- 不新增依赖、不升级依赖版本、不改 `package.json` / `tsconfig.json` / `vite.config.ts`，除非用户明确要求。
- 类型：不使用 `any` 落盘新代码；跨层契约必须走 `types.ts`；对象形状一律用 `interface` 定义（不用 `type`）。
- 已删除的零引用组件（checkbox / drawer / dropdown-menu / pagination / radio-group）不得重新引入——对应能力分别由 switch、sheet、原生方案、TablePager、toggle/radio 内联实现。
- 路由 redirect 保持现状（旧控制台路径 `/databases` 等八个 → `/console/*`；`/sql`、`/data` → `/console/databases`；`/llm` → `/console/agents`；`/settings` → `/console?settings=1`）。
