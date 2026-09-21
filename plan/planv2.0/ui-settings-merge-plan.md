# UI：合并侧栏「设置」与右上角「连接设置」

> **状态**：implementation（本 PR：先锁定审查决策，再改 `ui/src`）。  
> **制定日期**：2026-09-21  
> **Verified against**：`main` @ `3045b9a`（plan-only merge #19）对照 `ui/src` layout / router / Settings / ApiKeyDrawer / SbModal / stores / tests  
> **关联**：[`ui-settings-chat-plan.md`](./ui-settings-chat-plan.md)、[`ui-global-project-plan.md`](./ui-global-project-plan.md)、[`ui-plan-v2.md`](./ui-plan-v2.md)、[`ui-principles.md`](./ui-principles.md)、[`ui/AGENTS.md`](../../ui/AGENTS.md)  
> **需求原文**：将侧栏设置与右上角设置合并为全局设置，入口只留右上角；点击弹出 `max-width: 900px` 的 Modal（用户写的是 max-weight / min-weight，按 **max-width / min-width** 理解）。

### 已锁定的审查决策（覆盖任何 Dialog / 保留 `ApiKeyDrawer` 的旧表述）

1. **复用现成 `SbModal`，禁止另起 Dialog 壳。** 扩展 `SbModal`，让调用方配置 **max-width / min-width**。合并后的设置弹窗通过该 API 设 **`maxWidth: 900`（即 `max-width: 900px`）**。本需求不得再写一套独立 `Dialog` + `DialogContent`。
2. **删除抽屉面板。** `ApiKeyDrawer`（Sheet）迁走逻辑后整文件删除；清掉死引用、过时 import、以及只测抽屉的测试。厂商「配置」改为 Tab 内联表单，不嵌套 Sheet。

---

## 0. 一句话目标 / One-sentence goal

控制台只保留 **右上角一个「设置」入口**；点击打开 **`SbModal`（`maxWidth: 900` → `max-width: 900px`）**，把现在侧栏 `/settings` 页与顶栏 `ApiKeyDrawer` 的字段全部放进去，侧栏不再出现「设置」。

---

## 1. 现状盘点 / Current state inventory

两条独立入口，视觉上都叫「设置」，但职责、容器、作用域都不一样。（实现前基线，相对 `main` @ plan-only merge。）

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

合并后这条顺序不变，只是「设置」从「只开连接抽屉」变成「开合并 `SbModal`」。

### 1.3 Store / composable 一览

| 模块 | 路径 | 本需求相关职责 |
|---|---|---|
| `useSettingsStore` | `ui/src/stores/settings.ts` | 主题、按项目 LLM 默认值缓存、按项目厂商 Key |
| `useAuthStore` | `ui/src/stores/auth.ts` | 控制台 API Key、401、**抽屉开关**（实现期改名为 modal 开关，见 §4.4） |
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

仓库 `docs/**`、`README.md`、文档站 catalog **没有**链到 `/settings`。计划文档里仍把 Settings 当侧栏最后一项：`ui-global-project-plan.md` G3、`ui-gofunction-plan.md` §8、`ui-settings-chat-plan.md` §3.1。实现 PR 应改这些过时句子。

---

## 2. 目标 UX / Target UX

1. **唯一入口**：右上角齿轮。侧栏不再有「设置」。
2. **点击**（以及 401 自动打开）→ 居中 **`SbModal`**，内容区 **`max-width: 900px`**（相对视口仍受 DialogContent 已有的 `max-w-[calc(100%-2rem)]` 约束，小屏左右留白）。宽度只通过 `SbModal` 的 `maxWidth` / `minWidth` API 配置，不另写 Dialog class。
3. Modal 内展示 **合并后的全部设置**，字段不丢（§3）。
4. 保存行为保持现状：**即时写入**（主题立刻生效；LLM 默认值 `put`；厂商 Key 写 localStorage；控制台 Key 点「保存」）。因此 `SbModal` 必须能 **`hideFooter`**，不要做成「确定才提交」的表单对话框。连接 Tab 点「保存」toast 成功后 **留在 Modal 内**（X 关闭）；旧抽屉「保存即关」是因为抽屉本身就是整块 UI，Tab 化之后关整个设置不合理。
5. 关 Modal 后停留在 **当前业务页**（大盘 / 数据库 / …），不跳到空白设置页。

