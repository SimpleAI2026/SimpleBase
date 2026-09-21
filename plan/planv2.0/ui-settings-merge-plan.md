# UI：合并侧栏「设置」与右上角「连接设置」

> **状态**：plan only（本文件）。**本 PR 不实现 UI。**  
> **制定日期**：2026-09-21  
> **Verified against**：`main` @ `16ca6f7`（`ui/src` layout / router / Settings / ApiKeyDrawer / stores / tests）  
> **关联**：[`ui-settings-chat-plan.md`](./ui-settings-chat-plan.md)、[`ui-global-project-plan.md`](./ui-global-project-plan.md)、[`ui-plan-v2.md`](./ui-plan-v2.md)、[`ui-principles.md`](./ui-principles.md)、[`ui/AGENTS.md`](../../ui/AGENTS.md)  
> **需求原文**：将侧栏设置与右上角设置合并为全局设置，入口只留右上角；点击弹出 `max-width: 900px` 的 Modal（用户写的是 max-weight，按 **max-width** 理解）。

---

## 0. 一句话目标 / One-sentence goal

控制台只保留 **右上角一个「设置」入口**；点击打开 **`max-width: 900px` 的 Dialog/Modal**，把现在侧栏 `/settings` 页与顶栏 `ApiKeyDrawer` 的字段全部放进去，侧栏不再出现「设置」。

---

## 1. 现状盘点 / Current state inventory

两条独立入口，视觉上都叫「设置」，但职责、容器、作用域都不一样。

```
DefaultLayout
├── Sidebar → NavMenu → router-link name=settings → /settings → pages/Settings.vue
└── Header right: 使用文档 → GlobalProjectSwitcher → SettingsIcon → 刷新
                                                      └─ authStore.openDrawer()
                                                      └─ components/ApiKeyDrawer.vue (Sheet)
```

### 1.1 侧栏「设置」— 全页路由

| 项 | 现状 |
|---|---|
| 入口 | `ui/src/components/NavMenu.vue`：`iconMap.settings = SettingsIcon`；`routeOrder` 最后一项 `'settings'` |
| 路由 | `ui/src/router/index.ts`：`path: 'settings'`，`name: 'settings'`，`meta.title: '设置'`，**未** `hidden` |
| 页面 | `ui/src/pages/Settings.vue`，包在 `<ProjectScope>` + `<PageContainer subtitle="主题、默认模型与厂商 API Key（Key 仅缓存在本机）">` |
| 面包屑 | `DefaultLayout` 从 `route.meta.title` 取「设置」 |
| 容器 | 普通页面（全宽内容区），不是 Modal |

页面内三段 Card + 一个嵌套 Sheet：

| 区块 | 控件 | 作用域 | 存储 |
|---|---|---|---|
| **外观** | `ToggleGroup`：浅色 / 深色 / 跟随系统 | **实例全局** | `stores/settings.ts` `theme` → `localStorage` `sb_settings_v1`；`App.vue` 监听 `prefers-color-scheme` 并 `applyThemeToDom()`（给 `<html>` 加 `.dark`） |
| **模型默认值** | 默认供应商 `Select`、默认模型 `Combobox`、Temperature `Slider`、Max Tokens `Input` | **当前项目** | 读：`api.llmSettings.get(project.id)`（`GET /v1/projects/:p/llm/settings`）；写：`api.llmSettings.put`；Pinia `projectDefaults[projectId]` 只作缓存（`persist()` **不**把 `projectDefaults` 写回 localStorage） |
| **供应商与 API Key** | 预置厂商卡片（`constants/llmProviders.ts`）、「配置」打开 Sheet、「设为默认」 | **当前项目** | `stores/settings.ts` `providerConfigs[projectId][providerId]` → `sb_settings_v1`（明文 Key 仅本机）；**不**走后端 `provider-catalog`（契约仍标本期未做） |
| 厂商编辑 Sheet | `Sheet` `sm:max-w-md`：必填/可选字段 + 该厂商默认模型 | 同上 | `upsertProviderConfig` / `clearProviderCredentials` |

无项目时：`ProjectScope` 挡住整页，只显示「请先在右上角选择或创建一个项目」。主题因此在无项目时也改不了——这是现状缺陷，合并后应修掉（外观是全局的）。

