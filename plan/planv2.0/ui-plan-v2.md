# SimpleBase UI 优化总计划 v2.0（主入口）

> 目标目录：`ui/src`
> 技术栈：Vue 3.4 + TypeScript + Vite 5 + ant-design-vue 4 + pinia + axios
> 制定日期：2026-09-15
> 前置版本：`plan/planv1.0/ui-refactor-plan.md`（已完成 API 抽象、Mock 层、Claude 暖橙主题）、
> `plan/planv2.0/ui-style-plan.md`（样式优化，本计划吸收并升级）

## 配套文档

| 文档 | 内容 |
|---|---|
| `ui-principles.md` | UI 设计原则 + 代码结构原则（含与现行代码的差距索引） |
| `proto-http.md` | 前后端 API 交互协议（25 个路由的完整契约、错误码全集、后端 TODO） |
| `proto.http` | 可在 VS Code REST Client / JetBrains HTTP Client 直接执行的调用样例 |
| `ui-style-plan.md` | 纯样式层的详细问题清单与 token 表（本计划的 P1 部分由它派生，仍可查阅） |
| `ui-settings-chat-plan.md` | 设置页（模型/主题/厂商 Key）+ 通用 AiChat 组件；**本轮仅计划与契约，不写代码** |
| [`databases-and-s3-plan.md`](./databases-and-s3-plan.md) | 数据库 DuckLake-only + 用户 S3（AWS 协议）后端收敛计划（本轮只规划） |
| [`ui-databases-console-plan.md`](./ui-databases-console-plan.md) | 数据库管理页合一：SQL 工作台 + 集合/文档弹窗（已实现） |
| [`ui-global-project-plan.md`](./ui-global-project-plan.md) | 顶栏全局项目切换 + 创建项目（已实现） |
| [`cloud-agent-plan.md`](./cloud-agent-plan.md) | Cloud Agent：per-module agent、两栏 UI、`@` mention、eino 只读工具（替代 LLM 对话） |
| [`ui-docs-wiki-plan.md`](./ui-docs-wiki-plan.md) | 顶栏「使用文档」：GitHub Wiki 风（模块 + 左 TOC + 右 MD），源 `docs/<module>/*.md`（已实现） |
| [`js-sdk-plan.md`](./js-sdk-plan.md) | 公共 JS SDK（Supabase 风格 Token 客户端）+ examples + `docs/sdk` Wiki（MVP 已落地） |

> 阅读顺序建议：本文（全局与阶段） → `ui-principles.md`（写代码时的判据） → `proto-http.md` / `proto.http`（接接口时的契约）。

## 一、现状评估结论

对 `ui/src` 全量审查（6 页面 + 2 组件 + 1 布局 + services 层 + theme.css），结论：

**已做对的部分（保持不动）**
- services 层抽象（`types.ts` 定义 `Api` 接口，`http-api.ts` / `mock.js` 双实现，`api.ts` 通过 `VITE_USE_MOCK` 切换）——这是 v1.0 的正确产物。
- 整体视觉方向：Claude 暖橙 + 米白背景 + 衬线标题，风格统一且有辨识度。
- 移动端 Drawer 降级、毛玻璃 Header、路由过渡动画等细节到位。

**核心问题（按优先级）**

> 下表结论均已逐条比对源码核实：前端 `ui/src/**`、后端 `internal/api/router.go` 及各 handler。
> 「后端无此路由」类结论用 `rg` 在 `internal/`、`cmd/` 全部 `.go` 文件中确认（排除 `internal/web/dist` 构建产物）。

### P0 — 功能缺口（本次重点）