### 2.1 侧栏与路由策略（推荐）

**推荐：侧栏隐藏 + 保留 deep-link，重定向到 query 开 Modal。**

| 选项 | 做法 | 取舍 |
|---|---|---|
| **A. 推荐（本 PR 采用）** | `NavMenu` 去掉 `settings`；路由改为 `{ path: 'settings', name: 'settings', redirect: { path: '/', query: { settings: '1' } } }`；`DefaultLayout` watch query / auth 开关打开 Modal，打开后 `replace` 掉 query | 书签 `/settings` 仍可用；侧栏干净；不占一个「空页面」 |
| B. 保留隐藏页 | `meta.hidden: true`，`Settings.vue` 变成只负责 `onMounted` 开 Modal 再 `router.back()` | 面包屑会闪「设置」；多一个无内容页 |
| C. 删除路由 | 去掉 `name: 'settings'`，旧 URL 落到 SPA fallback | 破坏书签；`router/index.test.ts` 要改掉 `settings` name |

实现约束：

- **不要**再渲染全页 `Settings.vue` 作为主内容；该文件删除。
- 401 开 Modal **不必**改 URL（避免污染历史）。手动点齿轮同样只走 store 开关、不改 path。
- query `?settings=1`（或 `?settings=connection` 等 Tab 名）仅服务 deep-link / 刷新恢复；打开后 `router.replace` 清掉 query，避免分享带 query 的业务 URL。
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
| **供应商** | `Settings.vue` 第三张 Card + 原编辑 Sheet | 当前项目 | 预置厂商卡片、已配置徽章、掩码 Key、「配置」「设为默认」、**内联**编辑表单 |

401 / `markUnauthorized`：**强制落到「连接」Tab**。

无项目时：「连接」「外观」可用；「模型」「供应商」显示与 `ProjectScope` 同类的空态（选/建项目），不要整 Modal 空白。

### 3.2 全局 vs 项目级（合并后仍成立）

| 全局（与当前项目无关） | 仍按当前项目 |
|---|---|
| 主题 | `GET/PUT :p/llm/settings` 默认模型/供应商/温度/tokens |
| 控制台 Bearer API Key | 厂商 API Key（`sb_settings_v1.providerConfigs[projectId]`） |
| 401 红点 | 「设为默认供应商」 |

项目切换：Modal 若开着，模型/供应商应 `watch(project.id)` 重载（沿用 `Settings.vue` 已有 watch）。不在 Modal 里做第二套项目切换（`ui-global-project-plan.md` Phase C 已规定连接区项目只读）。

### 3.3 不做的 IA 收缩

- 不把厂商 Key 提升成「全实例一把 Key」（会串项目，违背现 store 分桶）。
- 不把主题或控制台 Key 改成项目级。
- 不在本需求接入 `GET/PUT /v1/settings`（实例 KV）或服务端托管 LLM 凭证。

---

## 4. 组件设计 / Component design

### 4.1 复用并扩展 `SbModal`（已锁定，覆盖旧「为什么不用 SbModal」）

`ui/src/components/modal/SbModal.vue` 今天 = shadcn `Dialog` + 默认 **取消 / 确定** footer。`width` 数字映射到 Tailwind 档位：

```ts
if (w >= 900) return 'sm:max-w-5xl'  // Tailwind max-w-5xl = 64rem = 1024px
```

旧稿因此建议「设置壳直接组合 Dialog」。**该建议作废。** 正确做法：

1. **扩展 `SbModal`**，新增精确尺寸 props（数字视为 px；字符串原样作为 CSS size）：
   - `maxWidth?: number | string` — 设置弹窗传 `900`，得到 **`max-width: 900px`**（`sm:max-w-[var(--sb-modal-max-w)]`，小屏仍吃 DialogContent 的 `max-w-[calc(100%-2rem)]`）。
   - `minWidth?: number | string` — 对称的最小宽度（`sm:min-w-[var(--sb-modal-min-w)]`）。本需求设置弹窗可以不传。
   - `hideFooter?: boolean` — 隐藏默认取消/确定。设置是分区即时保存。