### 1.2 右上角「设置」— 连接 Drawer

| 项 | 现状 |
|---|---|
| 入口 | `ui/src/layouts/DefaultLayout.vue` header 右侧 `Button` + `SettingsIcon`；`@click="authStore.openDrawer()"` |
| Tooltip | 默认「连接设置」；`lastUnauthorizedAt > 0` 时「API Key 无效，点击配置」，按钮上红点 |
| 容器 | `ui/src/components/ApiKeyDrawer.vue`：reka-ui **`Sheet` 右侧**，`sm:max-w-sm`（约 384px），**不是** Modal |
| 开关 | `stores/auth.ts`：`keyDrawerOpen` / `openDrawer` / `closeDrawer` / `markUnauthorized` |
| 401 | `ui/src/services/http.ts` 响应拦截器：401 → `useAuthStore().markUnauthorized()` → 自动打开抽屉 |
| 文档站 | `DocsLayout.vue` **没有**设置入口（符合「控制台 chrome」边界） |

抽屉内字段：

| 字段 | 作用域 | 存储 |
|---|---|---|
| 当前项目（只读 `Input`） | 展示 `projectStore` 当前项 | 不写；切换入口仍是 `GlobalProjectSwitcher` |
| SimpleBase **API Key**（password） | **浏览器全局**（所有项目共用一把控制台 Key） | `localStorage` `sb_api_key`，经 `services/http.ts` `getApiKey` / `setApiKey`；默认 DevMode `sb_live_dev_key_12345` |
| 保存 / 恢复默认（DevMode） | 全局 | `authStore.updateKey`；保存成功 toast「设置已保存」并关抽屉 |
| 401 Alert | 全局 | `lastUnauthorizedAt` |

顶栏顺序已定稿（`ui/AGENTS.md`，不得回退）：

> 使用文档 → 项目切换器（`w-56`）→ **设置** → 刷新，间距 `gap-4 sm:gap-5`

合并后这条顺序不变，只是「设置」从「只开连接抽屉」变成「开合并 Modal」。

### 1.3 Store / composable 一览

| 模块 | 路径 | 本需求相关职责 |
|---|---|---|
| `useSettingsStore` | `ui/src/stores/settings.ts` | 主题、按项目 LLM 默认值缓存、按项目厂商 Key |
| `useAuthStore` | `ui/src/stores/auth.ts` | 控制台 API Key、401、**抽屉开关**（实现期建议改名为 modal 开关，见 §4.4） |
| `useProjectStore` | `ui/src/stores/project.ts` | 当前项目；Settings 页与 ApiKeyDrawer 都读它；**不是**设置入口 |
| （无 composable） | — | 设置逻辑全在页面 / store，没有 `useSettingsPanel` |

后端已有、**前端未接**的实例级 KV：`GET/PUT /v1/settings`（`proto-http.md`、`internal/api/router.go`）。本计划 **不**把主题或控制台 Key 迁到该 API。

### 1.4 测试与文档里「设置在侧栏」的断言

| 文件 | 断言什么 |
|---|---|
| `ui/src/router/index.test.ts` | 路由 `name` 列表 **包含** `'settings'`；「七个控制台功能区」标题用例 **不含** Settings（dashboard / databases / s3 / gofunctions / cron-jobs / agents / logs） |
| `ui/src/test/helpers.ts` | stub 路由含 `{ path: '/settings', name: 'settings', meta: { title: '设置' } }` |
| `ui/src/pages/Settings.test.ts` | 直接 `mountWithApp(Settings)`：主题、llmSettings、厂商 Sheet |
| `ui/src/pages/pages-coverage.test.ts` | stub `/settings`；shallow mount `Settings.vue` 调 `onTheme` / `loadServerDefaults` 等 |
| `ui/src/coverage-gaps.test.ts` | 再 mount 一遍 Settings / ApiKeyDrawer |
| `ui/src/components/ApiKeyDrawer.test.ts` | 开抽屉、保存/重置 Key、401、项目标签 |
| `ui/src/components/interactions.test.ts` | `ApiKeyDrawer sheet close`；NavMenu 点击（不硬编码「设置」文案） |
| `ui/src/components/simple-components.test.ts` | `NavMenu lists console routes`（用自制短路由表，**不**断言 settings 项） |
| `ui/src/layouts/DefaultLayout.test.ts` / `layouts.test.ts` | 点 header 图标 → `auth.keyDrawerOpen === true`；mock 了 `ApiKeyDrawer` |
| `ui/src/stores/auth.test.ts` | `openDrawer` / `closeDrawer` / `keyDrawerOpen` |
| `ui/src/App.test.ts` / `stores/settings.test.ts` | 主题 DOM，与入口无关 |