| # | 问题 | 核实结论与影响 |
|---|------|------|
| F1 | **前端调用了 3 组不存在的后端路由** | `/metrics/summary`、`/metrics/trend`（Dashboard）、`/faas/*`（FaaSManager）、`/ws/logs`（Logs）在后端源码中完全不存在。请求命中 `web.Register` 的 SPA fallback 返回 `index.html`，前端拦截器转为「接口不存在」错误。**Dashboard / FaaS / Logs 三页在真实后端下整页不可用** |
| F2 | **db API 硬编码 `proj-01`** | `http-api.ts` 8 处路径写死项目 ID。注意：修复不只是替换字符串——`types.ts` 的 `Api.db.*` 方法签名里**没有 projectId 参数**（`collections()`、`rows(collection)`…），需要先改接口签名，再改 http/mock 两个实现 |
| F3 | **后端已实现能力大量未接入** | 后端共 25 个 `/v1` 路由，前端只接了 data(6) + s3(4) + llm(3) = 13 个。未接：数据库管理 8 个（含 2 个 501 占位）、SQL 3 个、quota 1 个、audit 1 个 |
| F4 | **LLM 前端防御不足（非字段名错误）** | 原判「字段名错误」不成立：`toLlmPayload` 的 camelCase→snake_case 映射是正确的。真实问题有两个：① `llmStream` 错误分支的 `resp.json()` 遇到 HTML fallback 会二次抛错（已有 `.catch(() => ({}))` 兜底，但 `resp.body` 为 null 的分支仍会走到这里）；② `messages` 为空时后端返回 **500 internal_error**（而非 400），前端必须本地校验，否则用户看到「internal error」 |
| F5 | **无项目切换能力（但后端也没有项目列表接口）** | 原判「UI 无法列出项目」需修正为：**后端不存在 `GET /v1/projects`**，只有 `/v1/projects/:projectID/*` 子路由，前端根本无法枚举项目。因此方案不是「加项目列表页」，而是：项目 ID 由用户手工输入 + localStorage 持久化 + 全局 store 注入（`ProjectSelector` 退化为「输入+记忆」组件） |
| F6 | **API Key 无管理入口** | `http.ts` 从 localStorage 读 key（默认 DevMode 种子 `sb_live_dev_key_12345`），`setApiKey` 已导出但**无任何调用方**。key 失效时用户只能看到 401 报错文案，无处修改 |
| F7 | **S3 上传无进度/无体积预校验** | `a-upload` 的 `custom-request` 一次性 POST，无 `onUploadProgress`；后端 `max_request_bytes`（config.yaml 10MB）超限返回 413，用户上传完才失败 |
| F8 | **Logs 页依赖不存在的 WebSocket** | 后端无任何 WS 实现（go.mod 无 gorilla/websocket 等依赖），不是「路由遗漏」而是「能力缺失」。原计划「改为轮询 `/metrics`」不可行——`/metrics` 是 Prometheus 文本，且与日志流无关。**应改为：保留 Mock 演示 + 页面显式提示「后端未支持实时日志」**，真实能力另立后端 plan |
| F9 | **文档 API 隐式绑定「第一个数据库」** | 新发现：`DataHandler.acquire` 取 `ListDatabases(limit=1)` 的第一个库，路径里没有 `databaseID`。多库项目下 DataManager 操作的库与 Databases 页选中的库**可能不是同一个**；项目下无库时返回 500。前端需在 UI 上明示这一行为，并在无库时给出「先创建数据库」引导 |
| F10 | **Mock 层 S3 列表恒为空（既有 bug）** | 新发现：`mock.js` 初始 `state.objects` 的 4 条数据没有 `_projectId` 字段，而 `s3.list` 过滤条件是 `o._projectId === projectId` → **mock 模式下 S3 列表永远为空**。需给种子数据补 `_projectId: 'proj-01'` |

### P1 — UI/UX 优化（吸收 `ui-style-plan.md` 的问题清单）

| 类别 | 问题 | 改法 |
|---|---|---|
| Token | 主色 `#d97757` 三处硬编码、Dashboard 统计卡 8 个 rgba 色值写死在 TS | 全部收敛到 `tokens.css` + `tokens.ts` 单一来源 |
| 样式复用 | `.sb-json`/`.sb-result`/等宽字体栈在 5+ 处复制 | 抽 `SbCodeBlock` 组件 + `utilities.css` 工具类 |
| 路由元信息 | 页面标题在 NavMenu / `titleMap` / `PageContainer` 三处维护 | `router/index.ts` 加 `meta.title`，菜单与面包屑从 meta 派生 |
| 死代码 | `theme.css:129-145` 的 `.sb-page-title` 被 `PageContainer.vue` 的 scoped 规则覆盖，全局那份从未生效 | 二选一保留（建议留在 utilities.css，组件不重复定义） |
| 布局 | 大屏无内容宽度上限（27 寸表格拉满 2000px+） | `.sb-content` 增加 `max-width: 1440px; margin-inline: auto` |
| 表格 | 无斑马纹/粘性表头；`{pageSize:10,size:'small',showTotal}` 在 3 页复制 | antd Table 组件 token + `usePagination` composable |
| 加载/空态 | 只有 spin，无骨架屏；空态无引导操作 | `a-skeleton` 首屏 + `SbEmptyState` 带 CTA |
| 状态展示 | 新增 Databases 页需要状态 tag，后端 `status` 有 **9 个枚举值**（`creating/opening/ready/closing/closed/degraded/deleting/deleted/recovering`），刚创建是 `creating` 不是 `active` | 配色映射需覆盖全部 9 值，勿假设二态 |
| 可访问性 | 无 `:focus-visible`、无 `prefers-reduced-motion`；`--sb-glow` 定义了却没用 | base.css 补齐 |
| 暗色模式 | 强制亮色 | `[data-theme='dark']` 变量覆盖 + antd darkAlgorithm（Phase 5） |