2. **`width` 档位映射保留**，现有 CRUD（`GoFunctionModal` 的 `width={960}` 等）不改。设置弹窗 **禁止**只传 `width={900}`（那会变成 1024px 的 `max-w-5xl`），必须走 `maxWidth`。
3. **禁止**为本功能再写一套 `Dialog` / `DialogContent` 外壳；`SettingsModal` 的根必须是 `<SbModal>`。
4. `AGENTS.md`「弹窗用 `SbModal`」继续成立，无需再开 Dialog 例外。设置弹窗补一句：全局设置走 `SbModal` + `maxWidth` + `hideFooter`，不是全页、不是 Sheet。

CSS 变量写法让任意 px 值都能过 Tailwind（避免 `sm:max-w-[${n}px]` 动态拼接导致 JIT 扫不到）。

### 4.2 推荐拆分

```
DefaultLayout
  header SettingsIcon → authStore.openSettings()  // 原 openDrawer
  SettingsModal          // 新：SbModal 壳 + Tabs + 开关
    ConnectionPanel      // 从 ApiKeyDrawer.vue 抽出；不再包 Sheet
    SettingsPanel        // 从 Settings.vue 抽出的外观 / 模型 / 供应商（含内联厂商表单）
```

| 组件 | 职责 |
|---|---|
| `components/SettingsModal.vue`（新） | **`SbModal`** `title="设置"` **`maxWidth={900}`** `hideFooter`；内层可滚动（`max-h-[min(80vh,720px)] overflow-y-auto`）；绑定 `authStore` 开闭；Tabs 受 `settingsTab` 控制（401 强制 `connection`） |
| `components/settings/SettingsPanel.vue`（新） | 现 `pages/Settings.vue` 的三段 UI + 厂商编辑逻辑；按 `section` 渲染 appearance / models / providers；去掉 `PageContainer`；无项目时 models/providers 显示空态 |
| `components/settings/ConnectionPanel.vue`（新） | 现 `ApiKeyDrawer` 表单；不再包 `Sheet` |
| `pages/Settings.vue` | **删除** |
| `components/ApiKeyDrawer.vue` | **删除**（零引用后全局确认） |

厂商「配置」子编辑：**内联展开表单**（计划备选，现已锁定）。不嵌套 Sheet，避免 Dialog + Sheet 双 `z-50` / 双 focus trap。

`DialogScrollContent.vue` 目前零引用，**不要**拿来当 900px 壳。

### 4.3 Layout / Nav 改动清单

| 文件 | 改动 |
|---|---|
| `layouts/DefaultLayout.vue` | 去掉 `<ApiKeyDrawer />`，改为 `<SettingsModal />`；齿轮 tooltip 改为「设置」（401 仍可「API Key 无效，点击配置」）；点击 `openSettings()`；watch `?settings=` deep-link |
| `components/NavMenu.vue` | 删除 `settings: SettingsIcon` 与 `routeOrder` 里的 `'settings'` |
| `router/index.ts` | `settings` 按 §2.1 A：redirect + query；注释里「hidden：文档站…」改为也覆盖设置 deep-link |
| `components/ApiKeyDrawer.vue` | 删除 |
| `pages/Settings.vue` | 删除 |
| `components/modal/SbModal.vue` | 新增 `maxWidth` / `minWidth` / `hideFooter` |

### 4.4 Store 开关命名

`keyDrawerOpen` / `openDrawer` / `closeDrawer` 会变成谎言。实现采用：

- `settingsOpen` + `settingsTab` + `openSettings({ tab?: 'connection' \| 'appearance' \| 'models' \| 'providers' })` + `closeSettings()`
- `markUnauthorized()` 调 `openSettings({ tab: 'connection' })`
- 旧抽屉 API **删除**，测试全量替换

`http.ts` 注释「打开 key 配置抽屉」改为打开设置弹窗。

---

## 5. 路由与导航影响 / Routing & navigation impact