仓库 `docs/**`、`README.md`、文档站 catalog **没有**链到 `/settings`。计划文档里仍把 Settings 当侧栏最后一项：`ui-global-project-plan.md` G3、`ui-gofunction-plan.md` §8、`ui-settings-chat-plan.md` §3.1。实现 PR 应改这些过时句子；**本 plan-only PR 不改它们**。

---

## 2. 目标 UX / Target UX

1. **唯一入口**：右上角齿轮。侧栏不再有「设置」。
2. **点击**（以及 401 自动打开）→ 居中 **Modal / Dialog**，内容区 **`max-width: 900px`**（相对视口仍受 `max-w-[calc(100%-2rem)]` 约束，小屏左右留白）。
3. Modal 内展示 **合并后的全部设置**，字段不丢（§3）。
4. 保存行为保持现状：**即时写入**（主题立刻生效；LLM 默认值 `put`；厂商 Key 写 localStorage；控制台 Key 点「保存」）。Modal **不要**做成「确定才提交」的 `SbModal` 表单对话框。
5. 关 Modal 后停留在 **当前业务页**（大盘 / 数据库 / …），不跳到空白设置页。

### 2.1 侧栏与路由策略（推荐）

**推荐：侧栏隐藏 + 保留 deep-link，重定向到 query 开 Modal。**

| 选项 | 做法 | 取舍 |
|---|---|---|
| **A. 推荐** | `NavMenu` 去掉 `settings`；路由改为 `{ path: 'settings', redirect: '/' }` 或 redirect 到「当前控制台路径」并带 `?settings=1`；`DefaultLayout` watch query / auth 开关打开 Modal，打开后可 `replace` 掉 query | 书签 `/settings` 仍可用；侧栏干净；不占一个「空页面」 |
| B. 保留隐藏页 | `meta.hidden: true`，`Settings.vue` 变成只负责 `onMounted` 开 Modal 再 `router.back()` | 面包屑会闪「设置」；多一个无内容页 |
| C. 删除路由 | 去掉 `name: 'settings'`，旧 URL 落到 SPA fallback | 破坏书签；`router/index.test.ts` 要改掉 `settings` name |

实现约束：

- **不要**再渲染全页 `Settings.vue` 作为主内容。
- 401 开 Modal **不必**改 URL（避免污染历史）。手动点齿轮同样可以只走 store 开关、不改 path。
- query `?settings=1`（或 `?settings=connection`）仅服务 deep-link / 刷新恢复；打开后建议 `router.replace` 清掉 query，避免分享带 query 的业务 URL。
- 文档站继续无设置入口。

---

## 3. 信息架构 / Information architecture

用户把两边都叫「全局设置」，指的是 **入口全局**。字段作用域不能混：LLM 默认值与厂商 Key 今天就是 **按 `projectStore.id` 分桶** 的，合并后仍跟当前顶栏项目走。

### 3.1 合并后分区（建议 Tab，避免 900px 里叠四张长 Card）

仓库已有 `components/ui/tabs/*`（文档站 `DocsModuleTabs` 在用）。Modal 内用 `TabsList variant="line"`：

| Tab | 来源 | 作用域 | 字段（不得丢失） |
|---|---|---|---|
| **连接** | `ApiKeyDrawer` | 全局 Key + 只读当前项目 | 401 Alert；当前项目只读；API Key；保存；恢复 DevMode 默认；DevMode 种子说明 |
| **外观** | `Settings.vue` 第一张 Card | 全局 | 浅色 / 深色 / 跟随系统 |
| **模型** | `Settings.vue` 第二张 Card | 当前项目 | 默认供应商、默认模型、Temperature、Max Tokens |
| **供应商** | `Settings.vue` 第三张 Card + 编辑 Sheet | 当前项目 | 预置厂商卡片、已配置徽章、掩码 Key、「配置」「设为默认」、编辑表单 |

