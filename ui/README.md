# SimpleBase 控制台（ui/）

Vue 3 + TypeScript + Vite 单页应用，管理 SimpleBase 后端的全部能力：数据库、SQL、文档数据、Key-Value、S3 对象、云函数、定时任务、云助手、日志与用户。构建产物经 `go:embed` 嵌入后端二进制（`internal/web/dist`），单进程部署即可拥有完整控制台；开发时也可独立启动，API 经 Vite proxy 转发到后端。

修改代码前请先阅读 [AGENTS.md](./AGENTS.md)（目录职责、API 层规则、样式定稿等硬约束）。

## 技术栈

Vue 3 · TypeScript 5 · Vite 5 · Pinia · vue-router · Tailwind CSS v4 · reka-ui（shadcn-vue 风格，见 `components.json`）· vue-sonner · axios · ECharts（图表）· Monaco Editor（SQL / Go 编辑器）· marked + DOMPurify（文档渲染）· Vitest + jsdom（单测，95% 覆盖率阈值）

## 快速开始

推荐从仓库根目录一键启动（自动装配配置并并行拉起前后端）：

```bash
./build.sh dev        # 前端 http://127.0.0.1:5173 + 后端 :8080
```

仅启动前端（需后端已在运行）：

```bash
cd ui
yarn install
yarn dev              # http://127.0.0.1:5173
```

## 常用命令

| 命令 | 说明 |
| --- | --- |
| `yarn dev` | Vite 开发服务器（默认 5173，`--strictPort`） |
| `yarn build` | 生产构建 → `dist/` |
| `yarn preview` | 本地预览构建产物（5173） |
| `yarn test` | Vitest 单测 + v8 coverage（语句/函数/分支 95% 阈值） |
| `yarn test:watch` | watch 模式 |

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `VITE_API_BASE_URL` | API 基础路径；留空走相对路径 `/v1/...`——开发由 Vite proxy 转发，生产由后端直接服务 |

开发代理：`/v1`、`/health`、`/go` 转发到 `SIMPLEBASE_DEV_API_PROXY`（默认 `http://127.0.0.1:8080`，由 `./build.sh dev` 注入），见 `vite.config.ts`。

## 页面与路由

路由单一数据源在 `src/router/index.ts`（标题、图标、角色收敛到 `meta`）。

| 路由 | 页面 | 说明 |
| --- | --- | --- |
| `/` | Home | 产品首页（独立布局，与控制台拆分） |
| `/console` | Dashboard | 监控大盘（指标趋势、项目用量） |
| `/console/databases` | Databases | 数据库管理、文档数据浏览、SQL 工作台 |
| `/console/key-value` | KeyValue | 项目 KV 数据管理 |
| `/console/s3` | S3Manager | 对象存储管理（上传 / 删除 / 预签名） |
| `/console/gofunctions` | GoFunctions | 云函数（版本、激活、在线测试） |
| `/console/cron-jobs` | CronJobs | 定时任务管理 |
| `/console/agents` | AgentManager | 云助手（工作台第一项）：会话与定时调度 |
| `/console/logs` | Logs | 日志查询与保留策略 |
| `/console/users` | Users | 用户管理（`meta.requiresRole: 'superadminl1'`） |
| `/docs`、`/docs/:module/:slug` | DocsWiki | 内嵌文档站（渲染仓库 `docs/`，DocsLayout） |

旧路径 redirect：`/databases` 等旧控制台路径 → `/console/*`；`/sql`、`/data` → `/console/databases`；`/llm` → `/console/agents`；`/settings` → `/console?settings=1`（全局设置是 SettingsModal 弹窗，不是页面）。

## 目录结构

