# UI：合并侧栏「设置」与右上角「连接设置」

> **状态**：implementing（本 PR：改计划 + 落地 UI）  
> **制定日期**：2026-09-21  
> **Verified against**：`main` @ `16ca6f7`，实现跟本文件 §2–§4  
> **关联**：[`ui-settings-chat-plan.md`](./ui-settings-chat-plan.md)、[`ui-global-project-plan.md`](./ui-global-project-plan.md)、[`ui-plan-v2.md`](./ui-plan-v2.md)、[`ui-principles.md`](./ui-principles.md)、[`ui/AGENTS.md`](../../ui/AGENTS.md)  
> **需求原文**：将侧栏设置与右上角设置合并为全局设置，入口只留右上角；点击弹出 `max-width: 900px` 的 Modal（用户写的是 max-weight / min-weight，按 **max-width / min-width** 理解）。

### 评审拍板（覆盖初稿）

1. **必须复用现有 `SbModal`**，禁止再包一层独立 Dialog 当设置壳。扩展 `SbModal`，调用方可设 **`maxWidth` / `minWidth`**；设置弹窗 `maxWidth=900`。
2. **删除抽屉式设置**：`ApiKeyDrawer`（Sheet）及合并后无引用的抽屉代码、import、测试一律清掉。

---

## 0. 一句话目标 / One-sentence goal

控制台只保留 **右上角一个「设置」入口**；点击打开 **`SbModal`（`maxWidth: 900`）**，合并侧栏 `/settings` 页与顶栏 `ApiKeyDrawer` 的字段；侧栏不再出现「设置」；连接抽屉删除。

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

### 1.2 右上角「设置」— 连接 Drawer（将删除）

| 项 | 现状 |
|---|---|
| 入口 | `ui/src/layouts/DefaultLayout.vue` header 右侧 `Button` + `SettingsIcon`；`@click="authStore.openDrawer()"` |
| Tooltip | 默认「连接设置」；`lastUnauthorizedAt > 0` 时「API Key 无效，点击配置」，按钮上红点 |
| 容器 | `ui/src/components/ApiKeyDrawer.vue`：reka-ui **`Sheet` 右侧**，`sm:max-w-sm`（约 384px） |
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

合并后这条顺序不变；齿轮从「打开连接抽屉」改为「打开 `SbModal` 设置」。`ApiKeyDrawer.vue` 与相关测试删除。

### 1.3 Store / composable 一览

| 模块 | 路径 | 本需求相关职责 |
|---|---|---|
| `useSettingsStore` | `ui/src/stores/settings.ts` | 主题、按项目 LLM 默认值缓存、按项目厂商 Key |
| `useAuthStore` | `ui/src/stores/auth.ts` | 控制台 API Key、401、**抽屉开关**（实现改为 modal 开关，见 §4.4） |
| `useProjectStore` | `ui/src/stores/project.ts` | 当前项目；Settings 页与 ApiKeyDrawer 都读它；**不是**设置入口 |

后端已有、**前端未接**的实例级 KV：`GET/PUT /v1/settings`。本计划 **不**把主题或控制台 Key 迁到该 API。

### 1.4 测试与文档里「设置在侧栏」的断言

| 文件 | 断言什么 |
|---|---|
| `ui/src/router/index.test.ts` | 路由 `name` 列表 **包含** `'settings'`；「七个控制台功能区」标题用例 **不含** Settings |
| `ui/src/test/helpers.ts` | stub 路由含 `{ path: '/settings', name: 'settings', meta: { title: '设置' } }` |
| `ui/src/pages/Settings.test.ts` | 直接 `mountWithApp(Settings)`：主题、llmSettings、厂商 Sheet |
| `ui/src/pages/pages-coverage.test.ts` | stub `/settings`；shallow mount `Settings.vue` |
| `ui/src/coverage-gaps.test.ts` | 再 mount 一遍 Settings / ApiKeyDrawer |
| `ui/src/components/ApiKeyDrawer.test.ts` | 开抽屉、保存/重置 Key、401、项目标签 |
| `ui/src/components/interactions.test.ts` | `ApiKeyDrawer sheet close` |
| `ui/src/layouts/DefaultLayout.test.ts` / `layouts.test.ts` | 点 header 图标 → `auth.keyDrawerOpen === true`；mock 了 `ApiKeyDrawer` |
| `ui/src/stores/auth.test.ts` | `openDrawer` / `closeDrawer` / `keyDrawerOpen` |

仓库 `docs/**`、`README.md`、文档站 catalog **没有**链到 `/settings`。

---

## 2. 目标 UX / Target UX

1. **唯一入口**：右上角齿轮。侧栏不再有「设置」。
2. **点击**（以及 401 自动打开）→ **`SbModal`**，`maxWidth=900`（像素；小屏仍受 `DialogContent` 的 `max-w-[calc(100%-2rem)]` 约束）。
3. Modal 内展示 **合并后的全部设置**，字段不丢（§3）。
4. 保存行为保持现状：**即时写入**（主题立刻生效；LLM 默认值 `put`；厂商 Key 写 localStorage；控制台 Key 点「保存」）。因此设置 Modal 使用 `hideFooter`（不要默认「确定/取消」一次提交）。
5. 关 Modal 后停留在 **当前业务页**，不跳到空白设置页。
6. **连接不再是 Sheet/Drawer**。`ApiKeyDrawer` 删除。厂商「配置」改为 Modal 内联表单，不再套一层设置抽屉。