401 / `markUnauthorized`：**强制落到「连接」Tab**。

无项目时：「连接」「外观」可用；「模型」「供应商」显示与 `ProjectScope` 同类的空态（选/建项目），不要整 Modal 空白。

### 3.2 全局 vs 项目级（合并后仍成立）

| 全局（与当前项目无关） | 仍按当前项目 |
|---|---|
| 主题 | `GET/PUT :p/llm/settings` 默认模型/供应商/温度/tokens |
| 控制台 Bearer API Key | 厂商 API Key（`sb_settings_v1.providerConfigs[projectId]`） |
| 401 红点 | 「设为默认供应商」 |

项目切换：Modal 若开着，模型/供应商应 `watch(project.id)` 重载（沿用 `Settings.vue` 已有 watch）。不在 Modal 里做第二套项目切换（`ui-global-project-plan.md` Phase C 已规定 ApiKeyDrawer 项目只读）。

### 3.3 不做的 IA 收缩

- 不把厂商 Key 提升成「全实例一把 Key」（会串项目，违背现 store 分桶）。
- 不把主题或控制台 Key 改成项目级。
- 不在本需求接入 `GET/PUT /v1/settings`（实例 KV）或服务端托管 LLM 凭证。

---

## 4. 组件设计 / Component design

### 4.1 为什么不用现成 `SbModal` 当外壳

`ui/src/components/modal/SbModal.vue` = shadcn `Dialog` + 默认 **取消 / 确定** footer。宽度映射：

```ts
if (w >= 900) return 'sm:max-w-5xl'  // Tailwind max-w-5xl = 64rem = 1024px
```

两处都不符合本需求：

1. 设置是 **分区即时保存**，不是一次 `ok`。
2. 用户要的是 **900px**，不是 1024px 的 `max-w-5xl`。

`AGENTS.md`「弹窗用 `SbModal`」对 CRUD 表单仍成立；设置壳用 **Dialog 直接组合**（与 `SbModal` 同源），实现时在 `AGENTS.md` 加一句例外。

### 4.2 推荐拆分

```
DefaultLayout
  header SettingsIcon → authStore.openSettings()  // 原 openDrawer
  SettingsModal          // 新：Dialog 壳 + Tabs + 开关
    SettingsPanel        // 新：从 Settings.vue 抽出的区块（外观/模型/供应商）
    ConnectionPanel      // 从 ApiKeyDrawer.vue 抽出；或内联进 SettingsModal 的「连接」Tab
```

| 组件 | 职责 |
|---|---|
| `components/SettingsModal.vue`（新） | `Dialog` + `DialogContent` `class="max-w-[calc(100%-2rem)] sm:max-w-[900px]"`；标题「设置」；可滚动（`max-h-[min(80vh,720px)] overflow-y-auto` 或内层 scroll，避免小屏裁切）；绑定 `authStore` 开闭；`defaultTab` prop（401 传 `connection`） |
| `components/settings/SettingsPanel.vue`（新，或同目录扁平） | 现 `pages/Settings.vue` 的三段 UI + 厂商编辑逻辑；去掉 `PageContainer` 外层（Modal 已有标题） |
| `components/settings/ConnectionPanel.vue`（新） | 现 `ApiKeyDrawer` 表单；不再包 `Sheet` |
| `pages/Settings.vue` | **删除**或改成 0 行 redirect 占位；内容禁止双份维护 |

厂商「配置」子编辑：

- **优先**：仍用 `Sheet`（现成），但 Sheet/Dialog 都是 `z-50`，必须把编辑 Sheet 提到 **`z-[60]`**（或更高），否则会被 Modal 挡住 / 抢 focus。
- **备选**：供应商 Tab 内联展开表单，去掉嵌套 overlay（更不容易 focus-trap 冲突）。

`DialogScrollContent.vue` 目前零引用，默认还是 `max-w-lg`，**不要**不改 class 直接拿来当 900px 壳。