### P2 — 代码卫生

- `NavMenu.vue:42` 未使用的 `ProjectOutlined` 导入
- `DefaultLayout.vue:91-98` 用 `getComputedStyle` 同步读 CSS 变量：移动端媒体查询把 `--sb-sider-width` 改为 `0px`，若首屏是窄屏则 `siderWidth = 0`，且只在 setup 执行一次不响应断点变化
- `Logs.vue:128` 绿色 `box-shadow: rgba(48,209,88,.6)` 与 `--sb-success #3f8a5a` 不符；`#9ca3af`/`#9a7b1a` 等硬编码灰
- `mock.d.ts` 仅 3 行声明（`mock.js` 是 JS 遗留），`sb-mock-tag` 规则在 2 处重复，`--sb-transition: all` 触发多余重排
- 多处 `any`：`NavMenu.onClick(e: any)`、`S3Manager.handleUpload({...}: any)`、各页 `catch (e: any)`
- `theme.css:300-312` 的 `.sb-chart-*` 断点规则写在全局，组件本体样式在 `Dashboard.vue` scoped 里，跨文件割裂

## 二、目标架构

```
ui/src
├── main.ts                 antd 按需/全量 + styles 顺序引入
├── App.vue                 ConfigProvider（token 从 tokens.ts 派生，不再硬编码）
├── router/index.ts         路由 + meta（title/icon/权限），懒加载
├── layouts/DefaultLayout   布局壳（不动结构，修宽度 bug）
├── components/             通用组件
│   ├── NavMenu.vue         从路由 meta 派生菜单项
│   ├── PageContainer.vue   页头 + slot（与 theme.css 的重复规则二选一）
│   ├── SbCodeBlock.vue     新增：统一代码/JSON 展示块（替代 .sb-json/.sb-result）
│   ├── SbEmptyState.vue    新增：空态 + 引导 CTA
│   ├── ProjectPicker.vue   新增：项目 ID 输入 + 本地记忆（后端无项目列表接口）
│   ├── ApiKeyDrawer.vue    新增：API Key 查看/修改入口（配合 401 提示）
│   └── ai/                 新增（Phase 6）：AiChat / AiChatComposer 等通用对话组件
├── pages/                  页面（每页只做「编排」，不写重复样式）
│   ├── Dashboard.vue       改造：改用 quota + databases 数据（metrics 接口不存在）
│   ├── Databases.vue       新增：数据库管理（8 路由，含 2 个 501 占位）
│   ├── SqlConsole.vue      新增：SQL query/execute/batch 控制台
│   ├── DataManager.vue     修复：projectId 参数化 + 明示「隐式第一个库」行为
│   ├── S3Manager.vue       增强：上传进度 + 体积预校验 + key 校验提示
│   ├── FaaSManager.vue     菜单隐藏（后端无 FaaS 模块），代码与 Mock 保留
│   ├── LlmManager.vue      微调：供应商改下拉 + messages 本地校验；实现阶段改为挂载 AiChat
│   ├── Settings.vue        新增（Phase 6）：主题 / 默认模型 / 厂商 Key——见 ui-settings-chat-plan.md
│   └── Logs.vue            改造：Mock 演示 + 显式提示后端未支持实时日志
├── stores/                 pinia（新增）
│   ├── project.ts          当前项目 ID（localStorage 持久化，替代硬编码 proj-01）
│   └── auth.ts             API key 管理 + 401 状态 + 配置抽屉开关
├── composables/
│   ├── useAsyncAction.ts   统一 loading/error/消息提示模式
│   └── usePagination.ts    表格分页配置
├── services/               （保持 v1.0 架构，补齐接口）
│   ├── types.ts            Api 接口：db.* 加 projectId 参数；补 databases/sql/quota 域
│   ├── http-api.ts         全部路由对齐后端 router.go，projectId 由调用方传入
│   ├── mock.js             同步补齐 mock（并修 S3 列表恒空的 bug）
│   └── http.ts             拦截器增加 401 → 打开 key 配置抽屉
├── styles/
│   ├── tokens.css          设计变量（含暗色覆盖）
│   ├── tokens.ts           从 CSS 变量派生的 TS 常量（供 antd token / 图表配色）
│   ├── base.css            reset / focus-visible / reduce-motion
│   ├── antd-patch.css      最小化 antd 补丁（目标 ≤ 60 行）
│   └── utilities.css       .sb-page/.sb-toolbar/.sb-card/.sb-code
└── utils/format.ts         保持
```