| 表面 | 影响 |
|---|---|
| 侧栏 | 少一项；顺序变为 dashboard → databases → s3 → gofunctions → cron-jobs → agents → logs（Settings 不再垫底） |
| 面包屑 | 不再出现「设置」作为当前页标题；Modal 打开时面包屑仍是底下那页 |
| 顶栏 | 顺序不变；齿轮语义从「连接设置」升级为「设置」 |
| 文档站 | 无改动 |
| 文档/README 链接 | 无 `/settings` 引用，无需改 `docs/` |
| `ui/AGENTS.md` | `pages/` 列表去掉 Settings 作为常驻路由页；补充全局设置走 `SbModal`（`maxWidth` + `hideFooter`），不是全页、不是 Sheet |
| 过期计划文案 | `ui-global-project-plan.md` G3、`ui-gofunction-plan.md` 侧栏顺序、`ui-settings-chat-plan.md`「侧栏可见」——本 PR 改一句，避免后人按旧图做 |

历史 redirect 风格（`/sql`→`/databases`、`/llm`→`/agents`）继续：`/settings` → 开 Modal 的 deep-link，而不是 404。

---

## 6. 分阶段改动与验收 / Phased change list

本 PR 直接落地（不必分两个合入点）。内部顺序：先扩 `SbModal`，再抽面板，再换入口、删旧文件。

### Phase 1 — `SbModal` 尺寸 API + 抽出面板

1. `SbModal`：`maxWidth` / `minWidth` / `hideFooter`；保留 `width` 档位映射。
2. 抽出 `SettingsPanel` / `ConnectionPanel`。
3. 单测从「mount 页面/抽屉」改为「mount 面板」，覆盖率不掉。

**验收 P1**

| # | 标准 |
|---|---|
| P1.1 | 主题 / llmSettings get-put / 厂商配置与清除 / 控制台 Key 保存与 DevMode 重置，现有断言仍绿 |
| P1.2 | `SbModal` 在 `maxWidth={900}` 时内容区 class/CSS 变量对应 **900px**，不是 `max-w-5xl`；`hideFooter` 时无取消/确定 |
| P1.3 | `ui/` `yarn test` 与 `yarn build` 通过 |

### Phase 2 — 唯一入口 + 900px `SbModal` + 去侧栏 + 删抽屉

1. `SettingsModal`：`<SbModal :max-width="900" hide-footer>` + 四 Tab + 滚动。
2. `DefaultLayout` 接 Modal；齿轮打开；401 开「连接」；`?settings=` deep-link。
3. `NavMenu` 去掉 settings；路由按 §2.1 A。
4. 删除 `ApiKeyDrawer.vue`、`pages/Settings.vue` 与只测它们的测试；更新 `AGENTS.md` 与过期计划一句。
5. 厂商编辑改为内联表单。

**验收 P2（含测试更新）**

| # | 标准 |
|---|---|
| P2.1 | 侧栏 **没有**「设置」；`NavMenu` 对真实 `router/index.ts` 渲染时不含 settings 项（补一条用生产路由表的测试，现在的 NavMenu 测试用的是自制短表，拦不住回归） |
| P2.2 | 右上角齿轮打开 **`SbModal`**；`maxWidth={900}`（`sm:max-w-[var(--sb-modal-max-w)]` 且变量为 `900px`），不要 `max-w-5xl`，不要独立 Dialog 壳 |
| P2.3 | Modal 内可完成：改主题、改控制台 Key、改当前项目 LLM 默认值、配置/清除厂商 Key；关 Modal 后仍在原业务路由 |
| P2.4 | 401 自动打开 Modal 且停在「连接」；红点逻辑不变 |
| P2.5 | `/settings` 不渲染旧全页；deep-link 能打开 Modal（redirect/query） |
| P2.6 | 无项目：仍能改主题与控制台 Key |
| P2.7 | 仓库内 **零** `ApiKeyDrawer` 引用；测试更新清单见下；`ui` Vitest + `yarn build` 通过 |

**必须跟着改的测试 / 夹具**