### 2.1 侧栏与路由策略

**侧栏隐藏 + 保留 `/settings` deep-link（redirect → `/?settings=1` 开同一 Modal）。**

| 选项 | 做法 | 取舍 |
|---|---|---|
| **A. 采用** | `NavMenu` 去掉 `settings`；路由 `{ path: 'settings', name: 'settings', redirect: { path: '/', query: { settings: '1' } }, meta: { hidden: true } }`；`DefaultLayout` watch query 打开 Modal 后 `replace` 掉 query | 书签仍可用；不占空页面 |
| B/C | 隐藏页或删路由 | 不采用 |

- 401 开 Modal **不改 URL**。
- 文档站继续无设置入口。

---

## 3. 信息架构 / Information architecture

入口全局；LLM 默认值与厂商 Key 仍按 `projectStore.id` 分桶。

### 3.1 合并后分区（Tab）

`components/ui/tabs/*`，`TabsList variant="line"`：

| Tab | 来源 | 作用域 | 字段（不得丢失） |
|---|---|---|---|
| **连接** | 原 `ApiKeyDrawer` 表单（无 Sheet） | 全局 Key + 只读当前项目 | 401 Alert；当前项目只读；API Key；保存；恢复 DevMode 默认；种子说明 |
| **外观** | 原 `Settings.vue` 第一张 Card | 全局 | 浅色 / 深色 / 跟随系统 |
| **模型** | 原 `Settings.vue` 第二张 Card | 当前项目 | 默认供应商、默认模型、Temperature、Max Tokens |
| **供应商** | 原 `Settings.vue` 第三张 Card + 编辑表单 | 当前项目 | 预置厂商卡片、已配置徽章、掩码 Key、「配置」「设为默认」、**内联**编辑表单 |

401 / `markUnauthorized`：**强制落到「连接」Tab**。

无项目时：「连接」「外观」可用；「模型」「供应商」显示空态（选/建项目），不要整 Modal 空白。

### 3.2 全局 vs 项目级

| 全局（与当前项目无关） | 仍按当前项目 |
|---|---|
| 主题 | `GET/PUT :p/llm/settings` 默认模型/供应商/温度/tokens |
| 控制台 Bearer API Key | 厂商 API Key（`sb_settings_v1.providerConfigs[projectId]`） |
| 401 红点 | 「设为默认供应商」 |

Modal 开着时 `watch(project.id)` 重载模型/供应商。不在 Modal 里做第二套项目切换。

### 3.3 不做的 IA 收缩

- 不把厂商 Key 提升成「全实例一把 Key」。
- 不把主题或控制台 Key 改成项目级。
- 不接入 `GET/PUT /v1/settings` 或服务端托管 LLM 凭证。

---

## 4. 组件设计 / Component design

### 4.1 扩展 `SbModal`（唯一弹窗壳）

`ui/src/components/modal/SbModal.vue` 继续包 shadcn `Dialog`。现有 `width` 断点映射（含 `>=900 → sm:max-w-5xl` = 1024px）**保留给旧调用方**，设置弹窗 **不要**走这条映射。

新增：

| Prop | 类型 | 作用 |
|---|---|---|
| `maxWidth` | `number \| string` | 写入 CSS 变量 `--sb-modal-max-w`，class `sm:max-w-[var(--sb-modal-max-w)]`（数字当 px）。覆盖默认 `sm:max-w-lg` / `width` 档位。 |
| `minWidth` | `number \| string` | 同理 `--sb-modal-min-w` + `sm:min-w-[var(--sb-modal-min-w)]` |
| `hideFooter` | `boolean` | 隐藏默认取消/确定（设置即时保存） |
| `bodyClass` | `string` | 内容槽 class；设置传滚动高度 |

设置调用：

```vue
<SbModal
  :open="open"
  title="设置"
  :max-width="900"
  hide-footer
  body-class="max-h-[min(70vh,640px)] overflow-y-auto"
  @update:open="onOpen"
>
```

其它 CRUD Modal 不改调用，行为不变。

### 4.2 拆分

```
DefaultLayout
  header SettingsIcon → authStore.openSettings()
  SettingsModal                    // SbModal maxWidth=900 + Tabs
    ConnectionPanel                // 原 ApiKeyDrawer 表单，无 Sheet
    AppSettingsPanel section=…     // 原 Settings.vue 三段，无 PageContainer
```

| 组件 | 职责 |
|---|---|
| `components/SettingsModal.vue` | `SbModal` 壳 + Tabs；绑定 `authStore.settingsOpen` / `settingsTab` |
| `components/settings/ConnectionPanel.vue` | 连接表单 |
| `components/settings/AppSettingsPanel.vue` | `section?: 'appearance' \| 'models' \| 'providers'`；厂商配置 **内联**，不使用 Sheet |
| `pages/Settings.vue` | **删除** |
| `components/ApiKeyDrawer.vue` | **删除**（含 `ApiKeyDrawer.test.ts`） |