## 三、API 对齐清单（详细契约见 proto-http.md）

后端 `/v1` 路由共 25 个，前端现接 13 个。对齐动作：

| 域 | 后端路由（`:p` = `/v1/projects/:projectID`） | 前端现状 | 动作 |
|---|---|---|---|
| 数据库 | `POST/GET :p/databases`、`GET/DELETE :p/databases/:id`、`POST :p/databases/:id/{open,close,backups,restore}` | 无 | 新增 Databases 页。注意 `close` 返回 204、`delete` 返回 202、`backups`/`restore` 返回 **501 且响应无 request_id** |
| SQL | `POST :p/databases/:id/{query,execute,batch}` | 无 | 新增 SqlConsole 页。`QueryResponse.rows` 是**二维数组**，需与 `columns` 按下标 zip |
| 文档 | `GET/POST :p/data/collections`、`GET :p/data/collections/:c`、`POST/PUT/DELETE .../documents[/:id]` | 已接，硬编码 proj-01 | 改 `Api.db.*` 签名加 projectId；UI 明示隐式取第一个库；无库时引导建库 |
| S3 | `GET/POST/DELETE :p/s3/objects`、`GET :p/s3/presign` | 已接 | 补上传进度 + 体积预校验；`lastModified` 是 camelCase 例外 |
| LLM | `GET :p/llm/providers`、`POST :p/llm/{chat,stream}` | 已接（deprecated） | UI `/llm` 重定向到 Cloud Agent；settings/sessions 仍供模型配置 |
| Cloud Agent | `GET/POST :p/agents`、threads/runs（见 proto-http.md §3.12） | `/agents` 两栏页 | 替代 LLM 对话；`@` mention + 只读工具；契约见 `cloud-agent-plan.md` |
| 配额 | `GET :p/quota` | 无 | Dashboard 卡片 |
| 审计 | `GET :p/audit` | 无 | **后端是占位实现（恒空数组）**，不做独立菜单，降级为 Logs 页一个 tab 或暂不接 |
| 指标 | `/metrics`（Prometheus 文本） | 前端调的 `/metrics/summary`、`/metrics/trend` 不存在 | 短期：Dashboard 改用 quota + databases；长期：后端补 JSON 接口（proto-http.md §6.1） |
| FaaS | **无后端模块** | 前端调用 `/faas/*` | 菜单隐藏，代码 + Mock 保留 |
| 日志 WS | **无后端实现** | 前端连 `/ws/logs` | Mock 演示 + 页面显式提示 |

## 四、实施阶段

### Phase 1：地基（先做，其他都依赖它）
1. `stores/project.ts` + `stores/auth.ts`（pinia，localStorage 持久化）
2. **`types.ts` 的 `Api.db.*` 方法签名加 `projectId` 参数**，再改 `http-api.ts`（去 8 处硬编码）与 `mock.js` 两个实现（顺序不能反，否则类型不匹配）
3. `router/index.ts` 加 `meta.title`/`meta.icon`/`meta.hidden`，NavMenu 遍历路由渲染，删除 `DefaultLayout.titleMap`
4. styles 拆分 tokens/base/antd-patch/utilities 四文件 + `tokens.ts`，App.vue 的 antd token 引用它
5. `composables/useAsyncAction.ts` 替换各页 try/catch/message 模板