- `pages/Settings.test.ts` → 测 `SettingsPanel` 或 `SettingsModal`（主题 + llm + 厂商内联表单）
- `components/ApiKeyDrawer.test.ts` → `ConnectionPanel` / `SettingsModal`（保存、重置、401 tab）；**删除**抽屉测试文件
- `layouts/DefaultLayout.test.ts`、`layouts.test.ts`：断言打开的是 `settingsOpen`，不是 `keyDrawerOpen`
- `stores/auth.test.ts`：新开关 API；旧 `openDrawer` 断言删除
- `router/index.test.ts`：`settings` 仍存在，断言是 **redirect**
- `test/helpers.ts` stub 路由：跟生产策略一致（可保留 `/settings` stub 供非生产 router 的测试）
- `pages-coverage.test.ts`、`coverage-gaps.test.ts`、`components-coverage.test.ts`、`interactions.test.ts`：去掉对已删 SFC 的 import
- **新增** `SettingsModal.test.ts`：open/close、默认 tab、401 tab、把 `maxWidth=900` 传给 `SbModal`
- **新增或加强** NavMenu + 真实 router：菜单 name 列表 equals `['dashboard','databases','s3','gofunctions','cron-jobs','agents','logs']`
- `SbModal.test.ts`：覆盖 `maxWidth` / `minWidth` / `hideFooter`

---

## 7. 范围外 / Out of scope（non-goals）

- 本 PR **会**改 plan 文件与 `ui/src/**`（含测试与 `ui/AGENTS.md`），以及把过期计划里「侧栏 Settings」改成一句指向本文的说明。不改 Go、不改 `docs/` 产品文档。
- 实现阶段也不做：
  - 后端 `GET/PUT /v1/settings`、LLM provider-catalog / 服务端托管厂商 Key
  - 新 npm 依赖；改 `package.json` / Tailwind 主题 token
  - 项目切换器、创建项目、文档站 chrome
  - 把 LLM 默认值改成全局、或把控制台 Key 改成按项目
  - 重做法厂商卡片视觉 / Token Plan catalog
  - `stores/settings.ts` 其它重构（例如把 `projectDefaults` 写回 localStorage）
  - 在 Modal 里做第二套项目 ID 编辑
  - 为设置弹窗新增独立 Dialog 组件

---

## 8. 风险与实现注意

| 风险 | 处理 |
|---|---|
| Dialog + 嵌套 Sheet 双 `z-50`、双 focus trap | **不嵌套**。厂商编辑内联；`ApiKeyDrawer` 删除 |
| `width={900}` 仍走 `max-w-5xl` | 设置壳 **必须** `maxWidth={900}`；单测锁 900px CSS 变量，而不是 `max-w-5xl` |
| Tailwind 动态 class `max-w-[${n}px]` JIT 扫不到 | 用 `--sb-modal-max-w` / `--sb-modal-min-w` + 静态 `sm:max-w-[var(--sb-modal-max-w)]` |
| Combobox / Select 在 Dialog 内 portal 被裁切 | 沿用现组件（已 portal）；QA 时看下拉是否被 `overflow-y-auto` 剪掉，必要时 `modal={false}` 或提高下拉 z |
| 覆盖率网依赖 `Settings.vue` / `ApiKeyDrawer.vue` 的 vm 私有方法 | 抽出面板后把断言迁到仍 export/仍可点的 UI，避免只测 `vm.onTheme` |
| 侧栏测试用假路由 | 必须加生产 `router` 的负向断言，否则「去掉 settings」会无测试保护 |

---

## 9. 建议实现顺序（本 PR）

1. 扩 `SbModal`（`maxWidth` / `minWidth` / `hideFooter`）+ 单测。
2. 抽 `ConnectionPanel` + `SettingsPanel`（内联厂商表单）；删全页 `Settings.vue` 与 `ApiKeyDrawer`。
3. 加 `SettingsModal`（`SbModal maxWidth=900` + Tabs），Layout 只接齿轮；Nav 去 settings；`/settings` redirect；改 401；改测试与 `AGENTS.md`。
4. 手动点：齿轮、401、`/settings`、无项目、切换项目时 Modal 仍打开、厂商内联配置。