```text
src/
├─ pages/              路由页面（Home、Dashboard、Databases、KeyValue 等）
├─ components/
│  ├─ ui/              基础组件库（shadcn 风格，视为本地 fork 的库代码）
│  ├─ databases/       数据库 / 集合 / 文档 / SQL / KV 相关
│  ├─ modal/           业务弹窗（SbModal、GoFunctionModal、SqlWorkModal、LoginModal 等）
│  ├─ ai/ chat/        AI 对话相关
│  ├─ editor/          Monaco 编辑器封装
│  ├─ settings/ docs/  设置面板 / 文档渲染
│  └─ 顶层通用件        ConfirmAction、NavMenu、PageContainer、TablePager、
│                      SettingsModal、GlobalProjectSwitcher、TrendChart、
│                      SbEmptyState、SbCodeBlock、UserMenu 等
├─ services/           API 层（见下节）
├─ stores/             Pinia（auth 登录态与角色 / project 项目切换 / settings）
├─ composables/        usePagination / useAsyncAction / useAiChat
├─ layouts/            DefaultLayout（控制台）/ DocsLayout（文档站）
├─ docs/               文档目录、按需加载 / 搜索与 Markdown 渲染（catalog.ts / render.ts）
├─ constants/          常量（llmProviders）
├─ lib/                utils（cn 合并 class）/ status 映射
├─ utils/              format
├─ router/             路由定义
└─ test/               Vitest setup 与测试基建（setup.ts / helpers.ts / api-mock.ts）
tests/                 全部单测（与 src/ 平级；import 经 `@/` 别名指回 src）
```

## 文档站

`/docs` 直接读取仓库根目录的 `docs/<模块>/*.md`：`_meta.json` 管模块顺序，frontmatter `title` / `order` / `group` 管侧栏。`catalog.ts` 将小文档打入文档页 chunk；大型 DuckLake 上游镜像只在打开或搜索时异步加载。`render.ts` 负责安全渲染、标题锚点、提示块、代码复制和站内链接；`DocsToc.vue` 提供页内导航，`DocsSearch.vue` 提供懒构建本地全文索引。新增文档前请阅读 [`docs/README.md`](../docs/README.md)，并运行 `yarn test` 中的 `docs-content.test.ts` 校验链接、锚点、frontmatter 和围栏语言。

## API 层架构

```text
页面/组件 ──> services/api.ts（固定导出 httpApi）
                └─ http-api.ts（实现）→ http.ts（axios 实例、token 存取、401 处理）
             └─ services/types.ts（唯一契约：Api 接口 + DTO）
```

- 后端 snake_case → camelCase 一律走映射函数（`toProjectItem` 风格），不透传 raw。
- 路径构造函数模式：`xxxPath(projectId, …)` 统一 `encodeURIComponent`。
- 页面/组件**禁止**直接 import axios 或 `http-api`，只能 `api.*`。
- 新增接口：改 `types.ts` 契约 + `http-api.ts` 实现；单测桩在 `src/test/api-mock.ts`。

## 登录态与权限

- JWT 与 API Key 双通道：未登录时可用 API Key（SettingsModal 设置）；登录后走 JWT（access + refresh 自动轮换，`services/http.ts` 存取）。
- `stores/auth.ts` 是登录态唯一入口：角色 `superadminl1` / `admin` / `user`，判定用 `isSuper` / `isAdminRole` / `canWrite` / `canManageUsers` 等 getter；`canWrite` 规则为「未登录（API Key 通道）可写，登录态下仅 superadminl1 与 user 可写」。
- 401 统一记录 `lastUnauthorizedAt` 并打开 `LoginModal`（含强制改密流程）。
- admin（系统）项目只读：`stores/project.ts` 的 `isAdmin` 是唯一判定；真正保护在后端（`ErrSystemProtected`）。

## 测试

测试位于 `ui/tests/`（85 个文件），与源码 `src/` 分离；import 通过 `@/` 别名指回 `src/`。技术栈：Vitest + jsdom + `@vue/test-utils`；coverage 用 v8 provider，语句/函数/分支 95% 阈值。不计入覆盖：`src/components/ui/**`、`*.d.ts`、`src/test/**`、`src/main.ts`、`src/services/types.ts`。

## 与后端的构建关系

仓库根 `./build.sh` 会执行 `yarn build` 并把 `ui/dist` 同步到 `internal/web/dist`（`go:embed` 嵌入点）。`internal/web/dist` 为生成产物，**不要手工编辑**。

## 相关文档

- 前端开发约束：[ui/AGENTS.md](./AGENTS.md)
- 项目总览与 API 清单：[../README.md](../README.md)
- 后端模块说明：[../internal/README.md](../internal/README.md)