### 4.3 Layout / Nav 改动清单

| 文件 | 改动 |
|---|---|
| `layouts/DefaultLayout.vue` | 去掉 `<ApiKeyDrawer />`，改为 `<SettingsModal />`；齿轮 tooltip 改为「设置」（401 仍可「API Key 无效，点击配置」）；点击仍开 store 开关 |
| `components/NavMenu.vue` | 删除 `settings: SettingsIcon` 与 `routeOrder` 里的 `'settings'` |
| `router/index.ts` | `settings` 按 §2.1 A：redirect + 可选 query；注释里「hidden：文档站…」改为也覆盖设置 deep-link |
| `components/ApiKeyDrawer.vue` | 删除（逻辑迁走后全局确认零引用） |

### 4.4 Store 开关命名

`keyDrawerOpen` / `openDrawer` / `closeDrawer` 会变成谎言。实现期二选一（测试全量替换）：

- **推荐**：`settingsOpen` + `openSettings({ tab?: 'connection' \| 'appearance' \| 'models' \| 'providers' })` + `closeSettings()`；`markUnauthorized()` 调 `openSettings({ tab: 'connection' })`。
- 或暂时复用旧字段，只改组件——可落地，但 header/401 语义更差。

`http.ts` 注释「打开 key 配置抽屉」一并改。

---

## 5. 路由与导航影响 / Routing & navigation impact

| 表面 | 影响 |
|---|---|
| 侧栏 | 少一项；顺序变为 dashboard → databases → s3 → gofunctions → cron-jobs → agents → logs（Settings 不再垫底） |
| 面包屑 | 不再出现「设置」作为当前页标题；Modal 打开时面包屑仍是底下那页 |
| 顶栏 | 顺序不变；齿轮语义从「连接设置」升级为「设置」 |
| 文档站 | 无改动 |
| 文档/README 链接 | 无 `/settings` 引用，无需改 `docs/` |
| `ui/AGENTS.md` | `pages/` 列表去掉 Settings 作为常驻路由页；补充 Settings Modal 例外 |
| 过期计划文案 | `ui-global-project-plan.md` G3、`ui-gofunction-plan.md` 侧栏顺序、`ui-settings-chat-plan.md`「侧栏可见」——实现 PR 里改一句，避免后人按旧图做 |

历史 redirect 风格（`/sql`→`/databases`、`/llm`→`/agents`）继续：`/settings` → 开 Modal 的 deep-link，而不是 404。

---

## 6. 分阶段改动与验收 / Phased change list

本文件只规划。实现时建议两阶段，均可在同一 UI PR。

### Phase 1 — 抽出面板，Modal 壳，双入口暂时都指向同一内容（可选内部步骤）

1. 抽出 `SettingsPanel` / `ConnectionPanel`，`Settings.vue` 与 `ApiKeyDrawer` 先改成薄包装，行为不变。
2. 单测从「mount 页面/抽屉」改为「mount 面板」，覆盖率不掉。

**验收 P1**

| # | 标准 |
|---|---|
| P1.1 | 主题 / llmSettings get-put / 厂商配置与清除 / 控制台 Key 保存与 DevMode 重置，现有断言仍绿 |
| P1.2 | `npm run build`（`ui/`）通过 |

### Phase 2 — 唯一入口 + 900px Modal + 去侧栏

1. `SettingsModal`：Dialog、`sm:max-w-[900px]`、四 Tab、滚动。
2. `DefaultLayout` 接 Modal；齿轮打开；401 开「连接」。
3. `NavMenu` 去掉 settings；路由按 §2.1 A。
4. 删除 `ApiKeyDrawer.vue` 与全页 Settings 内容；更新测试与 `AGENTS.md`。
5. 厂商编辑 Sheet 提高 z-index 或改为内联。

**验收 P2（含测试更新）**