`CronJobRunsDrawer` 等非设置抽屉保留。

### 4.3 Layout / Nav / Router

| 文件 | 改动 |
|---|---|
| `layouts/DefaultLayout.vue` | `<SettingsModal />` 替换 `<ApiKeyDrawer />`；tooltip「设置」；401 文案保留；watch `?settings=` |
| `components/NavMenu.vue` | 删除 `iconMap.settings` 与 `routeOrder` 的 `'settings'` |
| `router/index.ts` | settings 改为 hidden redirect，见 §2.1 |
| `stores/auth.ts` | 见 §4.4 |
| `services/http.ts` | 注释改为打开设置 Modal |

### 4.4 Store 开关命名

| 旧 | 新 |
|---|---|
| `keyDrawerOpen` | `settingsOpen` |
| `openDrawer()` | `openSettings(tab?: SettingsTab)` |
| `closeDrawer()` | `closeSettings()` |
| `markUnauthorized()` | 仍存在；顺带 `settingsTab = 'connection'` 并 `settingsOpen = true` |

`SettingsTab = 'connection' \| 'appearance' \| 'models' \| 'providers'`。不保留 drawer 别名。

---

## 5. 路由与导航影响 / Routing & navigation impact

| 表面 | 影响 |
|---|---|
| 侧栏 | dashboard → databases → s3 → gofunctions → cron-jobs → agents → logs |
| 面包屑 | Modal 打开时仍是底下业务页标题 |
| 顶栏 | 顺序不变；齿轮 = 设置 |
| `ui/AGENTS.md` | `pages/` 列表去掉 Settings 常驻页；写明设置走 `SbModal` + `maxWidth` |

---

## 6. 改动清单与验收 / Change list & acceptance

同一 PR：更新本计划 + 实现。

1. 扩展 `SbModal`：`maxWidth` / `minWidth` / `hideFooter` / `bodyClass`；补测。
2. 抽出 `ConnectionPanel` + `AppSettingsPanel`；`SettingsModal` 用 `SbModal :max-width="900"`。
3. Layout / Nav / router / auth store / 401。
4. **删除** `ApiKeyDrawer.vue`、`pages/Settings.vue` 及死引用测试。
5. 测试与 `ui/AGENTS.md`。

**验收**

| # | 标准 |
|---|---|
| A1 | 侧栏没有「设置」；真实 `router` 下 NavMenu names = dashboard…logs |
| A2 | 右上角齿轮打开 **SbModal**；`maxWidth=900`（class/CSS 变量，不是 `max-w-5xl`） |
| A3 | Modal 内可改主题、控制台 Key、当前项目 LLM 默认值、配置/清除厂商 Key；关闭后仍在原路由 |
| A4 | 401 打开 Modal 且停在「连接」；无 `ApiKeyDrawer` / `keyDrawerOpen` |
| A5 | `/settings` → 打开同一 Modal（deep-link） |
| A6 | 无项目仍能改主题与控制台 Key |
| A7 | 仓库内零引用 `ApiKeyDrawer`；`ui` Vitest + `npm run build` 通过 |

**测试迁移**

- `pages/Settings.test.ts` → `AppSettingsPanel` / `SettingsModal`
- `ApiKeyDrawer.test.ts` → `ConnectionPanel` / `SettingsModal`（删除原文件）
- Layout / auth / router / helpers / coverage 网去掉抽屉与全页 Settings
- `SbModal.test.ts` 覆盖 `maxWidth` / `minWidth` / `hideFooter`
- NavMenu + 生产 router：无 settings 菜单项

---

## 7. 范围外 / Out of scope（non-goals）

- 后端 `GET/PUT /v1/settings`、LLM provider-catalog / 服务端托管厂商 Key
- 新 npm 依赖
- 项目切换器、创建项目、文档站 chrome
- 把 LLM 默认值改成全局、或把控制台 Key 改成按项目
- 重做法厂商卡片 / Token Plan catalog
- `stores/settings.ts` 其它重构
- 删除非设置抽屉（如 `CronJobRunsDrawer`）

---

## 8. 风险与实现注意

| 风险 | 处理 |
|---|---|
| 动态 Tailwind class 不生成 | `maxWidth` 用 **静态** `sm:max-w-[var(--sb-modal-max-w)]` + CSS 变量，不用 `` sm:max-w-[${n}px] `` |
| `width: 900` 仍映射到 `max-w-5xl` | 设置只传 `maxWidth`，不传 `width: 900` |
| Dialog 内 Combobox/Select 被 overflow 裁切 | portal 已有；QA 看下拉 |
| 覆盖率网绑死已删 SFC | 迁到新面板 / Modal |

---

## 9. 实现顺序

1. `SbModal` 宽度 props + 测试。
2. auth store 改名 + ConnectionPanel / AppSettingsPanel / SettingsModal。
3. Layout / Nav / router；删除 `ApiKeyDrawer` 与 `pages/Settings.vue`。
4. 测试、`AGENTS.md`、build。