### Phase 2：API 对齐
6. `types.ts`/`http-api.ts`/`mock.js` 补 `databases.*`/`sql.*`/`quota.*` 域（签名对照 proto-http.md §3.1/§3.2/§3.6）
7. `http.ts` 响应拦截器：401 → 全局提示 + 打开 `ApiKeyDrawer`；保留 HTML fallback 防御；容忍 501 响应缺 `request_id`
8. `llmStream` 补 `resp.body` 为 null 与非 JSON 响应的防御
9. 修 `mock.js` S3 种子数据缺 `_projectId` 导致列表恒空的 bug

### Phase 3：新页面
10. Databases 页：列表 / 创建 / open / close / 删除（二次确认）；状态 tag 覆盖 9 个枚举值；备份恢复按钮标注「未实现」或禁用
11. SqlConsole 页：query/execute/batch 三 tab；结果表格按 `columns` zip `rows`；展示 `duration_ms`/`rows_affected`/`durability`；batch 支持事务开关与逐条错误展示
12. Dashboard 改造：卡片数据源换为 `:p/quota` + `:p/databases`（数据库数量/状态分布），趋势图在后端接口就绪前隐藏或标注「需后端支持」

### Phase 4：体验优化
13. `SbCodeBlock` / `SbEmptyState` / `ProjectPicker` / `ApiKeyDrawer` 落地，替换各页重复样式
14. 表格统一 `usePagination` + 斑马纹/粘性表头
15. 首屏骨架屏；空态带 CTA
16. 修复 `DefaultLayout` 侧边栏宽度 bug（改用 JS 常量或 `matchMedia` 响应式）、`Logs.vue` 阴影色、清理未用导入与 `any`
17. `.sb-content` 加 `max-width: 1440px`

### Phase 5（可延后）
18. 暗色模式（tokens + antd darkAlgorithm + localStorage 持久化）
19. `prefers-reduced-motion`、`:focus-visible` 全局规则
20. antd 按需引入（构建体积）

### Phase 6：设置页 + 通用 AI Chat（可与 Phase 5 并行设计，实现见独立文档）
21. 设置页：主题 / 默认模型 / 预置厂商 Token Plan（开发者只填 key）——详见 `ui-settings-chat-plan.md`
22. `components/ai/AiChat*` 通用对话组件；Composer 按参考图（大圆角、左 `+`、右麦克风、黑圆上箭头发送）
23. `LlmManager` 改为消费 AiChat；后端 §3.9 凭证接口未就绪前设置以本地存储为主，并明示网关仍用服务端供应商

## 五、验收标准

1. `yarn build` 零报错；`npx vue-tsc --noEmit` 无类型错误（当前 `package.json` 无 type-check 脚本，手动执行）
2. DevMode（`dev_mode: true`）启动后端，UI 直连 `:8080`，Databases / SqlConsole / DataManager / S3Manager / LlmManager 全部真实数据可用
3. `proto.http` 中未注释的请求在 REST Client 逐条执行，结果与文档标注的状态码一致（`@databaseId` 需先手工填入真实 ID）
4. 切换 `VITE_USE_MOCK=true` 后所有页面不白屏，且 S3 列表有数据（验证 F10 已修）
5. 375px / 768px / 1440px / 2560px 四档宽度布局不塌陷
6. 键盘 Tab 可见焦点；`rg "ProjectOutlined"` 无残留；业务代码无 `: any`
7. Dashboard / FaaS / Logs 三页不再出现「接口不存在或返回了 HTML 页面」错误提示（改为明确的「后端未支持」文案或已切换数据源）

## 六、风险与不做的事

- 不引入 ECharts/UnoCSS 等新依赖；不修改 `package.json`/`tsconfig.json`/`vite.config.ts`（若 Phase 5 做按需引入需单独立项）
- 不改后端代码：snake_case ↔ camelCase 映射、`rows` 二维数组转对象、错误码兜底全部在前端 adapter 层完成
- 不为「后端不存在的能力」造假 UI：FaaS 菜单隐藏，Logs 显式提示，Dashboard 趋势图在接口就绪前不展示假数据
- 审计页不做（后端占位实现恒返回空数组，接了也没有内容）
- 项目切换不做「项目列表下拉」——后端无 `GET /v1/projects`，只做输入 + 本地记忆
- proto-http.md §6 列出的 6 项后端 TODO 属后端 plan 范围，本计划不依赖它们完成；其中 §6.1（指标 JSON 接口）落地后 Dashboard 可再迭代一次