| # | 标准 |
|---|---|
| P2.1 | 侧栏 **没有**「设置」；`NavMenu` 对真实 `router/index.ts` 渲染时不含 settings 项（补一条用生产路由表的测试，现在的 NavMenu 测试用的是自制短表，拦不住回归） |
| P2.2 | 右上角齿轮打开 Dialog；内容区计算宽度 ≤ 900px（`sm:max-w-[900px]`，不要 `max-w-5xl`） |
| P2.3 | Modal 内可完成：改主题、改控制台 Key、改当前项目 LLM 默认值、配置/清除厂商 Key；关 Modal 后仍在原业务路由 |
| P2.4 | 401 自动打开 Modal 且停在「连接」；红点逻辑不变 |
| P2.5 | `/settings` 不渲染旧全页；deep-link 能打开 Modal（redirect/query） |
| P2.6 | 无项目：仍能改主题与控制台 Key |
| P2.7 | 测试更新清单见下；`ui` Vitest + `npm run build` 通过 |

**必须跟着改的测试 / 夹具**

- `pages/Settings.test.ts` → 测 `SettingsPanel` 或 `SettingsModal`（主题 + llm + 厂商）
- `components/ApiKeyDrawer.test.ts` → `ConnectionPanel` / `SettingsModal`（保存、重置、401 tab）
- `layouts/DefaultLayout.test.ts`、`layouts.test.ts`：断言打开的是 settings modal，而不是 `keyDrawerOpen` 抽屉（若字段改名则全替换）
- `stores/auth.test.ts`：新开关 API
- `router/index.test.ts`：`settings` 若仍存在，断言是 **redirect**（或 hidden），不再当普通 console page
- `test/helpers.ts` stub 路由：跟生产策略一致
- `pages-coverage.test.ts`、`coverage-gaps.test.ts`、`components-coverage.test.ts`、`interactions.test.ts`：去掉对已删 SFC 的 import
- **新增** `SettingsModal.test.ts`：open/close、默认 tab、401 tab、max-width class
- **新增或加强** NavMenu + 真实 router：菜单 name 列表 equals `['dashboard','databases','s3','gofunctions','cron-jobs','agents','logs']`

---

## 7. 范围外 / Out of scope（non-goals）

- **本 PR**：只新增本 markdown，不改 `ui/src/**`、不改 Go、不改 `docs/`。
- 实现阶段也不做：
  - 后端 `GET/PUT /v1/settings`、LLM provider-catalog / 服务端托管厂商 Key
  - 新 npm 依赖；改 `package.json` / Tailwind 主题 token
  - 项目切换器、创建项目、文档站 chrome
  - 把 LLM 默认值改成全局、或把控制台 Key 改成按项目
  - 重做法厂商卡片视觉 / Token Plan catalog
  - `stores/settings.ts` 其它重构（例如把 `projectDefaults` 写回 localStorage）
  - 在 Modal 里做第二套项目 ID 编辑

---

## 8. 风险与实现注意

| 风险 | 处理 |
|---|---|
| Dialog + 嵌套 Sheet 双 `z-50`、双 focus trap | Sheet 提 z-index，或供应商编辑改内联 |
| `SbModal` 的 900 → `max-w-5xl` 会被误用 | Settings 壳 **禁止** `width={900}` 走 SbModal 映射；写死 `sm:max-w-[900px]` |
| Combobox / Select 在 Dialog 内 portal 被裁切 | 沿用现组件（已 portal）；QA 时看下拉是否被 `overflow-y-auto` 剪掉，必要时 `modal={false}` 或提高下拉 z |
| 覆盖率网依赖 `Settings.vue` / `ApiKeyDrawer.vue` 的 vm 私有方法 | 抽出面板后把断言迁到仍 export/仍可点的 UI，避免只测 `vm.onTheme` |
| 侧栏测试用假路由 | 必须加生产 `router` 的负向断言，否则「去掉 settings」会无测试保护 |

---

## 9. 建议实现顺序（供后续 UI PR，非本文件）

1. 抽 `ConnectionPanel` + `SettingsPanel`（行为冻结）。
2. 加 `SettingsModal`（900px Dialog + Tabs），Layout 双开（齿轮走 Modal，侧栏仍进旧页）——仅本地验证用，不要合进 main。
3. Nav 去 settings、路由 redirect、删旧页/旧抽屉、改 401、改测试与 `AGENTS.md`。
4. 手动点：齿轮、401、`/settings`、无项目、切换项目时 Modal 仍打开、厂商配置 Sheet。
